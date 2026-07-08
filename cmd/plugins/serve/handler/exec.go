package handler

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
)

type OutputLine struct {
	NodeID string `json:"node_id"`
	Line   string `json:"line"`
	Type   string `json:"type"` // "stdout" or "stderr"
}

type Executor interface {
	Execute(ctx context.Context, nodeID, command string) (string, int, error)
	ExecuteStream(ctx context.Context, nodeID, command string, outputCh chan<- OutputLine) (int, error)
}

type ExecHandler struct {
	db    *sql.DB
	task  *store.TaskStore
	exec  Executor
	hub   *WSHub
}

func NewExecHandler(db *sql.DB, ts *store.TaskStore, hub *WSHub) *ExecHandler {
	return &ExecHandler{
		db:   db,
		task: ts,
		exec: &sshExecutor{db: db},
		hub:  hub,
	}
}

type execRequest struct {
	NodeID        string            `json:"node_id"`
	NodeIDs       []string          `json:"node_ids"`
	Command       string            `json:"command"`
	ScriptContent string            `json:"script_content"`
	ScriptName    string            `json:"script_name"`
	ScriptArgs    string            `json:"script_args"`
	Group         string            `json:"group"`
	Labels        map[string]string `json:"labels"`
	Status        string            `json:"status"`
	Force         string            `json:"force,omitempty"`

	Async             bool   `json:"async"`
	AsyncMaxPollCount int    `json:"async_max_poll_count"`
	AsyncPollInterval string `json:"async_poll_interval"`
	AsyncRemoteDir    string `json:"async_remote_dir"`
	AsyncTimeout      string `json:"async_timeout"`
	Format            string `json:"format"`
	Debug             bool   `json:"debug"`
	Parallel          bool   `json:"parallel"`
	Serial            bool   `json:"serial"`
	Retry             int    `json:"retry"`
	RetryInterval     string `json:"retry_interval"`
	RetryMaxInterval  string `json:"retry_max_interval"`
	NoRetry           bool   `json:"no_retry"`
	ConnectTimeout    string `json:"connect_timeout"`
	CommandTimeout    string `json:"command_timeout"`
	Timeout           string `json:"timeout"`
	NoColor           bool   `json:"no_color"`
	Silent            bool   `json:"silent"`
}

type ExecConfig struct {
	Command          string
	NodeIDs          []string
	Force            bool
	Async            bool
	Format           string
	Debug            bool
	Parallel         bool
	Retry            int
	RetryInterval    string
	RetryMaxInterval string
	NoRetry          bool
	ConnectTimeout   string
	CommandTimeout   string
	NoColor          bool
	Silent           bool
}

func resolveNodeIDs(db *sql.DB, req execRequest) []string {
	if len(req.NodeIDs) > 0 {
		return req.NodeIDs
	}
	if req.NodeID != "" {
		return []string{req.NodeID}
	}
	if req.Group != "" {
		groupNames := strings.Split(req.Group, ",")
		var allIDs []string
		seen := make(map[string]bool)
		for _, gn := range groupNames {
			gn = strings.TrimSpace(gn)
			if gn == "" {
				continue
			}
			pattern := "%" + gn + "%"
			rows, err := db.Query(`SELECT id FROM nodes WHERE groups LIKE ?`, pattern)
			if err != nil || rows == nil {
				continue
			}
			func() {
				defer rows.Close()
				for rows.Next() {
					var id string
					rows.Scan(&id)
					if !seen[id] {
						seen[id] = true
						allIDs = append(allIDs, id)
					}
				}
			}()
		}
		if len(allIDs) > 0 {
			return allIDs
		}
	}
	if len(req.Labels) > 0 {
		for k, v := range req.Labels {
			pattern := "%\"" + k + "\":\"" + v + "%"
			rows, _ := db.Query(`SELECT id FROM nodes WHERE labels LIKE ?`, pattern)
			if rows != nil {
				defer rows.Close()
				var ids []string
				for rows.Next() {
					var id string
					rows.Scan(&id)
					ids = append(ids, id)
				}
				return ids
			}
		}
	}
	if req.Status != "" {
		rows, _ := db.Query(`SELECT id FROM nodes WHERE status = ?`, req.Status)
		if rows != nil {
			defer rows.Close()
			var ids []string
			for rows.Next() {
				var id string
				rows.Scan(&id)
				ids = append(ids, id)
			}
			return ids
		}
	}
	return nil
}

