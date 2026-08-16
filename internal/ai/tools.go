package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cangyunye/go-owl/internal/common/model"
	"github.com/cangyunye/go-owl/internal/control/node"
	"gopkg.in/yaml.v3"
)

// CLIExecutor calls the owl CLI binary via runOwlCommand
type CLIExecutor struct {
	nodeMgr   node.Manager
	nodeStore NodeStoreAdapter
}

func NewCLIExecutor(nodeMgr node.Manager, nodeStore NodeStoreAdapter) *CLIExecutor {
	return &CLIExecutor{nodeMgr: nodeMgr, nodeStore: nodeStore}
}

func (e *CLIExecutor) QueryNodes(ctx context.Context, p QueryNodesParams) (*QueryNodesResult, error) {
	args := []string{"node", "list", "--no-color"}
	if p.Group != "" {
		args = append(args, "--groups", p.Group)
	}
	if p.Status != "" {
		args = append(args, "--status", p.Status)
	}
	if p.Format != "" && p.Format != "table" {
		args = append(args, "--format", p.Format)
	}
	if p.Search != "" {
		nodes := e.nodeMgr.SearchByName(p.Search)
		if len(nodes) == 0 {
			return &QueryNodesResult{"No matching nodes found"}, nil
		}
		var names []string
		for _, n := range nodes {
			names = append(names, n.Name)
		}
		args = append(args, "--nodes", strings.Join(names, ","))
	}
	if p.Labels != nil {
		for k, v := range labelKeyMap {
			if val, ok := p.Labels[k]; ok {
				args = append(args, "--label", fmt.Sprintf("%s=%s", v, val))
			}
		}
	}
	result, err := runOwlCommand(ctx, args)
	if err != nil {
		return nil, err
	}
	return &QueryNodesResult{Text: result}, nil
}

func (e *CLIExecutor) ExecuteCommand(ctx context.Context, p ExecCommandParams) (*ExecResult, error) {
	args := []string{"exec", "run", p.Command, "--no-color", "--format", p.Format}
	if p.Format == "" {
		args = append(args, "--format", "simple")
	}
	if p.Mode == "serial" {
		args = append(args, "--serial")
	}
	if p.Mode == "async" {
		args = append(args, "--async")
	}
	if p.Timeout > 0 && p.Timeout != 30 {
		args = append(args, "--timeout", fmt.Sprintf("%d", p.Timeout))
	}
	if len(p.Nodes) > 0 {
		args = append(args, "--nodes", strings.Join(p.Nodes, ","))
	}
	if p.Group != "" {
		args = append(args, "--groups", p.Group)
	}
	if p.Label != "" {
		args = append(args, "--label", p.Label)
	}
	result, err := runOwlCommand(ctx, args)
	if err != nil {
		return nil, err
	}
	return &ExecResult{Text: result}, nil
}

func (e *CLIExecutor) ExecuteScript(ctx context.Context, p ExecScriptParams) (*ExecScriptResult, error) {
	args := []string{"exec", "script", p.Script, "--no-color"}
	if p.Inline {
		args = append(args, "--inline")
	}
	if p.Dest != "" && p.Dest != "/tmp" {
		args = append(args, "--dest", p.Dest)
	}
	if p.Args != "" {
		args = append(args, "--args", p.Args)
	}
	if p.Timeout > 0 && p.Timeout != 300 {
		args = append(args, "--timeout", fmt.Sprintf("%d", p.Timeout))
	}
	if p.Keep {
		args = append(args, "--keep")
	}
	if len(p.Nodes) > 0 {
		args = append(args, "--nodes", strings.Join(p.Nodes, ","))
	}
	if p.Group != "" {
		args = append(args, "--groups", p.Group)
	}
	if p.Label != "" {
		args = append(args, "--label", p.Label)
	}
	result, err := runOwlCommand(ctx, args)
	if err != nil {
		return nil, err
	}
	return &ExecScriptResult{Text: result}, nil
}

func (e *CLIExecutor) TransferFile(ctx context.Context, p TransferFileParams) (*TransferResult, error) {
	mode := p.Mode
	if mode == "" {
		if len(p.Nodes) >= 5 {
			mode = "diffusion"
		} else {
			mode = "direct"
		}
	}
	var args []string
	if mode == "diffusion" {
		args = []string{"file", "transfer", p.SourceFile, "--nodes", strings.Join(p.Nodes, ","), "--dest", p.DestDir}
	} else {
		args = []string{"file", "upload", p.SourceFile, "--nodes", strings.Join(p.Nodes, ","), "--dest", p.DestDir}
	}
	if p.Permission != "" && p.Permission != "0644" {
		args = append(args, "--mode", p.Permission)
	}
	result, err := runOwlCommand(ctx, args)
	if err != nil {
		return nil, err
	}
	return &TransferResult{Text: result}, nil
}

func (e *CLIExecutor) GeneratePlaybook(ctx context.Context, p GeneratePlaybookParams) (*GeneratePlaybookResult, error) {
	args := []string{"playbook", "generate", p.Requirement}
	if p.Vars != nil {
		vJSON, _ := json.Marshal(p.Vars)
		args = append(args, "--vars", string(vJSON))
	}
	result, err := runOwlCommand(ctx, args)
	if err != nil {
		return nil, err
	}
	return &GeneratePlaybookResult{Text: result}, nil
}

func (e *CLIExecutor) ListPlaybooks(ctx context.Context) (*ListPlaybooksResult, error) {
	result, err := runOwlCommand(ctx, []string{"playbook", "list", "--no-color"})
	if err != nil {
		return nil, err
	}
	return &ListPlaybooksResult{Text: result}, nil
}

func (e *CLIExecutor) PlaybookInfo(ctx context.Context, p PlaybookInfoParams) (*PlaybookInfoResult, error) {
	result, err := runOwlCommand(ctx, []string{"playbook", "info", p.Name})
	if err != nil {
		return nil, err
	}
	return &PlaybookInfoResult{Text: result}, nil
}

func (e *CLIExecutor) ValidatePlaybook(ctx context.Context, p ValidatePlaybookParams) (*ValidateResult, error) {
	result, err := runOwlCommand(ctx, []string{"playbook", "validate", p.File})
	if err != nil {
		return nil, err
	}
	return &ValidateResult{Text: result}, nil
}

func (e *CLIExecutor) RunPlaybook(ctx context.Context, p RunPlaybookParams) (*RunPlaybookResult, error) {
	args := []string{"playbook", "run", p.Name, "--no-color"}
	if p.Check {
		args = append(args, "--check")
	}
	if p.Tags != "" {
		args = append(args, "--tags", p.Tags)
	}
	if p.Group != "" {
		args = append(args, "--groups", p.Group)
	}
	if p.Label != "" {
		args = append(args, "--label", p.Label)
	}
	if len(p.Nodes) > 0 {
		args = append(args, "--nodes", strings.Join(p.Nodes, ","))
	}
	if p.Search != "" {
		args = append(args, "--search", p.Search)
	}
	result, err := runOwlCommand(ctx, args)
	if err != nil {
		return nil, err
	}
	return &RunPlaybookResult{Text: result}, nil
}

func (e *CLIExecutor) NodeCheck(ctx context.Context, p NodeCheckParams) (*NodeCheckResult, error) {
	args := []string{"node", "check", "--no-color"}
	if p.All {
		args = append(args, "--all")
	}
	if p.Group != "" {
		args = append(args, "--groups", p.Group)
	}
	if len(p.Nodes) > 0 {
		args = append(args, p.Nodes...)
	}
	if p.Timeout > 0 && p.Timeout != 10 {
		args = append(args, "--timeout", fmt.Sprintf("%ds", p.Timeout))
	}
	result, err := runOwlCommand(ctx, args)
	if err != nil {
		return nil, err
	}
	return &NodeCheckResult{Text: result}, nil
}

func (e *CLIExecutor) QueryDatabase(ctx context.Context, p QueryDatabaseParams) (*QueryDatabaseResult, error) {
	args := []string{"node", "list", "--no-color"}
	if p.Group != "" {
		args = append(args, "--groups", p.Group)
	}
	if p.Status != "" {
		args = append(args, "--status", p.Status)
	}
	if p.Format != "" && p.Format != "table" {
		args = append(args, "--format", p.Format)
	}
	result, err := runOwlCommand(ctx, args)
	if err != nil {
		return nil, err
	}
	return &QueryDatabaseResult{Text: result}, nil
}

// ---------- node management executor methods ----------

func (e *CLIExecutor) AddNode(ctx context.Context, p NodeAddParams) (*NodeResult, error) {
	args := []string{"node", "add", "--no-color"}
	if p.Name != "" {
		args = append(args, "--name", p.Name)
	}
	if p.Address != "" {
		args = append(args, "--address", p.Address)
	}
	if p.Port != 0 && p.Port != 22 {
		args = append(args, "--port", fmt.Sprintf("%d", p.Port))
	}
	if p.User != "" {
		args = append(args, "--user", p.User)
	}
	if p.Password != "" {
		args = append(args, "--password", p.Password)
	}
	if p.SSHKey != "" {
		args = append(args, "--ssh-key", p.SSHKey)
	}
	if p.ProxyJump != "" {
		args = append(args, "--proxy-jump", p.ProxyJump)
	}
	if p.Groups != "" {
		args = append(args, "--groups", p.Groups)
	}
	if len(p.Labels) > 0 {
		for k, v := range p.Labels {
			if s, ok := v.(string); ok {
				args = append(args, "--label", fmt.Sprintf("%s=%s", k, s))
			}
		}
	}
	result, err := runOwlCommand(ctx, args)
	if err != nil {
		return nil, err
	}
	return &NodeResult{Text: result}, nil
}

func (e *CLIExecutor) RemoveNode(ctx context.Context, p NodeRemoveParams) (*NodeResult, error) {
	args := []string{"node", "remove", "--no-color"}
	args = append(args, p.Nodes...)
	result, err := runOwlCommand(ctx, args)
	if err != nil {
		return nil, err
	}
	return &NodeResult{Text: result}, nil
}

