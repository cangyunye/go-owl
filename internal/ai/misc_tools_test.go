package ai

import (
	"strings"
	"testing"
)

func TestMiscToolsValidate(t *testing.T) {
	tests := []struct {
		name        string
		tool        Tool
		params      map[string]interface{}
		shouldError bool
	}{
		{"async_status ok", NewAsyncStatusTool(nil), map[string]interface{}{"task_id": "t1"}, false},
		{"async_status missing id", NewAsyncStatusTool(nil), map[string]interface{}{}, true},
		{"async_cancel ok", NewAsyncCancelTool(nil), map[string]interface{}{"task_id": "t1"}, false},
		{"async_cancel missing id", NewAsyncCancelTool(nil), map[string]interface{}{}, true},
		{"async_list ok", NewAsyncListTool(nil), map[string]interface{}{}, false},
		{"settings_show ok", NewSettingsShowTool(nil), map[string]interface{}{}, false},
		{"settings_set ok", NewSettingsSetTool(nil), map[string]interface{}{"key": "ssh.timeout", "value": "30"}, false},
		{"settings_set missing key", NewSettingsSetTool(nil), map[string]interface{}{"value": "30"}, true},
		{"history_list ok", NewHistoryListTool(nil), map[string]interface{}{"limit": 10}, false},
		{"history_clean ok", NewHistoryCleanTool(nil), map[string]interface{}{"days": 30}, false},
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

func TestMiscToolsRegistered(t *testing.T) {
	agent := newTestAgentForRoute(nil)
	want := []string{
		"async_list", "async_status", "async_cancel",
		"settings_show", "settings_set",
		"history_list", "history_clean",
	}
	for _, name := range want {
		if _, ok := agent.registry.Get(name); !ok {
			t.Errorf("tool %s not registered", name)
		}
	}
}

func TestMiscWriteOpsRequireConfirmation(t *testing.T) {
	sess := NewSession(newTestAgentForRoute(nil))
	for _, name := range []string{"async_cancel", "settings_set", "history_clean"} {
		call := ToolCall{Name: name, Arguments: map[string]interface{}{}}
		d := sess.agent.confirmGate(call)
		if !d.Confirm {
			t.Errorf("write op %s should require confirmation", name)
		}
	}
	for _, name := range []string{"async_list", "async_status", "settings_show", "history_list"} {
		call := ToolCall{Name: name, Arguments: map[string]interface{}{}}
		if d := sess.agent.confirmGate(call); d.Confirm {
			t.Errorf("read op %s should not require confirmation", name)
		}
	}
}

func TestSummarizeToolCallWithNodeArgs(t *testing.T) {
	s := SummarizeToolCall(ToolCall{
		Name: "execute_command",
		Arguments: map[string]interface{}{
			"command": "systemctl restart nginx",
			"nodes":   []interface{}{"web-01", "web-02"},
		},
	})
	if !strings.Contains(s, "command=systemctl restart nginx") {
		t.Errorf("expected command in summary, got %q", s)
	}
	if !strings.Contains(s, "web-01") {
		t.Errorf("expected node in summary, got %q", s)
	}
}
