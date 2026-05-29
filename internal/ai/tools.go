package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/cangyunye/go-owl/internal/common/model"
	"github.com/cangyunye/go-owl/internal/control/node"
)

type Tool interface {
	Name() string
	Description() string
	Parameters() string
	Validate(params map[string]interface{}) error
	Execute(ctx context.Context, params map[string]interface{}) (string, error)
}

type QueryNodesTool struct {
	nodeMgr node.Manager
}

func NewQueryNodesTool(nodeMgr node.Manager) *QueryNodesTool {
	return &QueryNodesTool{nodeMgr: nodeMgr}
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

func (t *QueryNodesTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	group, _ := params["group"].(string)
	labels, _ := params["labels"].(map[string]interface{})
	status, _ := params["status"].(string)
	format, _ := params["format"].(string)
	if format == "" {
		format = "table"
	}

	var nodes []*model.Node

	if group != "" {
		nodes = t.nodeMgr.GetByGroup(group)
	} else if labels != nil {
		labelMap := make(map[string]string)
		for k, v := range labels {
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

	if search, ok := params["search"].(string); ok && search != "" {
		filtered := make([]*model.Node, 0)
		lowerSearch := strings.ToLower(search)
		for _, n := range nodes {
			if strings.Contains(strings.ToLower(n.Name), lowerSearch) {
				filtered = append(filtered, n)
			}
		}
		nodes = filtered
	}

	if len(nodes) == 0 {
		return "No matching nodes found", nil
	}

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
	sb.WriteString(fmt.Sprintf("%-12s %-15s %-6s %-12s %-10s %-20s\n",
		"NAME", "ADDRESS", "PORT", "STATUS", "GROUPS", "LABELS"))
	sb.WriteString(strings.Repeat("-", 80))
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
		sb.WriteString(fmt.Sprintf("%-12s %-15s %-6d %-12s %-10s %-20s\n",
			n.Name, n.Address, n.Port, n.Status, groups, labels))
	}
	sb.WriteString(fmt.Sprintf("\nTotal %d nodes, %d online", len(nodes), t.countOnline(nodes)))
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

type ExecuteCommandTool struct {
	nodeMgr node.Manager
}

func NewExecuteCommandTool(nodeMgr node.Manager) *ExecuteCommandTool {
	return &ExecuteCommandTool{nodeMgr: nodeMgr}
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

	var nodes []*model.Node
	var filterDesc string

	if nodeList, ok := params["nodes"].([]interface{}); ok && len(nodeList) > 0 {
		var nodeNames []string
		for _, node := range nodeList {
			if s, ok := node.(string); ok {
				nodeNames = append(nodeNames, s)
			}
		}
		for _, name := range nodeNames {
			n, err := t.nodeMgr.GetByID(name)
			if err != nil {
				continue
			}
			nodes = append(nodes, n)
		}
		filterDesc = fmt.Sprintf("nodes: %s", strings.Join(nodeNames, ", "))
	} else if group, _ := params["group"].(string); group != "" {
		nodes = t.nodeMgr.GetByGroup(group)
		filterDesc = fmt.Sprintf("group: %s", group)
	} else if label, _ := params["label"].(string); label != "" {
		labelMap := parseLabelFilter(label)
		nodes = t.nodeMgr.GetByLabels(labelMap)
		filterDesc = fmt.Sprintf("label: %s", label)
	} else if search, ok := params["search"].(string); ok && search != "" {
		nodes = t.nodeMgr.SearchByName(search)
		filterDesc = fmt.Sprintf("search: %s", search)
	} else {
		nodes = t.nodeMgr.List()
		filterDesc = "all nodes"
	}

	if len(nodes) == 0 {
		return "No matching nodes found", nil
	}

	if err := t.validateCommand(command); err != nil {
		return "", fmt.Errorf("dangerous command: %w", err)
	}

	var sb strings.Builder

	switch format {
	case "json":
		sb.WriteString(t.formatExecuteJSON(command, nodes, timeout, mode, filterDesc))
	case "detail":
		sb.WriteString(t.formatExecuteDetail(command, nodes, timeout, mode, filterDesc))
	default:
		sb.WriteString(t.formatExecuteSimple(command, nodes, timeout, mode, filterDesc))
	}

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
	nodeMgr node.Manager
}

func NewExecuteScriptTool(nodeMgr node.Manager) *ExecuteScriptTool {
	return &ExecuteScriptTool{nodeMgr: nodeMgr}
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

	if !inline {
		if _, err := os.Stat(script); err != nil {
			return "", fmt.Errorf("script file not found: %s", script)
		}
	}

	dest, _ := params["dest"].(string)
	if dest == "" {
		dest = "/tmp"
	}

	args, _ := params["args"].(string)

	timeout := 300
	if tv, ok := params["timeout"].(float64); ok {
		timeout = int(tv)
	}

	keep, _ := params["keep"].(bool)

	var nodes []*model.Node
	var filterDesc string

	if targets, ok := params["targets"].([]interface{}); ok && len(targets) > 0 {
		var targetNames []string
		for _, target := range targets {
			if s, ok := target.(string); ok {
				targetNames = append(targetNames, s)
			}
		}
		for _, name := range targetNames {
			n, err := t.nodeMgr.GetByID(name)
			if err != nil {
				continue
			}
			nodes = append(nodes, n)
		}
		filterDesc = fmt.Sprintf("targets: %s", strings.Join(targetNames, ", "))
	} else if group, _ := params["group"].(string); group != "" {
		nodes = t.nodeMgr.GetByGroup(group)
		filterDesc = fmt.Sprintf("group: %s", group)
	} else if label, _ := params["label"].(string); label != "" {
		labelMap := parseLabelFilter(label)
		nodes = t.nodeMgr.GetByLabels(labelMap)
		filterDesc = fmt.Sprintf("label: %s", label)
	} else if search, ok := params["search"].(string); ok && search != "" {
		nodes = t.nodeMgr.SearchByName(search)
		filterDesc = fmt.Sprintf("search: %s", search)
	} else {
		nodes = t.nodeMgr.List()
		filterDesc = "all nodes"
	}

	if len(nodes) == 0 {
		return "No matching nodes found", nil
	}

	execType := "File upload+exec"
	if inline {
		execType = "Inline execution"
	}

	keepStr := "No"
	if keep {
		keepStr = "Yes"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Script execution task:\n"))
	sb.WriteString(fmt.Sprintf("Script: %s\n", script))
	sb.WriteString(fmt.Sprintf("Type: %s\n", execType))
	sb.WriteString(fmt.Sprintf("Target: %s (%d nodes)\n", filterDesc, len(nodes)))
	sb.WriteString(fmt.Sprintf("Destination: %s\n", dest))
	if args != "" {
		sb.WriteString(fmt.Sprintf("Arguments: %s\n", args))
	}
	sb.WriteString(fmt.Sprintf("Timeout: %ds\n", timeout))
	sb.WriteString(fmt.Sprintf("Keep after exec: %s\n\n", keepStr))
	sb.WriteString("Target nodes:\n")
	sb.WriteString(strings.Repeat("-", 60))
	sb.WriteString("\n")

	for _, n := range nodes {
		sb.WriteString(fmt.Sprintf("[%s] %s:%d | Status: %s\n", n.Name, n.Address, n.Port, n.Status))
	}

	return sb.String(), nil
}

type GeneratePlaybookTool struct {
	nodeMgr node.Manager
}

func NewGeneratePlaybookTool(nodeMgr node.Manager) *GeneratePlaybookTool {
	return &GeneratePlaybookTool{nodeMgr: nodeMgr}
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
	nodeMgr node.Manager
}

func NewTransferFileTool(nodeMgr node.Manager) *TransferFileTool {
	return &TransferFileTool{nodeMgr: nodeMgr}
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

	nodeList, ok := params["nodes"].([]interface{})
	if !ok || len(nodeList) == 0 {
		return "", fmt.Errorf("missing nodes")
	}

	destDir, ok := params["dest_dir"].(string)
	if !ok || destDir == "" {
		return "", fmt.Errorf("missing target directory")
	}

	mode, _ := params["mode"].(string)
	if mode == "" {
		if len(nodeList) >= 5 {
			mode = "diffusion"
		} else {
			mode = "direct"
		}
	}

	permission, _ := params["permission"].(string)
	if permission == "" {
		permission = "0644"
	}

	var nodeNames []string
	for _, node := range nodeList {
		if s, ok := node.(string); ok {
			nodeNames = append(nodeNames, s)
		}
	}

	transferMode := "Direct transfer"
	if mode == "diffusion" {
		transferMode = "Diffusion transfer"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("File transfer task:\n"))
	sb.WriteString(fmt.Sprintf("Source file: %s\n", sourceFile))
	sb.WriteString(fmt.Sprintf("Target directory: %s\n", destDir))
	sb.WriteString(fmt.Sprintf("Nodes: %s\n", strings.Join(nodeNames, ", ")))
	sb.WriteString(fmt.Sprintf("Transfer mode: %s\n", transferMode))
	sb.WriteString(fmt.Sprintf("File permission: %s\n", permission))
	sb.WriteString(fmt.Sprintf("Node count: %d\n", len(nodeNames)))

	if mode == "diffusion" {
		sourceCount := len(nodeNames) / 3
		if sourceCount < 2 {
			sourceCount = 2
		}
		sb.WriteString(fmt.Sprintf("Source node count: %d\n", sourceCount))
		sb.WriteString("Diffusion transfer: First N nodes as sources, other nodes get files from source nodes\n")
	}

	return sb.String(), nil
}

type QueryDatabaseTool struct {
	nodeMgr node.Manager
}

func NewQueryDatabaseTool(nodeMgr node.Manager) *QueryDatabaseTool {
	return &QueryDatabaseTool{nodeMgr: nodeMgr}
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

	query, hasQuery := params["query"].(string)

	var nodes []*model.Node

	if hasQuery && query != "" {
		nodes = t.nodeMgr.List()
		nodes = t.filterBySQL(nodes, query)
	} else {
		group, _ := params["group"].(string)
		labelsRaw, _ := params["labels"].(map[string]interface{})
		status, _ := params["status"].(string)
		search, _ := params["search"].(string)

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

		if search != "" && len(nodes) > 0 {
			filtered := make([]*model.Node, 0)
			lowerSearch := strings.ToLower(search)
			for _, n := range nodes {
				if strings.Contains(strings.ToLower(n.Name), lowerSearch) {
					filtered = append(filtered, n)
				}
			}
			nodes = filtered
		}
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
	sb.WriteString(fmt.Sprintf("%-12s %-15s %-6s %-12s %-10s %-20s\n",
		"NAME", "ADDRESS", "PORT", "STATUS", "GROUPS", "LABELS"))
	sb.WriteString(strings.Repeat("-", 80))
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
		sb.WriteString(fmt.Sprintf("%-12s %-15s %-6d %-12s %-10s %-20s\n",
			n.Name, n.Address, n.Port, n.Status, groups, labels))
	}
	count := 0
	for _, n := range nodes {
		if n.Status == model.NodeStatusOnline {
			count++
		}
	}
	sb.WriteString(fmt.Sprintf("\nTotal %d nodes, %d online", len(nodes), count))
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

func GetToolDefinitions() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "query_nodes",
				"description": "Query node information, support filtering by group, label, and status.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"group": map[string]interface{}{
							"type":        "string",
							"description": "Filter by group, e.g. 'web', 'db'",
						},
						"labels": map[string]interface{}{
							"type":        "object",
							"description": "Filter by labels, e.g. {\"env\": \"prod\"}",
						},
						"status": map[string]interface{}{
							"type":        "string",
							"description": "Filter by status: online, offline, unknown",
						},
						"search": map[string]interface{}{
							"type":        "string",
							"description": "Fuzzy search by node name (case-insensitive substring match)",
						},
						"format": map[string]interface{}{
							"type":        "string",
							"description": "Output format: table (default), json, summary",
						},
					},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "execute_command",
				"description": "Execute commands on specified nodes, return execution results.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"nodes": map[string]interface{}{
							"type":        "array",
							"items":       map[string]interface{}{"type": "string"},
							"description": "Node name list (mutually exclusive with group/label)",
						},
						"command": map[string]interface{}{
							"type":        "string",
							"description": "Command to execute",
						},
						"group": map[string]interface{}{
							"type":        "string",
							"description": "Filter by group, e.g. 'web', 'db' (mutually exclusive with nodes/label)",
						},
						"label": map[string]interface{}{
							"type":        "string",
							"description": "Filter by label, e.g. 'env=prod' (mutually exclusive with nodes/group)",
						},
						"timeout": map[string]interface{}{
							"type":        "integer",
							"description": "Timeout in seconds, default 30",
						},
						"format": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"simple", "detail", "json"},
							"description": "Output format: simple (default), detail, json",
						},
						"mode": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"parallel", "serial", "async"},
							"description": "Execution mode: parallel (default), serial, async",
						},
						"search": map[string]interface{}{
							"type":        "string",
							"description": "Fuzzy search by node name, case-insensitive substring match (mutually exclusive with nodes/group/label)",
						},
					},
					"required": []string{"command"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "generate_playbook",
				"description": "Generate Ansible-like YAML playbook from natural language requirements.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"requirement": map[string]interface{}{
							"type":        "string",
							"description": "User requirement description",
						},
						"vars": map[string]interface{}{
							"type":        "object",
							"description": "Custom variables",
						},
					},
					"required": []string{"requirement"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "transfer_file",
				"description": "Transfer files to specified nodes, supports direct and diffusion transfer.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"source_file": map[string]interface{}{
							"type":        "string",
							"description": "Source file path (local)",
						},
						"nodes": map[string]interface{}{
							"type":        "array",
							"items":       map[string]interface{}{"type": "string"},
							"description": "Node name list",
						},
						"dest_dir": map[string]interface{}{
							"type":        "string",
							"description": "Target directory (remote)",
						},
						"mode": map[string]interface{}{
							"type":        "string",
							"description": "Transfer mode: direct or diffusion, default auto",
						},
						"permission": map[string]interface{}{
							"type":        "string",
							"description": "File permission, e.g. 0644",
						},
					},
					"required": []string{"source_file", "nodes", "dest_dir"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "execute_script",
				"description": "Execute script files on specified nodes. Supports local file upload+exec and inline execution.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"script": map[string]interface{}{
							"type":        "string",
							"description": "Script file path or inline script content",
						},
						"targets": map[string]interface{}{
							"type":        "array",
							"items":       map[string]interface{}{"type": "string"},
							"description": "Target node name list (mutually exclusive with group/label)",
						},
						"group": map[string]interface{}{
							"type":        "string",
							"description": "Filter by group, e.g. 'web', 'db' (mutually exclusive with targets/label)",
						},
						"label": map[string]interface{}{
							"type":        "string",
							"description": "Filter by label, e.g. 'env=prod' (mutually exclusive with targets/group)",
						},
						"search": map[string]interface{}{
							"type":        "string",
							"description": "Fuzzy search by node name, case-insensitive substring match (mutually exclusive with targets/group/label)",
						},
						"dest": map[string]interface{}{
							"type":        "string",
							"description": "Destination path on remote nodes, default /tmp",
						},
						"args": map[string]interface{}{
							"type":        "string",
							"description": "Arguments to pass to the script",
						},
						"timeout": map[string]interface{}{
							"type":        "integer",
							"description": "Timeout in seconds, default 300",
						},
						"inline": map[string]interface{}{
							"type":        "boolean",
							"description": "If true, treat script param as inline content instead of file path, default false",
						},
						"keep": map[string]interface{}{
							"type":        "boolean",
							"description": "If true, keep script file on remote after execution, default false",
						},
					},
					"required": []string{"script"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "query_database",
				"description": "Query the owl database directly. Supports SQL SELECT queries and structured filters (group/labels/status/search).",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "SQL SELECT query to execute on the nodes table",
						},
						"group": map[string]interface{}{
							"type":        "string",
							"description": "Filter by group, e.g. 'web', 'db'",
						},
						"labels": map[string]interface{}{
							"type":        "object",
							"description": "Filter by labels, e.g. {\"env\": \"prod\"}",
						},
						"status": map[string]interface{}{
							"type":        "string",
							"description": "Filter by status: online, offline, unknown",
						},
						"search": map[string]interface{}{
							"type":        "string",
							"description": "Fuzzy search by node name (case-insensitive substring match)",
						},
						"format": map[string]interface{}{
							"type":        "string",
							"description": "Output format: table (default), json, summary",
						},
					},
				},
			},
		},
	}
}
