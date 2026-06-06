package command

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/internal/common/model"
	"github.com/cangyunye/go-owl/internal/control/task"
)

type MockNodeExecutor struct {
	Results map[string]struct {
		ExitCode int
		Output   string
		Err      error
	}
}

func (m *MockNodeExecutor) Execute(command string, timeout time.Duration) (int, string, error) {
	return 0, "", nil
}

type MockNodeManager struct {
	nodes map[string]*model.Node
}

func NewMockNodeManager() *MockNodeManager {
	return &MockNodeManager{
		nodes: make(map[string]*model.Node),
	}
}

func (m *MockNodeManager) AddNode(node *model.Node) {
	m.nodes[node.ID] = node
}

func (m *MockNodeManager) Register(n *model.Node) error {
	if _, exists := m.nodes[n.ID]; exists {
		return fmt.Errorf("node already exists")
	}
	n.SetStatus(model.NodeStatusOnline)
	m.nodes[n.ID] = n
	return nil
}

func (m *MockNodeManager) Unregister(id string) error {
	if _, exists := m.nodes[id]; !exists {
		return fmt.Errorf("node not found")
	}
	delete(m.nodes, id)
	return nil
}

func (m *MockNodeManager) GetByID(id string) (*model.Node, error) {
	n, ok := m.nodes[id]
	if !ok {
		return nil, fmt.Errorf("node not found")
	}
	return n, nil
}

func (m *MockNodeManager) List() []*model.Node {
	return nil
}

func (m *MockNodeManager) GetByGroup(group string) []*model.Node {
	return nil
}

func (m *MockNodeManager) GetByLabels(labels map[string]string) []*model.Node {
	return nil
}

func (m *MockNodeManager) SearchByName(pattern string) []*model.Node {
	return nil
}

func (m *MockNodeManager) SearchByAddress(pattern string) []*model.Node {
	return nil
}

func (m *MockNodeManager) UpdateStatus(id string, status model.NodeStatus) error {
	n, ok := m.nodes[id]
	if !ok {
		return fmt.Errorf("node not found")
	}
	n.SetStatus(status)
	return nil
}

func (m *MockNodeManager) GetOnlineNodes() []*model.Node {
	return nil
}

func (m *MockNodeManager) Count() int {
	return len(m.nodes)
}

func TestLocalNodeExecutor_Execute(t *testing.T) {
	executor := &LocalNodeExecutor{}

	t.Run("successful command", func(t *testing.T) {
		exitCode, output, err := executor.Execute("echo hello", 5*time.Second)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if exitCode != 0 {
			t.Errorf("expected exit code 0, got %d", exitCode)
		}
		if !strings.Contains(output, "hello") {
			t.Errorf("expected output to contain 'hello', got '%s'", output)
		}
	})

	t.Run("command with error", func(t *testing.T) {
		exitCode, _, err := executor.Execute("ls /nonexistent", 5*time.Second)
		if err != nil {
			t.Error("unexpected error for ls command")
		}
		if exitCode == 0 {
			t.Error("expected non-zero exit code for failed command")
		}
	})

	t.Run("command with timeout", func(t *testing.T) {
		_, _, err := executor.Execute("sleep 10", 100*time.Millisecond)
		if err == nil {
			t.Error("expected timeout error")
		}
		if !strings.Contains(err.Error(), "timed out") {
			t.Errorf("expected timeout error message, got '%v'", err)
		}
	})

	t.Run("pipeline command", func(t *testing.T) {
		exitCode, output, err := executor.Execute("echo 'line1\nline2' | grep line1", 5*time.Second)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if exitCode != 0 {
			t.Errorf("expected exit code 0, got %d", exitCode)
		}
		if !strings.Contains(output, "line1") {
			t.Errorf("expected output to contain 'line1', got '%s'", output)
		}
	})
}

func TestNewCommandExecutor(t *testing.T) {
	nodeMgr := NewMockNodeManager()
	exec := &LocalNodeExecutor{}

	executor := NewCommandExecutor(nodeMgr, exec)
	if executor == nil {
		t.Fatal("expected executor, got nil")
	}

	executorNil := NewCommandExecutor(nodeMgr, nil)
	if executorNil == nil {
		t.Fatal("expected executor with nil exec, got nil")
	}
}

func TestCommandExecutor_Execute(t *testing.T) {
	nodeMgr := NewMockNodeManager()
	nodeMgr.AddNode(model.NewNode("node-1", "Test Node 1", "127.0.0.1", 8080, "root"))
	nodeMgr.UpdateStatus("node-1", model.NodeStatusOnline)
	nodeMgr.AddNode(model.NewNode("node-2", "Test Node 2", "127.0.0.2", 8080, "root"))
	nodeMgr.UpdateStatus("node-2", model.NodeStatusOnline)

	mockExec := &MockNodeExecutor{}
	executor := NewCommandExecutor(nodeMgr, mockExec)

	payload := &task.CommandPayload{
		Command: "echo test",
		Timeout: 5 * time.Second,
	}
	tk := task.NewTask("task-1", task.TaskTypeCommand, []string{"node-1", "node-2"}, payload)

	err := executor.Execute(tk, nodeMgr)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(tk.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(tk.Results))
	}
}

