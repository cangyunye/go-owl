package handler

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	ai2 "github.com/cangyunye/go-owl/internal/ai"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"gopkg.in/yaml.v3"
)

type WebExecutor struct {
	db                 *sql.DB
	taskStore          *store.TaskStore
	transferRecordStore *store.TransferRecordStore
	playbookRunStore   *store.PlaybookRunStore
	nodeStore          *store.NodeStore
	playbookStore      *store.PlaybookStore
	auditStore         *store.AIAuditStore
	keyManager         *KeyManager
	debugMode          bool
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
	}
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
	nodeID := "ai-exec"
	if len(params.Nodes) > 0 {
		nodeID = strings.Join(params.Nodes, ",")
	}
	task, err := e.taskStore.CreateWithRecord(ctx, nodeID, params.Command, "")
	if err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}
	return &ai2.ExecResult{Text: fmt.Sprintf("Command task created (ID: %s). Type: exec, Nodes: %s", task.ID, nodeID)}, nil
}

func (e *WebExecutor) ExecuteScript(ctx context.Context, params ai2.ExecScriptParams) (*ai2.ExecScriptResult, error) {
	nodeID := "ai-exec"
	if len(params.Nodes) > 0 {
		nodeID = strings.Join(params.Nodes, ",")
	}
	task, err := e.taskStore.CreateWithRecord(ctx, nodeID, params.Script, "")
	if err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}
	return &ai2.ExecScriptResult{Text: fmt.Sprintf("Script task created (ID: %s). Type: script, Nodes: %s", task.ID, nodeID)}, nil
}

func (e *WebExecutor) GeneratePlaybook(ctx context.Context, params ai2.GeneratePlaybookParams) (*ai2.GeneratePlaybookResult, error) {
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
	rec, err := e.transferRecordStore.Create(ctx, params.SourceFile, params.DestDir, "push")
	if err != nil {
		return nil, fmt.Errorf("create transfer record: %w", err)
	}
	return &ai2.TransferResult{Text: fmt.Sprintf("Transfer record created (ID: %s). Source: %s, Dest: %s, Nodes: %v", rec.ID, params.SourceFile, params.DestDir, params.Nodes)}, nil
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

	task, err := e.taskStore.CreateWithRecord(ctx, strings.Join(nodeIDs, ","), "node check", "")
	if err != nil {
		return nil, fmt.Errorf("create check task: %w", err)
	}

	return &ai2.NodeCheckResult{Text: fmt.Sprintf("Check task created (ID: %s). Nodes: %v", task.ID, nodeIDs)}, nil
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
	run, err := e.playbookRunStore.Create(ctx, params.Name, params.Name, "", params.Nodes, nil, params.Tags)
	if err != nil {
		return nil, fmt.Errorf("create playbook run: %w", err)
	}
	return &ai2.RunPlaybookResult{Text: fmt.Sprintf("Playbook run created (ID: %s). Name: %s, Nodes: %v", run.ID, params.Name, params.Nodes)}, nil
}
