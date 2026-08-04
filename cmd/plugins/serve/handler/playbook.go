package handler

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/cangyunye/go-owl/internal/control/blacklist"
	pb "github.com/cangyunye/go-owl/pkg/playbook"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

type PlaybookHandler struct {
	db        *sql.DB
	playbooks *store.PlaybookStore
	runs      *store.PlaybookRunStore
	nodes     *store.NodeStore
	hub       *WSHub
	History   *store.HistoryStore
	checker   *blacklist.Checker
}

func NewPlaybookHandler(db *sql.DB, ps *store.PlaybookStore, rs *store.PlaybookRunStore, ns *store.NodeStore, hub *WSHub) *PlaybookHandler {
	return &PlaybookHandler{db: db, playbooks: ps, runs: rs, nodes: ns, hub: hub, checker: newBlacklistChecker()}
}

type refreshRequest struct {
	Path string `json:"path"`
}

type createTemplateRequest struct {
	Name            string                 `json:"name"`
	Description     string                 `json:"description,omitempty"`
	Version         string                 `json:"version,omitempty"`
	ExecutionMode   string                 `json:"execution_mode,omitempty"`
	Vars            map[string]interface{} `json:"vars,omitempty"`
	DefaultGroups   []string               `json:"default_groups,omitempty"`
	DefaultTags     []string               `json:"default_tags,omitempty"`
	DefaultSkipTags []string               `json:"default_skip_tags,omitempty"`
	Tasks           []createTemplateTask   `json:"tasks"`
}

type createTemplateTask struct {
	Name   string                 `json:"name"`
	Action string                 `json:"action"`
	Args   map[string]interface{} `json:"args"`
}

func (h *PlaybookHandler) Create(c *gin.Context) {
	var req createTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request: " + err.Error()})
		return
	}
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "name is required"})
		return
	}
	if req.Version == "" {
		req.Version = "1.0"
	}

	var defaultCfg *pb.TemplateDefaultConfig
	if len(req.DefaultGroups) > 0 || len(req.DefaultTags) > 0 || len(req.DefaultSkipTags) > 0 {
		defaultCfg = &pb.TemplateDefaultConfig{
			Groups:   req.DefaultGroups,
			Tags:     req.DefaultTags,
			SkipTags: req.DefaultSkipTags,
		}
	}

	tasks := make([]pb.TemplateTask, len(req.Tasks))
	for i, t := range req.Tasks {
		tasks[i] = pb.TemplateTask{
			Name:   t.Name,
			Action: t.Action,
			Args:   t.Args,
		}
	}

	tpl := &pb.TemplatePlaybook{
		Name:          req.Name,
		Description:   req.Description,
		Version:       req.Version,
		Hosts:         []string{},
		ExecutionMode: req.ExecutionMode,
		Default:       defaultCfg,
		Vars:          req.Vars,
		PreTasks:      []pb.TemplateTask{},
		Tasks:         tasks,
		PostTasks:     []pb.TemplateTask{},
	}

	yamlData, err := pb.RenderTemplateYAML(tpl)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "generate yaml failed"})
		return
	}

	libraryPath := h.getPlaybookLibraryPath()
	outputPath := filepath.Join(libraryPath, req.Name+".yaml")

	if err := os.MkdirAll(libraryPath, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "create directory failed"})
		return
	}
	if err := os.WriteFile(outputPath, yamlData, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "write file failed"})
		return
	}

	_, _, err = h.playbooks.SyncFromDir(c.Request.Context(), libraryPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "sync failed: " + err.Error()})
		return
	}

	all, _ := h.playbooks.List(c.Request.Context())
	var created *model.Playbook
	for i := range all {
		if all[i].Name == req.Name {
			created = all[i]
			break
		}
	}

	c.JSON(http.StatusCreated, gin.H{"data": created, "file_path": outputPath})
}

func (h *PlaybookHandler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "path is required"})
		return
	}

	upsertSetting(h.db, "playbook_library_path", req.Path)

	playbooks, syncErrors, err := h.playbooks.SyncFromDir(c.Request.Context(), req.Path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	all, _ := h.playbooks.List(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{
		"data":      all,
		"refreshed": len(playbooks),
		"total":     len(all),
		"errors":    syncErrors,
	})
}

func (h *PlaybookHandler) List(c *gin.Context) {
	playbooks, err := h.playbooks.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "query error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": playbooks})
}

func (h *PlaybookHandler) Get(c *gin.Context) {
	id := c.Param("id")
	pb, err := h.playbooks.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "playbook not found"})
		return
	}
	c.JSON(http.StatusOK, pb)
}