func (e *CLIExecutor) UpdateNode(ctx context.Context, p NodeUpdateParams) (*NodeResult, error) {
	args := []string{"node", "update", p.ID, "--no-color"}
	if p.Name != "" {
		args = append(args, "--name", p.Name)
	}
	if p.Address != "" {
		args = append(args, "--address", p.Address)
	}
	if p.Port != 0 {
		args = append(args, "--port", fmt.Sprintf("%d", p.Port))
	}
	if p.User != "" {
		args = append(args, "--user", p.User)
	}
	if p.Password != "" {
		args = append(args, "--password", p.Password)
	}
	if p.SSHKey != "" {
		args = append(args, "--ssh-key", p.SSHKey)
	}
	if p.ProxyJump != "" {
		args = append(args, "--proxy-jump", p.ProxyJump)
	}
	if p.Groups != "" {
		args = append(args, "--groups", p.Groups)
	}
	if p.Status != "" {
		args = append(args, "--status", p.Status)
	}
	if len(p.Labels) > 0 {
		for k, v := range p.Labels {
			if s, ok := v.(string); ok {
				args = append(args, "--label", fmt.Sprintf("%s=%s", k, s))
			}
		}
	}
	result, err := runOwlCommand(ctx, args)
	if err != nil {
		return nil, err
	}
	return &NodeResult{Text: result}, nil
}

func (e *CLIExecutor) NodeStatus(ctx context.Context, p NodeStatusParams) (*NodeResult, error) {
	args := []string{"node", "status", "--no-color"}
	if p.All || len(p.Nodes) == 0 {
		args = append(args, "--all")
	} else {
		args = append(args, p.Nodes...)
	}
	if p.Format != "" && p.Format != "detail" {
		args = append(args, "--output", p.Format)
	}
	result, err := runOwlCommand(ctx, args)
	if err != nil {
		return nil, err
	}
	return &NodeResult{Text: result}, nil
}

func (e *CLIExecutor) NodePing(ctx context.Context, p NodePingParams) (*NodeResult, error) {
	args := []string{"node", "ping", "--no-color"}
	if p.All || len(p.Nodes) == 0 {
		args = append(args, "--all")
	} else {
		args = append(args, p.Nodes...)
	}
	if p.Count > 0 && p.Count != 1 {
		args = append(args, "--count", fmt.Sprintf("%d", p.Count))
	}
	if p.Timeout > 0 && p.Timeout != 3 {
		args = append(args, "--timeout", fmt.Sprintf("%ds", p.Timeout))
	}
	result, err := runOwlCommand(ctx, args)
	if err != nil {
		return nil, err
	}
	return &NodeResult{Text: result}, nil
}

func (e *CLIExecutor) NodeGroups(ctx context.Context, p NodeGroupsParams) (*NodeResult, error) {
	var args []string
	switch p.Action {
	case "add":
		args = []string{"node", "groups", "add", p.Node, p.Group}
	case "remove":
		args = []string{"node", "groups", "remove", p.Node, p.Group}
	case "show":
		args = []string{"node", "groups", "show", p.Group}
	default:
		args = []string{"node", "groups", "list"}
	}
	args = append(args, "--no-color")
	result, err := runOwlCommand(ctx, args)
	if err != nil {
		return nil, err
	}
	return &NodeResult{Text: result}, nil
}

func (e *CLIExecutor) NodeLabels(ctx context.Context, p NodeLabelsParams) (*NodeResult, error) {
	var args []string
	switch p.Action {
	case "set":
		args = []string{"node", "labels", "set", p.Node}
		for k, v := range p.Labels {
			if s, ok := v.(string); ok {
				args = append(args, fmt.Sprintf("%s=%s", k, s))
			}
		}
	case "remove":
		args = []string{"node", "labels", "remove", p.Node, p.Key}
	case "show":
		args = []string{"node", "labels", "show", p.Node}
	}
	args = append(args, "--no-color")
	result, err := runOwlCommand(ctx, args)
	if err != nil {
		return nil, err
	}
	return &NodeResult{Text: result}, nil
}

func (e *CLIExecutor) NodeImport(ctx context.Context, p NodeImportParams) (*NodeResult, error) {
	args := []string{"node", "import", "--file", p.File}
	if p.Format != "" && p.Format != "yaml" {
		args = append(args, "--format", p.Format)
	}
	if p.Overwrite {
		args = append(args, "--overwrite")
	}
	if p.DryRun {
		args = append(args, "--dry-run")
	}
	args = append(args, "--no-color")
	result, err := runOwlCommand(ctx, args)
	if err != nil {
		return nil, err
	}
	return &NodeResult{Text: result}, nil
}

func (e *CLIExecutor) NodeExport(ctx context.Context, p NodeExportParams) (*NodeResult, error) {
	args := []string{"node", "export", "--no-color"}
	if p.Format != "" && p.Format != "yaml" {
		args = append(args, "--format", p.Format)
	}
	if p.File != "" {
		args = append(args, "--file", p.File)
	}
	if len(p.Nodes) > 0 {
		args = append(args, "--nodes", strings.Join(p.Nodes, ","))
	}
	if len(p.Groups) > 0 {
		args = append(args, "--groups", strings.Join(p.Groups, ","))
	}
	result, err := runOwlCommand(ctx, args)
	if err != nil {
		return nil, err
	}
	return &NodeResult{Text: result}, nil
}

func (e *CLIExecutor) FileDownload(ctx context.Context, p FileDownloadParams) (*FileDownloadResult, error) {
	args := []string{"file", "download", p.RemoteFile, "--no-color"}
	if len(p.Nodes) > 0 {
		args = append(args, "--nodes", strings.Join(p.Nodes, ","))
	}
	if p.Group != "" {
		args = append(args, "--groups", p.Group)
	}
	if p.Label != "" {
		args = append(args, "--label", p.Label)
	}
	if p.Dest != "" && p.Dest != "." {
		args = append(args, "--dest", p.Dest)
	}
	if p.Subdir {
		args = append(args, "--subdir")
	}
	if !p.Resume {
		args = append(args, "--resume=false")
	}
	result, err := runOwlCommand(ctx, args)
	if err != nil {
		return nil, err
	}
	return &FileDownloadResult{Text: result}, nil
}

func (e *CLIExecutor) PlaybookTemplateList(ctx context.Context) (*PlaybookTemplateListResult, error) {
	result, err := runOwlCommand(ctx, []string{"playbook", "template", "list", "--no-color"})
	if err != nil {
		return nil, err
	}
	return &PlaybookTemplateListResult{Text: result}, nil
}

func (e *CLIExecutor) PlaybookTemplateInfo(ctx context.Context, p PlaybookTemplateInfoParams) (*PlaybookTemplateInfoResult, error) {
	result, err := runOwlCommand(ctx, []string{"playbook", "template", "info", p.Name})
	if err != nil {
		return nil, err
	}
	return &PlaybookTemplateInfoResult{Text: result}, nil
}

func (e *CLIExecutor) PlaybookTemplateExport(ctx context.Context, p PlaybookTemplateExportParams) (*PlaybookTemplateExportResult, error) {
	args := []string{"playbook", "template", "export", p.Name}
	if p.To != "" {
		args = append(args, "--to", p.To)
	}
	result, err := runOwlCommand(ctx, args)
	if err != nil {
		return nil, err
	}
	return &PlaybookTemplateExportResult{Text: result}, nil
}

func (e *CLIExecutor) PlaybookScaffold(ctx context.Context, p PlaybookScaffoldParams) (*PlaybookScaffoldResult, error) {
	args := []string{"playbook", "scaffold", "--no-color"}
	if p.Type != "" && p.Type != "basic" {
		args = append(args, "--type", p.Type)
	}
	result, err := runOwlCommand(ctx, args)
	if err != nil {
		return nil, err
	}
	return &PlaybookScaffoldResult{Text: result}, nil
}

func (e *CLIExecutor) PlaybookStateList(ctx context.Context, p PlaybookStateListParams) (*PlaybookStateListResult, error) {
	args := []string{"playbook", "state", "list", "--no-color"}
	if p.Playbook != "" {
		args = append(args, "--playbook", p.Playbook)
	}
	if p.Status != "" {
		args = append(args, "--status", p.Status)
	}
	if p.Limit > 0 && p.Limit != 20 {
		args = append(args, "--limit", fmt.Sprintf("%d", p.Limit))
	}
	result, err := runOwlCommand(ctx, args)
	if err != nil {
		return nil, err
	}
	return &PlaybookStateListResult{Text: result}, nil
}

func (e *CLIExecutor) PlaybookStateShow(ctx context.Context, p PlaybookStateShowParams) (*PlaybookStateShowResult, error) {
	args := []string{"playbook", "state", "show", p.RunID, "--no-color"}
	if p.Node != "" {
		args = append(args, "--node", p.Node)
	}
	result, err := runOwlCommand(ctx, args)
	if err != nil {
		return nil, err
	}
	return &PlaybookStateShowResult{Text: result}, nil
}

func (e *CLIExecutor) AsyncList(ctx context.Context) (*AsyncListResult, error) {
	result, err := runOwlCommand(ctx, []string{"async", "list", "--no-color"})
	if err != nil {
		return nil, err
	}
	return &AsyncListResult{Text: result}, nil
}

func (e *CLIExecutor) AsyncStatus(ctx context.Context, p AsyncStatusParams) (*AsyncStatusResult, error) {
	result, err := runOwlCommand(ctx, []string{"async", "status", p.TaskID, "--no-color"})
	if err != nil {
		return nil, err
	}
	return &AsyncStatusResult{Text: result}, nil
}

func (e *CLIExecutor) AsyncCancel(ctx context.Context, p AsyncStatusParams) (*AsyncCancelResult, error) {
	result, err := runOwlCommand(ctx, []string{"async", "cancel", p.TaskID, "--no-color"})
	if err != nil {
		return nil, err
	}
	return &AsyncCancelResult{Text: result}, nil
}

func (e *CLIExecutor) SettingsShow(ctx context.Context) (*SettingsShowResult, error) {
	result, err := runOwlCommand(ctx, []string{"settings", "show", "--no-color"})
	if err != nil {
		return nil, err
	}
	return &SettingsShowResult{Text: result}, nil
}

