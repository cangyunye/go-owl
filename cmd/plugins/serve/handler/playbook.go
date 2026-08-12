package handler

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/cangyunye/go-owl/internal/control/blacklist"
	pbexec "github.com/cangyunye/go-owl/internal/control/playbook"
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
	Category        string                 `json:"category,omitempty"`
	Version         string                 `json:"version,omitempty"`
	ExecutionMode   string                 `json:"execution_mode,omitempty"`
	Vars            map[string]interface{} `json:"vars,omitempty"`
	DefaultGroups   []string               `json:"default_groups,omitempty"`
	DefaultTags     []string               `json:"default_tags,omitempty"`
	DefaultSkipTags []string               `json:"default_skip_tags,omitempty"`
	Tags            []string               `json:"tags,omitempty"`
	Tasks           []createTemplateTask   `json:"tasks"`
	PreTasks        []createTemplateTask   `json:"pre_tasks,omitempty"`
	PostTasks       []createTemplateTask   `json:"post_tasks,omitempty"`
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

	preTasks := make([]pb.TemplateTask, len(req.PreTasks))
	for i, t := range req.PreTasks {
		preTasks[i] = pb.TemplateTask{Name: t.Name, Action: t.Action, Args: t.Args}
	}
	postTasks := make([]pb.TemplateTask, len(req.PostTasks))
	for i, t := range req.PostTasks {
		postTasks[i] = pb.TemplateTask{Name: t.Name, Action: t.Action, Args: t.Args}
	}

	tpl := &pb.TemplatePlaybook{
		Name:          req.Name,
		Description:   req.Description,
		Category:      req.Category,
		Version:       req.Version,
		Hosts:         []string{},
		ExecutionMode: req.ExecutionMode,
		Default:       defaultCfg,
		Vars:          req.Vars,
		PreTasks:      preTasks,
		Tasks:         tasks,
		PostTasks:     postTasks,
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

const maxPlaybookUploadSize = 2 * 1024 * 1024 // 2MB

// Upload 从网页上传 playbook 文件到 playbook library（同名覆盖，引用 ID 保持稳定）
func (h *PlaybookHandler) Upload(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "file is required"})
		return
	}
	defer file.Close()

	name := filepath.Base(header.Filename)
	if ext := filepath.Ext(name); ext != ".yaml" && ext != ".yml" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "only .yaml/.yml playbook files are allowed"})
		return
	}

	free, err := diskFreeOf(h.getPlaybookLibraryPath())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "cannot check disk space"})
		return
	}
	if free < minFreeBytes(h.db) {
		c.JSON(http.StatusInsufficientStorage, gin.H{"code": 507, "message": "insufficient disk space for playbook library"})
		return
	}

	if header.Size > maxPlaybookUploadSize || header.Size > int64(free-minFreeBytes(h.db)) {
		c.JSON(http.StatusInsufficientStorage, gin.H{"code": 507, "message": "playbook file too large"})
		return
	}

	libraryPath := h.getPlaybookLibraryPath()
	if err := os.MkdirAll(libraryPath, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "create directory failed"})
		return
	}

	dest := filepath.Join(libraryPath, name)
	out, err := os.Create(dest)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "cannot write file"})
		return
	}
	if _, err := io.Copy(out, file); err != nil {
		out.Close()
		os.Remove(dest)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "write failed"})
		return
	}
	out.Close()

	if _, _, err := h.playbooks.SyncFromDir(c.Request.Context(), libraryPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "sync failed: " + err.Error()})
		return
	}

	all, _ := h.playbooks.List(c.Request.Context())
	for i := range all {
		if filepath.Clean(all[i].FilePath) == filepath.Clean(dest) {
			c.JSON(http.StatusCreated, gin.H{"data": all[i], "file_path": dest})
			return
		}
	}
	c.JSON(http.StatusCreated, gin.H{"file_path": dest})
}

// Download 下载 playbook 文件到浏览器
func (h *PlaybookHandler) Download(c *gin.Context) {
	id := c.Param("id")
	pb, err := h.playbooks.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "playbook not found"})
		return
	}
	if !pb.FileExists {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "playbook file missing"})
		return
	}
	c.FileAttachment(pb.FilePath, filepath.Base(pb.FilePath))
}

// Edit 返回 playbook 的结构化编辑数据，供前端 wizard 二次编辑
func (h *PlaybookHandler) Edit(c *gin.Context) {
	id := c.Param("id")
	pb, err := h.playbooks.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "playbook not found"})
		return
	}
	if !pb.FileExists {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "playbook file missing"})
		return
	}

	category := pb.Category

	parser := pbexec.NewParser()
	parsed, err := parser.ParseFromFile(pb.FilePath)
	if err != nil {
		// 宽松解析：playbook 可能因业务校验（如 pipeline 含 post_tasks）无法执行，
		// 但编辑接口必须能打开，让用户修复
		data, rerr := os.ReadFile(pb.FilePath)
		if rerr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "read playbook failed: " + rerr.Error()})
			return
		}
		var raw pbexec.Playbook
		if yerr := yaml.Unmarshal(data, &raw); yerr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "parse playbook failed: " + err.Error()})
			return
		}
		h.renderEditResponse(c, &raw, category)
		return
	}
	h.renderEditResponse(c, parsed.Raw, category)
}