func (h *PlaybookHandler) GetFile(c *gin.Context) {
	id := c.Param("id")
	pb, err := h.playbooks.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "playbook not found"})
		return
	}
	if !pb.FileExists {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "file not found"})
		return
	}
	data, err := os.ReadFile(pb.FilePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "read failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"name": pb.Name, "content": string(data)})
}

type runRequest struct {
	TargetNodes     []string          `json:"target_nodes"`
	Groups          []string          `json:"groups,omitempty"`
	ExtraVars       map[string]string `json:"extra_vars,omitempty"`
	Tags            string            `json:"tags,omitempty"`
	DangerConfirmed bool              `json:"danger_confirmed,omitempty"`
}

func (h *PlaybookHandler) Run(c *gin.Context) {
	id := c.Param("id")
	pb, err := h.playbooks.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "playbook not found"})
		return
	}
	if !pb.FileExists {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "playbook file no longer exists at: " + pb.FilePath})
		return
	}

	var req runRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request"})
		return
	}

	if len(req.TargetNodes) == 0 && len(req.Groups) > 0 {
		nodeIDs, err := h.nodes.ListByGroups(c.Request.Context(), req.Groups)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "group resolve failed"})
			return
		}
		req.TargetNodes = nodeIDs
	}

	if len(req.TargetNodes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "target_nodes or groups is required"})
		return
	}

	run, err := h.runs.Create(c.Request.Context(), pb.ID, pb.Name, pb.FilePath, req.TargetNodes, req.ExtraVars, req.Tags, req.DangerConfirmed)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "create run failed"})
		return
	}

	op := &store.Operation{TaskID: run.ID, OpType: "playbook", Command: "playbook run " + pb.Name, Targets: req.TargetNodes, PlaybookPath: pb.FilePath, Status: "running", CreatedAt: time.Now().UTC()}
	if err := h.History.RecordOperation(c.Request.Context(), op); err != nil {
		log.Printf("record history: %v", err)
	}
	if h.hub != nil {
		h.hub.BroadcastHistoryUpdate()
	}

	go h.executePlaybookRunV2(run.ID)

	if h.hub != nil {
		h.hub.Broadcast(WSMessage{Type: "playbook_run_update", Data: run})
	}

	c.JSON(http.StatusAccepted, run)
}

func (h *PlaybookHandler) RunList(c *gin.Context) {
	runs, total, err := h.runs.List(c.Request.Context(), 50, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "query error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": runs,
		"meta": gin.H{"total": total},
	})
}

func (h *PlaybookHandler) RunGet(c *gin.Context) {
	id := c.Param("id")
	run, err := h.runs.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "run not found"})
		return
	}
	c.JSON(http.StatusOK, run)
}

func (h *PlaybookHandler) GetSettingsPath(c *gin.Context) {
	var path string
	h.db.QueryRow(`SELECT value FROM settings WHERE key = 'playbook_library_path'`).Scan(&path)
	if path == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, ".owl", "playbooks")
		}
	}
	c.JSON(http.StatusOK, gin.H{"value": path})
}

func (h *PlaybookHandler) getPlaybookLibraryPath() string {
	var path string
	err := h.db.QueryRow(`SELECT value FROM settings WHERE key = 'playbook_library_path'`).Scan(&path)
	if err != nil || path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".owl", "playbooks")
	}
	return path
}

func (h *PlaybookHandler) RunCancel(c *gin.Context) {
	id := c.Param("id")
	run, err := h.runs.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "run not found"})
		return
	}
	if run.Status == model.RunStatusCompleted || run.Status == model.RunStatusFailed || run.Status == model.RunStatusCancelled {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "run already finished"})
		return
	}
	h.runs.UpdateStatus(c.Request.Context(), id, model.RunStatusCancelled, "cancelled by user")
	c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
}