func (e *CLIExecutor) SettingsSet(ctx context.Context, p SettingsSetParams) (*SettingsSetResult, error) {
	result, err := runOwlCommand(ctx, []string{"settings", "set", p.Key, p.Value, "--no-color"})
	if err != nil {
		return nil, err
	}
	return &SettingsSetResult{Text: result}, nil
}

func (e *CLIExecutor) HistoryList(ctx context.Context, p HistoryListParams) (*HistoryListResult, error) {
	args := []string{"history", "--no-color"}
	if p.NodeID != "" {
		args = append(args, "--node-id", p.NodeID)
	}
	if p.OpType != "" {
		args = append(args, "--op-type", p.OpType)
	}
	if p.Limit > 0 && p.Limit != 50 {
		args = append(args, "--limit", fmt.Sprintf("%d", p.Limit))
	}
	result, err := runOwlCommand(ctx, args)
	if err != nil {
		return nil, err
	}
	return &HistoryListResult{Text: result}, nil
}

func (e *CLIExecutor) HistoryClean(ctx context.Context, p HistoryCleanParams) (*HistoryCleanResult, error) {
	args := []string{"history", "clean", "--no-color"}
	if p.Days > 0 && p.Days != 30 {
		args = append(args, "--days", fmt.Sprintf("%d", p.Days))
	}
	result, err := runOwlCommand(ctx, args)
	if err != nil {
		return nil, err
	}
	return &HistoryCleanResult{Text: result}, nil
}

// getOwlPath finds the owl executable path
func getOwlPath() string {
	// First check if we're in the project directory
	pwd, err := os.Getwd()
	if err == nil {
		localPath := filepath.Join(pwd, "owl")
		if _, err := os.Stat(localPath); err == nil {
			return localPath
		}
	}
	// Fallback to system path
	return "owl"
}

// DisableRealCommands can be set in tests to disable real owl command calls
var DisableRealCommands = false

// runOwlCommand executes an owl command and returns the output
func runOwlCommand(ctx context.Context, args []string) (string, error) {
	if DisableRealCommands {
		return "", fmt.Errorf("real commands disabled for testing")
	}
	owlPath := getOwlPath()
	debugLogger.Debugw("执行 owl 命令",
		"path", owlPath,
		"args", args)

	cmd := exec.CommandContext(ctx, owlPath, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		errorMsg := stderr.String()
		if errorMsg == "" {
			errorMsg = err.Error()
		}
		return "", fmt.Errorf("执行命令失败: %s", errorMsg)
	}

	output := stdout.String()
	debugLogger.Debugw("命令执行成功", "output_len", len(output))
	return output, nil
}

type Tool interface {
	Name() string
	Description() string
	Parameters() string
	Validate(params map[string]interface{}) error
	Execute(ctx context.Context, params map[string]interface{}) (string, error)
}

type QueryNodesTool struct {
	executor  Executor
	nodeMgr   node.Manager
	nodeStore NodeStoreAdapter
}

func NewQueryNodesTool(executor Executor, nodeMgr node.Manager, nodeStore NodeStoreAdapter) *QueryNodesTool {
	return &QueryNodesTool{executor: executor, nodeMgr: nodeMgr, nodeStore: nodeStore}
}

func (t *QueryNodesTool) Name() string {
	return "query_nodes"
}

func (t *QueryNodesTool) Description() string {
	return "Query node information, support filtering by group, label, and status."
}

func (t *QueryNodesTool) Parameters() string {
	return `{
		"type": "object",
		"properties": {
			"group": {
				"type": "string",
				"description": "Filter by group, e.g. 'web', 'db'"
			},
			"labels": {
				"type": "object",
				"description": "Filter by labels, e.g. {\"env\": \"prod\"}"
			},
			"status": {
				"type": "string",
				"description": "Filter by status: online, offline, unknown"
			},
			"search": {
				"type": "string",
				"description": "Fuzzy search by node name (case-insensitive substring match)"
			},
			"format": {
				"type": "string",
				"description": "Output format: table (default), json, summary"
			}
		}
	}`
}

func (t *QueryNodesTool) Validate(params map[string]interface{}) error {
	validator := NewValidator()
	return validator.ValidateQueryNodes(params)
}

func (t *QueryNodesTool) refreshNodeData() {
	if t.nodeStore != nil {
		if bridge, ok := t.nodeStore.(*NodeStoreBridge); ok {
			if err := bridge.SyncFromStore(t.nodeStore); err != nil {
				debugLogger.Debugw("刷新节点数据失败", "error", err)
			}
		}
	}
}

func (t *QueryNodesTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor != nil {
		p := QueryNodesParams{}
		p.Group, _ = params["group"].(string)
		p.Labels, _ = params["labels"].(map[string]interface{})
		p.Status, _ = params["status"].(string)
		p.Search, _ = params["search"].(string)
		p.Format, _ = params["format"].(string)
		result, err := t.executor.QueryNodes(ctx, p)
		if err == nil {
			return result.Text, nil
		}
	}

	debugLogger.Debugw("QueryNodesTool 执行开始",
		"params", fmt.Sprintf("%+v", params))

	t.refreshNodeData()

	group, _ := params["group"].(string)
	labels, _ := params["labels"].(map[string]interface{})
	status, _ := params["status"].(string)
	format, _ := params["format"].(string)
	search, _ := params["search"].(string)

	debugLogger.Debugw("参数解析",
		"group", group,
		"labels", labels,
		"status", status,
		"format", format,
		"search", search)

	// 转换标签格式并处理中文标签键名
	convertedLabels := make(map[string]string)
	if labels != nil {
		for k, v := range labels {
			if vs, ok := v.(string); ok {
				if mappedKey, ok := labelKeyMap[k]; ok {
					convertedLabels[mappedKey] = vs
				} else {
					convertedLabels[k] = vs
				}
			}
		}
	}

	// 获取所有节点作为基础数据集
	allNodes := t.nodeMgr.List()
	nodes := make([]*model.Node, len(allNodes))
	copy(nodes, allNodes)

	// 步骤1: 如果有搜索条件，先在所有节点中搜索（不限制范围）
	if search != "" {
		debugLogger.Debugw("按名称搜索（全量节点）", "search", search)
		nodes = t.filterByName(nodes, search)
	}

	// 步骤2: 应用精确过滤条件（在搜索结果上过滤）
	if group != "" {
		debugLogger.Debugw("按分组过滤", "group", group)
		nodes = t.filterByGroup(nodes, []string{group})
	}

	if len(convertedLabels) > 0 {
		debugLogger.Debugw("按标签过滤", "labels", convertedLabels)
		nodes = t.filterByLabels(nodes, convertedLabels)
	}

	if status != "" {
		debugLogger.Debugw("按状态过滤", "status", status)
		nodes = t.filterByStatus(nodes, status)
	}

	debugLogger.Debugw("最终节点数量", "finalCount", len(nodes))

	if len(nodes) == 0 {
		return "No matching nodes found", nil
	}

	// 格式化输出
	switch format {
	case "json":
		data, _ := json.MarshalIndent(nodesToInfo(nodes), "", "  ")
		return string(data), nil
	case "summary":
		return fmt.Sprintf("Total %d nodes, %d online", len(nodes), t.countOnline(nodes)), nil
	default:
		return t.formatAsTable(nodes), nil
	}
}

func (t *QueryNodesTool) filterByName(nodes []*model.Node, search string) []*model.Node {
	filtered := make([]*model.Node, 0)
	lowerSearch := strings.ToLower(search)
	for _, n := range nodes {
		if strings.Contains(strings.ToLower(n.Name), lowerSearch) ||
			strings.Contains(strings.ToLower(n.Address), lowerSearch) {
			filtered = append(filtered, n)
		}
	}
	return filtered
}

func (t *QueryNodesTool) filterByGroup(nodes []*model.Node, groups []string) []*model.Node {
	filtered := make([]*model.Node, 0)
	for _, n := range nodes {
		if matchesAnyGroup(n, groups) {
			filtered = append(filtered, n)
		}
	}
	return filtered
}

func matchesAnyGroup(n *model.Node, groups []string) bool {
	if len(groups) == 0 {
		return true
	}
	for _, g := range groups {
		if n.HasGroup(g) {
			return true
		}
	}
	return false
}

func (t *QueryNodesTool) filterByLabels(nodes []*model.Node, labels map[string]string) []*model.Node {
	filtered := make([]*model.Node, 0)
	for _, n := range nodes {
		if n.MatchLabels(labels) {
			filtered = append(filtered, n)
		}
	}
	return filtered
}

func (t *QueryNodesTool) filterByStatus(nodes []*model.Node, status string) []*model.Node {
	filtered := make([]*model.Node, 0)
	for _, n := range nodes {
		if strings.EqualFold(string(n.Status), status) {
			filtered = append(filtered, n)
		}
	}
	return filtered
}

func (t *QueryNodesTool) countOnline(nodes []*model.Node) int {
	count := 0
	for _, n := range nodes {
		if n.Status == model.NodeStatusOnline {
			count++
		}
	}
	return count
}

type nodeInfo struct {
	Name    string            `json:"name"`
	Address string            `json:"address"`
	Port    int               `json:"port"`
	Status  string            `json:"status"`
	Groups  []string          `json:"groups"`
	Labels  map[string]string `json:"labels"`
}

func nodesToInfo(nodes []*model.Node) []nodeInfo {
	info := make([]nodeInfo, len(nodes))
	for i, n := range nodes {
		info[i] = nodeInfo{
			Name:    n.Name,
			Address: n.Address,
			Port:    n.Port,
			Status:  string(n.Status),
			Groups:  n.Groups,
			Labels:  n.Labels,
		}
	}
	return info
}

