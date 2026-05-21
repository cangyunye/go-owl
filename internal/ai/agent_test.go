package ai

import (
	"context"
	"testing"

	"github.com/cangyunye/go-owl/internal/common/model"
)

type mockNodeMgr struct{}

func (m *mockNodeMgr) Register(node *model.Node) error {
	return nil
}

func (m *mockNodeMgr) Unregister(id string) error {
	return nil
}

func (m *mockNodeMgr) GetByID(id string) (*model.Node, error) {
	return &model.Node{
		Name:    id,
		Address: "127.0.0.1",
		Port:    22,
		Status:  "online",
	}, nil
}

func (m *mockNodeMgr) List() []*model.Node {
	return []*model.Node{{Name: "node1", Address: "127.0.0.1", Port: 22, Status: "online"}}
}

func (m *mockNodeMgr) GetByGroup(group string) []*model.Node {
	return []*model.Node{{Name: "node1", Address: "127.0.0.1", Port: 22, Status: "online"}}
}

func (m *mockNodeMgr) GetByLabels(labels map[string]string) []*model.Node {
	return nil
}

func (m *mockNodeMgr) UpdateStatus(id string, status model.NodeStatus) error {
	return nil
}

func (m *mockNodeMgr) GetOnlineNodes() []*model.Node {
	return nil
}

func (m *mockNodeMgr) Count() int {
	return 1
}

func (m *mockNodeMgr) GetAll() []*model.Node {
	return nil
}

func (m *mockNodeMgr) Refresh() error {
	return nil
}

func TestBuildToolCall(t *testing.T) {
	agent := &Agent{}

	params := map[string]interface{}{
		"key": "value",
		"num": 123,
	}

	result := agent.buildToolCall("test-tool", params)
	if result == "" {
		t.Fatal("Expected buildToolCall to return non-empty string")
	}
	if len(result) == 0 {
		t.Error("Expected result to have content")
	}
}

