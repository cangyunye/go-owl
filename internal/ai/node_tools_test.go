package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/internal/common/model"
)

func TestNodeToolsValidate(t *testing.T) {
	tests := []struct {
		name        string
		tool        Tool
		params      map[string]interface{}
		shouldError bool
	}{
		{"node_add valid", NewNodeAddTool(nil, nil), map[string]interface{}{"name": "web-01", "address": "10.0.0.1"}, false},
		{"node_add missing name", NewNodeAddTool(nil, nil), map[string]interface{}{"address": "10.0.0.1"}, true},
		{"node_add missing address", NewNodeAddTool(nil, nil), map[string]interface{}{"name": "web-01"}, true},
		{"node_remove valid", NewNodeRemoveTool(nil, nil), map[string]interface{}{"nodes": []interface{}{"web-01"}}, false},
		{"node_remove empty", NewNodeRemoveTool(nil, nil), map[string]interface{}{"nodes": []interface{}{}}, true},
		{"node_update valid", NewNodeUpdateTool(nil, nil), map[string]interface{}{"id": "web-01", "groups": "web"}, false},
		{"node_update missing id", NewNodeUpdateTool(nil, nil), map[string]interface{}{"groups": "web"}, true},
		{"node_ping all ok", NewNodePingTool(nil, nil), map[string]interface{}{"all": true}, false},
		{"node_ping nodes ok", NewNodePingTool(nil, nil), map[string]interface{}{"nodes": []interface{}{"web-01"}}, false},
		{"node_ping empty", NewNodePingTool(nil, nil), map[string]interface{}{}, true},
		{"node_groups add ok", NewNodeGroupsTool(nil), map[string]interface{}{"action": "add", "node": "web-01", "group": "web"}, false},
		{"node_groups add missing group", NewNodeGroupsTool(nil), map[string]interface{}{"action": "add", "node": "web-01"}, true},
		{"node_groups bad action", NewNodeGroupsTool(nil), map[string]interface{}{"action": "explode"}, true},
		{"node_labels set ok", NewNodeLabelsTool(nil), map[string]interface{}{"action": "set", "node": "web-01", "labels": map[string]interface{}{"env": "prod"}}, false},
		{"node_labels set missing labels", NewNodeLabelsTool(nil), map[string]interface{}{"action": "set", "node": "web-01"}, true},
		{"node_labels remove ok", NewNodeLabelsTool(nil), map[string]interface{}{"action": "remove", "node": "web-01", "key": "env"}, false},
		{"node_labels bad action", NewNodeLabelsTool(nil), map[string]interface{}{"action": "explode"}, true},
		{"node_import ok", NewNodeImportTool(nil), map[string]interface{}{"file": "/tmp/nodes.yaml"}, false},
		{"node_import missing file", NewNodeImportTool(nil), map[string]interface{}{}, true},
		{"node_export ok", NewNodeExportTool(nil), map[string]interface{}{"format": "json"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.tool.Validate(tt.params)
			if tt.shouldError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

func TestNodeToolsRegistered(t *testing.T) {
	agent := newTestAgentForRoute(nil)
	want := []string{
		"node_add", "node_remove", "node_update", "node_status",
		"node_ping", "node_groups", "node_labels", "node_import", "node_export",
	}
	for _, name := range want {
		if _, ok := agent.registry.Get(name); !ok {
			t.Errorf("tool %s not registered", name)
		}
	}
}

func TestNodeStatusFallback(t *testing.T) {
	mgr := &mockNodeMgrForAI{nodes: []*model.Node{
		{Name: "node1", Address: "127.0.0.1", Port: 22, Status: "online"},
		{Name: "node2", Address: "127.0.0.2", Port: 22, Status: "offline"},
	}}
	tool := NewNodeStatusTool(nil, mgr)

	out, err := tool.Execute(context.Background(), map[string]interface{}{"all": true})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(out, "node1") || !strings.Contains(out, "online") {
		t.Errorf("expected fallback output with node1/online, got %q", out)
	}

	out, err = tool.Execute(context.Background(), map[string]interface{}{"nodes": []interface{}{"node2"}})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if strings.Contains(out, "node1") || !strings.Contains(out, "node2") {
		t.Errorf("expected filtered output with node2 only, got %q", out)
	}
}

func TestNodeWriteOpsRequireConfirmation(t *testing.T) {
	sess := NewSession(newTestAgentForRoute(nil))

	for name := range confirmRequiredTools {
		if !strings.HasPrefix(name, "node_") {
			continue
		}
		call := ToolCall{Name: name, Arguments: map[string]interface{}{}}
		d := sess.agent.confirmGate(call)
		if !d.Confirm {
			t.Errorf("write op %s should require confirmation", name)
		}
	}
}