func (h *ExecHandler) Create(c *gin.Context) {
	var req execRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request body"})
		return
	}

	nodeIDs := resolveNodeIDs(h.db, req)
	if len(nodeIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "no target nodes specified"})
		return
	}

	command := req.Command
	if command == "" && req.ScriptContent != "" {
		command = req.ScriptContent
	}
	if command == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "command or script_content is required"})
		return
	}

	cfg := ExecConfig{
		Command:          command,
		NodeIDs:          nodeIDs,
		Force:            req.Force == "true",
		Async:            req.Async,
		Format:           req.Format,
		Debug:            req.Debug,
		Parallel:         !req.Serial,
		Retry:            req.Retry,
		RetryInterval:    req.RetryInterval,
		RetryMaxInterval: req.RetryMaxInterval,
		NoRetry:          req.NoRetry,
		ConnectTimeout:   req.ConnectTimeout,
		CommandTimeout:   req.CommandTimeout,
		NoColor:          req.NoColor,
		Silent:           req.Silent,
	}
	if cfg.Retry == 0 && !cfg.NoRetry {
		cfg.Retry = 3
	}
	if cfg.RetryInterval == "" {
		cfg.RetryInterval = "1s"
	}
	if cfg.RetryMaxInterval == "" {
		cfg.RetryMaxInterval = "30s"
	}

	var tasks []*store.Task
	isMerge := false

	if cfg.Parallel {
		for _, nid := range nodeIDs {
			task, err := h.createSingleTask(c, nid, command, cfg.Force)
			if err != nil {
				if len(nodeIDs) == 1 {
					c.JSON(err.Code, gin.H{"code": err.Code, "message": err.Message})
					return
				}
				continue
			}
			if task.merged {
				isMerge = true
				tasks = append(tasks, task.task)
				continue
			}
			tasks = append(tasks, task.task)
			if h.exec != nil {
				go h.executeTask(task.task.ID, cfg)
			}
			if h.hub != nil {
				h.hub.BroadcastTaskUpdate(task.task)
			}
		}
	} else {
		var serialTasks []*store.Task
		for _, nid := range nodeIDs {
			task, err := h.createSingleTask(c, nid, command, cfg.Force)
			if err != nil {
				continue
			}
			if task.merged {
				isMerge = true
			}
			tasks = append(tasks, task.task)
			if !task.merged {
				serialTasks = append(serialTasks, task.task)
			}
		}
		if len(serialTasks) > 0 && h.exec != nil {
			go func() {
				for _, t := range serialTasks {
					h.executeTask(t.ID, cfg)
				}
			}()
		}
	}

	if len(tasks) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "failed to create any tasks"})
		return
	}

	statusCode := http.StatusAccepted
	if isMerge {
		statusCode = http.StatusOK
	}
	c.JSON(statusCode, gin.H{"tasks": tasks})
}

type taskResult struct {
	task   *store.Task
	merged bool
}

type apiError struct {
	Code    int
	Message string
}

func (h *ExecHandler) createSingleTask(c *gin.Context, nid, command string, force bool) (*taskResult, *apiError) {
	var exists bool
	h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM nodes WHERE id = ?)", nid).Scan(&exists)
	if !exists {
		return nil, &apiError{Code: http.StatusNotFound, Message: "node not found"}
	}

	if !force {
		running, _ := h.task.ListByNode(c.Request.Context(), nid, store.TaskStatusRunning)
		for _, r := range running {
			if strings.TrimSpace(r.Command) == strings.TrimSpace(command) {
				return &taskResult{task: r, merged: true}, nil
			}
		}
		if len(running) > 0 {
			return nil, &apiError{Code: http.StatusConflict, Message: "node has running tasks with different commands; use 'force': 'true' to override"}
		}
	}

	task, err := h.task.Create(c.Request.Context(), nid, command)
	if err != nil {
		return nil, &apiError{Code: http.StatusInternalServerError, Message: "failed to create task"}
	}
	return &taskResult{task: task}, nil
}

func (h *ExecHandler) Get(c *gin.Context) {
	id := c.Param("id")
	task, err := h.task.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "task not found"})
		return
	}
	c.JSON(http.StatusOK, task)
}

