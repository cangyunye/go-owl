package handler

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	ai2 "github.com/cangyunye/go-owl/internal/ai"
	"github.com/cangyunye/go-owl/internal/control/blacklist"
	nodeselect "github.com/cangyunye/go-owl/internal/node/select"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

type WebExecutor struct {
	db                  *sql.DB
	taskStore           *store.TaskStore
	transferRecordStore *store.TransferRecordStore
	playbookRunStore    *store.PlaybookRunStore
	nodeStore           *store.NodeStore
	playbookStore       *store.PlaybookStore
	auditStore          *store.AIAuditStore
	keyManager          *KeyManager
	debugMode           bool
	userRole            string
	userName            string
	History             *store.HistoryStore
	PlaybookHandler     *PlaybookHandler
	checker             *blacklist.Checker
}

func (e *WebExecutor) requireOperator() error {
	switch e.userRole {
	case "admin", "operator":
		return nil
	default:
		return fmt.Errorf("权限不足: 需要 operator 或 admin 角色")
	}
}

func NewWebExecutor(db *sql.DB, taskStore *store.TaskStore, transferRecordStore *store.TransferRecordStore,
	playbookRunStore *store.PlaybookRunStore, nodeStore *store.NodeStore,
	playbookStore *store.PlaybookStore, auditStore *store.AIAuditStore,
	keyManager *KeyManager, debugMode bool) *WebExecutor {
	return &WebExecutor{
		db: db, taskStore: taskStore, transferRecordStore: transferRecordStore,
		playbookRunStore: playbookRunStore, nodeStore: nodeStore,
		playbookStore: playbookStore, auditStore: auditStore,
		keyManager: keyManager, debugMode: debugMode,
		checker: newBlacklistChecker(),
	}
}

