package handler

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"

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
}

func resolveNodeIDs(db *sql.DB, req execRequest) []string {
	if len(req.NodeIDs) > 0 {
		return req.NodeIDs
	}
	if req.NodeID != "" {
		return []string{req.NodeID}
	}
	if req.Group != "" {
		pattern := "%" + req.Group + "%"
		rows, err := db.Query(`SELECT id FROM nodes WHERE groups LIKE ?`, pattern)
		if err == nil && rows != nil {
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

	force := req.Force == "true"

	var tasks []*store.Task
	isMerge := false

	for _, nid := range nodeIDs {
		var exists bool
		h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM nodes WHERE id = ?)", nid).Scan(&exists)
		if !exists {
			if len(nodeIDs) == 1 {
				c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "node not found"})
				return
			}
			continue
		}

		if !force {
			running, _ := h.task.ListByNode(c.Request.Context(), nid, store.TaskStatusRunning)
			conflict := false
			for _, r := range running {
				if strings.TrimSpace(r.Command) == strings.TrimSpace(command) {
					tasks = append(tasks, r)
					isMerge = true
					conflict = true
					break
				}
			}
			if conflict {
				continue
			}
			if len(running) > 0 {
				if len(nodeIDs) == 1 {
					c.JSON(http.StatusConflict, gin.H{
						"code":    409,
						"message": "node has running tasks with different commands; use 'force': 'true' to override",
						"running_tasks": running,
					})
					return
				}
				continue
			}
		}

		task, err := h.task.Create(c.Request.Context(), nid, command)
		if err != nil {
			continue
		}

		if h.exec != nil {
			go h.executeTask(task.ID)
		}
		if h.hub != nil {
			h.hub.BroadcastTaskUpdate(task)
		}

		tasks = append(tasks, task)
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

func (h *ExecHandler) executeTask(taskID string) {
	if h.exec == nil {
		return
	}
	ctx := context.Background()
	task, err := h.task.Get(ctx, taskID)
	if err != nil {
		return
	}

	h.task.UpdateStatus(ctx, taskID, store.TaskStatusRunning, "", nil)
	task, _ = h.task.Get(ctx, taskID)
	if h.hub != nil {
		h.hub.BroadcastTaskUpdate(task)
	}

	output, exitCode, execErr := h.exec.Execute(ctx, task.NodeID, task.Command)
	if execErr != nil {
		h.task.UpdateStatus(ctx, taskID, store.TaskStatusFailed, execErr.Error(), nil)
		task, _ = h.task.Get(ctx, taskID)
		if h.hub != nil {
			h.hub.BroadcastTaskUpdate(task)
		}
		return
	}

	h.task.UpdateStatus(ctx, taskID, store.TaskStatusCompleted, output, &exitCode)
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