func TestCommandExecutor_ExecuteWithInvalidNode(t *testing.T) {
	nodeMgr := NewMockNodeManager()
	nodeMgr.AddNode(model.NewNode("node-1", "Test Node 1", "127.0.0.1", 8080, "root"))
	nodeMgr.UpdateStatus("node-1", model.NodeStatusOnline)

	mockExec := &MockNodeExecutor{}
	executor := NewCommandExecutor(nodeMgr, mockExec)

	payload := &task.CommandPayload{
		Command: "echo test",
		Timeout: 5 * time.Second,
	}
	tk := task.NewTask("task-1", task.TaskTypeCommand, []string{"node-1", "nonexistent"}, payload)

	err := executor.Execute(tk, nodeMgr)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(tk.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(tk.Results))
	}
}

func TestCommandExecutor_ExecuteWithOfflineNode(t *testing.T) {
	nodeMgr := NewMockNodeManager()
	nodeMgr.AddNode(model.NewNode("node-1", "Test Node 1", "127.0.0.1", 8080, "root"))
	nodeMgr.UpdateStatus("node-1", model.NodeStatusOnline)
	nodeMgr.AddNode(model.NewNode("node-2", "Test Node 2", "127.0.0.2", 8080, "root"))
	nodeMgr.UpdateStatus("node-2", model.NodeStatusOffline)

	mockExec := &MockNodeExecutor{}
	executor := NewCommandExecutor(nodeMgr, mockExec)

	payload := &task.CommandPayload{
		Command: "echo test",
		Timeout: 5 * time.Second,
	}
	tk := task.NewTask("task-1", task.TaskTypeCommand, []string{"node-1", "node-2"}, payload)

	err := executor.Execute(tk, nodeMgr)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if tk.Results["node-2"].ExitCode != -1 {
		t.Errorf("expected exit code -1 for offline node, got %d", tk.Results["node-2"].ExitCode)
	}
}

func TestCommandExecutor_ExecuteOnNode(t *testing.T) {
	nodeMgr := NewMockNodeManager()
	nodeMgr.AddNode(model.NewNode("node-1", "Test Node 1", "127.0.0.1", 8080, "root"))
	nodeMgr.UpdateStatus("node-1", model.NodeStatusOnline)

	executor := NewCommandExecutor(nodeMgr, nil)

	result, err := executor.ExecuteOnNode("node-1", "echo hello", 5*time.Second)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
}

func TestCommandExecutor_ExecuteOnNode_NotFound(t *testing.T) {
	nodeMgr := NewMockNodeManager()
	executor := NewCommandExecutor(nodeMgr, nil)

	_, err := executor.ExecuteOnNode("nonexistent", "echo hello", 5*time.Second)
	if err == nil {
		t.Error("expected error for nonexistent node")
	}
}

func TestCommandExecutor_ExecuteOnNode_Offline(t *testing.T) {
	nodeMgr := NewMockNodeManager()
	nodeMgr.AddNode(model.NewNode("node-1", "Test Node 1", "127.0.0.1", 8080, "root"))

	executor := NewCommandExecutor(nodeMgr, nil)

	_, err := executor.ExecuteOnNode("node-1", "echo hello", 5*time.Second)
	if err == nil {
		t.Error("expected error for offline node")
	}
}

func TestCommandBuilder(t *testing.T) {
	t.Run("basic append", func(t *testing.T) {
		cmd := NewCommandBuilder().
			Append("echo").
			Append("hello").
			String()
		if cmd != "echohello" {
			t.Errorf("expected 'echohello', got '%s'", cmd)
		}
	})

	t.Run("appendf", func(t *testing.T) {
		cmd := NewCommandBuilder().
			Append("echo").
			Appendf(" %s", "world").
			String()
		if cmd != "echo world" {
			t.Errorf("expected 'echo world', got '%s'", cmd)
		}
	})
}

func TestParseCommandArgs(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{
			name:  "simple command",
			input: "ls -la",
			want:  []string{"ls", "-la"},
		},
		{
			name:  "command with double quotes",
			input: `echo "hello world"`,
			want:  []string{"echo", "hello world"},
		},
		{
			name:  "command with single quotes",
			input: "echo 'hello world'",
			want:  []string{"echo", "hello world"},
		},
		{
			name:  "command with mixed quotes",
			input: `echo "hello" 'world'`,
			want:  []string{"echo", "hello", "world"},
		},
		{
			name:    "unclosed double quote",
			input:   `echo "hello`,
			wantErr: true,
		},
		{
			name:    "unclosed single quote",
			input:   "echo 'hello",
			wantErr: true,
		},
		{
			name:  "empty string",
			input: "",
			want:  nil,
		},
		{
			name:  "single word",
			input: "ls",
			want:  []string{"ls"},
		},
		{
			name:  "multiple spaces",
			input: "ls    -la",
			want:  []string{"ls", "-la"},
		},
		{
			name:  "quoted with spaces inside",
			input: `ls -la "/path/with spaces"`,
			want:  []string{"ls", "-la", "/path/with spaces"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCommandArgs(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("expected %d args, got %d: %v", len(tt.want), len(got), got)
				return
			}
			for i, arg := range tt.want {
				if got[i] != arg {
					t.Errorf("arg[%d]: expected '%s', got '%s'", i, arg, got[i])
				}
			}
		})
	}
}

func TestExecutionResult(t *testing.T) {
	result := &ExecutionResult{
		NodeID:   "node-1",
		ExitCode: 0,
		Output:   "success",
	}

	if result.NodeID != "node-1" {
		t.Errorf("expected NodeID 'node-1', got '%s'", result.NodeID)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected ExitCode 0, got %d", result.ExitCode)
	}
	if result.Output != "success" {
		t.Errorf("expected Output 'success', got '%s'", result.Output)
	}
}
