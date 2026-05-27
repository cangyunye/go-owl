package ai

import (
	"context"
	"encoding/json"
	"fmt"
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

	if len(nodes) == 0 {
		return "No matching nodes found", nil
	}

	switch format {
	case "json":
		data, _ := json.MarshalIndent(t.nodesToInfo(nodes), "", "  ")
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

func (t *QueryNodesTool) nodesToInfo(nodes []*model.Node) []nodeInfo {
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
			"targets": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Target node name list"
			},
			"command": {
				"type": "string",
				"description": "Command to execute"
			},
			"timeout": {
				"type": "integer",
				"description": "Timeout in seconds, default 60"
			}
		},
		"required": ["targets", "command"]
	}`
}

func (t *ExecuteCommandTool) Validate(params map[string]interface{}) error {
	validator := NewValidator()
	return validator.ValidateExecuteCommand(params)
}

func (t *ExecuteCommandTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	targets, ok := params["targets"].([]interface{})
	if !ok || len(targets) == 0 {
		return "", fmt.Errorf("missing target nodes")
	}

	command, ok := params["command"].(string)
	if !ok || command == "" {
		return "", fmt.Errorf("missing command")
	}

	timeout := 60
	if t, ok := params["timeout"].(float64); ok {
		timeout = int(t)
	}

	var targetNames []string
	for _, target := range targets {
		if s, ok := target.(string); ok {
			targetNames = append(targetNames, s)
		}
	}

	if err := t.validateCommand(command); err != nil {
		return "", fmt.Errorf("dangerous command: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Execute command: %s\n", command))
	sb.WriteString(fmt.Sprintf("Target nodes: %s\n", strings.Join(targetNames, ", ")))
	sb.WriteString(fmt.Sprintf("Timeout: %ds\n\n", timeout))
	sb.WriteString("Results:\n")
	sb.WriteString(strings.Repeat("-", 60))
	sb.WriteString("\n")

	for _, name := range targetNames {
		nodeInfo, err := t.nodeMgr.GetByID(name)
		if err != nil {
			sb.WriteString(fmt.Sprintf("[%s] Error: Node not found\n", name))
			continue
		}
		sb.WriteString(fmt.Sprintf("\n>>> %s (%s:%d) <<<\n", nodeInfo.Name, nodeInfo.Address, nodeInfo.Port))
		sb.WriteString(fmt.Sprintf("Status: %s\n", nodeInfo.Status))
	}

	return sb.String(), nil
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
			"targets": {
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
		"required": ["source_file", "targets", "dest_dir"]
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

	targets, ok := params["targets"].([]interface{})
	if !ok || len(targets) == 0 {
		return "", fmt.Errorf("missing target nodes")
	}

	destDir, ok := params["dest_dir"].(string)
	if !ok || destDir == "" {
		return "", fmt.Errorf("missing target directory")
	}

	mode, _ := params["mode"].(string)
	if mode == "" {
		if len(targets) >= 5 {
			mode = "diffusion"
		} else {
			mode = "direct"
		}
	}

	permission, _ := params["permission"].(string)
	if permission == "" {
		permission = "0644"
	}

	var targetNames []string
	for _, target := range targets {
		if s, ok := target.(string); ok {
			targetNames = append(targetNames, s)
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
	sb.WriteString(fmt.Sprintf("Target nodes: %s\n", strings.Join(targetNames, ", ")))
	sb.WriteString(fmt.Sprintf("Transfer mode: %s\n", transferMode))
	sb.WriteString(fmt.Sprintf("File permission: %s\n", permission))
	sb.WriteString(fmt.Sprintf("Node count: %d\n", len(targetNames)))

	if mode == "diffusion" {
		sourceCount := len(targetNames) / 3
		if sourceCount < 2 {
			sourceCount = 2
		}
		sb.WriteString(fmt.Sprintf("Source node count: %d\n", sourceCount))
		sb.WriteString("Diffusion transfer: First N nodes as sources, other nodes get files from source nodes\n")
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
						"targets": map[string]interface{}{
							"type":        "array",
							"items":       map[string]interface{}{"type": "string"},
							"description": "Target node name list",
						},
						"command": map[string]interface{}{
							"type":        "string",
							"description": "Command to execute",
						},
						"timeout": map[string]interface{}{
							"type":        "integer",
							"description": "Timeout in seconds, default 60",
						},
					},
					"required": []string{"targets", "command"},
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
						"targets": map[string]interface{}{
							"type":        "array",
							"items":       map[string]interface{}{"type": "string"},
							"description": "Target node name list",
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
					"required": []string{"source_file", "targets", "dest_dir"},
				},
			},
		},
	}
}