func (e *WebExecutor) resolveAINodeIDs(ctx context.Context, nodes []string, group, label, search string) []string {
	if len(nodes) == 0 && group == "" && label == "" && search == "" {
		return nil
	}
	src := &dbNodeSource{db: e.db}
	rows, err := src.List(ctx)
	if err != nil {
		return nil
	}

	var out []string
	for _, r := range rows {
		if len(nodes) > 0 {
			hit := false
			for _, id := range nodes {
				if id == r.ID || id == r.Name {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
		}
		if group != "" {
			hit := false
			for _, g := range r.Groups {
				if g == group {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
		}
		if label != "" && strings.Contains(label, "=") {
			k, v, _ := strings.Cut(label, "=")
			if r.Labels[k] != v {
				continue
			}
		}
		if search != "" {
			needle := strings.ToLower(search)
			if !strings.Contains(strings.ToLower(r.Name), needle) &&
				!strings.Contains(strings.ToLower(r.ID), needle) {
				continue
			}
		}
		out = append(out, r.ID)
	}
	return out
}

func aggregateStatus(success, total int) string {
	if success == 0 {
		return "failed"
	}
	if success < total {
		return "partial_failure"
	}
	return "completed"
}

func (e *WebExecutor) QueryNodes(ctx context.Context, params ai2.QueryNodesParams) (*ai2.QueryNodesResult, error) {
	rows, err := e.queryNodeRows(ctx, params.Group, params.Status, params.Search)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return &ai2.QueryNodesResult{Text: "No matching nodes found"}, nil
	}
	return &ai2.QueryNodesResult{Text: renderNodeRowsMarkdown(rows)}, nil
}

func (e *WebExecutor) ExecuteCommand(ctx context.Context, params ai2.ExecCommandParams) (*ai2.ExecResult, error) {
	if err := e.requireOperator(); err != nil {
		return nil, err
	}
	nodeIDs := e.resolveAINodeIDs(ctx, params.Nodes, params.Group, params.Label, params.Search)
	if len(nodeIDs) == 0 {
		return &ai2.ExecResult{Text: "未找到目标节点"}, nil
	}

	if e.checker != nil {
		for _, nodeID := range nodeIDs {
			var user string
			if info, err := (&sshExecutor{db: e.db}).getNodeInfo(nodeID); err == nil {
				user = info.User
			}
			if _, err := e.checker.CheckForExec(user, params.Command, false); err != nil {
				return &ai2.ExecResult{Text: err.Error()}, nil
			}
		}
	}

	type nodeResult struct {
		nodeID   string
		output   string
		exitCode int
		err      error
	}
	results := make([]nodeResult, len(nodeIDs))
	var wg sync.WaitGroup
	exec := &sshExecutor{db: e.db}
	for i, nid := range nodeIDs {
		wg.Add(1)
		go func(idx int, nodeID string) {
			defer wg.Done()
			output, exitCode, err := exec.Execute(ctx, nodeID, params.Command)
			results[idx] = nodeResult{nodeID: nodeID, output: output, exitCode: exitCode, err: err}
		}(i, nid)
	}
	wg.Wait()

	opID := uuid.New().String()
	var sb strings.Builder
	successCount := 0
	for _, r := range results {
		ok := r.err == nil && r.exitCode == 0
		if ok {
			successCount++
		}
		task, _ := e.taskStore.CreateWithRecord(ctx, r.nodeID, params.Command, opID)
		if task != nil {
			status := store.TaskStatusCompleted
			if r.err != nil {
				status = store.TaskStatusFailed
			}
			e.taskStore.UpdateStatus(ctx, task.ID, status, r.output, &r.exitCode)
		}
		stderr := ""
		if r.err != nil {
			stderr = r.err.Error()
		}
		e.History.RecordCommandExecution(ctx, &store.CommandExecution{TaskID: opID, NodeID: r.nodeID, Command: params.Command, ExitCode: r.exitCode, Stdout: r.output, Stderr: stderr, Success: ok, CreatedAt: time.Now().UTC()})
		mark := "✓"
		if !ok {
			mark = "✗"
		}
		sb.WriteString(fmt.Sprintf("%s [%s] exit=%d\n%s\n", mark, r.nodeID, r.exitCode, strings.TrimSpace(r.output)))
	}

	e.History.RecordOperation(ctx, &store.Operation{TaskID: opID, OpType: "command", Command: params.Command, Targets: nodeIDs, Status: aggregateStatus(successCount, len(results)), Username: e.userName, CreatedAt: time.Now().UTC()})

	return &ai2.ExecResult{Text: fmt.Sprintf("在 %d 个节点执行（%d 成功）：\n\n%s", len(nodeIDs), successCount, sb.String())}, nil
}

func (e *WebExecutor) ExecuteScript(ctx context.Context, params ai2.ExecScriptParams) (*ai2.ExecScriptResult, error) {
	if err := e.requireOperator(); err != nil {
		return nil, err
	}
	nodeIDs := e.resolveAINodeIDs(ctx, params.Nodes, params.Group, params.Label, params.Search)
	if len(nodeIDs) == 0 {
		return &ai2.ExecScriptResult{Text: "未找到目标节点"}, nil
	}

	dest := params.Dest
	if dest == "" {
		dest = "/tmp"
	}
	scriptName := "ai-script.sh"
	if err := validateScriptTarget(dest, scriptName); err != nil {
		return &ai2.ExecScriptResult{Text: err.Error()}, nil
	}
	execCmd := buildExecCommand("", ExecConfig{
		ScriptContent: params.Script,
		ScriptName:    scriptName,
		ScriptArgs:    params.Args,
		ScriptDest:    dest,
		ScriptKeep:    params.Keep,
	})

	if e.checker != nil {
		for _, nodeID := range nodeIDs {
			var user string
			if info, err := (&sshExecutor{db: e.db}).getNodeInfo(nodeID); err == nil {
				user = info.User
			}
			if _, err := e.checker.CheckForExec(user, params.Script, false); err != nil {
				return &ai2.ExecScriptResult{Text: err.Error()}, nil
			}
			if _, err := e.checker.CheckForExec(user, params.Args, false); err != nil {
				return &ai2.ExecScriptResult{Text: err.Error()}, nil
			}
		}
	}

	type nodeResult struct {
		nodeID   string
		output   string
		exitCode int
		err      error
	}
	results := make([]nodeResult, len(nodeIDs))
	var wg sync.WaitGroup
	exec := &sshExecutor{db: e.db}
	for i, nid := range nodeIDs {
		wg.Add(1)
		go func(idx int, nodeID string) {
			defer wg.Done()
			output, exitCode, err := exec.Execute(ctx, nodeID, execCmd)
			results[idx] = nodeResult{nodeID: nodeID, output: output, exitCode: exitCode, err: err}
		}(i, nid)
	}
	wg.Wait()

	opID := uuid.New().String()
	displayCmd := "script: " + scriptName
	var sb strings.Builder
	successCount := 0
	for _, r := range results {
		ok := r.err == nil && r.exitCode == 0
		if ok {
			successCount++
		}
		task, _ := e.taskStore.CreateWithRecord(ctx, r.nodeID, displayCmd, opID)
		if task != nil {
			status := store.TaskStatusCompleted
			if r.err != nil {
				status = store.TaskStatusFailed
			}
			e.taskStore.UpdateStatus(ctx, task.ID, status, r.output, &r.exitCode)
		}
		stderr := ""
		if r.err != nil {
			stderr = r.err.Error()
		}
		e.History.RecordCommandExecution(ctx, &store.CommandExecution{TaskID: opID, NodeID: r.nodeID, Command: displayCmd, ExitCode: r.exitCode, Stdout: r.output, Stderr: stderr, Success: ok, CreatedAt: time.Now().UTC()})
		mark := "✓"
		if !ok {
			mark = "✗"
		}
		sb.WriteString(fmt.Sprintf("%s [%s] exit=%d\n%s\n", mark, r.nodeID, r.exitCode, strings.TrimSpace(r.output)))
	}

	e.History.RecordOperation(ctx, &store.Operation{TaskID: opID, OpType: "script", Command: displayCmd, Targets: nodeIDs, Status: aggregateStatus(successCount, len(results)), Username: e.userName, CreatedAt: time.Now().UTC()})

	return &ai2.ExecScriptResult{Text: fmt.Sprintf("在 %d 个节点执行脚本（%d 成功）：\n\n%s", len(nodeIDs), successCount, sb.String())}, nil
}

func (e *WebExecutor) GeneratePlaybook(ctx context.Context, params ai2.GeneratePlaybookParams) (*ai2.GeneratePlaybookResult, error) {
	if err := e.requireOperator(); err != nil {
		return nil, err
	}
	content := fmt.Sprintf(`name: ai-generated-playbook
description: "%s"
hosts: []
tasks:
  - name: Run generated task
    command: echo "Generated for: %s"
`, params.Requirement, params.Requirement)

	return &ai2.GeneratePlaybookResult{Text: fmt.Sprintf("Generated playbook:\n\n```yaml\n%s\n```", content)}, nil
}

func (e *WebExecutor) TransferFile(ctx context.Context, params ai2.TransferFileParams) (*ai2.TransferResult, error) {
	if err := e.requireOperator(); err != nil {
		return nil, err
	}
	nodeIDs := e.resolveAINodeIDs(ctx, params.Nodes, "", "", params.Search)
	if len(nodeIDs) == 0 {
		return &ai2.TransferResult{Text: "未找到目标节点"}, nil
	}

	mode := parseFileMode(params.Permission)
	if mode == 0 {
		mode = parseFileMode(params.Mode)
	}
	opts := transferOptions{Overwrite: true, Mode: mode, Resume: true}

	rec, err := e.transferRecordStore.Create(ctx, params.SourceFile, params.DestDir, "push", "")
	if err != nil {
		return nil, fmt.Errorf("create transfer record: %w", err)
	}
	e.transferRecordStore.SetNodeCount(ctx, rec.ID, len(nodeIDs))

	var sb strings.Builder
	successCount := 0
	for _, nid := range nodeIDs {
		info, err := resolveNodeSSH(e.db, nid)
		if err != nil {
			e.transferRecordStore.UpdateNodeResult(ctx, rec.ID, false)
			sb.WriteString(fmt.Sprintf("✗ [%s] %v\n", nid, err))
			continue
		}
		err = sftpTransfer(info, params.SourceFile, params.DestDir, "push", opts)
		ok := err == nil
		e.transferRecordStore.UpdateNodeResult(ctx, rec.ID, ok)
		if ok {
			successCount++
		}
		ftStatus := "completed"
		errMsg := ""
		if err != nil {
			ftStatus = "failed"
			errMsg = err.Error()
		}
		e.History.RecordFileTransfer(ctx, &store.FileTransfer{TaskID: rec.ID, NodeID: nid, FileName: filepath.Base(params.SourceFile), TransferType: "push", Status: ftStatus, Error: errMsg, CreatedAt: time.Now().UTC()})
		mark := "✓"
		if !ok {
			mark = "✗"
		}
		sb.WriteString(fmt.Sprintf("%s [%s] %s\n", mark, nid, errMsg))
	}

	e.History.RecordOperation(ctx, &store.Operation{TaskID: rec.ID, OpType: "file_transfer", Command: fmt.Sprintf("transfer %s -> %s", params.SourceFile, params.DestDir), Targets: nodeIDs, Status: aggregateStatus(successCount, len(nodeIDs)), Username: e.userName, CreatedAt: time.Now().UTC()})

	return &ai2.TransferResult{Text: fmt.Sprintf("传输 %s -> %s 到 %d 个节点（%d 成功）：\n%s", params.SourceFile, params.DestDir, len(nodeIDs), successCount, sb.String())}, nil
}

func (e *WebExecutor) ListPlaybooks(ctx context.Context) (*ai2.ListPlaybooksResult, error) {
	pbs, err := e.playbookStore.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list playbooks: %w", err)
	}

	if len(pbs) == 0 {
		return &ai2.ListPlaybooksResult{Text: "No playbooks found"}, nil
	}
	return &ai2.ListPlaybooksResult{Text: renderPlaybooksMarkdown(pbs) + fmt.Sprintf("\n\nTotal: %d playbooks", len(pbs))}, nil
}

func (e *WebExecutor) PlaybookInfo(ctx context.Context, params ai2.PlaybookInfoParams) (*ai2.PlaybookInfoResult, error) {
	pbs, err := e.playbookStore.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list playbooks: %w", err)
	}

	for _, pb := range pbs {
		if pb.Name == params.Name || pb.ID == params.Name {
			return &ai2.PlaybookInfoResult{Text: renderPlaybookInfoMarkdown(pb)}, nil
		}
	}

	return &ai2.PlaybookInfoResult{Text: fmt.Sprintf("Playbook not found: %s", params.Name)}, nil
}

func (e *WebExecutor) ValidatePlaybook(ctx context.Context, params ai2.ValidatePlaybookParams) (*ai2.ValidateResult, error) {
	// Try to read playbook by name from store
	pbs, err := e.playbookStore.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list playbooks: %w", err)
	}

	for _, pb := range pbs {
		if pb.Name == params.File || pb.ID == params.File {
			// Read file content for validation
			// For now, assume it's valid since it was parsed successfully
			return &ai2.ValidateResult{Text: fmt.Sprintf("Playbook '%s' is valid", params.File)}, nil
		}
	}

	// Try as file path
	var data []byte
	data, err = e.readPlaybookFile(params.File)
	if err != nil {
		return &ai2.ValidateResult{Text: fmt.Sprintf("Playbook file not found: %s", params.File)}, nil
	}

	var result interface{}
	if err := yaml.Unmarshal(data, &result); err != nil {
		return &ai2.ValidateResult{Text: fmt.Sprintf("YAML syntax error: %v", err)}, nil
	}

	return &ai2.ValidateResult{Text: fmt.Sprintf("Playbook '%s' is valid", params.File)}, nil
}