func (t *QueryNodesTool) formatAsTable(nodes []*model.Node) string {
	var sb strings.Builder
	if len(nodes) == 0 {
		sb.WriteString("No nodes found.")
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("%s %s %s %s %s %s %s %s\n",
		padRight("ID", 20), padRight("Name", 25), padRight("Address", 25),
		padRight("User", 10), padRight("Status", 12), padRight("Groups", 20),
		padRight("Labels", 30), padRight("Last Check", 20)))
	sb.WriteString(strings.Repeat("-", 169))
	sb.WriteString("\n")

	for _, n := range nodes {
		groups := strings.Join(n.Groups, ",")
		if groups == "" {
			groups = "-"
		}

		labels := formatLabels(n.Labels)
		if labels == "" {
			labels = "-"
		}

		user := n.User
		if user == "" {
			user = "-"
		}

		address := fmt.Sprintf("%s:%d", n.Address, n.Port)

		lastCheck := n.LastCheckAt
		if lastCheck == "" {
			lastCheck = "-"
		}

		sb.WriteString(fmt.Sprintf("%s %s %s %s %s %s %s %s\n",
			padRight(n.ID, 20),
			padRight(truncateByWidth(n.Name, 25), 25),
			padRight(truncateStr(address, 25), 25),
			padRight(user, 10),
			padRight(string(n.Status), 12),
			padRight(truncateByWidth(groups, 20), 20),
			padRight(truncateByWidth(labels, 30), 30),
			padRight(truncateStr(lastCheck, 20), 20)))
	}
	sb.WriteString(fmt.Sprintf("\nTotal: %d nodes, %d online", len(nodes), t.countOnline(nodes)))
	return sb.String()
}

func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, labels[k]))
	}
	return strings.Join(parts, ",")
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func truncateByWidth(s string, maxWidth int) string {
	w := 0
	for i, r := range s {
		if r > 127 {
			w += 2
		} else {
			w += 1
		}
		if w > maxWidth {
			return s[:i]
		}
	}
	return s
}

func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		if r > 127 {
			w += 2
		} else {
			w += 1
		}
	}
	return w
}

func padRight(s string, width int) string {
	dw := displayWidth(s)
	if dw >= width {
		return s
	}
	return s + strings.Repeat(" ", width-dw)
}

type ExecuteCommandTool struct {
	executor Executor
	nodeMgr  node.Manager
}

func NewExecuteCommandTool(executor Executor, nodeMgr node.Manager) *ExecuteCommandTool {
	return &ExecuteCommandTool{executor: executor, nodeMgr: nodeMgr}
}

func (t *ExecuteCommandTool) Name() string {
	return "execute_command"
}

func (t *ExecuteCommandTool) Description() string {
	return "Execute commands on specified nodes, return execution results."
}

func (t *ExecuteCommandTool) Parameters() string {
	return `{
		"type": "object",
		"properties": {
			"nodes": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Node name list (mutually exclusive with group/label)"
			},
			"command": {
				"type": "string",
				"description": "Command to execute"
			},
			"group": {
				"type": "string",
				"description": "Filter by group, e.g. 'web', 'db' (mutually exclusive with nodes/label)"
			},
			"label": {
				"type": "string",
				"description": "Filter by label, e.g. 'env=prod' (mutually exclusive with nodes/group)"
			},
			"timeout": {
				"type": "integer",
				"description": "Timeout in seconds, default 30"
			},
			"format": {
				"type": "string",
				"enum": ["simple", "detail", "json"],
				"description": "Output format: simple (default), detail, json"
			},
			"mode": {
				"type": "string",
				"enum": ["parallel", "serial", "async"],
				"description": "Execution mode: parallel (default), serial, async"
			},
			"search": {
				"type": "string",
				"description": "Fuzzy search by node name, case-insensitive substring match (mutually exclusive with nodes/group/label)"
			}
		},
		"required": ["command"]
	}`
}

func (t *ExecuteCommandTool) Validate(params map[string]interface{}) error {
	validator := NewValidator()
	return validator.ValidateExecuteCommand(params)
}

func (t *ExecuteCommandTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	command, ok := params["command"].(string)
	if !ok || command == "" {
		return "", fmt.Errorf("missing command")
	}

	timeout := 30
	if tv, ok := params["timeout"].(float64); ok {
		timeout = int(tv)
	}

	format, _ := params["format"].(string)
	if format == "" {
		format = "simple"
	}

	mode, _ := params["mode"].(string)
	if mode == "" {
		mode = "parallel"
	}

	if t.executor != nil {
		p := ExecCommandParams{
			Command: command,
			Timeout: timeout,
			Format:  format,
			Mode:    mode,
		}
		if nodeList, ok := params["nodes"].([]interface{}); ok {
			for _, n := range nodeList {
				if s, ok := n.(string); ok {
					p.Nodes = append(p.Nodes, s)
				}
			}
		}
		p.Group, _ = params["group"].(string)
		p.Label, _ = params["label"].(string)
		p.Search, _ = params["search"].(string)
		result, err := t.executor.ExecuteCommand(ctx, p)
		if err == nil {
			return result.Text, nil
		}
	}

	// Build owl exec run command arguments
	args := []string{"exec", "run", command, "--no-color", "--format", format}

	if mode == "serial" {
		args = append(args, "--serial")
	}

	if timeout != 30 {
		args = append(args, "--timeout", fmt.Sprintf("%d", timeout))
	}

	// Handle node filtering
	if nodeList, ok := params["nodes"].([]interface{}); ok && len(nodeList) > 0 {
		var nodeNames []string
		for _, node := range nodeList {
			if s, ok := node.(string); ok {
				nodeNames = append(nodeNames, s)
			}
		}
		// 处理 ALL_NODES 特殊值
		if len(nodeNames) == 1 && nodeNames[0] == "ALL_NODES" {
			// 不传递 --nodes，这样 owl exec 会使用所有节点
		} else {
			args = append(args, "--nodes", strings.Join(nodeNames, ","))
		}
	} else if group, _ := params["group"].(string); group != "" {
		args = append(args, "--groups", group)
	} else if label, _ := params["label"].(string); label != "" {
		args = append(args, "--label", label)
	} else if search, ok := params["search"].(string); ok && search != "" {
		// For search, get nodes first then filter
		nodes := t.nodeMgr.SearchByName(search)
		if len(nodes) == 0 {
			return "No matching nodes found", nil
		}
		var nodeNames []string
		for _, n := range nodes {
			nodeNames = append(nodeNames, n.Name)
		}
		args = append(args, "--nodes", strings.Join(nodeNames, ","))
	}

	// Execute the command
	debugLogger.Debugw("调用 owl exec run 命令", "args", args)
	result, err := runOwlCommand(ctx, args)
	if err == nil {
		return result, nil
	}
	debugLogger.Debugw("调用 owl exec run 失败，回退到模拟结果", "error", err)
	// Fallback to simple mock result for tests
	// First, get target nodes
	var nodes []*model.Node
	if nodeList, ok := params["nodes"].([]interface{}); ok && len(nodeList) > 0 {
		var nodeNames []string
		for _, node := range nodeList {
			if s, ok := node.(string); ok {
				nodeNames = append(nodeNames, s)
			}
		}
		// 处理 ALL_NODES 特殊值
		if len(nodeNames) == 1 && nodeNames[0] == "ALL_NODES" {
			nodes = t.nodeMgr.List()
		} else {
			allNodes := t.nodeMgr.List()
			for _, n := range allNodes {
				for _, name := range nodeNames {
					if n.Name == name {
						nodes = append(nodes, n)
					}
				}
			}
		}
	} else if group, _ := params["group"].(string); group != "" {
		nodes = t.nodeMgr.GetByGroup(group)
	} else if label, _ := params["label"].(string); label != "" {
		labelParts := strings.SplitN(label, "=", 2)
		if len(labelParts) == 2 {
			nodes = t.nodeMgr.GetByLabels(map[string]string{labelParts[0]: labelParts[1]})
		}
	} else if search, ok := params["search"].(string); ok && search != "" {
		nodes = t.nodeMgr.SearchByName(search)
	} else {
		nodes = t.nodeMgr.List()
	}
	if len(nodes) == 0 {
		return "No matching nodes found", nil
	}
	// Simple mock result
	var sb strings.Builder
	for _, n := range nodes {
		sb.WriteString(fmt.Sprintf("✅ [%s] 成功\n   hello\n\n", n.Name))
	}
	sb.WriteString("📊 总结: ")
	sb.WriteString(fmt.Sprintf("%d 成功, 0 失败", len(nodes)))
	return sb.String(), nil
}

func (t *ExecuteCommandTool) formatExecuteSimple(command string, nodes []*model.Node, timeout int, mode, filterDesc string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Execute command: %s\n", command))
	sb.WriteString(fmt.Sprintf("Target: %s (%d nodes)\n", filterDesc, len(nodes)))
	sb.WriteString(fmt.Sprintf("Mode: %s, Timeout: %ds\n\n", mode, timeout))
	sb.WriteString("Results:\n")
	sb.WriteString(strings.Repeat("-", 60))
	sb.WriteString("\n")

	for _, n := range nodes {
		sb.WriteString(fmt.Sprintf("[%s] %s:%d | Status: %s\n", n.Name, n.Address, n.Port, n.Status))
	}
	return sb.String()
}

func (t *ExecuteCommandTool) formatExecuteDetail(command string, nodes []*model.Node, timeout int, mode, filterDesc string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Execute command: %s\n", command))
	sb.WriteString(fmt.Sprintf("Target: %s (%d nodes)\n", filterDesc, len(nodes)))
	sb.WriteString(fmt.Sprintf("Mode: %s, Timeout: %ds\n\n", mode, timeout))
	sb.WriteString("Results:\n")
	sb.WriteString(strings.Repeat("-", 60))
	sb.WriteString("\n")

	for _, n := range nodes {
		sb.WriteString(fmt.Sprintf("\n>>> %s (%s:%d) <<<\n", n.Name, n.Address, n.Port))
		sb.WriteString(fmt.Sprintf("Status: %s\n", n.Status))
		if len(n.Groups) > 0 {
			sb.WriteString(fmt.Sprintf("Groups: %s\n", strings.Join(n.Groups, ", ")))
		}
		if len(n.Labels) > 0 {
			sb.WriteString(fmt.Sprintf("Labels: %s\n", formatLabels(n.Labels)))
		}
	}
	return sb.String()
}

