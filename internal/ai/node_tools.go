package ai

import (
	"context"
	"fmt"
	"strings"

	"github.com/cangyunye/go-owl/internal/common/model"
	"github.com/cangyunye/go-owl/internal/control/node"
)

// ---------- node add ----------

type NodeAddTool struct {
	executor  Executor
	nodeStore NodeStoreAdapter
}

func NewNodeAddTool(executor Executor, nodeStore NodeStoreAdapter) *NodeAddTool {
	return &NodeAddTool{executor: executor, nodeStore: nodeStore}
}

func (t *NodeAddTool) Name() string         { return "node_add" }
func (t *NodeAddTool) Description() string  { return "Add a new node to the inventory." }
func (t *NodeAddTool) Parameters() string   { return nodeAddParamsSchema }
func (t *NodeAddTool) Validate(p map[string]interface{}) error {
	if s, ok := p["name"].(string); !ok || strings.TrimSpace(s) == "" {
		return fmt.Errorf("name is required")
	}
	if s, ok := p["address"].(string); !ok || strings.TrimSpace(s) == "" {
		return fmt.Errorf("address is required")
	}
	return nil
}

const nodeAddParamsSchema = `{
	"type": "object",
	"properties": {
		"name": {"type": "string", "description": "Node name, must be unique"},
		"address": {"type": "string", "description": "IP or hostname"},
		"port": {"type": "integer", "description": "SSH port, default 22"},
		"user": {"type": "string", "description": "SSH login user"},
		"password": {"type": "string", "description": "SSH password"},
		"ssh_key": {"type": "string", "description": "SSH private key path"},
		"proxy_jump": {"type": "string", "description": "SSH ProxyJump target"},
		"groups": {"type": "string", "description": "Comma separated groups, e.g. web,db"},
		"labels": {"type": "object", "description": "Labels map, e.g. {\"env\":\"prod\"}"}
	},
	"required": ["name", "address"]
}`

func (t *NodeAddTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor != nil {
		p := NodeAddParams{
			Name:      strOf(params["name"]),
			Address:   strOf(params["address"]),
			Port:      intOf(params["port"]),
			User:      strOf(params["user"]),
			Password:  strOf(params["password"]),
			SSHKey:    strOf(params["ssh_key"]),
			ProxyJump: strOf(params["proxy_jump"]),
			Groups:    strOf(params["groups"]),
		}
		if l, ok := params["labels"].(map[string]interface{}); ok {
			p.Labels = l
		}
		result, err := t.executor.AddNode(ctx, p)
		if err == nil {
			return result.Text, nil
		}
	}
	return "", fmt.Errorf("node_add failed: no node store available for local fallback")
}

// ---------- node remove ----------

type NodeRemoveTool struct {
	executor  Executor
	nodeStore NodeStoreAdapter
}

func NewNodeRemoveTool(executor Executor, nodeStore NodeStoreAdapter) *NodeRemoveTool {
	return &NodeRemoveTool{executor: executor, nodeStore: nodeStore}
}

func (t *NodeRemoveTool) Name() string        { return "node_remove" }
func (t *NodeRemoveTool) Description() string { return "Remove one or more nodes by name or id." }
func (t *NodeRemoveTool) Parameters() string  { return nodeRemoveParamsSchema }
func (t *NodeRemoveTool) Validate(p map[string]interface{}) error {
	if id, ok := p["id"].(string); ok && id != "" {
		return nil
	}
	nodes, ok := p["nodes"].([]interface{})
	if !ok || len(nodes) == 0 {
		return fmt.Errorf("nodes is required (non-empty array)")
	}
	return nil
}

const nodeRemoveParamsSchema = `{
	"type": "object",
	"properties": {
		"nodes": {"type": "array", "items": {"type": "string"}, "description": "Node name or id list to remove"}
	},
	"required": ["nodes"]
}`

func (t *NodeRemoveTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor != nil {
		p := NodeRemoveParams{Nodes: strSliceOf(params["nodes"])}
		if len(p.Nodes) == 0 {
			if id := strOf(params["id"]); id != "" {
				p.Nodes = []string{id}
			}
		}
		result, err := t.executor.RemoveNode(ctx, p)
		if err == nil {
			return result.Text, nil
		}
	}
	return "", fmt.Errorf("node_remove failed")
}

// ---------- node update ----------

type NodeUpdateTool struct {
	executor  Executor
	nodeStore NodeStoreAdapter
}

func NewNodeUpdateTool(executor Executor, nodeStore NodeStoreAdapter) *NodeUpdateTool {
	return &NodeUpdateTool{executor: executor, nodeStore: nodeStore}
}