func (h *PlaybookHandler) renderEditResponse(c *gin.Context, raw *pbexec.Playbook, category string) {
	resp := createTemplateRequest{
		Name:          raw.Name,
		Description:   raw.Description,
		Category:      category,
		Version:       raw.Version,
		ExecutionMode: raw.ExecutionMode,
		Vars:          raw.Vars,
		Tasks:         toTemplateTasks(raw.Tasks),
		PreTasks:      toTemplateTasks(raw.PreTasks),
		PostTasks:     toTemplateTasks(raw.PostTasks),
		Tags:          collectPlaybookTags(raw),
	}
	if raw.Default != nil {
		resp.DefaultGroups = raw.Default.Groups
		resp.DefaultTags = raw.Default.Tags
		resp.DefaultSkipTags = raw.Default.SkipTags
	}

	c.JSON(http.StatusOK, resp)
}

// collectPlaybookTags 汇总 playbook 可用的执行标签（default.tags + 所有任务的 tags，去重排序）
func collectPlaybookTags(raw *pbexec.Playbook) []string {
	set := make(map[string]bool)
	if raw.Default != nil {
		for _, t := range raw.Default.Tags {
			if t != "" {
				set[t] = true
			}
		}
	}
	all := make([]pbexec.PlaybookTask, 0, len(raw.PreTasks)+len(raw.Tasks)+len(raw.PostTasks))
	all = append(all, raw.PreTasks...)
	all = append(all, raw.Tasks...)
	all = append(all, raw.PostTasks...)
	for _, tk := range all {
		for _, t := range tk.Tags {
			if t != "" {
				set[t] = true
			}
		}
	}
	tags := make([]string, 0, len(set))
	for t := range set {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	return tags
}

func toTemplateTasks(tasks []pbexec.PlaybookTask) []createTemplateTask {
	out := make([]createTemplateTask, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, createTemplateTask{Name: t.Name, Action: t.Action, Args: t.Args})
	}
	return out
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

	run.Warnings = h.preflightPlaybook(pb.FilePath)

	op := &store.Operation{TaskID: run.ID, OpType: "playbook", Command: "playbook run " + pb.Name, Targets: req.TargetNodes, PlaybookPath: pb.FilePath, Status: "running", CreatedAt: time.Now().UTC(), Forced: req.DangerConfirmed}
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

// preflightPlaybook 运行前预检：检查 upload/script 引用的本地源文件是否存在，
// 缺失项返回 warning（不阻塞执行）
func (h *PlaybookHandler) preflightPlaybook(pbFile string) []string {
	parser := pbexec.NewParser()
	parsed, err := parser.ParseFromFile(pbFile)
	if err != nil {
		return nil
	}

	allTasks := make([]*pbexec.ParsedTask, 0, len(parsed.PreTasks)+len(parsed.Tasks)+len(parsed.PostTasks))
	allTasks = append(allTasks, parsed.PreTasks...)
	allTasks = append(allTasks, parsed.Tasks...)
	allTasks = append(allTasks, parsed.PostTasks...)

	baseDir := filepath.Dir(pbFile)
	var warnings []string
	for _, t := range allTasks {
		var ref string
		switch strings.ToLower(t.Action) {
		case "upload":
			if s, ok := t.Args["src"].(string); ok {
				ref = s
			}
		case "script":
			if s, ok := t.Args["script"].(string); ok {
				ref = s
			}
		}
		if ref == "" {
			continue
		}
		if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
			continue
		}
		ref = strings.ReplaceAll(ref, "{{PLAYBOOK_DIR}}", baseDir)
		if !filepath.IsAbs(ref) {
			ref = filepath.Join(baseDir, ref)
		}
		if _, err := os.Stat(ref); os.IsNotExist(err) {
			warnings = append(warnings, fmt.Sprintf("task %q: src file not found: %s (upload it via the staging area first)", t.Name, ref))
		}
	}
	return warnings
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

	if h.checker != nil {
		var user string
		if info, err := (&sshExecutor{db: h.db}).getNodeInfo(nodeID); err == nil {
			user = info.User
		}
		if _, err := h.checker.CheckForExec(user, command, false); err != nil {
			return &model.StepResult{
				TaskName: taskName, NodeID: nodeID, Action: extractAction(taskMap),
				Status: "failed", ExitCode: -1, Error: err.Error(),
			}
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