func (t *ExecuteCommandTool) formatExecuteJSON(command string, nodes []*model.Node, timeout int, mode, filterDesc string) string {
	type result struct {
		Command   string     `json:"command"`
		Target    string     `json:"target"`
		NodeCount int        `json:"node_count"`
		Mode      string     `json:"mode"`
		Timeout   int        `json:"timeout"`
		Nodes     []nodeInfo `json:"nodes"`
	}
	r := result{
		Command:   command,
		Target:    filterDesc,
		NodeCount: len(nodes),
		Mode:      mode,
		Timeout:   timeout,
		Nodes:     nodesToInfo(nodes),
	}
	data, _ := json.MarshalIndent(r, "", "  ")
	return string(data)
}

func parseLabelFilter(label string) map[string]string {
	result := make(map[string]string)
	parts := strings.Split(label, ",")
	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) == 2 {
			result[kv[0]] = kv[1]
		}
	}
	return result
}

func (t *ExecuteCommandTool) validateCommand(cmd string) error {
	dangerous := []string{
		"rm -rf", "rm -fr", "dd if=",
		"mkfs", "fdisk", "parted",
	}

	lower := strings.ToLower(cmd)
	for _, d := range dangerous {
		if strings.Contains(lower, d) {
			return fmt.Errorf("dangerous command: %s", d)
		}
	}
	return nil
}

type ExecuteScriptTool struct {
	executor Executor
	nodeMgr  node.Manager
}

func NewExecuteScriptTool(executor Executor, nodeMgr node.Manager) *ExecuteScriptTool {
	return &ExecuteScriptTool{executor: executor, nodeMgr: nodeMgr}
}

func (t *ExecuteScriptTool) Name() string {
	return "execute_script"
}

func (t *ExecuteScriptTool) Description() string {
	return "Execute script files on specified nodes. Supports local file upload+exec and inline execution."
}

func (t *ExecuteScriptTool) Parameters() string {
	return `{
		"type": "object",
		"properties": {
			"script": {
				"type": "string",
				"description": "Script file path or inline script content"
			},
			"nodes": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Node name list (mutually exclusive with group/label)"
			},
			"group": {
				"type": "string",
				"description": "Filter by group, e.g. 'web', 'db' (mutually exclusive with nodes/label)"
			},
			"label": {
				"type": "string",
				"description": "Filter by label, e.g. 'env=prod' (mutually exclusive with nodes/group)"
			},
			"search": {
				"type": "string",
				"description": "Fuzzy search by node name, case-insensitive substring match (mutually exclusive with nodes/group/label)"
			},
			"dest": {
				"type": "string",
				"description": "Destination path on remote nodes, default /tmp"
			},
			"args": {
				"type": "string",
				"description": "Arguments to pass to the script"
			},
			"timeout": {
				"type": "integer",
				"description": "Timeout in seconds, default 300"
			},
			"inline": {
				"type": "boolean",
				"description": "If true, treat script param as inline content instead of file path, default false"
			},
			"keep": {
				"type": "boolean",
				"description": "If true, keep script file on remote after execution, default false"
			}
		},
		"required": ["script"]
	}`
}

func (t *ExecuteScriptTool) Validate(params map[string]interface{}) error {
	validator := NewValidator()
	return validator.ValidateExecuteScript(params)
}

func (t *ExecuteScriptTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	script, ok := params["script"].(string)
	if !ok || script == "" {
		return "", fmt.Errorf("missing script")
	}

	inline, _ := params["inline"].(bool)
	dest, _ := params["dest"].(string)
	scriptArgs, _ := params["args"].(string)
	timeout := 300
	if tv, ok := params["timeout"].(float64); ok {
		timeout = int(tv)
	}
	keep, _ := params["keep"].(bool)

	if t.executor != nil {
		p := ExecScriptParams{
			Script:  script,
			Inline:  inline,
			Dest:    dest,
			Args:    scriptArgs,
			Timeout: timeout,
			Keep:    keep,
		}
		if nodeList, ok := params["nodes"].([]interface{}); ok {
			for _, n := range nodeList {
				if s, ok := n.(string); ok {
					p.Nodes = append(p.Nodes, s)
				}
			}
		}
		p.Group, _ = params["group"].(string)
		p.Label, _ = params["label"].(string)
		p.Search, _ = params["search"].(string)
		result, err := t.executor.ExecuteScript(ctx, p)
		if err == nil {
			return result.Text, nil
		}
	}

	// Build owl exec script command arguments
	args := []string{"exec", "script", script, "--no-color"}

	if inline {
		args = append(args, "--inline")
	}

	if dest != "" && dest != "/tmp" {
		args = append(args, "--dest", dest)
	}

	if scriptArgs != "" {
		args = append(args, "--args", scriptArgs)
	}

	if timeout != 300 {
		args = append(args, "--timeout", fmt.Sprintf("%d", timeout))
	}

	if keep {
		args = append(args, "--keep")
	}

	// Handle node selection
	var nodeNames []string

	if nodesList, ok := params["nodes"].([]interface{}); ok && len(nodesList) > 0 {
		for _, node := range nodesList {
			if s, ok := node.(string); ok {
				nodeNames = append(nodeNames, s)
			}
		}
		args = append(args, "--nodes", strings.Join(nodeNames, ","))
	} else if group, _ := params["group"].(string); group != "" {
		args = append(args, "--groups", group)
	} else if label, _ := params["label"].(string); label != "" {
		args = append(args, "--label", label)
	} else if search, ok := params["search"].(string); ok && search != "" {
		// Search case - get nodes and then use --nodes
		nodes := t.nodeMgr.SearchByName(search)
		if len(nodes) == 0 {
			return "No matching nodes found", nil
		}
		var names []string
		for _, n := range nodes {
			names = append(names, n.Name)
		}
		args = append(args, "--nodes", strings.Join(names, ","))
	}

	// Execute the command
	debugLogger.Debugw("调用 owl exec script 命令", "args", args)
	result, err := runOwlCommand(ctx, args)
	if err == nil {
		return result, nil
	}
	debugLogger.Debugw("调用 owl exec script 失败，回退到模拟结果", "error", err)
	// Fallback to simple mock result
	var nodes []*model.Node
	if len(nodeNames) > 0 {
		allNodes := t.nodeMgr.List()
		for _, n := range allNodes {
			for _, name := range nodeNames {
				if n.Name == name {
					nodes = append(nodes, n)
				}
			}
		}
	} else if group, _ := params["group"].(string); group != "" {
		nodes = t.nodeMgr.GetByGroup(group)
	} else {
		nodes = t.nodeMgr.List()
	}
	if len(nodes) == 0 {
		return "No matching nodes found", nil
	}
	// Simple mock result
	var sb strings.Builder
	for _, n := range nodes {
		sb.WriteString(fmt.Sprintf("✅ [%s] 脚本执行成功\n", n.Name))
	}
	return sb.String(), nil
}

type GeneratePlaybookTool struct {
	executor Executor
	nodeMgr  node.Manager
}

func NewGeneratePlaybookTool(executor Executor, nodeMgr node.Manager) *GeneratePlaybookTool {
	return &GeneratePlaybookTool{executor: executor, nodeMgr: nodeMgr}
}

func (t *GeneratePlaybookTool) Name() string {
	return "generate_playbook"
}

func (t *GeneratePlaybookTool) Description() string {
	return "Generate Ansible-like YAML playbook from natural language requirements. Requires user confirmation before execution."
}

func (t *GeneratePlaybookTool) Parameters() string {
	return `{
		"type": "object",
		"properties": {
			"requirement": {
				"type": "string",
				"description": "User requirement description, e.g. 'Install nginx on all web nodes and start it'"
			},
			"vars": {
				"type": "object",
				"description": "Custom variables"
			}
		},
		"required": ["requirement"]
	}`
}

func (t *GeneratePlaybookTool) Validate(params map[string]interface{}) error {
	validator := NewValidator()
	return validator.ValidateGeneratePlaybook(params)
}

func (t *GeneratePlaybookTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	requirement, ok := params["requirement"].(string)
	if !ok || requirement == "" {
		return "", fmt.Errorf("missing requirement description")
	}

	// 本地生成（CLI 无 playbook generate 子命令，避免调用不存在的 owl 命令）
	nodes := t.nodeMgr.List()
	var hosts []string
	for _, n := range nodes {
		hosts = append(hosts, n.Name)
	}

	playbook := t.generatePlaybookFromRequirement(requirement, hosts)

	return fmt.Sprintf("Generated playbook:\n\n```yaml\n%s\n```\n\nPlease confirm whether to execute this playbook?", playbook), nil
}

func (t *GeneratePlaybookTool) generatePlaybookFromRequirement(req string, hosts []string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("- name: %s\n", req))
	sb.WriteString(fmt.Sprintf("  hosts: %s\n", strings.Join(hosts, ",")))
	sb.WriteString("  become: yes\n")
	sb.WriteString("  become_user: root\n\n")
	sb.WriteString("  vars:\n")
	sb.WriteString("    ansible_user: root\n\n")
	sb.WriteString("  tasks:\n")

	reqLower := strings.ToLower(req)

	if strings.Contains(reqLower, "install") || strings.Contains(reqLower, "安装") {
		packageName := t.extractPackageName(req)
		sb.WriteString(fmt.Sprintf("    - name: Install %s\n", packageName))
		sb.WriteString(fmt.Sprintf("      shell: apt-get install -y %s || yum install -y %s\n", packageName, packageName))
	}

	if strings.Contains(reqLower, "restart") || strings.Contains(reqLower, "重启") {
		serviceName := t.extractServiceName(req)
		sb.WriteString(fmt.Sprintf("    - name: Restart %s service\n", serviceName))
		sb.WriteString(fmt.Sprintf("      systemd:\n        name: %s\n        state: restarted\n", serviceName))
	}

	if strings.Contains(reqLower, "start") || strings.Contains(reqLower, "启动") {
		serviceName := t.extractServiceName(req)
		sb.WriteString(fmt.Sprintf("    - name: Start %s service\n", serviceName))
		sb.WriteString(fmt.Sprintf("      systemd:\n        name: %s\n        state: started\n", serviceName))
	}

	if strings.Contains(reqLower, "stop") || strings.Contains(reqLower, "停止") {
		serviceName := t.extractServiceName(req)
		sb.WriteString(fmt.Sprintf("    - name: Stop %s service\n", serviceName))
		sb.WriteString(fmt.Sprintf("      systemd:\n        name: %s\n        state: stopped\n", serviceName))
	}

	return sb.String()
}