func (t *NodeUpdateTool) Name() string        { return "node_update" }
func (t *NodeUpdateTool) Description() string { return "Update an existing node's attributes." }
func (t *NodeUpdateTool) Parameters() string  { return nodeUpdateParamsSchema }
func (t *NodeUpdateTool) Validate(p map[string]interface{}) error {
	if s, ok := p["id"].(string); !ok || strings.TrimSpace(s) == "" {
		return fmt.Errorf("id is required")
	}
	return nil
}

const nodeUpdateParamsSchema = `{
	"type": "object",
	"properties": {
		"id": {"type": "string", "description": "Node id or name to update"},
		"name": {"type": "string"}, "address": {"type": "string"},
		"port": {"type": "integer"}, "user": {"type": "string"},
		"password": {"type": "string"}, "ssh_key": {"type": "string"},
		"proxy_jump": {"type": "string"},
		"groups": {"type": "string", "description": "Comma separated groups"},
		"labels": {"type": "object"},
		"status": {"type": "string", "enum": ["online", "offline", "unknown"]}
	},
	"required": ["id"]
}`

func (t *NodeUpdateTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor != nil {
		p := NodeUpdateParams{
			ID:        strOf(params["id"]),
			Name:      strOf(params["name"]),
			Address:   strOf(params["address"]),
			Port:      intOf(params["port"]),
			User:      strOf(params["user"]),
			Password:  strOf(params["password"]),
			SSHKey:    strOf(params["ssh_key"]),
			ProxyJump: strOf(params["proxy_jump"]),
			Groups:    strOf(params["groups"]),
			Status:    strOf(params["status"]),
		}
		if l, ok := params["labels"].(map[string]interface{}); ok {
			p.Labels = l
		}
		result, err := t.executor.UpdateNode(ctx, p)
		if err == nil {
			return result.Text, nil
		}
	}
	return "", fmt.Errorf("node_update failed")
}

// ---------- node status ----------

type NodeStatusTool struct {
	executor Executor
	nodeMgr  node.Manager
}

func NewNodeStatusTool(executor Executor, nodeMgr node.Manager) *NodeStatusTool {
	return &NodeStatusTool{executor: executor, nodeMgr: nodeMgr}
}

func (t *NodeStatusTool) Name() string        { return "node_status" }
func (t *NodeStatusTool) Description() string { return "Show status of nodes (online/offline/unknown)." }
func (t *NodeStatusTool) Parameters() string  { return nodeStatusParamsSchema }
func (t *NodeStatusTool) Validate(p map[string]interface{}) error { return nil }

const nodeStatusParamsSchema = `{
	"type": "object",
	"properties": {
		"nodes": {"type": "array", "items": {"type": "string"}, "description": "Node name list"},
		"all": {"type": "boolean", "description": "Show all nodes"},
		"format": {"type": "string", "enum": ["detail", "simple", "json"], "description": "Output format, default detail"}
	}
}`

func (t *NodeStatusTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor != nil {
		p := NodeStatusParams{
			Nodes:  strSliceOf(params["nodes"]),
			All:    boolOf(params["all"]),
			Format: strOf(params["format"]),
		}
		result, err := t.executor.NodeStatus(ctx, p)
		if err == nil {
			return result.Text, nil
		}
	}
	// fallback: nodeMgr 内存状态
	var sb strings.Builder
	var nodes []*nodeView
	if all, _ := params["all"].(bool); all || t.nodeMgr == nil {
		nodes = nodeViews(t.nodeMgr.List())
	} else if ids := strSliceOf(params["nodes"]); len(ids) > 0 {
		var filtered []*nodeView
		for _, id := range ids {
			if n, err := t.nodeMgr.GetByID(id); err == nil {
				filtered = append(filtered, &nodeView{Name: n.Name, Address: n.Address, Port: n.Port, Status: string(n.Status)})
			}
		}
		nodes = filtered
	} else {
		nodes = nodeViews(t.nodeMgr.List())
	}
	if len(nodes) == 0 {
		return "No nodes found", nil
	}
	for _, n := range nodes {
		sb.WriteString(fmt.Sprintf("%s\t%s:%d\t%s\n", n.Name, n.Address, n.Port, n.Status))
	}
	return sb.String(), nil
}

// ---------- node ping ----------

type NodePingTool struct {
	executor Executor
	nodeMgr  node.Manager
}

func NewNodePingTool(executor Executor, nodeMgr node.Manager) *NodePingTool {
	return &NodePingTool{executor: executor, nodeMgr: nodeMgr}
}

