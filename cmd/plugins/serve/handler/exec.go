package handler

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/cangyunye/go-owl/internal/control/blacklist"
	"github.com/cangyunye/go-owl/internal/logfile"
	nodeselect "github.com/cangyunye/go-owl/internal/node/select"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
	db      *sql.DB
	task    *store.TaskStore
	exec    Executor
	hub     *WSHub
	History *store.HistoryStore
	checker *blacklist.Checker
	LogWriter *logfile.NodeLogWriter
}

func newBlacklistChecker() *blacklist.Checker {
	cfg, err := blacklist.LoadConfig()
	if err != nil {
		log.Printf("load blacklist config: %v (using defaults)", err)
		cfg = &blacklist.Config{Rules: blacklist.DefaultRules()}
	}
	return blacklist.NewChecker(cfg)
}

func NewExecHandler(db *sql.DB, ts *store.TaskStore, hub *WSHub) *ExecHandler {
	return &ExecHandler{
		db:      db,
		task:    ts,
		exec:    &sshExecutor{db: db},
		hub:     hub,
		checker: newBlacklistChecker(),
	}
}

type execRequest struct {
	NodeID          string            `json:"node_id"`
	NodeIDs         []string          `json:"node_ids"`
	Mode            string            `json:"mode"`
	Command         string            `json:"command"`
	ScriptContent   string            `json:"script_content"`
	ScriptName      string            `json:"script_name"`
	ScriptArgs      string            `json:"script_args"`
	ScriptURL       string            `json:"script_url"`
	ScriptRef       string            `json:"script_ref"`
	ScriptDest      string            `json:"script_dest"`
	ScriptKeep      bool              `json:"script_keep"`
	Group           string            `json:"group"`
	Groups          []string          `json:"groups"`
	Labels          map[string]string `json:"labels"`
	Status          string            `json:"status"`
	Force           string            `json:"force,omitempty"`
	DangerConfirmed bool              `json:"danger_confirmed"`

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

	ScriptContent string
	ScriptName    string
	ScriptArgs    string
	ScriptDest    string
	ScriptKeep    bool
}

func resolveNodeIDs(ctx context.Context, db *sql.DB, req execRequest) ([]string, error) {
	sel := nodeselect.NewSelector(&dbNodeSource{db: db})

	opts := nodeselect.SelectOptions{NodeIDs: req.NodeIDs}
	if len(opts.NodeIDs) == 0 && req.NodeID != "" {
		opts.NodeIDs = []string{req.NodeID}
	}
	groups := req.Groups
	if len(groups) == 0 && req.Group != "" {
		groups = strings.Split(req.Group, ",")
	}
	for _, g := range groups {
		if g = strings.TrimSpace(g); g != "" {
			opts.Groups = append(opts.Groups, g)
		}
	}
	opts.Labels = req.Labels
	opts.Status = req.Status

	// Web 语义:未选 node_ids 时 groups/labels 取交集(与左侧预览一致);
	// 空选项返回全部节点,即「零筛选=全量执行」。
	nodes, err := sel.SelectIntersect(ctx, opts)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(nodes))
	for _, n := range nodes {
		ids = append(ids, n.ID)
	}
	return ids, nil
}