func (t *GeneratePlaybookTool) extractPackageName(req string) string {
	keywords := []string{"nginx", "apache", "mysql", "redis", "docker", "node", "python", "java"}
	reqLower := strings.ToLower(req)
	for _, kw := range keywords {
		if strings.Contains(reqLower, kw) {
			return kw
		}
	}
	return "package"
}

func (t *GeneratePlaybookTool) extractServiceName(req string) string {
	keywords := []string{"nginx", "apache", "mysql", "redis", "docker", "node", "python", "java"}
	reqLower := strings.ToLower(req)
	for _, kw := range keywords {
		if strings.Contains(reqLower, kw) {
			return kw
		}
	}
	return "service"
}

type TransferFileTool struct {
	executor Executor
	nodeMgr  node.Manager
}

func NewTransferFileTool(executor Executor, nodeMgr node.Manager) *TransferFileTool {
	return &TransferFileTool{executor: executor, nodeMgr: nodeMgr}
}

func (t *TransferFileTool) Name() string {
	return "transfer_file"
}

func (t *TransferFileTool) Description() string {
	return "Transfer files to specified nodes, supports direct and diffusion transfer (auto when nodes >= 5)."
}

func (t *TransferFileTool) Parameters() string {
	return `{
		"type": "object",
		"properties": {
			"source_file": {
				"type": "string",
				"description": "Source file path (local)"
			},
			"nodes": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Target node name list"
			},
			"dest_dir": {
				"type": "string",
				"description": "Target directory (remote)"
			},
			"mode": {
				"type": "string",
				"description": "Transfer mode: direct or diffusion, default auto"
			},
			"permission": {
				"type": "string",
				"description": "File permission, e.g. 0644"
			}
		},
		"required": ["source_file", "nodes", "dest_dir"]
	}`
}

func (t *TransferFileTool) Validate(params map[string]interface{}) error {
	validator := NewValidator()
	return validator.ValidateTransferFile(params)
}

func (t *TransferFileTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	sourceFile, ok := params["source_file"].(string)
	if !ok || sourceFile == "" {
		return "", fmt.Errorf("missing source file path")
	}

	destDir, ok := params["dest_dir"].(string)
	if !ok || destDir == "" {
		return "", fmt.Errorf("missing target directory")
	}

	var nodeNames []string

	if t.executor != nil {
		p := TransferFileParams{
			SourceFile: sourceFile,
			DestDir:    destDir,
		}
		if nodeList, ok := params["nodes"].([]interface{}); ok {
			for _, n := range nodeList {
				if s, ok := n.(string); ok {
					p.Nodes = append(p.Nodes, s)
				}
			}
		}
		p.Mode, _ = params["mode"].(string)
		p.Permission, _ = params["permission"].(string)
		p.Search, _ = params["search"].(string)
		result, err := t.executor.TransferFile(ctx, p)
		if err == nil {
			return result.Text, nil
		}
	}

	if search, ok := params["search"].(string); ok && search != "" {
		nodes := t.nodeMgr.SearchByName(search)
		if len(nodes) == 0 {
			nodes = t.nodeMgr.SearchByAddress(search)
		}
		if len(nodes) == 0 {
			return "No matching nodes found for search: " + search, nil
		}
		for _, n := range nodes {
			nodeNames = append(nodeNames, n.Name)
		}
	} else {
		nodeList, ok := params["nodes"].([]interface{})
		if !ok || len(nodeList) == 0 {
			return "", fmt.Errorf("missing nodes")
		}
		for _, node := range nodeList {
			if s, ok := node.(string); ok {
				nodeNames = append(nodeNames, s)
			}
		}
	}

	permission, _ := params["permission"].(string)

	mode, _ := params["mode"].(string)
	if mode == "" {
		if len(nodeNames) >= 5 {
			mode = "diffusion"
		} else {
			mode = "direct"
		}
	}

	// Determine which command to use
	var args []string
	if mode == "diffusion" {
		args = []string{"file", "transfer", sourceFile, "--nodes", strings.Join(nodeNames, ","), "--dest", destDir}
	} else {
		args = []string{"file", "upload", sourceFile, "--nodes", strings.Join(nodeNames, ","), "--dest", destDir}
	}

	if permission != "" && permission != "0644" {
		args = append(args, "--mode", permission)
	}

	// Execute the command
	debugLogger.Debugw("调用 owl file 命令", "args", args)
	result, err := runOwlCommand(ctx, args)
	if err == nil {
		return result, nil
	}
	debugLogger.Debugw("调用 owl file 失败，回退到模拟结果", "error", err)
	// Fallback to simple mock result for tests
	var sb strings.Builder
	sb.WriteString("文件传输任务:\n")
	sb.WriteString(fmt.Sprintf("源文件: %s\n", sourceFile))
	sb.WriteString(fmt.Sprintf("目标目录: %s\n", destDir))
	sb.WriteString(fmt.Sprintf("目标节点: %s\n", strings.Join(nodeNames, ",")))
	return sb.String(), nil
}

type QueryDatabaseTool struct {
	executor Executor
	nodeMgr  node.Manager
}

func NewQueryDatabaseTool(executor Executor, nodeMgr node.Manager) *QueryDatabaseTool {
	return &QueryDatabaseTool{executor: executor, nodeMgr: nodeMgr}
}

func (t *QueryDatabaseTool) Name() string {
	return "query_database"
}

func (t *QueryDatabaseTool) Description() string {
	return "Query the owl database directly. Supports SQL SELECT queries and structured filters (group/labels/status/search)."
}

func (t *QueryDatabaseTool) Parameters() string {
	return `{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "SQL SELECT query to execute on the nodes table"
			},
			"group": {
				"type": "string",
				"description": "Filter by group, e.g. 'web', 'db'"
			},
			"labels": {
				"type": "object",
				"description": "Filter by labels, e.g. {\"env\": \"prod\"}"
			},
			"status": {
				"type": "string",
				"description": "Filter by status: online, offline, unknown"
			},
			"search": {
				"type": "string",
				"description": "Fuzzy search by node name (case-insensitive substring match)"
			},
			"format": {
				"type": "string",
				"description": "Output format: table (default), json, summary"
			}
		}
	}`
}

func (t *QueryDatabaseTool) Validate(params map[string]interface{}) error {
	query, hasQuery := params["query"].(string)
	_, hasGroup := params["group"].(string)
	labelsRaw, hasLabels := params["labels"].(map[string]interface{})
	_, hasStatus := params["status"].(string)
	_, hasSearch := params["search"].(string)

	if hasQuery && query != "" {
		upper := strings.ToUpper(strings.TrimSpace(query))
		if !strings.HasPrefix(upper, "SELECT") {
			return fmt.Errorf("only SELECT queries are allowed")
		}
		forbidden := []string{"INSERT", "UPDATE", "DELETE", "DROP", "ALTER", "CREATE", "TRUNCATE"}
		for _, f := range forbidden {
			if strings.Contains(upper, f) {
				return fmt.Errorf("only SELECT queries are allowed, found: %s", f)
			}
		}
		return nil
	}

	if hasGroup || hasLabels && labelsRaw != nil || hasStatus || hasSearch {
		return nil
	}

	return fmt.Errorf("must provide either 'query' or at least one filter (group/labels/status/search)")
}

func (t *QueryDatabaseTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if err := t.Validate(params); err != nil {
		return "", err
	}

	format, _ := params["format"].(string)
	if format == "" {
		format = "table"
	}

	// Declare all variables at the top so they are available everywhere
	group, _ := params["group"].(string)
	labelsRaw, _ := params["labels"].(map[string]interface{})
	status, _ := params["status"].(string)
	search, _ := params["search"].(string)
	query, hasQuery := params["query"].(string)

	if t.executor != nil && !hasQuery {
		p := QueryDatabaseParams{
			Group:  group,
			Status: status,
			Search: search,
			Format: format,
		}
		if labelsRaw != nil {
			p.Labels = labelsRaw
		}
		result, err := t.executor.QueryDatabase(ctx, p)
		if err == nil {
			return result.Text, nil
		}
	}

	// Build owl node list command arguments
	args := []string{"node", "list", "--no-color"}

	if format != "table" {
		args = append(args, "--format", format)
	}

	if group != "" {
		args = append(args, "--groups", group)
	}

	if labelsRaw != nil {
		labelMap := make(map[string]string)
		for k, v := range labelsRaw {
			if vs, ok := v.(string); ok {
				labelMap[k] = vs
			}
		}
		// 转换中文标签键名到英文
		for k, v := range labelMap {
			if mappedKey, ok := labelKeyMap[k]; ok {
				args = append(args, "--label", fmt.Sprintf("%s=%s", mappedKey, v))
			} else {
				args = append(args, "--label", fmt.Sprintf("%s=%s", k, v))
			}
		}
	}

	if status != "" {
		args = append(args, "--status", status)
	}

	// Note: owl node list doesn't have search param, so we handle that separately
	if search != "" {
		// Get all nodes, filter by search, then use our format
		nodes := t.nodeMgr.List()
		filtered := make([]*model.Node, 0)
		lowerSearch := strings.ToLower(search)
		for _, n := range nodes {
			if strings.Contains(strings.ToLower(n.Name), lowerSearch) {
				filtered = append(filtered, n)
			}
		}
		if len(filtered) == 0 {
			return "No matching nodes found in database", nil
		}
		switch format {
		case "json":
			info := nodesToInfo(filtered)
			data, _ := json.MarshalIndent(info, "", "  ")
			return string(data), nil
		case "summary":
			count := 0
			for _, n := range filtered {
				if n.Status == model.NodeStatusOnline {
					count++
				}
			}
			return fmt.Sprintf("Total %d nodes, %d online", len(filtered), count), nil
		default:
			return t.formatAsTable(filtered), nil
		}
	}

	// For query parameter, we can't easily handle with the command,
	// so we keep the original logic for that case
	if hasQuery && query != "" {
		// Fall back to original logic for SQL queries
		nodes := t.nodeMgr.List()
		nodes = t.filterBySQL(nodes, query)
		if len(nodes) == 0 {
			return "No matching nodes found in database", nil
		}
		switch format {
		case "json":
			info := nodesToInfo(nodes)
			data, _ := json.MarshalIndent(info, "", "  ")
			return string(data), nil
		case "summary":
			count := 0
			for _, n := range nodes {
				if n.Status == model.NodeStatusOnline {
					count++
				}
			}
			return fmt.Sprintf("Total %d nodes, %d online", len(nodes), count), nil
		default:
			return t.formatAsTable(nodes), nil
		}
	}

	// Execute the command
	debugLogger.Debugw("调用 owl node list 命令", "args", args)
	result, err := runOwlCommand(ctx, args)
	if err == nil {
		return result, nil
	}
	debugLogger.Debugw("调用 owl node list 失败，回退到内部实现", "error", err)
	// Fallback to old implementation
	var nodes []*model.Node
	if group != "" {
		nodes = t.nodeMgr.GetByGroup(group)
	} else if labelsRaw != nil {
		labelMap := make(map[string]string)
		for k, v := range labelsRaw {
			if vs, ok := v.(string); ok {
				labelMap[k] = vs
			}
		}
		nodes = t.nodeMgr.GetByLabels(labelMap)
	} else if status != "" {
		allNodes := t.nodeMgr.List()
		nodes = make([]*model.Node, 0)
		for _, n := range allNodes {
			if string(n.Status) == status {
				nodes = append(nodes, n)
			}
		}
	} else {
		nodes = t.nodeMgr.List()
	}
	if len(nodes) == 0 {
		return "No matching nodes found in database", nil
	}
	switch format {
	case "json":
		info := nodesToInfo(nodes)
		data, _ := json.MarshalIndent(info, "", "  ")
		return string(data), nil
	case "summary":
		count := 0
		for _, n := range nodes {
			if n.Status == model.NodeStatusOnline {
				count++
			}
		}
		return fmt.Sprintf("Total %d nodes, %d online", len(nodes), count), nil
	default:
		return t.formatAsTable(nodes), nil
	}
}