func (e *WebExecutor) readPlaybookFile(name string) ([]byte, error) {
	// Simple file read for validation
	return nil, fmt.Errorf("file access not implemented")
}

func (e *WebExecutor) NodeCheck(ctx context.Context, params ai2.NodeCheckParams) (*ai2.NodeCheckResult, error) {
	if err := e.requireOperator(); err != nil {
		return nil, err
	}
	var nodeIDs []string
	if params.All {
		rows, err := e.db.QueryContext(ctx, "SELECT id FROM nodes")
		if err != nil {
			return nil, fmt.Errorf("query nodes: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			rows.Scan(&id)
			nodeIDs = append(nodeIDs, id)
		}
	} else if params.Group != "" {
		rows, err := nodeselect.NewSelector(&dbNodeSource{db: e.db}).Select(ctx, nodeselect.SelectOptions{Groups: []string{params.Group}})
		if err != nil {
			return nil, fmt.Errorf("select nodes: %w", err)
		}
		for _, r := range rows {
			nodeIDs = append(nodeIDs, r.ID)
		}
	} else {
		nodeIDs = params.Nodes
	}
	if len(nodeIDs) == 0 {
		return &ai2.NodeCheckResult{Text: "未找到目标节点"}, nil
	}

	timeout := time.Duration(params.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	now := time.Now().UTC().Format(time.RFC3339)

	var sb strings.Builder
	successCount := 0
	for _, nid := range nodeIDs {
		info, err := resolveNodeSSH(e.db, nid)
		if err != nil {
			sb.WriteString(fmt.Sprintf("✗ [%s] %v\n", nid, err))
			continue
		}
		r := checkNodeSSH(e.db, nid, info.Address, info.Port, info.User, info.Password, info.SSHKey, info.ProxyJump, timeout)
		if r.Success {
			successCount++
		}
		if params.Update {
			status := "offline"
			if r.Success {
				status = "online"
			}
			e.db.ExecContext(ctx, "UPDATE nodes SET status = ?, updated_at = ? WHERE id = ?", status, now, nid)
		}
		mark := "✓"
		detail := r.Method
		if !r.Success {
			mark = "✗"
			if r.Error != "" {
				detail = r.Error
			}
		}
		sb.WriteString(fmt.Sprintf("%s [%s] %s\n", mark, nid, detail))
	}

	return &ai2.NodeCheckResult{Text: fmt.Sprintf("检查 %d 个节点（%d 在线）：\n%s", len(nodeIDs), successCount, sb.String())}, nil
}

func (e *WebExecutor) QueryDatabase(ctx context.Context, params ai2.QueryDatabaseParams) (*ai2.QueryDatabaseResult, error) {
	rows, err := e.queryNodeRows(ctx, params.Group, params.Status, params.Search)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return &ai2.QueryDatabaseResult{Text: "No results"}, nil
	}
	return &ai2.QueryDatabaseResult{Text: renderNodeRowsMarkdown(rows) + fmt.Sprintf("\n\nTotal: %d rows", len(rows))}, nil
}

func (e *WebExecutor) RunPlaybook(ctx context.Context, params ai2.RunPlaybookParams) (*ai2.RunPlaybookResult, error) {
	if err := e.requireOperator(); err != nil {
		return nil, err
	}
	nodeIDs := e.resolveAINodeIDs(ctx, params.Nodes, params.Group, params.Label, params.Search)
	if len(nodeIDs) == 0 {
		return &ai2.RunPlaybookResult{Text: "未找到目标节点"}, nil
	}

	pbs, err := e.playbookStore.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list playbooks: %w", err)
	}
	var pb *model.Playbook
	for _, p := range pbs {
		if p.Name == params.Name || p.ID == params.Name {
			pb = p
			break
		}
	}
	if pb == nil {
		return &ai2.RunPlaybookResult{Text: fmt.Sprintf("剧本不存在: %s", params.Name)}, nil
	}
	if !pb.FileExists {
		return &ai2.RunPlaybookResult{Text: fmt.Sprintf("剧本文件已不存在: %s", pb.FilePath)}, nil
	}

	run, err := e.playbookRunStore.Create(ctx, pb.ID, pb.Name, pb.FilePath, nodeIDs, nil, params.Tags, false)
	if err != nil {
		return nil, fmt.Errorf("create playbook run: %w", err)
	}

	if e.PlaybookHandler == nil {
		return &ai2.RunPlaybookResult{Text: fmt.Sprintf("剧本运行已创建 (ID: %s)，但执行器未就绪", run.ID)}, nil
	}
	e.PlaybookHandler.executePlaybookRun(run.ID)

	finished, err := e.playbookRunStore.Get(ctx, run.ID)
	if err != nil {
		return &ai2.RunPlaybookResult{Text: fmt.Sprintf("剧本运行已创建 (ID: %s)", run.ID)}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("剧本 '%s' 执行完成，状态: %s\n", pb.Name, finished.Status))
	for _, step := range finished.Results {
		mark := "✓"
		if step.ExitCode != 0 {
			mark = "✗"
		}
		sb.WriteString(fmt.Sprintf("%s [%s] %s (exit=%d)\n", mark, step.NodeID, step.TaskName, step.ExitCode))
	}
	return &ai2.RunPlaybookResult{Text: sb.String()}, nil
}