// loadNodeUsers 加载节点 user 映射；Scan/rows.Err 出错即返回错误，
// fail-closed：部分节点 user 缺失会导致 root 作用域黑名单规则失效。
func loadNodeUsers(ctx context.Context, db *sql.DB) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, user FROM nodes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := map[string]string{}
	for rows.Next() {
		var id, user string
		if err := rows.Scan(&id, &user); err != nil {
			return nil, err
		}
		users[id] = user
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

func resolveScriptContent(req execRequest, stagingDir string) (content string, name string, err error) {
	if req.ScriptContent != "" {
		name = req.ScriptName
		if name == "" {
			name = "script.sh"
		}
		return req.ScriptContent, name, nil
	}
	if req.ScriptURL != "" {
		resp, err := http.Get(req.ScriptURL)
		if err != nil {
			return "", "", fmt.Errorf("fetch script url: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", "", fmt.Errorf("fetch script url: status %d", resp.StatusCode)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", "", fmt.Errorf("read script url: %w", err)
		}
		name = req.ScriptName
		if name == "" {
			name = filepath.Base(req.ScriptURL)
			if name == "" || name == "." || name == "/" {
				name = "script.sh"
			}
		}
		return string(body), name, nil
	}
	if req.ScriptRef != "" {
		return resolveStagingScriptRef(stagingDir, req.ScriptRef)
	}
	return "", "", fmt.Errorf("script_content, script_url or script_ref is required")
}

// resolveStagingScriptRef 从中转站目录读取脚本内容；
// 文件名复用 scriptNameRe 白名单（仅 [A-Za-z0-9._-]），天然禁止 / 与 ..，防路径穿越。
func resolveStagingScriptRef(dir, ref string) (string, string, error) {
	if !scriptNameRe.MatchString(ref) || ref == "." || ref == ".." {
		return "", "", fmt.Errorf("invalid script_ref %q: only [A-Za-z0-9._-] allowed", ref)
	}
	path := filepath.Join(dir, ref)
	fi, err := os.Stat(path)
	if err != nil {
		return "", "", fmt.Errorf("script not found in staging: %s", ref)
	}
	if fi.IsDir() {
		return "", "", fmt.Errorf("script_ref %q is a directory", ref)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("read staging script: %w", err)
	}
	if len(b) == 0 {
		return "", "", fmt.Errorf("staging script %q is empty", ref)
	}
	return string(b), ref, nil
}

func (h *ExecHandler) Create(c *gin.Context) {
	var req execRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request body"})
		return
	}

	nodeIDs, err := resolveNodeIDs(c.Request.Context(), h.db, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if len(nodeIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "no target nodes specified"})
		return
	}

	isScript := req.Mode == "script"
	command := req.Command
	var scriptContent, scriptName string

	if isScript {
		content, name, err := resolveScriptContent(req, stagingDirFromDB(h.db))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
			return
		}
		scriptContent = content
		scriptName = name
		command = "script: " + name

		scriptDest := req.ScriptDest
		if scriptDest == "" {
			scriptDest = "/tmp"
		}
		if err := validateScriptTarget(scriptDest, scriptName); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
			return
		}
	} else {
		if command == "" && req.ScriptContent != "" {
			command = req.ScriptContent
		}
		if command == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "command or script_content is required"})
			return
		}
	}

	checkCmd := command
	checkArgs := ""
	if isScript {
		checkCmd = scriptContent
		checkArgs = req.ScriptArgs
	}
	users, err := loadNodeUsers(c.Request.Context(), h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "query node users failed: " + err.Error()})
		return
	}

	type blockedMatch struct {
		Node    string `json:"node"`
		Pattern string `json:"pattern"`
		Line    string `json:"line"`
	}
	var blocked []blockedMatch
	for _, nid := range nodeIDs {
		if h.checker == nil {
			break
		}
		result, err := h.checker.CheckForExec(users[nid], checkCmd, req.DangerConfirmed)
		if err != nil {
			for _, m := range result.Matches {
				blocked = append(blocked, blockedMatch{Node: nid, Pattern: m.Pattern, Line: m.Line})
			}
		}
		if checkArgs != "" {
			result, err = h.checker.CheckForExec(users[nid], checkArgs, req.DangerConfirmed)
			if err != nil {
				for _, m := range result.Matches {
					blocked = append(blocked, blockedMatch{Node: nid, Pattern: m.Pattern, Line: m.Line})
				}
			}
		}
	}
	if len(blocked) > 0 {
		c.JSON(http.StatusForbidden, gin.H{
			"code":    http.StatusForbidden,
			"message": "危险命令已被黑名单拦截; 如确需执行请带 danger_confirmed=true 重新提交",
			"blocked": true,
			"matches": blocked,
		})
		return
	}

	scriptDest := req.ScriptDest
	if scriptDest == "" {
		scriptDest = "/tmp"
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
		ScriptContent:    scriptContent,
		ScriptName:       scriptName,
		ScriptArgs:       req.ScriptArgs,
		ScriptDest:       scriptDest,
		ScriptKeep:       req.ScriptKeep,
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

	opID := uuid.New().String()
	var opTargets []string

	var tasks []*store.Task
	isMerge := false

	if cfg.Parallel {
		for _, nid := range nodeIDs {
			task, err := h.createSingleTask(c, nid, command, cfg.Force, opID)
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
			opTargets = append(opTargets, nid)
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
			task, err := h.createSingleTask(c, nid, command, cfg.Force, opID)
			if err != nil {
				continue
			}
			if task.merged {
				isMerge = true
			}
			tasks = append(tasks, task.task)
			if !task.merged {
				serialTasks = append(serialTasks, task.task)
				opTargets = append(opTargets, nid)
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

	if len(opTargets) > 0 {
		opType := "command"
		if isScript {
			opType = "script"
		}
		op := &store.Operation{TaskID: opID, OpType: opType, Command: command, Targets: opTargets, Status: "running", CreatedAt: time.Now().UTC(), Forced: req.DangerConfirmed}
		if err := h.History.RecordOperation(c.Request.Context(), op); err != nil {
			log.Printf("record history: %v", err)
		}
		if h.hub != nil {
			h.hub.BroadcastHistoryUpdate()
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

func (h *ExecHandler) createSingleTask(c *gin.Context, nid, command string, force bool, recordID string) (*taskResult, *apiError) {
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

	task, err := h.task.CreateWithRecord(c.Request.Context(), nid, command, recordID)
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
	start := time.Now()
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

		output, exitCode, lastError = h.streamExecute(ctx, taskID, task.NodeID, task.Command, cfg)
		if lastError == nil {
			break
		}

		debug("尝试 %d/%d 失败: %s", attempt+1, retryCount+1, lastError.Error())
	}

	if lastError != nil {
		outputStr := output
		if outputStr != "" {
			outputStr += "\n"
		}
		errMsg := lastError.Error()
		if cfg.Debug {
			errMsg = fmt.Sprintf("所有 %d 次尝试均失败: %s", retryCount+1, errMsg)
		}
		h.updateTaskStatus(ctx, taskID, store.TaskStatusFailed, outputStr+errMsg, &exitCode)
		h.writeExecutionLog(task, exitCode, output, errMsg, time.Since(start))
		h.recordCommandExecution(ctx, task, exitCode, output, errMsg, time.Since(start).Milliseconds(), false)
		h.updateOpStatus(ctx, task.RecordID)
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

	h.updateTaskStatus(ctx, taskID, store.TaskStatusCompleted, outputStr, &exitCode)
	h.writeExecutionLog(task, exitCode, outputStr, "", time.Since(start))
	h.recordCommandExecution(ctx, task, exitCode, outputStr, "", time.Since(start).Milliseconds(), true)
	h.updateOpStatus(ctx, task.RecordID)
	task, _ = h.task.Get(ctx, taskID)
	if h.hub != nil {
		h.hub.BroadcastTaskUpdate(task)
	}
}

// updateTaskStatus 带有限重试的状态落库,规避 sqlite 并发写瞬时锁冲突。
func (h *ExecHandler) updateTaskStatus(ctx context.Context, taskID string, status store.TaskStatus, output string, exitCode *int) {
	for attempt := 0; attempt < 5; attempt++ {
		if err := h.task.UpdateStatus(ctx, taskID, status, output, exitCode); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

type streamResult struct {
	code int
	err  error
}

// streamExecute 以 ExecuteStream 方式执行单条命令：逐行广播到 WS(task_output)
// 并累积到任务输出，保证前端能实时看到执行输出。
func (h *ExecHandler) streamExecute(ctx context.Context, taskID, nodeID, command string, cfg ExecConfig) (string, int, error) {
	outputCh := make(chan OutputLine, 256)
	resCh := make(chan streamResult, 1)
	go func() {
		code, err := h.exec.ExecuteStream(ctx, nodeID, buildExecCommand(command, cfg), outputCh)
		resCh <- streamResult{code: code, err: err}
	}()

	var buf strings.Builder
	var lastFlush time.Time
	flush := func() {
		h.task.UpdateStatus(ctx, taskID, store.TaskStatusRunning, buf.String(), nil)
	}
	defer flush()
	appendLine := func(line OutputLine) {
		buf.WriteString(line.Line)
		buf.WriteString("\n")
		if h.hub != nil {
			h.hub.BroadcastTaskOutput(taskID, line.NodeID, line.Line, line.Type)
		}
		// 限频落库:逐行写库在并发执行时会放大 sqlite 锁竞争,实时性以 WS 广播承担
		if time.Since(lastFlush) >= 150*time.Millisecond {
			lastFlush = time.Now()
			h.task.UpdateStatus(ctx, taskID, store.TaskStatusRunning, buf.String(), nil)
		}
	}

	for {
		select {
		case line, ok := <-outputCh:
			if !ok {
				res := <-resCh
				return buf.String(), res.code, res.err
			}
			appendLine(line)
		case res := <-resCh:
			for {
				select {
				case line, ok := <-outputCh:
					if !ok {
						return buf.String(), res.code, res.err
					}
					appendLine(line)
				default:
					return buf.String(), res.code, res.err
				}
			}
		}
	}
}

var (
	scriptDestRe = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
	scriptNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

// validateScriptTarget 校验 script_dest/script_name 仅含白名单字符，
// 防止未加引号拼入 shell 命令时注入元字符（如 ; | $()）。
func validateScriptTarget(dest, name string) error {
	if !strings.HasPrefix(dest, "/") || !scriptDestRe.MatchString(dest) {
		return fmt.Errorf("invalid script_dest %q: must be an absolute path containing only [A-Za-z0-9._/-]", dest)
	}
	if strings.Contains(dest, "..") {
		return fmt.Errorf("invalid script_dest %q: \"..\" is not allowed", dest)
	}
	if !scriptNameRe.MatchString(name) || name == "." || name == ".." {
		return fmt.Errorf("invalid script_name %q: only [A-Za-z0-9._-] allowed", name)
	}
	return nil
}

func buildExecCommand(command string, cfg ExecConfig) string {
	if cfg.ScriptContent == "" {
		return command
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(cfg.ScriptContent))
	remotePath := cfg.ScriptDest + "/" + cfg.ScriptName
	runCmd := remotePath
	if cfg.ScriptArgs != "" {
		runCmd += " " + cfg.ScriptArgs
	}
	execCmd := fmt.Sprintf("echo '%s' | base64 -d > %s && chmod +x %s && %s", encoded, remotePath, remotePath, runCmd)
	if !cfg.ScriptKeep {
		execCmd += "; rc=$?; rm -f " + remotePath + "; exit $rc"
	}
	return execCmd
}

// writeExecutionLog 将本次执行(单节点)日志落盘;失败不阻塞任务执行。
func (h *ExecHandler) writeExecutionLog(task *store.Task, exitCode int, output, errMsg string, duration time.Duration) {
	if h.LogWriter == nil || task == nil || task.RecordID == "" {
		return
	}
	if _, err := h.LogWriter.WriteExecutionLog(task.RecordID, task.NodeID, task.ID, task.Command, exitCode, output, errMsg, duration); err != nil {
		log.Printf("write execution log: %v", err)
	}
}

func (h *ExecHandler) recordCommandExecution(ctx context.Context, task *store.Task, exitCode int, stdout, stderr string, durationMs int64, success bool) {
	if task == nil || task.RecordID == "" {
		return
	}
	exec := &store.CommandExecution{TaskID: task.RecordID, NodeID: task.NodeID, Command: task.Command, ExitCode: exitCode, Stdout: stdout, Stderr: stderr, DurationMs: durationMs, Success: success, CreatedAt: time.Now().UTC()}
	if err := h.History.RecordCommandExecution(ctx, exec); err != nil {
		log.Printf("record command execution: %v", err)
	}
}

func (h *ExecHandler) updateOpStatus(ctx context.Context, opID string) {
	if opID == "" || h.History == nil {
		return
	}
	rows, err := h.db.QueryContext(ctx, `SELECT status FROM tasks WHERE record_id = ?`, opID)
	if err != nil {
		return
	}
	defer rows.Close()
	var statuses []string
	for rows.Next() {
		var st string
		if err := rows.Scan(&st); err == nil {
			statuses = append(statuses, st)
		}
	}
	if len(statuses) == 0 {
		return
	}
	allDone := true
	anyFail := false
	for _, st := range statuses {
		if st == "running" || st == "queued" || st == "pending" {
			allDone = false
		}
		if st == "failed" || st == "cancelled" {
			anyFail = true
		}
	}
	if !allDone {
		return
	}
	status := "completed"
	if anyFail {
		status = "failed"
	}
	if err := h.History.UpdateOperationStatus(ctx, opID, status); err != nil {
		log.Printf("update op status: %v", err)
	}
	if h.hub != nil {
		h.hub.BroadcastHistoryUpdate()
	}
}

func parseInt(s string, defaultVal int) (int, error) {
	if s == "" {
		return defaultVal, nil
	}
	return strconv.Atoi(s)
}