func TestExtractCommand(t *testing.T) {
	agent := &Agent{}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"uptime command", "Show me the uptime", "uptime"},
		{"disk space command", "Check disk space", "df -h"},
		{"memory command", "How is memory usage?", "free -m"},
		{"process command", "List running processes", "ps aux"},
		{"status command", "What is the status?", "uptime && df -h"},
		{"no matching keyword", "Some random text", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := agent.extractCommand(tt.input)
			if result != tt.expected {
				t.Errorf("Expected extractCommand to return '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestExtractFilePath(t *testing.T) {
	agent := &Agent{}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"absolute path", "Transfer /home/user/file.txt", "/home/user/file.txt"},
		{"tar file", "Backup archive.tar.gz", "archive.tar.gz"},
		{"zip file", "Upload package.zip", "package.zip"},
		{"no file path", "Just some text", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := agent.extractFilePath(tt.input)
			if result != tt.expected {
				t.Errorf("Expected extractFilePath to return '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestNewAgent(t *testing.T) {
	config := &Config{}
	agent, err := NewAgent(config, nil, nil)
	if err != nil {
		t.Fatalf("Expected NewAgent to succeed, got error: %v", err)
	}
	if agent == nil {
		t.Fatal("Expected agent to be non-nil")
	}
	if agent.registry == nil {
		t.Error("Expected registry to be initialized")
	}
}

func TestSessionManager(t *testing.T) {
	sm := NewSessionManager()
	if sm == nil {
		t.Fatal("Expected NewSessionManager to return non-nil")
	}
	if sm.sessions == nil {
		t.Fatal("Expected sessions map to be initialized")
	}

	sessions := sm.ListSessions()
	if len(sessions) != 0 {
		t.Errorf("Expected empty session list, got %d sessions", len(sessions))
	}
}

func TestExecuteToolCallWithInvalidParams(t *testing.T) {
	ctx := context.Background()
	registry := NewToolRegistry()
	registry.Register(NewExecuteCommandTool(&mockNodeMgr{}))
	agent := &Agent{registry: registry}

	invalidParams := map[string]interface{}{
		"targets": "not-an-array",
		"command": "",
	}

	_, err := agent.executeToolCall(ctx, ToolCall{
		Name:      "execute_command",
		Arguments: invalidParams,
	})

	if err == nil {
		t.Fatal("Expected executeToolCall to fail with invalid params")
	}
}

func TestExecuteToolCallWithMissingRequiredParams(t *testing.T) {
	ctx := context.Background()
	registry := NewToolRegistry()
	registry.Register(NewExecuteCommandTool(&mockNodeMgr{}))
	agent := &Agent{registry: registry}

	missingParams := map[string]interface{}{
		"targets": []interface{}{"node1"},
	}

	_, err := agent.executeToolCall(ctx, ToolCall{
		Name:      "execute_command",
		Arguments: missingParams,
	})

	if err == nil {
		t.Fatal("Expected executeToolCall to fail with missing required param 'command'")
	}
}

func TestExecuteToolCallWithValidParams(t *testing.T) {
	ctx := context.Background()
	registry := NewToolRegistry()
	registry.Register(NewExecuteCommandTool(&mockNodeMgr{}))
	agent := &Agent{registry: registry}

	validParams := map[string]interface{}{
		"targets": []interface{}{"node1"},
		"command": "echo hello",
	}

	_, err := agent.executeToolCall(ctx, ToolCall{
		Name:      "execute_command",
		Arguments: validParams,
	})

	if err != nil {
		t.Errorf("Expected executeToolCall to succeed with valid params, got error: %v", err)
	}
}

func TestExecuteToolCallWithUnknownTool(t *testing.T) {
	ctx := context.Background()
	agent := &Agent{}
	agent.registry = NewToolRegistry()

	_, err := agent.executeToolCall(ctx, ToolCall{
		Name:      "unknown_tool",
		Arguments: map[string]interface{}{},
	})

	if err == nil {
		t.Fatal("Expected executeToolCall to fail with unknown tool")
	}
}

func TestTransferFileValidation(t *testing.T) {
	ctx := context.Background()
	registry := NewToolRegistry()
	registry.Register(NewTransferFileTool(&mockNodeMgr{}))
	agent := &Agent{registry: registry}

	tests := []struct {
		name        string
		params      map[string]interface{}
		shouldError bool
	}{
		{
			name: "valid params",
			params: map[string]interface{}{
				"source_file": "/tmp/test.txt",
				"targets":     []interface{}{"node1"},
				"dest_dir":    "/tmp",
			},
			shouldError: false,
		},
		{
			name: "missing source file",
			params: map[string]interface{}{
				"targets":  []interface{}{"node1"},
				"dest_dir": "/tmp",
			},
			shouldError: true,
		},
		{
			name: "relative path for dest_dir",
			params: map[string]interface{}{
				"source_file": "/tmp/test.txt",
				"targets":     []interface{}{"node1"},
				"dest_dir":    "relative/path",
			},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := agent.executeToolCall(ctx, ToolCall{
				Name:      "transfer_file",
				Arguments: tt.params,
			})

			if tt.shouldError && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestQueryNodesValidation(t *testing.T) {
	ctx := context.Background()
	registry := NewToolRegistry()
	registry.Register(NewQueryNodesTool(&mockNodeMgr{}))
	agent := &Agent{registry: registry}

	tests := []struct {
		name        string
		params      map[string]interface{}
		shouldError bool
	}{
		{
			name:        "empty params",
			params:      map[string]interface{}{},
			shouldError: false,
		},
		{
			name: "valid group",
			params: map[string]interface{}{
				"group": "web",
			},
			shouldError: false,
		},
		{
			name: "invalid status",
			params: map[string]interface{}{
				"status": "invalid-status",
			},
			shouldError: true,
		},
		{
			name: "invalid format",
			params: map[string]interface{}{
				"format": "invalid-format",
			},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := agent.executeToolCall(ctx, ToolCall{
				Name:      "query_nodes",
				Arguments: tt.params,
			})

			if tt.shouldError && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}