func (h *ExecHandler) List(c *gin.Context) {
	page := 1
	pageSize := 50
	if p, err := parseInt(c.Query("page"), 1); err == nil && p > 0 {
		page = p
	}
	if ps, err := parseInt(c.Query("page_size"), 50); err == nil && ps > 0 && ps <= 100 {
		pageSize = ps
	}
	offset := (page - 1) * pageSize
	tasks, total, err := h.task.List(c.Request.Context(), pageSize, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "list failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": tasks,
		"meta": gin.H{"total": total, "page": page, "page_size": pageSize},
	})
}

func (h *ExecHandler) Cancel(c *gin.Context) {
	id := c.Param("id")
	task, err := h.task.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "task not found"})
		return
	}
	if task.Status == store.TaskStatusCompleted || task.Status == store.TaskStatusFailed {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "task already finished"})
		return
	}
	err = h.task.UpdateStatus(c.Request.Context(), id, store.TaskStatusCancelled, "cancelled by user", nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "cancel failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
}

func (h *ExecHandler) executeTask(taskID string, cfg ExecConfig) {
	if h.exec == nil {
		return
	}
	ctx := context.Background()
	task, err := h.task.Get(ctx, taskID)
	if err != nil {
		return
	}

	debug := func(format string, args ...interface{}) {
		if cfg.Debug {
			msg := fmt.Sprintf("[DEBUG] "+format, args...)
			h.task.UpdateStatus(ctx, taskID, store.TaskStatusRunning,
				task.Output+"\n"+msg, task.ExitCode)
		}
	}

	if err := h.task.UpdateStatus(ctx, taskID, store.TaskStatusRunning, "", nil); err != nil {
		return
	}
	task, err = h.task.Get(ctx, taskID)
	if err != nil || task == nil {
		return
	}
	if h.hub != nil {
		h.hub.BroadcastTaskUpdate(task)
	}

	retryCount := cfg.Retry
	if cfg.NoRetry {
		retryCount = 0
	}
	if retryCount < 0 {
		retryCount = 0
	}

	retryInterval, _ := time.ParseDuration(cfg.RetryInterval)
	if retryInterval <= 0 {
		retryInterval = 1 * time.Second
	}
	retryMaxInterval, _ := time.ParseDuration(cfg.RetryMaxInterval)
	if retryMaxInterval <= 0 {
		retryMaxInterval = 30 * time.Second
	}

	var lastError error
	var output string
	var exitCode int

	for attempt := 0; attempt <= retryCount; attempt++ {
		if attempt > 0 {
			debug("重试 %d/%d (等待 %v)...", attempt, retryCount, retryInterval)
			time.Sleep(retryInterval)
			retryInterval = time.Duration(math.Min(
				float64(retryInterval*2),
				float64(retryMaxInterval),
			))
		}

		output, exitCode, lastError = h.exec.Execute(ctx, task.NodeID, task.Command)
		if lastError == nil {
			break
		}

		debug("尝试 %d/%d 失败: %s", attempt+1, retryCount+1, lastError.Error())
	}

	if lastError != nil {
		errMsg := lastError.Error()
		if cfg.Debug {
			errMsg = fmt.Sprintf("所有 %d 次尝试均失败: %s", retryCount+1, errMsg)
		}
		h.task.UpdateStatus(ctx, taskID, store.TaskStatusFailed, errMsg, &exitCode)
		task, _ = h.task.Get(ctx, taskID)
		if h.hub != nil {
			h.hub.BroadcastTaskUpdate(task)
		}
		return
	}

	outputStr := output
	if cfg.Format == "json" {
		outputStr = fmt.Sprintf(`{"node_id":"%s","command":"%s","exit_code":%d,"output":%s}`,
			task.NodeID, task.Command, exitCode, output)
	} else if cfg.Format == "detail" {
		outputStr = fmt.Sprintf("Node: %s\nCommand: %s\nExit Code: %d\n---\n%s",
			task.NodeID, task.Command, exitCode, output)
	}

	h.task.UpdateStatus(ctx, taskID, store.TaskStatusCompleted, outputStr, &exitCode)
	task, _ = h.task.Get(ctx, taskID)
	if h.hub != nil {
		h.hub.BroadcastTaskUpdate(task)
	}
}

func parseInt(s string, defaultVal int) (int, error) {
	if s == "" {
		return defaultVal, nil
	}
	return strconv.Atoi(s)
}