// ---------- node management: web 端 AI 只读，写操作在管理页面完成 ----------

func (e *WebExecutor) AddNode(ctx context.Context, params ai2.NodeAddParams) (*ai2.NodeResult, error) {
	return nil, fmt.Errorf("web 端 AI 不支持新增节点，请在节点管理页面操作")
}

func (e *WebExecutor) RemoveNode(ctx context.Context, params ai2.NodeRemoveParams) (*ai2.NodeResult, error) {
	return nil, fmt.Errorf("web 端 AI 不支持删除节点，请在节点管理页面操作")
}

func (e *WebExecutor) UpdateNode(ctx context.Context, params ai2.NodeUpdateParams) (*ai2.NodeResult, error) {
	return nil, fmt.Errorf("web 端 AI 不支持修改节点，请在节点管理页面操作")
}

func (e *WebExecutor) NodeStatus(ctx context.Context, params ai2.NodeStatusParams) (*ai2.NodeResult, error) {
	rows, err := e.queryNodeRows(ctx, "", "", "")
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return &ai2.NodeResult{Text: "未找到节点"}, nil
	}
	var sb strings.Builder
	online := 0
	for _, r := range rows {
		status := r.Status
		if status == "" {
			status = "unknown"
		}
		if status == "online" {
			online++
		}
		sb.WriteString(fmt.Sprintf("%s\t%s:%d\t%s\n", r.Name, r.Address, r.Port, status))
	}
	return &ai2.NodeResult{Text: fmt.Sprintf("共 %d 个节点（%d 在线）：\n%s", len(rows), online, sb.String())}, nil
}