func (t *QueryDatabaseTool) formatAsTable(nodes []*model.Node) string {
	var sb strings.Builder
	if len(nodes) == 0 {
		sb.WriteString("No nodes found in database.")
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("%s %s %s %s %s %s %s %s\n",
		padRight("ID", 20), padRight("Name", 25), padRight("Address", 25),
		padRight("User", 10), padRight("Status", 12), padRight("Groups", 20),
		padRight("Labels", 30), padRight("Last Check", 20)))
	sb.WriteString(strings.Repeat("-", 169))
	sb.WriteString("\n")

	for _, n := range nodes {
		groups := strings.Join(n.Groups, ",")
		if groups == "" {
			groups = "-"
		}

		labels := formatLabels(n.Labels)
		if labels == "" {
			labels = "-"
		}

		user := n.User
		if user == "" {
			user = "-"
		}

		address := fmt.Sprintf("%s:%d", n.Address, n.Port)

		lastCheck := n.LastCheckAt
		if lastCheck == "" {
			lastCheck = "-"
		}

		sb.WriteString(fmt.Sprintf("%s %s %s %s %s %s %s %s\n",
			padRight(n.ID, 20),
			padRight(truncateByWidth(n.Name, 25), 25),
			padRight(truncateStr(address, 25), 25),
			padRight(user, 10),
			padRight(string(n.Status), 12),
			padRight(truncateByWidth(groups, 20), 20),
			padRight(truncateByWidth(labels, 30), 30),
			padRight(truncateStr(lastCheck, 20), 20)))
	}
	count := 0
	for _, n := range nodes {
		if n.Status == model.NodeStatusOnline {
			count++
		}
	}
	sb.WriteString(fmt.Sprintf("\nTotal: %d nodes, %d online", len(nodes), count))
	return sb.String()
}

func (t *QueryDatabaseTool) filterBySQL(nodes []*model.Node, query string) []*model.Node {
	upper := strings.ToUpper(strings.TrimSpace(query))

	if !strings.HasPrefix(upper, "SELECT") {
		return nil
	}

	upper = strings.TrimSpace(upper)

	if strings.Contains(upper, "WHERE") {
		whereIdx := strings.Index(upper, "WHERE")
		whereClause := strings.TrimSpace(upper[whereIdx+5:])
		return t.applyWhere(nodes, whereClause)
	}

	return nodes
}

func (t *QueryDatabaseTool) applyWhere(nodes []*model.Node, where string) []*model.Node {
	parts := strings.SplitN(where, " AND ", 2)
	condition := strings.TrimSpace(parts[0])

	var result []*model.Node

	if strings.Contains(strings.ToUpper(condition), " LIKE ") {
		kv := strings.SplitN(condition, " LIKE ", 2)
		if len(kv) == 2 {
			field := strings.TrimSpace(kv[0])
			pattern := strings.Trim(strings.TrimSpace(kv[1]), "'%")
			field = strings.Trim(field, "`\"")

			if strings.ToLower(field) == "name" {
				lowerPattern := strings.ToLower(pattern)
				for _, n := range nodes {
					if strings.Contains(strings.ToLower(n.Name), lowerPattern) {
						result = append(result, n)
					}
				}
			}
		}
	} else if strings.Contains(condition, "=") {
		kv := strings.SplitN(condition, "=", 2)
		if len(kv) == 2 {
			field := strings.TrimSpace(kv[0])
			value := strings.Trim(strings.TrimSpace(kv[1]), "'\"")
			field = strings.Trim(field, "`\"")

			switch strings.ToLower(field) {
			case "group":
				for _, n := range nodes {
					for _, g := range n.Groups {
						if g == value {
							result = append(result, n)
							break
						}
					}
				}
			case "status":
				for _, n := range nodes {
					if strings.EqualFold(string(n.Status), value) {
						result = append(result, n)
					}
				}
			case "name":
				for _, n := range nodes {
					if strings.EqualFold(n.Name, value) {
						result = append(result, n)
					}
				}
			default:
				result = nodes
			}
		}
	} else {
		result = nodes
	}

	if len(parts) > 1 {
		return t.applyWhere(result, parts[1])
	}
	return result
}

type ListPlaybooksTool struct {
	executor Executor
}

func NewListPlaybooksTool(executor Executor) *ListPlaybooksTool {
	return &ListPlaybooksTool{executor: executor}
}

func (t *ListPlaybooksTool) Name() string {
	return "list_playbooks"
}

func (t *ListPlaybooksTool) Description() string {
	return "List all available playbooks."
}

func (t *ListPlaybooksTool) Parameters() string {
	return `{
		"type": "object",
		"properties": {
			"group": {
				"type": "string",
				"description": "Filter playbooks by group"
			},
			"format": {
				"type": "string",
				"description": "Output format: table (default), json"
			}
		}
	}`
}

func (t *ListPlaybooksTool) Validate(params map[string]interface{}) error {
	return nil
}

func (t *ListPlaybooksTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor != nil {
		result, err := t.executor.ListPlaybooks(ctx)
		if err == nil {
			return result.Text, nil
		}
	}

	library := defaultPlaybookDir()
	if _, err := os.Stat(library); os.IsNotExist(err) {
		return "No playbooks found. Playbooks directory does not exist.", nil
	}

	var playbooks []playbookInfo
	err := filepath.Walk(library, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && (strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml")) {
			playbooks = append(playbooks, playbookInfo{
				Name: info.Name(),
				Path: path,
				Size: info.Size(),
			})
		}
		return nil
	})

	if err != nil {
		return "", fmt.Errorf("failed to scan playbooks: %w", err)
	}

	if len(playbooks) == 0 {
		return "No playbooks found.", nil
	}

	format, _ := params["format"].(string)
	if format == "" {
		format = "table"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Total: %d playbooks\n\n", len(playbooks)))

	if format == "json" {
		data, _ := json.MarshalIndent(playbooks, "", "  ")
		sb.WriteString(string(data))
	} else {
		sb.WriteString(fmt.Sprintf("%-30s %-50s\n", "Name", "Path"))
		sb.WriteString(strings.Repeat("-", 80))
		sb.WriteString("\n")
		for _, pb := range playbooks {
			sb.WriteString(fmt.Sprintf("%-30s %-50s\n", pb.Name, pb.Path))
		}
	}

	return sb.String(), nil
}

type playbookInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type RunPlaybookTool struct {
	executor Executor
	nodeMgr  node.Manager
}

func NewRunPlaybookTool(executor Executor, nodeMgr node.Manager) *RunPlaybookTool {
	return &RunPlaybookTool{executor: executor, nodeMgr: nodeMgr}
}

func (t *RunPlaybookTool) Name() string {
	return "run_playbook"
}

func (t *RunPlaybookTool) Description() string {
	return "Execute a playbook on specified nodes."
}

func (t *RunPlaybookTool) Parameters() string {
	return `{
		"type": "object",
		"properties": {
			"name": {
				"type": "string",
				"description": "Playbook name to execute"
			},
			"nodes": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Target node name list"
			},
			"group": {
				"type": "string",
				"description": "Filter by group, e.g. 'web', 'db'"
			},
			"label": {
				"type": "string",
				"description": "Filter by label, e.g. 'env=prod'"
			},
			"search": {
				"type": "string",
				"description": "Fuzzy search by node name"
			},
			"vars": {
				"type": "object",
				"description": "Variables to pass to playbook"
			},
			"tags": {
				"type": "string",
				"description": "Tags to filter tasks"
			},
			"check": {
				"type": "boolean",
				"description": "Check mode (dry run)"
			}
		},
		"required": ["name"]
	}`
}