func (t *NodePingTool) Name() string        { return "node_ping" }
func (t *NodePingTool) Description() string { return "Ping nodes to test network reachability." }
func (t *NodePingTool) Parameters() string  { return nodePingParamsSchema }
func (t *NodePingTool) Validate(p map[string]interface{}) error {
	all, _ := p["all"].(bool)
	nodes := strSliceOf(p["nodes"])
	if !all && len(nodes) == 0 {
		return fmt.Errorf("must provide nodes or all=true")
	}
	return nil
}

const nodePingParamsSchema = `{
	"type": "object",
	"properties": {
		"nodes": {"type": "array", "items": {"type": "string"}, "description": "Node name list"},
		"all": {"type": "boolean", "description": "Ping all nodes"},
		"count": {"type": "integer", "description": "Ping count, default 1"},
		"timeout_sec": {"type": "integer", "description": "Timeout seconds, default 3"}
	}
}`

func (t *NodePingTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor != nil {
		p := NodePingParams{
			Nodes:    strSliceOf(params["nodes"]),
			All:      boolOf(params["all"]),
			Count:    intOf(params["count"]),
			Timeout:  intOf(params["timeout_sec"]),
		}
		result, err := t.executor.NodePing(ctx, p)
		if err == nil {
			return result.Text, nil
		}
	}
	return "", fmt.Errorf("node_ping failed")
}

// ---------- node groups ----------

type NodeGroupsTool struct {
	executor Executor
}

func NewNodeGroupsTool(executor Executor) *NodeGroupsTool {
	return &NodeGroupsTool{executor: executor}
}

func (t *NodeGroupsTool) Name() string        { return "node_groups" }
func (t *NodeGroupsTool) Description() string { return "Manage node groups: add/remove group for a node, or list/show groups." }
func (t *NodeGroupsTool) Parameters() string  { return nodeGroupsParamsSchema }
func (t *NodeGroupsTool) Validate(p map[string]interface{}) error {
	action := strings.ToLower(strOf(p["action"]))
	switch action {
	case "add", "remove", "delete":
		if strOf(p["node"]) == "" && len(strSliceOf(p["nodes"])) == 0 {
			return fmt.Errorf("node is required for add/remove")
		}
		if strOf(p["group"]) == "" {
			return fmt.Errorf("group is required for add/remove")
		}
	case "list", "show":
	default:
		return fmt.Errorf("action must be one of add/remove/list/show")
	}
	return nil
}

const nodeGroupsParamsSchema = `{
	"type": "object",
	"properties": {
		"action": {"type": "string", "enum": ["add", "remove", "list", "show"], "description": "Operation"},
		"node": {"type": "string", "description": "Node id (required for add/remove)"},
		"group": {"type": "string", "description": "Group name (required for add/remove/show)"}
	},
	"required": ["action"]
}`

func (t *NodeGroupsTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor != nil {
		p := NodeGroupsParams{
			Action: normalizeGroupsAction(strOf(params["action"])),
			Node:   strOf(params["node"]),
			Group:  strOf(params["group"]),
		}
		if p.Node == "" {
			if nodes := strSliceOf(params["nodes"]); len(nodes) > 0 {
				p.Node = nodes[0]
			}
		}
		result, err := t.executor.NodeGroups(ctx, p)
		if err == nil {
			return result.Text, nil
		}
	}
	return "", fmt.Errorf("node_groups failed")
}

// ---------- node labels ----------

type NodeLabelsTool struct {
	executor Executor
}

func NewNodeLabelsTool(executor Executor) *NodeLabelsTool {
	return &NodeLabelsTool{executor: executor}
}

func (t *NodeLabelsTool) Name() string        { return "node_labels" }
func (t *NodeLabelsTool) Description() string { return "Manage node labels: set/remove/show key=value labels on nodes." }
func (t *NodeLabelsTool) Parameters() string  { return nodeLabelsParamsSchema }
func (t *NodeLabelsTool) Validate(p map[string]interface{}) error {
	action := strings.ToLower(strOf(p["action"]))
	switch action {
	case "set", "add":
		if strOf(p["node"]) == "" {
			return fmt.Errorf("node is required for set")
		}
		if l, ok := p["labels"].(map[string]interface{}); !ok || len(l) == 0 {
			return fmt.Errorf("labels map is required for set")
		}
	case "remove":
		if strOf(p["node"]) == "" || strOf(p["key"]) == "" {
			return fmt.Errorf("node and key are required for remove")
		}
	case "show", "list":
		if strOf(p["node"]) == "" {
			return fmt.Errorf("node is required for show")
		}
	default:
		return fmt.Errorf("action must be one of set/remove/show")
	}
	return nil
}

const nodeLabelsParamsSchema = `{
	"type": "object",
	"properties": {
		"action": {"type": "string", "enum": ["set", "remove", "show"], "description": "Operation"},
		"node": {"type": "string", "description": "Node id"},
		"key": {"type": "string", "description": "Label key (for remove)"},
		"labels": {"type": "object", "description": "Labels map for set, e.g. {\"env\":\"prod\"}"}
	},
	"required": ["action"]
}`

