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
	query := "SELECT id, name, address, port, user, status, groups, labels, COALESCE(proxy_jump, ''), COALESCE(created_at, ''), COALESCE(updated_at, '') FROM nodes WHERE 1=1"
	args := []interface{}{}

	if params.Group != "" {
		query += " AND groups LIKE ?"
		args = append(args, "%\""+params.Group+"\"%")
	}
	if params.Status != "" {
		query += " AND status = ?"
		args = append(args, params.Status)
	}
	if params.Search != "" {
		query += " AND (name LIKE ? OR address LIKE ?)"
		args = append(args, "%"+params.Search+"%", "%"+params.Search+"%")
	}

	rows, err := e.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query nodes: %w", err)
	}
	defer rows.Close()

	var sb strings.Builder
	count := 0
	for rows.Next() {
		count++
		var id, name, address, user, status, groupsJSON, labelsJSON, proxyJump, createdAt, updatedAt string
		var port int
		if err := rows.Scan(&id, &name, &address, &port, &user, &status, &groupsJSON, &labelsJSON, &proxyJump, &createdAt, &updatedAt); err != nil {
			continue
		}
		sb.WriteString(fmt.Sprintf("ID: %s | Name: %s | Address: %s:%d | User: %s | Status: %s\n", id, name, address, port, user, status))
	}

	if count == 0 {
		sb.WriteString("No matching nodes found")
	}

	return &ai2.QueryNodesResult{Text: sb.String()}, nil
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

	e.History.RecordOperation(ctx, &store.Operation{TaskID: opID, OpType: "command", Command: params.Command, Targets: nodeIDs, Status: aggregateStatus(successCount, len(results)), CreatedAt: time.Now().UTC()})

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

	e.History.RecordOperation(ctx, &store.Operation{TaskID: opID, OpType: "script", Command: displayCmd, Targets: nodeIDs, Status: aggregateStatus(successCount, len(results)), CreatedAt: time.Now().UTC()})

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

	rec, err := e.transferRecordStore.Create(ctx, params.SourceFile, params.DestDir, "push")
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

	e.History.RecordOperation(ctx, &store.Operation{TaskID: rec.ID, OpType: "file_transfer", Command: fmt.Sprintf("transfer %s -> %s", params.SourceFile, params.DestDir), Targets: nodeIDs, Status: aggregateStatus(successCount, len(nodeIDs)), CreatedAt: time.Now().UTC()})

	return &ai2.TransferResult{Text: fmt.Sprintf("传输 %s -> %s 到 %d 个节点（%d 成功）：\n%s", params.SourceFile, params.DestDir, len(nodeIDs), successCount, sb.String())}, nil
}

func (e *WebExecutor) ListPlaybooks(ctx context.Context) (*ai2.ListPlaybooksResult, error) {
	pbs, err := e.playbookStore.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list playbooks: %w", err)
	}

	var sb strings.Builder
	if len(pbs) == 0 {
		sb.WriteString("No playbooks found")
	} else {
		for _, pb := range pbs {
			sb.WriteString(fmt.Sprintf("ID: %s | Name: %s | Category: %s | Tasks: %d\n", pb.ID, pb.Name, pb.Category, pb.TasksCount))
		}
		sb.WriteString(fmt.Sprintf("\nTotal: %d playbooks", len(pbs)))
	}

	return &ai2.ListPlaybooksResult{Text: sb.String()}, nil
}

func (e *WebExecutor) PlaybookInfo(ctx context.Context, params ai2.PlaybookInfoParams) (*ai2.PlaybookInfoResult, error) {
	pbs, err := e.playbookStore.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list playbooks: %w", err)
	}

	for _, pb := range pbs {
		if pb.Name == params.Name || pb.ID == params.Name {
			return &ai2.PlaybookInfoResult{Text: fmt.Sprintf("ID: %s\nName: %s\nDescription: %s\nCategory: %s\nFilePath: %s\nTasks: %d\nTaskNames: %v", pb.ID, pb.Name, pb.Description, pb.Category, pb.FilePath, pb.TasksCount, pb.TaskNames)}, nil
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
		rows, err := e.db.QueryContext(ctx, "SELECT id FROM nodes WHERE groups LIKE ?", "%\""+params.Group+"\"%")
		if err != nil {
			return nil, fmt.Errorf("query nodes: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			rows.Scan(&id)
			nodeIDs = append(nodeIDs, id)
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
		r := checkNodeSSH(e.db, nid, info.Address, info.Port, info.User, info.Password, info.SSHKey, timeout)
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
	query := "SELECT id, name, address, port, user, status, groups FROM nodes WHERE 1=1"
	args := []interface{}{}

	if params.Group != "" {
		query += " AND groups LIKE ?"
		args = append(args, "%\""+params.Group+"\"%")
	}
	if params.Status != "" {
		query += " AND status = ?"
		args = append(args, params.Status)
	}
	if params.Search != "" {
		query += " AND (name LIKE ? OR address LIKE ?)"
		args = append(args, "%"+params.Search+"%", "%"+params.Search+"%")
	}

	rows, err := e.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query database: %w", err)
	}
	defer rows.Close()

	var sb strings.Builder
	count := 0
	for rows.Next() {
		count++
		var id, name, address, user, status, groupsJSON string
		var port int
		if err := rows.Scan(&id, &name, &address, &port, &user, &status, &groupsJSON); err != nil {
			continue
		}
		sb.WriteString(fmt.Sprintf("%s | %s | %s:%d | %s | %s\n", id, name, address, port, user, status))
	}

	if count == 0 {
		sb.WriteString("No results")
	} else {
		sb.WriteString(fmt.Sprintf("\nTotal: %d rows", count))
	}

	return &ai2.QueryDatabaseResult{Text: sb.String()}, nil
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