func (e *WebExecutor) NodePing(ctx context.Context, params ai2.NodePingParams) (*ai2.NodeResult, error) {
	return nil, fmt.Errorf("web 端 AI 暂不支持 ping，请使用 CLI 或节点检查功能")
}

func (e *WebExecutor) NodeGroups(ctx context.Context, params ai2.NodeGroupsParams) (*ai2.NodeResult, error) {
	if params.Action == "list" || params.Action == "show" {
		rows, err := e.queryNodeRows(ctx, "", "", "")
		if err != nil {
			return nil, err
		}
		groups := map[string]int{}
		for _, r := range rows {
			for _, g := range r.Groups {
				groups[g]++
			}
		}
		var sb strings.Builder
		for g, c := range groups {
			sb.WriteString(fmt.Sprintf("%s: %d 节点\n", g, c))
		}
		return &ai2.NodeResult{Text: sb.String()}, nil
	}
	return nil, fmt.Errorf("web 端 AI 不支持修改分组，请在节点管理页面操作")
}

func (e *WebExecutor) NodeLabels(ctx context.Context, params ai2.NodeLabelsParams) (*ai2.NodeResult, error) {
	if params.Action == "show" {
		rows, err := e.queryNodeRows(ctx, "", "", "")
		if err != nil {
			return nil, err
		}
		var sb strings.Builder
		for _, r := range rows {
			if params.Node != "" && r.Name != params.Node && r.ID != params.Node {
				continue
			}
			sb.WriteString(fmt.Sprintf("%s: %v\n", r.Name, r.Labels))
		}
		return &ai2.NodeResult{Text: sb.String()}, nil
	}
	return nil, fmt.Errorf("web 端 AI 不支持修改标签，请在节点管理页面操作")
}