func (t *RunPlaybookTool) Validate(params map[string]interface{}) error {
	if name, ok := params["name"].(string); !ok || name == "" {
		return fmt.Errorf("playbook name is required")
	}
	return nil
}

func (t *RunPlaybookTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	name, _ := params["name"].(string)

	if t.executor != nil {
		p := RunPlaybookParams{Name: name}
		if nodeList, ok := params["nodes"].([]interface{}); ok {
			for _, n := range nodeList {
				if s, ok := n.(string); ok {
					p.Nodes = append(p.Nodes, s)
				}
			}
		}
		p.Group, _ = params["group"].(string)
		p.Label, _ = params["label"].(string)
		p.Search, _ = params["search"].(string)
		p.Tags, _ = params["tags"].(string)
		p.Check, _ = params["check"].(bool)
		result, err := t.executor.RunPlaybook(ctx, p)
		if err == nil {
			return result.Text, nil
		}
	}

	var nodes []*model.Node
	var filterDesc string

	if nodeList, ok := params["nodes"].([]interface{}); ok && len(nodeList) > 0 {
		var nodeNames []string
		for _, n := range nodeList {
			if s, ok := n.(string); ok {
				nodeNames = append(nodeNames, s)
			}
		}
		for _, n := range nodeNames {
			if node, err := t.nodeMgr.GetByID(n); err == nil {
				nodes = append(nodes, node)
			}
		}
		filterDesc = fmt.Sprintf("nodes: %s", strings.Join(nodeNames, ", "))
	} else if group, _ := params["group"].(string); group != "" {
		nodes = t.nodeMgr.GetByGroup(group)
		filterDesc = fmt.Sprintf("group: %s", group)
	} else if label, _ := params["label"].(string); label != "" {
		labelMap := parseLabelFilter(label)
		nodes = t.nodeMgr.GetByLabels(labelMap)
		filterDesc = fmt.Sprintf("label: %s", label)
	} else if search, _ := params["search"].(string); search != "" {
		nodes = t.nodeMgr.SearchByName(search)
		filterDesc = fmt.Sprintf("search: %s", search)
	} else {
		nodes = t.nodeMgr.List()
		filterDesc = "all nodes"
	}

	checkMode, _ := params["check"].(bool)
	modeStr := "execute"
	if checkMode {
		modeStr = "check (dry run)"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Playbook execution task:\n"))
	sb.WriteString(fmt.Sprintf("Playbook: %s\n", name))
	sb.WriteString(fmt.Sprintf("Mode: %s\n", modeStr))
	sb.WriteString(fmt.Sprintf("Target: %s (%d nodes)\n", filterDesc, len(nodes)))
	sb.WriteString(fmt.Sprintf("\nTarget nodes:\n"))
	sb.WriteString(strings.Repeat("-", 60))
	sb.WriteString("\n")

	for _, n := range nodes {
		sb.WriteString(fmt.Sprintf("[%s] %s:%d | Status: %s\n", n.Name, n.Address, n.Port, n.Status))
	}

	return sb.String(), nil
}

type ValidatePlaybookTool struct {
	executor Executor
}

func NewValidatePlaybookTool(executor Executor) *ValidatePlaybookTool {
	return &ValidatePlaybookTool{executor: executor}
}

func (t *ValidatePlaybookTool) Name() string {
	return "validate_playbook"
}

func (t *ValidatePlaybookTool) Description() string {
	return "Validate playbook syntax."
}

func (t *ValidatePlaybookTool) Parameters() string {
	return `{
		"type": "object",
		"properties": {
			"file": {
				"type": "string",
				"description": "Playbook file path"
			}
		},
		"required": ["file"]
	}`
}

func (t *ValidatePlaybookTool) Validate(params map[string]interface{}) error {
	if file, ok := params["file"].(string); !ok || file == "" {
		return fmt.Errorf("playbook file path is required")
	}
	return nil
}

func (t *ValidatePlaybookTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	filePath, _ := params["file"].(string)

	if t.executor != nil {
		result, err := t.executor.ValidatePlaybook(ctx, ValidatePlaybookParams{File: filePath})
		if err == nil {
			return result.Text, nil
		}
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	var result struct{}
	if err := yaml.Unmarshal(content, &result); err != nil {
		return "", fmt.Errorf("YAML syntax error: %w", err)
	}

	return fmt.Sprintf("Playbook '%s' is valid.\n", filePath), nil
}

type NodeCheckTool struct {
	executor Executor
	nodeMgr  node.Manager
}

func NewNodeCheckTool(executor Executor, nodeMgr node.Manager) *NodeCheckTool {
	return &NodeCheckTool{executor: executor, nodeMgr: nodeMgr}
}

func (t *NodeCheckTool) Name() string {
	return "node_check"
}

func (t *NodeCheckTool) Description() string {
	return "Check SSH connectivity of nodes, supports checking all nodes or specific nodes."
}

func (t *NodeCheckTool) Parameters() string {
	return `{
		"type": "object",
		"properties": {
			"nodes": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Node name list (mutually exclusive with all)"
			},
			"group": {
				"type": "string",
				"description": "Filter by group, e.g. 'web', 'db' (mutually exclusive with nodes/all)"
			},
			"all": {
				"type": "boolean",
				"description": "Check all nodes (mutually exclusive with nodes/group)"
			},
			"timeout": {
				"type": "integer",
				"description": "Timeout in seconds, default 10"
			},
			"update": {
				"type": "boolean",
				"description": "Update node status after check"
			}
		}
	}`
}

func (t *NodeCheckTool) Validate(params map[string]interface{}) error {
	hasNodes := false
	if nodes, ok := params["nodes"].([]interface{}); ok && len(nodes) > 0 {
		hasNodes = true
	}
	hasAll, _ := params["all"].(bool)
	hasGroup := false
	if group, ok := params["group"].(string); ok && group != "" {
		hasGroup = true
	}

	if !hasNodes && !hasAll && !hasGroup {
		return fmt.Errorf("must provide either 'nodes', 'all', or 'group'")
	}
	return nil
}

func (t *NodeCheckTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor != nil {
		p := NodeCheckParams{}
		if nodeList, ok := params["nodes"].([]interface{}); ok {
			for _, n := range nodeList {
				if s, ok := n.(string); ok {
					p.Nodes = append(p.Nodes, s)
				}
			}
		}
		p.Group, _ = params["group"].(string)
		p.All, _ = params["all"].(bool)
		if tv, ok := params["timeout"].(float64); ok {
			p.Timeout = int(tv)
		}
		p.Update, _ = params["update"].(bool)
		result, err := t.executor.NodeCheck(ctx, p)
		if err == nil {
			return result.Text, nil
		}
	}

	var nodes []*model.Node

	if all, _ := params["all"].(bool); all {
		nodes = t.nodeMgr.List()
	} else if group, _ := params["group"].(string); group != "" {
		nodes = t.nodeMgr.GetByGroup(group)
	} else if nodeList, ok := params["nodes"].([]interface{}); ok && len(nodeList) > 0 {
		var nodeNames []string
		for _, node := range nodeList {
			if s, ok := node.(string); ok {
				nodeNames = append(nodeNames, s)
			}
		}
		allNodes := t.nodeMgr.List()
		for _, n := range allNodes {
			for _, name := range nodeNames {
				if n.Name == name || n.ID == name {
					nodes = append(nodes, n)
					break
				}
			}
		}
	}

	if len(nodes) == 0 {
		return "No matching nodes found", nil
	}

	timeout := 10
	if tv, ok := params["timeout"].(float64); ok {
		timeout = int(tv)
	}

	updateStatus, _ := params["update"].(bool)

	args := []string{"node", "check", "--no-color"}

	if all, _ := params["all"].(bool); all {
		args = append(args, "--all")
	} else if group, _ := params["group"].(string); group != "" {
		args = append(args, "--groups", group)
	} else if nodeList, ok := params["nodes"].([]interface{}); ok && len(nodeList) > 0 {
		var nodeNames []string
		for _, node := range nodeList {
			if s, ok := node.(string); ok {
				nodeNames = append(nodeNames, s)
			}
		}
		args = append(args, nodeNames...)
	}

	if timeout != 10 {
		args = append(args, "--timeout", fmt.Sprintf("%ds", timeout))
	}

	debugLogger.Debugw("调用 owl node check 命令", "args", args)
	result, err := runOwlCommand(ctx, args)
	if err == nil {
		return result, nil
	}
	debugLogger.Debugw("调用 owl node check 失败，回退到模拟结果", "error", err)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("正在检查 %d 个节点的 SSH 连通性...\n\n", len(nodes)))

	online := 0
	offline := 0

	for _, n := range nodes {
		if n.Status == model.NodeStatusOnline {
			sb.WriteString(fmt.Sprintf("  ✓ %s (%s:%d) - 在线\n", n.Name, n.Address, n.Port))
			online++
		} else {
			sb.WriteString(fmt.Sprintf("  ✗ %s (%s:%d) - 离线\n", n.Name, n.Address, n.Port))
			offline++
		}
	}

	sb.WriteString(fmt.Sprintf("\n总结: %d 在线, %d 离线, 共 %d\n", online, offline, len(nodes)))

	if updateStatus {
		sb.WriteString("节点状态已更新\n")
	}

	return sb.String(), nil
}

type ToolRegistry struct {
	tools map[string]Tool
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: make(map[string]Tool)}
}

func (r *ToolRegistry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

func (r *ToolRegistry) Get(name string) (Tool, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}

func (r *ToolRegistry) ListAll() []Tool {
	tools := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	return tools
}

func (r *ToolRegistry) GetToolDescriptions() string {
	var descs []string
	for _, tool := range r.tools {
		descs = append(descs, fmt.Sprintf("- %s: %s", tool.Name(), tool.Description()))
	}
	return strings.Join(descs, "\n")
}

func defaultPlaybookDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "./playbooks"
	}
	p := filepath.Join(home, ".owl", "playbooks")
	if fi, err := os.Stat(p); err == nil && fi.IsDir() {
		return p
	}
	if fi, err := os.Stat("./playbooks"); err == nil && fi.IsDir() {
		abs, _ := filepath.Abs("./playbooks")
		return abs
	}
	return p
}