func (h *PlaybookHandler) executePlaybookRun(runID string) {
	ctx := context.Background()

	run, err := h.runs.Get(ctx, runID)
	if err != nil {
		return
	}

	h.runs.UpdateStatus(ctx, runID, model.RunStatusRunning, "")
	run.Status = model.RunStatusRunning
	if h.hub != nil {
		h.hub.Broadcast(WSMessage{Type: "playbook_run_update", Data: run})
	}

	exec, err := func() (Executor, error) {
		return &sshExecutor{db: h.db}, nil
	}()
	if err != nil {
		h.runs.UpdateStatus(ctx, runID, model.RunStatusFailed, err.Error())
		return
	}

	// Parse playbook YAML to get task list
	data, err := os.ReadFile(run.PlaybookFile)
	if err != nil {
		h.runs.UpdateStatus(ctx, runID, model.RunStatusFailed, "cannot read playbook file: "+err.Error())
		return
	}

	var pbDef struct {
		Name      string                   `yaml:"name"`
		PreTasks  []map[string]interface{} `yaml:"pre_tasks"`
		Tasks     []map[string]interface{} `yaml:"tasks"`
		PostTasks []map[string]interface{} `yaml:"post_tasks"`
	}
	if err := yaml.Unmarshal(data, &pbDef); err != nil {
		h.runs.UpdateStatus(ctx, runID, model.RunStatusFailed, "parse playbook failed: "+err.Error())
		return
	}

	allTasks := append(pbDef.PreTasks, pbDef.Tasks...)
	allTasks = append(allTasks, pbDef.PostTasks...)

	if len(allTasks) == 0 {
		h.runs.UpdateStatus(ctx, runID, model.RunStatusCompleted, "")
		if h.hub != nil {
			run, _ = h.runs.Get(ctx, runID)
			h.hub.Broadcast(WSMessage{Type: "playbook_run_update", Data: run})
		}
		return
	}

	failed := false
	for _, nodeID := range run.TargetNodes {
		runCheck, _ := h.runs.Get(ctx, runID)
		if runCheck.Status == model.RunStatusCancelled {
			return
		}

		for _, taskDef := range allTasks {
			for taskName, taskBody := range taskDef {
				runCheck, _ = h.runs.Get(ctx, runID)
				if runCheck.Status == model.RunStatusCancelled {
					return
				}

				start := time.Now()
				step := h.executePlaybookTask(ctx, exec, nodeID, taskName, taskBody)
				step.DurationMs = time.Since(start).Milliseconds()
				h.runs.AppendResult(ctx, runID, step)

				ce := &store.CommandExecution{TaskID: runID, NodeID: step.NodeID, Command: step.TaskName, ExitCode: step.ExitCode, Stdout: step.Output, Stderr: step.Error, DurationMs: step.DurationMs, Success: step.ExitCode == 0, CreatedAt: time.Now().UTC()}
				if e := h.History.RecordCommandExecution(ctx, ce); e != nil {
					log.Printf("record command execution: %v", e)
				}

				if step.ExitCode != 0 {
					failed = true
				}

				run, _ = h.runs.Get(ctx, runID)
				if h.hub != nil {
					h.hub.Broadcast(WSMessage{Type: "playbook_run_update", Data: run})
				}
			}
		}
	}

	finalStatus := model.RunStatusCompleted
	if failed {
		finalStatus = model.RunStatusFailed
	}
	h.runs.UpdateStatus(ctx, runID, finalStatus, "")
	opStatus := "completed"
	if failed {
		opStatus = "failed"
	}
	if err := h.History.UpdateOperationStatus(ctx, runID, opStatus); err != nil {
		log.Printf("update op status: %v", err)
	}
	if h.hub != nil {
		h.hub.BroadcastHistoryUpdate()
	}
	run, _ = h.runs.Get(ctx, runID)
	if h.hub != nil {
		h.hub.Broadcast(WSMessage{Type: "playbook_run_update", Data: run})
	}
}

func (h *PlaybookHandler) executePlaybookTask(ctx context.Context, exec Executor, nodeID, taskName string, taskBody interface{}) *model.StepResult {
	taskMap, ok := taskBody.(map[string]interface{})
	if !ok {
		return &model.StepResult{
			TaskName: taskName, NodeID: nodeID, Status: "failed",
			ExitCode: 1, Error: "invalid task body",
		}
	}

	command := extractCommand(taskMap)
	if command == "" {
		return &model.StepResult{
			TaskName: taskName, NodeID: nodeID, Action: extractAction(taskMap),
			Status: "skipped", ExitCode: 0,
		}
	}

	output, exitCode, execErr := exec.Execute(ctx, nodeID, command)
	status := "completed"
	errMsg := ""
	if execErr != nil {
		status = "failed"
		errMsg = execErr.Error()
	}
	if len(output) > 4096 {
		output = output[:4093] + "..."
	}

	return &model.StepResult{
		TaskName: taskName, NodeID: nodeID, Action: extractAction(taskMap),
		Status: status, ExitCode: exitCode, Output: output, Error: errMsg,
	}
}

func extractCommand(m map[string]interface{}) string {
	for _, key := range []string{"command", "shell", "cmd", "line"} {
		if v, ok := m[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func extractAction(m map[string]interface{}) string {
	if a, ok := m["action"].(string); ok {
		return a
	}
	for _, key := range []string{"command", "shell", "script", "template", "copy", "file", "service", "package", "git"} {
		if _, ok := m[key]; ok {
			return key
		}
	}
	return ""
}

func upsertSetting(db *sql.DB, key, value string) {
	db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
}