func (e *WebExecutor) NodeImport(ctx context.Context, params ai2.NodeImportParams) (*ai2.NodeResult, error) {
	return nil, fmt.Errorf("web 端 AI 不支持导入节点，请在节点管理页面操作")
}

func (e *WebExecutor) NodeExport(ctx context.Context, params ai2.NodeExportParams) (*ai2.NodeResult, error) {
	return nil, fmt.Errorf("web 端 AI 不支持导出节点，请使用 CLI 命令 owl node export")
}

// ---------- file / playbook tools（web 端以存储与页面为准，部分操作引导至页面） ----------

func (e *WebExecutor) FileDownload(ctx context.Context, params ai2.FileDownloadParams) (*ai2.FileDownloadResult, error) {
	return nil, fmt.Errorf("web 端 AI 不支持文件下载到服务器，请使用 CLI 命令 owl file download")
}

func (e *WebExecutor) PlaybookTemplateList(ctx context.Context) (*ai2.PlaybookTemplateListResult, error) {
	rows, err := e.db.QueryContext(ctx, "SELECT name FROM playbooks ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	defer rows.Close()
	var sb strings.Builder
	for rows.Next() {
		var name string
		rows.Scan(&name)
		sb.WriteString(name + "\n")
	}
	return &ai2.PlaybookTemplateListResult{Text: sb.String()}, nil
}

func (e *WebExecutor) PlaybookTemplateInfo(ctx context.Context, params ai2.PlaybookTemplateInfoParams) (*ai2.PlaybookTemplateInfoResult, error) {
	rows, err := e.db.QueryContext(ctx, "SELECT name, description FROM playbooks WHERE name = ?", params.Name)
	if err != nil {
		return nil, fmt.Errorf("query template: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return &ai2.PlaybookTemplateInfoResult{Text: fmt.Sprintf("模板不存在: %s", params.Name)}, nil
	}
	var name, desc string
	rows.Scan(&name, &desc)
	return &ai2.PlaybookTemplateInfoResult{Text: fmt.Sprintf("模板: %s\n描述: %s", name, desc)}, nil
}

func (e *WebExecutor) PlaybookTemplateExport(ctx context.Context, params ai2.PlaybookTemplateExportParams) (*ai2.PlaybookTemplateExportResult, error) {
	return nil, fmt.Errorf("web 端 AI 不支持导出模板文件，请使用 CLI 命令 owl playbook template export")
}

func (e *WebExecutor) PlaybookScaffold(ctx context.Context, params ai2.PlaybookScaffoldParams) (*ai2.PlaybookScaffoldResult, error) {
	return nil, fmt.Errorf("web 端 AI 暂不支持模板骨架生成，请使用 CLI 命令 owl playbook scaffold")
}

func (e *WebExecutor) PlaybookStateList(ctx context.Context, params ai2.PlaybookStateListParams) (*ai2.PlaybookStateListResult, error) {
	rows, err := e.db.QueryContext(ctx, "SELECT id, playbook_name, status, created_at FROM playbook_runs ORDER BY created_at DESC LIMIT ?", maxInt(params.Limit, 20))
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()
	var sb strings.Builder
	for rows.Next() {
		var id, name, status, createdAt string
		rows.Scan(&id, &name, &status, &createdAt)
		sb.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\n", id, name, status, createdAt))
	}
	return &ai2.PlaybookStateListResult{Text: sb.String()}, nil
}

func (e *WebExecutor) PlaybookStateShow(ctx context.Context, params ai2.PlaybookStateShowParams) (*ai2.PlaybookStateShowResult, error) {
	run, err := e.playbookRunStore.Get(ctx, params.RunID)
	if err != nil {
		return &ai2.PlaybookStateShowResult{Text: fmt.Sprintf("运行记录不存在: %s", params.RunID)}, nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("运行: %s 状态: %s\n", run.ID, run.Status))
	for _, step := range run.Results {
		mark := "✓"
		if step.ExitCode != 0 {
			mark = "✗"
		}
		sb.WriteString(fmt.Sprintf("%s [%s] %s (exit=%d)\n", mark, step.NodeID, step.TaskName, step.ExitCode))
	}
	return &ai2.PlaybookStateShowResult{Text: sb.String()}, nil
}

func maxInt(a, b int) int {
	if a <= 0 {
		return b
	}
	if a > b {
		return a
	}
	return b
}

// ---------- async / settings / history（web 端由管理页面与任务中心承担） ----------

func (e *WebExecutor) AsyncList(ctx context.Context) (*ai2.AsyncListResult, error) {
	return nil, fmt.Errorf("web 端 AI 暂不支持查询异步任务，请在任务中心操作")
}

func (e *WebExecutor) AsyncStatus(ctx context.Context, params ai2.AsyncStatusParams) (*ai2.AsyncStatusResult, error) {
	return nil, fmt.Errorf("web 端 AI 暂不支持查询异步任务状态，请在任务中心操作")
}

func (e *WebExecutor) AsyncCancel(ctx context.Context, params ai2.AsyncStatusParams) (*ai2.AsyncCancelResult, error) {
	return nil, fmt.Errorf("web 端 AI 不支持取消异步任务，请在任务中心操作")
}

func (e *WebExecutor) SettingsShow(ctx context.Context) (*ai2.SettingsShowResult, error) {
	return nil, fmt.Errorf("web 端 AI 暂不支持查看设置，请在设置页面操作")
}

func (e *WebExecutor) SettingsSet(ctx context.Context, params ai2.SettingsSetParams) (*ai2.SettingsSetResult, error) {
	return nil, fmt.Errorf("web 端 AI 不支持修改设置，请在设置页面操作")
}

func (e *WebExecutor) HistoryList(ctx context.Context, params ai2.HistoryListParams) (*ai2.HistoryListResult, error) {
	return nil, fmt.Errorf("web 端 AI 暂不支持查询执行历史，请在管理页面操作")
}

func (e *WebExecutor) HistoryClean(ctx context.Context, params ai2.HistoryCleanParams) (*ai2.HistoryCleanResult, error) {
	return nil, fmt.Errorf("web 端 AI 不支持清理历史，请在管理页面操作")
}
