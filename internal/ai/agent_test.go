package ai

import (
	"context"
	"fmt"
	"os"
	"strings"
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
	return []*model.Node{
		{Name: "mac-mini-m4", Address: "192.168.1.100", Port: 22, Status: "online", Groups: []string{"test"}},
		{Name: "web-server-01", Address: "192.168.1.101", Port: 22, Status: "online", Groups: []string{"web"}},
		{Name: "db-server-01", Address: "192.168.1.102", Port: 22, Status: "offline", Groups: []string{"db"}},
	}
}

func (m *mockNodeMgr) GetByGroup(group string) []*model.Node {
	nodes := m.List()
	result := make([]*model.Node, 0)
	for _, n := range nodes {
		for _, g := range n.Groups {
			if g == group {
				result = append(result, n)
				break
			}
		}
	}
	return result
}

func (m *mockNodeMgr) GetByLabels(labels map[string]string) []*model.Node {
	return nil
}

func (m *mockNodeMgr) SearchByName(pattern string) []*model.Node {
	if pattern == "" {
		return nil
	}
	nodes := m.List()
	result := make([]*model.Node, 0)
	lowerPattern := strings.ToLower(pattern)
	for _, n := range nodes {
		if strings.Contains(strings.ToLower(n.Name), lowerPattern) {
			result = append(result, n)
		}
	}
	return result
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

func TestMain(m *testing.M) {
	DisableRealCommands = true
	os.Exit(m.Run())
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

func TestQueryDatabaseTool_StructuredFilter(t *testing.T) {
	mgr := &mockNodeMgr{}
	tool := NewQueryDatabaseTool(mgr)
	ctx := context.Background()

	t.Run("Group filter", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{"group": "test"})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "mac-mini-m4") {
			t.Errorf("expected result to contain 'mac-mini-m4', got: %s", result)
		}
	})

	t.Run("Search filter", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{"search": "web"})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "web-server-01") {
			t.Errorf("expected result to contain 'web-server-01', got: %s", result)
		}
	})

	t.Run("Status filter", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{"status": "offline"})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "db-server-01") {
			t.Errorf("expected result to contain 'db-server-01', got: %s", result)
		}
	})
}

func TestQueryDatabaseTool_SQLSelect(t *testing.T) {
	mgr := &mockNodeMgr{}
	tool := NewQueryDatabaseTool(mgr)
	ctx := context.Background()

	t.Run("SELECT all", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{"query": "SELECT * FROM nodes"})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "mac-mini-m4") {
			t.Errorf("expected result to contain 'mac-mini-m4', got: %s", result)
		}
	})

	t.Run("SELECT with LIKE", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{"query": "SELECT * FROM nodes WHERE name LIKE '%web%'"})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "web-server-01") {
			t.Errorf("expected result to contain 'web-server-01', got: %s", result)
		}
	})
}

func TestQueryDatabaseTool_RejectWrite(t *testing.T) {
	mgr := &mockNodeMgr{}
	tool := NewQueryDatabaseTool(mgr)
	ctx := context.Background()

	forbidden := []string{"INSERT INTO nodes", "UPDATE nodes SET", "DELETE FROM nodes", "DROP TABLE nodes", "ALTER TABLE nodes"}
	for _, q := range forbidden {
		_, err := tool.Execute(ctx, map[string]interface{}{"query": q})
		if err == nil {
			t.Errorf("expected error for forbidden query '%s', got nil", q)
		}
	}
}

func TestQueryDatabaseTool_Validate(t *testing.T) {
	tool := &QueryDatabaseTool{}

	if err := tool.Validate(map[string]interface{}{"query": "SELECT * FROM nodes"}); err != nil {
		t.Errorf("expected no error for SELECT, got: %v", err)
	}

	if err := tool.Validate(map[string]interface{}{"group": "test"}); err != nil {
		t.Errorf("expected no error for group filter, got: %v", err)
	}

	if err := tool.Validate(map[string]interface{}{}); err == nil {
		t.Error("expected error for empty params, got nil")
	}

	if err := tool.Validate(map[string]interface{}{"query": "DELETE FROM nodes"}); err == nil {
		t.Error("expected error for DELETE, got nil")
	}
}