func (t *NodeLabelsTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor != nil {
		p := NodeLabelsParams{
			Action: normalizeLabelsAction(strOf(params["action"])),
			Node:   strOf(params["node"]),
			Key:    strOf(params["key"]),
		}
		if l, ok := params["labels"].(map[string]interface{}); ok {
			p.Labels = l
		}
		result, err := t.executor.NodeLabels(ctx, p)
		if err == nil {
			return result.Text, nil
		}
	}
	return "", fmt.Errorf("node_labels failed")
}

func normalizeLabelsAction(action string) string {
	switch strings.ToLower(action) {
	case "add":
		return "set"
	case "list":
		return "show"
	default:
		return action
	}
}

func normalizeGroupsAction(action string) string {
	switch strings.ToLower(action) {
	case "delete":
		return "remove"
	default:
		return action
	}
}

// ---------- node import / export ----------

type NodeImportTool struct {
	executor Executor
}

func NewNodeImportTool(executor Executor) *NodeImportTool {
	return &NodeImportTool{executor: executor}
}

func (t *NodeImportTool) Name() string        { return "node_import" }
func (t *NodeImportTool) Description() string { return "Import nodes from a YAML/JSON file." }
func (t *NodeImportTool) Parameters() string  { return nodeImportParamsSchema }
func (t *NodeImportTool) Validate(p map[string]interface{}) error {
	if strOf(p["file"]) == "" {
		return fmt.Errorf("file is required")
	}
	return nil
}

const nodeImportParamsSchema = `{
	"type": "object",
	"properties": {
		"file": {"type": "string", "description": "Path to YAML/JSON file with nodes"},
		"format": {"type": "string", "enum": ["yaml", "json"], "description": "File format, default yaml"},
		"overwrite": {"type": "boolean", "description": "Overwrite existing nodes"},
		"dry_run": {"type": "boolean", "description": "Validate without applying changes"}
	},
	"required": ["file"]
}`

func (t *NodeImportTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor != nil {
		p := NodeImportParams{
			File:      strOf(params["file"]),
			Format:    strOf(params["format"]),
			Overwrite: boolOf(params["overwrite"]),
			DryRun:    boolOf(params["dry_run"]),
		}
		result, err := t.executor.NodeImport(ctx, p)
		if err == nil {
			return result.Text, nil
		}
	}
	return "", fmt.Errorf("node_import failed")
}

type NodeExportTool struct {
	executor Executor
}

func NewNodeExportTool(executor Executor) *NodeExportTool {
	return &NodeExportTool{executor: executor}
}

func (t *NodeExportTool) Name() string        { return "node_export" }
func (t *NodeExportTool) Description() string { return "Export nodes to a YAML/JSON file." }
func (t *NodeExportTool) Parameters() string  { return nodeExportParamsSchema }
func (t *NodeExportTool) Validate(p map[string]interface{}) error { return nil }

const nodeExportParamsSchema = `{
	"type": "object",
	"properties": {
		"file": {"type": "string", "description": "Output file path"},
		"format": {"type": "string", "enum": ["yaml", "json"], "description": "Output format, default yaml"},
		"nodes": {"type": "array", "items": {"type": "string"}, "description": "Filter by node names"},
		"groups": {"type": "array", "items": {"type": "string"}, "description": "Filter by groups"}
	}
}`

func (t *NodeExportTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	if t.executor != nil {
		p := NodeExportParams{
			File:   strOf(params["file"]),
			Format: strOf(params["format"]),
			Nodes:  strSliceOf(params["nodes"]),
			Groups: strSliceOf(params["groups"]),
		}
		result, err := t.executor.NodeExport(ctx, p)
		if err == nil {
			return result.Text, nil
		}
	}
	return "", fmt.Errorf("node_export failed")
}

// ---------- helpers ----------

type nodeView struct {
	Name    string
	Address string
	Port    int
	Status  string
}

func nodeViews(nodes []*model.Node) []*nodeView {
	views := make([]*nodeView, 0, len(nodes))
	for _, n := range nodes {
		views = append(views, &nodeView{Name: n.Name, Address: n.Address, Port: n.Port, Status: string(n.Status)})
	}
	return views
}

func strOf(v interface{}) string {
	s, _ := v.(string)
	return s
}

func intOf(v interface{}) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	if i, ok := v.(int); ok {
		return i
	}
	return 0
}

func boolOf(v interface{}) bool {
	b, _ := v.(bool)
	return b
}

func strSliceOf(v interface{}) []string {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