func TestAffirmativeReplies(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"是", true},
		{"好的", true},
		{"yes", true},
		{"YES", true},
		{"ok", true},
		{"对", true},
		{"行", true},
		{"不是", false},
		{"no", false},
		{"不确定", false},
	}
	for _, tt := range tests {
		got := affirmativeReplies[strings.ToLower(tt.input)]
		if got != tt.expected {
			t.Errorf("affirmativeReplies[%q] = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestMaybeSetPendingContext(t *testing.T) {
	agent := &Agent{}
	session := NewSession(agent)

	t.Run("Sets pending on question", func(t *testing.T) {
		session.maybeSetPendingContext("是否需要我列出全部节点详情？")
		if session.pendingContext == nil {
			t.Error("expected pendingContext to be set")
		}
		if session.pendingContext.State != "awaiting_confirmation" {
			t.Errorf("expected 'awaiting_confirmation', got '%s'", session.pendingContext.State)
		}
		session.pendingContext = nil
	})

	t.Run("Does not set on statement", func(t *testing.T) {
		session.maybeSetPendingContext("查询到 1 个节点")
		if session.pendingContext != nil {
			t.Error("expected pendingContext to be nil for statement")
		}
	})

	t.Run("Sets on question mark with keyword", func(t *testing.T) {
		session.maybeSetPendingContext("需要我帮你查询吗？")
		if session.pendingContext == nil {
			t.Error("expected pendingContext to be set")
		}
		session.pendingContext = nil
	})

	t.Run("Does not set on general question", func(t *testing.T) {
		session.maybeSetPendingContext("当前负载多少？")
		if session.pendingContext != nil {
			t.Error("expected pendingContext to be nil for general question without keyword")
		}
	})
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
		"nodes": "not-an-array",
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
		"nodes": []interface{}{"node1"},
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
		"nodes": []interface{}{"node1"},
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
				"nodes":     []interface{}{"node1"},
				"dest_dir":    "/tmp",
			},
			shouldError: false,
		},
		{
			name: "missing source file",
			params: map[string]interface{}{
				"nodes":  []interface{}{"node1"},
				"dest_dir": "/tmp",
			},
			shouldError: true,
		},
		{
			name: "relative path for dest_dir",
			params: map[string]interface{}{
				"source_file": "/tmp/test.txt",
				"nodes":     []interface{}{"node1"},
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

type mockChatModel struct {
	responses []string
	callCount int
	returnErr error
}

func (m *mockChatModel) Generate(ctx context.Context, messages []Message) (string, error) {
	if m.returnErr != nil {
		return "", m.returnErr
	}
	if m.callCount >= len(m.responses) {
		return "", fmt.Errorf("mock: no more responses (callCount=%d, total=%d)", m.callCount, len(m.responses))
	}
	resp := m.responses[m.callCount]
	m.callCount++
	return resp, nil
}

type mockNodeMgrForAI struct {
	nodes []*model.Node
}

func (m *mockNodeMgrForAI) Register(node *model.Node) error { return nil }
func (m *mockNodeMgrForAI) Unregister(id string) error      { return nil }
func (m *mockNodeMgrForAI) GetByID(id string) (*model.Node, error) {
	for _, n := range m.nodes {
		if n.Name == id {
			return n, nil
		}
	}
	return nil, fmt.Errorf("node %s not found", id)
}
func (m *mockNodeMgrForAI) UpdateStatus(id string, status model.NodeStatus) error { return nil }
func (m *mockNodeMgrForAI) GetOnlineNodes() []*model.Node                         { return nil }
func (m *mockNodeMgrForAI) Count() int                                            { return len(m.nodes) }
func (m *mockNodeMgrForAI) GetByLabels(labels map[string]string) []*model.Node    { return nil }
func (m *mockNodeMgrForAI) SearchByName(pattern string) []*model.Node {
	if pattern == "" {
		return nil
	}
	result := make([]*model.Node, 0)
	lowerPattern := strings.ToLower(pattern)
	for _, n := range m.nodes {
		if strings.Contains(strings.ToLower(n.Name), lowerPattern) {
			result = append(result, n)
		}
	}
	return result
}

func (m *mockNodeMgrForAI) List() []*model.Node { return m.nodes }

func (m *mockNodeMgrForAI) GetByGroup(group string) []*model.Node {
	var result []*model.Node
	for _, n := range m.nodes {
		for _, g := range n.Groups {
			if g == group {
				result = append(result, n)
			}
		}
	}
	return result
}

func newTestAgentForRoute(responses []string) *Agent {
	config := &Config{}
	mgr := &mockNodeMgrForAI{
		nodes: []*model.Node{
			{Name: "node1", Address: "127.0.0.1", Port: 22, Status: "online"},
		},
	}
	agent, _ := NewAgent(config, mgr, nil)
	agent.SetChatModel(&mockChatModel{responses: responses})
	return agent
}

func TestProcessRouteExec(t *testing.T) {
	agent := newTestAgentForRoute([]string{"exec", "```json\n" + `{"tool_calls":[{"name":"execute_command","arguments":{"command":"uptime","nodes":["node1"]}}]}` + "\n```", ""})
	ctx := context.Background()
	resp, err := agent.Process(ctx, "execute uptime on node1", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if resp == "" {
		t.Error("expected non-empty response")
	}
	if resp == "我不确定您要做什么" {
		t.Error("should route to exec, not reject")
	}
}

func TestProcessRouteNode(t *testing.T) {
	agent := newTestAgentForRoute([]string{"node",
		"```json\n{\"tool_calls\":[{\"name\":\"query_nodes\",\"arguments\":{}}]}\n```",
		"",
	})
	ctx := context.Background()
	resp, err := agent.Process(ctx, "list all nodes", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if resp == "我不确定您要做什么" {
		t.Error("should route to node, not reject")
	}
}

func TestProcessRouteFile(t *testing.T) {
	agent := newTestAgentForRoute([]string{"file",
		"```json\n{\"tool_calls\":[{\"name\":\"transfer_file\",\"arguments\":{\"source_file\":\"/tmp/test.txt\",\"nodes\":[\"node1\"],\"dest_dir\":\"/tmp\"}}]}\n```",
		"",
	})
	ctx := context.Background()
	resp, err := agent.Process(ctx, "upload test.txt to node1", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if resp == "我不确定您要做什么" {
		t.Error("should route to file, not reject")
	}
}

func TestProcessRoutePlaybook(t *testing.T) {
	agent := newTestAgentForRoute([]string{"playbook",
		"```json\n{\"tool_calls\":[{\"name\":\"generate_playbook\",\"arguments\":{\"requirement\":\"install nginx\"}}]}\n```",
		"",
	})
	ctx := context.Background()
	resp, err := agent.Process(ctx, "install nginx on web nodes", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if resp == "我不确定您要做什么" {
		t.Error("should route to playbook, not reject")
	}
}

func TestProcessRouteUncertain(t *testing.T) {
	agent := newTestAgentForRoute([]string{"uncertain"})
	ctx := context.Background()
	resp, err := agent.Process(ctx, "random gibberish", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if resp != "我不确定您要做什么" {
		t.Errorf("expected rejection, got: %s", resp)
	}
}

func TestProcessRouteEmpty(t *testing.T) {
	agent := newTestAgentForRoute([]string{""})
	ctx := context.Background()
	resp, err := agent.Process(ctx, "", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if resp != "我不确定您要做什么" {
		t.Errorf("expected rejection for empty route, got: %s", resp)
	}
}

func TestProcessRouteWithMarkdownCleanup(t *testing.T) {
	agent := newTestAgentForRoute([]string{"```exec```",
		"```json\n{\"tool_calls\":[{\"name\":\"execute_command\",\"arguments\":{\"command\":\"uptime\",\"nodes\":[\"node1\"]}}]}\n```",
		"",
	})
	ctx := context.Background()
	resp, err := agent.Process(ctx, "execute uptime", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if resp == "我不确定您要做什么" {
		t.Error("markdown cleanup should strip ``` from 'exec'")
	}
}

func TestProcessRouteWithPeriodCleanup(t *testing.T) {
	agent := newTestAgentForRoute([]string{"exec.",
		"```json\n{\"tool_calls\":[{\"name\":\"execute_command\",\"arguments\":{\"command\":\"uptime\",\"nodes\":[\"node1\"]}}]}\n```",
		"",
	})
	ctx := context.Background()
	resp, err := agent.Process(ctx, "execute uptime", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if resp == "我不确定您要做什么" {
		t.Error("period cleanup should strip '.' from 'exec.'")
	}
}

func TestProcessRouteFuzzyMatch(t *testing.T) {
	agent := newTestAgentForRoute([]string{"execute",
		"```json\n{\"tool_calls\":[{\"name\":\"execute_command\",\"arguments\":{\"command\":\"uptime\",\"nodes\":[\"node1\"]}}]}\n```",
		"",
	})
	ctx := context.Background()
	resp, err := agent.Process(ctx, "execute something", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if resp == "我不确定您要做什么" {
		t.Error("fuzzy match should match 'execute' to exec group")
	}
}

func TestProcessRouterError(t *testing.T) {
	mock := &mockChatModel{returnErr: fmt.Errorf("API unavailable")}
	config := &Config{}
	mgr := &mockNodeMgrForAI{
		nodes: []*model.Node{
			{Name: "node1", Address: "127.0.0.1", Port: 22, Status: "online"},
		},
	}
	agent, _ := NewAgent(config, mgr, nil)
	agent.SetChatModel(mock)

	ctx := context.Background()
	_, err := agent.Process(ctx, "hello", nil)
	if err == nil {
		t.Fatal("expected error from router failure")
	}
	if !strings.Contains(err.Error(), "路由失败") {
		t.Errorf("error should contain '路由失败', got: %v", err)
	}
}

func TestParseToolCallsValid(t *testing.T) {
	agent := &Agent{}
	response := "```json\n{\"tool_calls\":[{\"name\":\"execute_command\",\"arguments\":{\"command\":\"uptime\",\"nodes\":[\"node1\"]}}]}\n```"
	calls := agent.parseToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(calls))
	}
	if calls[0].Name != "execute_command" {
		t.Errorf("expected 'execute_command', got '%s'", calls[0].Name)
	}
	args := calls[0].Arguments
	if args["command"] != "uptime" {
		t.Errorf("expected command='uptime', got '%v'", args["command"])
	}
}

func TestParseToolCallsMultiple(t *testing.T) {
	agent := &Agent{}
	response := "Here is my response:\n```json\n{\"tool_calls\":[{\"name\":\"execute_command\",\"arguments\":{\"command\":\"uptime\"}},{\"name\":\"query_nodes\",\"arguments\":{\"status\":\"online\"}}]}\n```"
	calls := agent.parseToolCalls(response)
	if len(calls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(calls))
	}
	if calls[0].Name != "execute_command" || calls[1].Name != "query_nodes" {
		t.Errorf("unexpected tool names: %s, %s", calls[0].Name, calls[1].Name)
	}
}

func TestParseToolCallsInvalidJSON(t *testing.T) {
	agent := &Agent{}
	response := "some random text without json block"
	calls := agent.parseToolCalls(response)
	if len(calls) != 0 {
		t.Errorf("expected 0 tool calls for invalid input, got %d", len(calls))
	}
}

func TestParseToolCallsMissingToolCallsField(t *testing.T) {
	agent := &Agent{}
	response := "```json\n{\"foo\":\"bar\"}\n```"
	calls := agent.parseToolCalls(response)
	if len(calls) != 0 {
		t.Errorf("expected 0 tool calls for missing field, got %d", len(calls))
	}
}

func TestParseToolCallsIncompleteJSON(t *testing.T) {
	agent := &Agent{}
	response := "```json\n{\"tool_calls\":[{\"name\":\"execute_command\""
	calls := agent.parseToolCalls(response)
	if len(calls) != 0 {
		t.Errorf("expected 0 tool calls for incomplete JSON, got %d", len(calls))
	}
}

func TestParseToolCallsBareJSON(t *testing.T) {
	agent := &Agent{}
	response := `{"tool_calls":[{"name":"query_nodes","arguments":{}}]}`
	calls := agent.parseToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call for bare JSON, got %d", len(calls))
	}
	if calls[0].Name != "query_nodes" {
		t.Errorf("expected query_nodes, got %s", calls[0].Name)
	}
}

func TestParseToolCallsBareJSONWithText(t *testing.T) {
	agent := &Agent{}
	response := "Here is the result:\n{\"tool_calls\":[{\"name\":\"execute_command\",\"arguments\":{\"command\":\"uptime\"}}]}\nDone."
	calls := agent.parseToolCalls(response)
	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call for bare JSON with text, got %d", len(calls))
	}
	if calls[0].Name != "execute_command" {
		t.Errorf("expected execute_command, got %s", calls[0].Name)
	}
}

func TestDynamicHintInjectionExecuteCommand(t *testing.T) {
	agent := newTestAgentForRoute([]string{
		"exec",
		"```json\n{\"tool_calls\":[{\"name\":\"execute_command\",\"arguments\":{\"command\":\"uptime\",\"nodes\":[\"node1\"]}}]}\n```",
		"",
	})
	ctx := context.Background()
	_, err := agent.Process(ctx, "execute uptime on node1", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
}

func TestDynamicHintInjectionExecuteScript(t *testing.T) {
	agent := newTestAgentForRoute([]string{
		"exec",
		"```json\n{\"tool_calls\":[{\"name\":\"execute_script\",\"arguments\":{\"script\":\"./test.sh\",\"nodes\":[\"node1\"]}}]}\n```",
		"",
	})
	ctx := context.Background()
	_, err := agent.Process(ctx, "execute script test.sh on node1", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
}

func TestDynamicHintInjectionPlaybook(t *testing.T) {
	agent := newTestAgentForRoute([]string{
		"playbook",
		"```json\n{\"tool_calls\":[{\"name\":\"generate_playbook\",\"arguments\":{\"requirement\":\"install nginx\"}}]}\n```",
		"",
	})
	ctx := context.Background()
	_, err := agent.Process(ctx, "install nginx on web", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
}

func TestDynamicHintInjectionTransferFile(t *testing.T) {
	agent := newTestAgentForRoute([]string{
		"file",
		"```json\n{\"tool_calls\":[{\"name\":\"transfer_file\",\"arguments\":{\"source_file\":\"/tmp/test.txt\",\"nodes\":[\"node1\"],\"dest_dir\":\"/tmp\"}}]}\n```",
		"",
	})
	ctx := context.Background()
	_, err := agent.Process(ctx, "upload test.txt to node1", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
}

func TestDynamicHintNoInjectionForQueryNodes(t *testing.T) {
	agent := newTestAgentForRoute([]string{
		"node",
		"```json\n{\"tool_calls\":[{\"name\":\"query_nodes\",\"arguments\":{}}]}\n```",
		"",
	})
	ctx := context.Background()
	_, err := agent.Process(ctx, "list all nodes", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
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
