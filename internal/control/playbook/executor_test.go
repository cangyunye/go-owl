package playbook

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/internal/common/model"
	"github.com/cangyunye/go-owl/internal/control/node"
	"github.com/cangyunye/go-owl/internal/control/task"
)

type MockExecutor struct{}

func (m *MockExecutor) Execute(playbook *ParsedPlaybook, targets []*model.Node, extraVars map[string]interface{}) (*PlaybookExecution, error) {
	return &PlaybookExecution{
		ID:      fmt.Sprintf("mock-exec-%d", time.Now().UnixNano()),
		Status:  ExecutionStatusCompleted,
		Results: make(map[string][]*TaskResult),
		Vars:    make(map[string]interface{}),
	}, nil
}

func (m *MockExecutor) ExecuteTask(exec *PlaybookExecution, task *ParsedTask) ([]*TaskResult, error) {
	return nil, nil
}

func (m *MockExecutor) Stop(execID string) error {
	return nil
}

func TestPlaybookExecution(t *testing.T) {
	exec := &PlaybookExecution{
		ID:        "exec-1",
		Status:    ExecutionStatusRunning,
		Results:   make(map[string][]*TaskResult),
		Vars:      make(map[string]interface{}),
		StartTime: time.Now(),
	}

	if exec.ID != "exec-1" {
		t.Errorf("expected ID 'exec-1', got '%s'", exec.ID)
	}
	if exec.Status != ExecutionStatusRunning {
		t.Errorf("expected Status 'running', got '%s'", exec.Status)
	}
}

func TestTaskResult(t *testing.T) {
	result := &TaskResult{
		TaskName:  "test task",
		NodeID:    "node-1",
		Action:    "command",
		ExitCode:  0,
		Output:    "success",
		StartTime: time.Now(),
		EndTime:   time.Now(),
	}

	if result.TaskName != "test task" {
		t.Errorf("expected TaskName 'test task', got '%s'", result.TaskName)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected ExitCode 0, got %d", result.ExitCode)
	}
}

func TestPlaybookExecution_GetTaskResult(t *testing.T) {
	exec := &PlaybookExecution{
		Results: map[string][]*TaskResult{
			"task-1": {
				{TaskName: "task-1", ExitCode: 0},
				{TaskName: "task-1", ExitCode: 1},
			},
			"task-2": {
				{TaskName: "task-2", ExitCode: 0},
			},
		},
	}

	results := exec.GetTaskResult("task-1")
	if len(results) != 2 {
		t.Errorf("expected 2 results for task-1, got %d", len(results))
	}

	results = exec.GetTaskResult("nonexistent")
	if len(results) != 0 {
		t.Errorf("expected 0 results for nonexistent task, got %d", len(results))
	}
}

func TestPlaybookExecution_GetAllResults(t *testing.T) {
	exec := &PlaybookExecution{
		Results: map[string][]*TaskResult{
			"task-1": {
				{TaskName: "task-1"},
				{TaskName: "task-1"},
			},
			"task-2": {
				{TaskName: "task-2"},
			},
		},
	}

	all := exec.GetAllResults()
	if len(all) != 3 {
		t.Errorf("expected 3 results, got %d", len(all))
	}
}

func TestPlaybookExecution_SuccessCount(t *testing.T) {
	exec := &PlaybookExecution{
		Results: map[string][]*TaskResult{
			"task-1": {
				{ExitCode: 0},
				{ExitCode: 1},
				{ExitCode: 0},
			},
		},
	}

	count := exec.SuccessCount()
	if count != 2 {
		t.Errorf("expected 2 successes, got %d", count)
	}
}

func TestPlaybookExecution_FailureCount(t *testing.T) {
	exec := &PlaybookExecution{
		Results: map[string][]*TaskResult{
			"task-1": {
				{ExitCode: 0},
				{ExitCode: 1},
				{ExitCode: 1},
			},
		},
	}

	count := exec.FailureCount()
	if count != 2 {
		t.Errorf("expected 2 failures, got %d", count)
	}
}

func TestPlaybookExecution_Duration(t *testing.T) {
	now := time.Now()
	exec := &PlaybookExecution{
		StartTime: now,
		EndTime:   &now,
	}

	duration := exec.Duration()
	if duration != 0 {
		t.Errorf("expected duration 0, got %v", duration)
	}

	exec2 := &PlaybookExecution{
		StartTime: now,
	}

	duration2 := exec2.Duration()
	if duration2 < 0 {
		t.Errorf("expected non-negative duration, got %v", duration2)
	}
}

func TestExecutionStatus(t *testing.T) {
	statuses := []ExecutionStatus{
		ExecutionStatusPending,
		ExecutionStatusRunning,
		ExecutionStatusCompleted,
		ExecutionStatusFailed,
		ExecutionStatusAborted,
	}

	for _, status := range statuses {
		if status == "" {
			t.Error("expected non-empty status")
		}
	}
}

func TestTaskContext(t *testing.T) {
	ctx := &TaskContext{
		Execution: &PlaybookExecution{},
		Task: &ParsedTask{
			Name: "test task",
		},
		NodeID:            "node-1",
		Item:              "item1",
		Vars:              map[string]interface{}{"key": "value"},
		RegisteredResults: make(map[string]interface{}),
	}

	if ctx.Task.Name != "test task" {
		t.Errorf("expected Name 'test task', got '%s'", ctx.Task.Name)
	}
	if ctx.Item != "item1" {
		t.Errorf("expected Item 'item1', got '%v'", ctx.Item)
	}
}

func TestExecutor_Stop(t *testing.T) {
	executor := &playbookExecutor{}
	err := executor.Stop("exec-1")
	if err == nil {
		t.Error("expected error for stop (not implemented)")
	}
}

type MockNodeManager struct {
	nodes map[string]*model.Node
}

func (m *MockNodeManager) Register(n *model.Node) error {
	return nil
}

func (m *MockNodeManager) Unregister(id string) error {
	return nil
}

func (m *MockNodeManager) GetByID(id string) (*model.Node, error) {
	return nil, nil
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
	return nil
}

func (m *MockNodeManager) GetOnlineNodes() []*model.Node {
	return nil
}

func (m *MockNodeManager) Count() int {
	return 0
}

func TestExecutor_ExecuteTask(t *testing.T) {
	mockNodeMgr := &MockNodeManager{}
	executor := &playbookExecutor{nodeMgr: mockNodeMgr}

	exec := &PlaybookExecution{
		ID:      "exec-1",
		Vars:    make(map[string]interface{}),
		Results: make(map[string][]*TaskResult),
		TargetNodes: []*model.Node{
			{ID: "node-1"},
		},
	}

	task := &ParsedTask{
		Name:   "test task",
		Action: "debug",
		Args:   map[string]interface{}{"msg": "hello"},
	}

	results, err := executor.ExecuteTask(exec, task)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if results == nil {
		t.Error("expected non-nil results")
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestPlaybookExecutor_shouldContinueExecution(t *testing.T) {
	executor := &playbookExecutor{}

	exec := &PlaybookExecution{
		Status: ExecutionStatusRunning,
	}

	if !executor.shouldContinueExecution(exec) {
		t.Error("expected to continue when status is running")
	}

	exec.Status = ExecutionStatusAborted
	if executor.shouldContinueExecution(exec) {
		t.Error("expected not to continue when status is aborted")
	}
}

func TestPlaybookExecutor_executeTaskForNode(t *testing.T) {
	executor := &playbookExecutor{}

	exec := &PlaybookExecution{
		Vars: map[string]interface{}{
			"var1": "value1",
		},
	}

	task := &ParsedTask{
		Name:   "test",
		Action: "command",
		Args:   map[string]interface{}{"cmd": "echo test"},
	}

	results, err := executor.executeTaskForNode(exec, task, "node-1", nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestPlaybookExecutor_executeTaskInternal_WithCondition(t *testing.T) {
	executor := &playbookExecutor{}

	exec := &PlaybookExecution{
		TargetNodes: []*model.Node{
			{ID: "node-1"},
		},
		Vars: map[string]interface{}{
			"debug": false,
		},
		Results: make(map[string][]*TaskResult),
	}

	task := &ParsedTask{
		Name:   "conditional task",
		Action: "debug",
		Condition: &Condition{
			Expression: "debug == true",
		},
	}

	results, err := executor.executeTaskInternal(exec, task)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results (condition false), got %d", len(results))
	}
}

func TestPlaybookExecutor_executeTaskInternal_WithLoop(t *testing.T) {
	executor := &playbookExecutor{}

	exec := &PlaybookExecution{
		TargetNodes: []*model.Node{
			{ID: "node-1"},
		},
		Vars:    map[string]interface{}{},
		Results: make(map[string][]*TaskResult),
	}

	task := &ParsedTask{
		Name:   "loop task",
		Action: "debug",
		Loop: &Loop{
			Items:   []interface{}{"item1", "item2", "item3"},
			VarName: "item",
		},
	}

	results, err := executor.executeTaskInternal(exec, task)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results (3 loop items), got %d", len(results))
	}
}

func TestDefaultActionRunner_RunAction(t *testing.T) {
	t.Run("with cmd arg", func(t *testing.T) {
		runner := &defaultActionRunner{}
		result, err := runner.RunAction("command", map[string]interface{}{"cmd": "echo hello"}, "node-1", nil, nil)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result.Action != "command" {
			t.Errorf("expected Action 'command', got '%s'", result.Action)
		}
	})

	t.Run("with command arg", func(t *testing.T) {
		runner := &defaultActionRunner{}
		result, err := runner.RunAction("shell", map[string]interface{}{"command": "ls"}, "node-1", nil, nil)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result.Action != "shell" {
			t.Errorf("expected Action 'shell', got '%s'", result.Action)
		}
	})

}

func TestExecutor_Execute(t *testing.T) {
	mockNodeMgr := &MockNodeManager{}
	executor := NewExecutorWithOptions(mockNodeMgr, nil, nil, nil, nil)

	playbook := &ParsedPlaybook{
		Raw: &Playbook{
			Name:  "test playbook",
			Hosts: []string{"web"},
		},
		Tasks: []*ParsedTask{
			{
				Name:   "task 1",
				Action: "debug",
				Args:   map[string]interface{}{"msg": "hello"},
			},
		},
		Variables: make(map[string]interface{}),
	}

	exec, err := executor.Execute(playbook, []*model.Node{}, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if exec == nil {
		t.Fatal("expected non-nil execution")
	}
	if exec.Status != ExecutionStatusRunning && exec.Status != ExecutionStatusCompleted {
		t.Errorf("expected status running or completed, got '%s'", exec.Status)
	}
}

type mockCmdExecutor struct {
	callCount      int
	failOnCall     int
	executedCmds   []string
}

func (m *mockCmdExecutor) ExecuteOnNode(nodeID string, command string, timeout time.Duration) (*task.TaskResult, error) {
	m.callCount++
	m.executedCmds = append(m.executedCmds, command)
	if m.callCount >= m.failOnCall {
		return &task.TaskResult{ExitCode: 1, Output: "mock failure"}, fmt.Errorf("mock failure")
	}
	return &task.TaskResult{ExitCode: 0, Output: "ok"}, nil
}

func (m *mockCmdExecutor) Execute(tk *task.Task, nodeMgr node.Manager) error {
	return nil
}

func TestExecutor_Execute_PipelineModeFailsFast(t *testing.T) {
	mockNodeMgr := &MockNodeManager{}
	mockCmd := &mockCmdExecutor{
		failOnCall: 2, // 2nd task fails
	}
	executor := NewExecutorWithOptions(mockNodeMgr, mockCmd, nil, nil, nil)

	playbook := &ParsedPlaybook{
		Raw: &Playbook{
			Name:  "pipeline test",
			Hosts: []string{"web"},
		},
		ExecutionMode: ExecutionModePipeline,
		Variables:     make(map[string]interface{}),
		Tasks: []*ParsedTask{
			{
				Name:   "task 1",
				Action: "shell",
				Args:   map[string]interface{}{"cmd": "echo ok"},
				Options: TaskOptions{IgnoreErrors: false, AnyErrorsFatal: false},
			},
			{
				Name:   "task 2",
				Action: "shell",
				Args:   map[string]interface{}{"cmd": "fail"},
				Options: TaskOptions{IgnoreErrors: false, AnyErrorsFatal: false},
			},
			{
				Name:   "task 3",
				Action: "shell",
				Args:   map[string]interface{}{"cmd": "should not run"},
				Options: TaskOptions{IgnoreErrors: false, AnyErrorsFatal: false},
			},
		},
	}

	exec, _ := executor.Execute(playbook, []*model.Node{{ID: "node-1"}}, nil)
	if exec.Error == "" {
		t.Error("expected error for pipeline failure")
	}
	if exec.Status != ExecutionStatusFailed {
		t.Errorf("expected Status Failed, got '%s'", exec.Status)
	}
	if len(mockCmd.executedCmds) != 2 {
		t.Errorf("expected 2 commands executed (task3 skipped), got %d: %v", len(mockCmd.executedCmds), mockCmd.executedCmds)
	}
	_, task3Executed := exec.Results["task 3"]
	if task3Executed {
		t.Error("task 3 should not have been executed in pipeline mode")
	}
}

func TestExecutor_Execute_FailContinueRunsAll(t *testing.T) {
	mockNodeMgr := &MockNodeManager{}
	mockCmd := &mockCmdExecutor{
		failOnCall: 2,
	}
	executor := NewExecutorWithOptions(mockNodeMgr, mockCmd, nil, nil, nil)

	playbook := &ParsedPlaybook{
		Raw: &Playbook{
			Name:  "fail_continue test",
			Hosts: []string{"web"},
		},
		ExecutionMode: ExecutionModeFailContinue,
		Variables:     make(map[string]interface{}),
		Tasks: []*ParsedTask{
			{
				Name:   "task 1",
				Action: "shell",
				Args:   map[string]interface{}{"cmd": "ok"},
				Options: TaskOptions{IgnoreErrors: false, AnyErrorsFatal: false},
			},
			{
				Name:   "task 2",
				Action: "shell",
				Args:   map[string]interface{}{"cmd": "fail"},
				Options: TaskOptions{IgnoreErrors: false, AnyErrorsFatal: false},
			},
			{
				Name:   "task 3",
				Action: "shell",
				Args:   map[string]interface{}{"cmd": "should also run"},
				Options: TaskOptions{IgnoreErrors: false, AnyErrorsFatal: false},
			},
		},
	}

	exec, err := executor.Execute(playbook, []*model.Node{{ID: "node-1"}}, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(mockCmd.executedCmds) != 3 {
		t.Errorf("expected 3 commands executed (fail_continue), got %d: %v", len(mockCmd.executedCmds), mockCmd.executedCmds)
	}
	_, task3Executed := exec.Results["task 3"]
	if !task3Executed {
		t.Error("task 3 should have been executed in fail_continue mode")
	}
	if exec.Status != ExecutionStatusCompleted {
		t.Errorf("expected Status Completed (fail_continue swallows error), got '%s'", exec.Status)
	}
}

func TestExecutor_CheckMode_NoSideEffects(t *testing.T) {
	mockNodeMgr := &MockNodeManager{}
	mockCmd := &mockCmdExecutor{}
	opts := &PlaybookOptions{CheckMode: true}
	executor := NewExecutorWithOptions(mockNodeMgr, mockCmd, nil, nil, opts)

	playbook := &ParsedPlaybook{
		Raw:           &Playbook{Name: "check test", Hosts: []string{"web"}},
		ExecutionMode: ExecutionModeFailContinue,
		Variables:     make(map[string]interface{}),
		Tasks: []*ParsedTask{
			{Name: "task 1", Action: "command", Args: map[string]interface{}{"cmd": "echo hello"}},
			{Name: "task 2", Action: "command", Args: map[string]interface{}{"cmd": "echo world"}},
		},
	}

	exec, err := executor.Execute(playbook, []*model.Node{{ID: "node-1"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mockCmd.executedCmds) != 0 {
		t.Errorf("check mode should not execute commands, but got: %v", mockCmd.executedCmds)
	}

	for taskName, results := range exec.Results {
		for _, r := range results {
			if r.ExitCode != 0 {
				t.Errorf("check mode result for %s should have exit code 0, got %d", taskName, r.ExitCode)
			}
			if !strings.Contains(r.Output, "[check mode]") {
				t.Errorf("check mode output should contain '[check mode]', got: %s", r.Output)
			}
		}
	}

	if exec.Status != ExecutionStatusCompleted {
		t.Errorf("expected completed status, got %s", exec.Status)
	}
}

func TestExecutor_CheckMode_MultipleNodes(t *testing.T) {
	mockNodeMgr := &MockNodeManager{}
	mockCmd := &mockCmdExecutor{}
	opts := &PlaybookOptions{CheckMode: true}
	executor := NewExecutorWithOptions(mockNodeMgr, mockCmd, nil, nil, opts)

	playbook := &ParsedPlaybook{
		Raw:           &Playbook{Name: "multi-node check", Hosts: []string{"web"}},
		ExecutionMode: ExecutionModeFailContinue,
		Variables:     make(map[string]interface{}),
		Tasks: []*ParsedTask{
			{Name: "deploy", Action: "command", Args: map[string]interface{}{"cmd": "deploy.sh"}},
		},
	}

	nodes := []*model.Node{{ID: "node-1"}, {ID: "node-2"}, {ID: "node-3"}}
	exec, err := executor.Execute(playbook, nodes, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mockCmd.executedCmds) != 0 {
		t.Errorf("check mode executed %d commands", len(mockCmd.executedCmds))
	}

	results := exec.Results["deploy"]
	if len(results) != 3 {
		t.Errorf("expected 3 results (one per node), got %d", len(results))
	}
}

func TestExecutor_CheckMode_UploadAction(t *testing.T) {
	mockNodeMgr := &MockNodeManager{}
	mockCmd := &mockCmdExecutor{}
	opts := &PlaybookOptions{CheckMode: true}
	executor := NewExecutorWithOptions(mockNodeMgr, mockCmd, nil, nil, opts)

	playbook := &ParsedPlaybook{
		Raw:           &Playbook{Name: "upload check", Hosts: []string{"web"}},
		ExecutionMode: ExecutionModeFailContinue,
		Variables:     make(map[string]interface{}),
		Tasks: []*ParsedTask{
			{Name: "upload file", Action: "upload", Args: map[string]interface{}{"src": "/local/file", "dest": "/remote/file"}},
		},
	}

	exec, err := executor.Execute(playbook, []*model.Node{{ID: "node-1"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	results := exec.Results["upload file"]
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ExitCode != 0 {
		t.Errorf("check mode upload should succeed, got exit code %d", results[0].ExitCode)
	}
}

func TestExecutor_SuccessFailureCount_MultiNode(t *testing.T) {
	mockNodeMgr := &MockNodeManager{}
	mockCmd := &mockCmdExecutor{failOnCall: 100}
	executor := NewExecutorWithOptions(mockNodeMgr, mockCmd, nil, nil, nil)

	playbook := &ParsedPlaybook{
		Raw:           &Playbook{Name: "count test", Hosts: []string{"web"}},
		ExecutionMode: ExecutionModeFailContinue,
		Variables:     make(map[string]interface{}),
		Tasks: []*ParsedTask{
			{Name: "task 1", Action: "command", Args: map[string]interface{}{"cmd": "ok"}},
		},
	}

	nodes := []*model.Node{{ID: "node-1"}, {ID: "node-2"}, {ID: "node-3"}}
	exec, err := executor.Execute(playbook, nodes, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if exec.SuccessCount() != 3 {
		t.Errorf("expected SuccessCount 3 (1 task 脳 3 nodes), got %d", exec.SuccessCount())
	}
	t.Logf("SuccessCount=%d, FailureCount=%d for 1 task 脳 3 nodes", exec.SuccessCount(), exec.FailureCount())
}

func TestDefaultActionRunner_ResolveDownloadDest(t *testing.T) {
	dest := "/staging"

	t.Run("absolute dest stays absolute", func(t *testing.T) {
		runner := &defaultActionRunner{}
		runner.SetDownloadBaseDir(dest)
		got := runner.resolveDownloadDest("/opt/backup/app.log", nil)
		if got != "/opt/backup/app.log" {
			t.Errorf("expected absolute dest preserved, got %q", got)
		}
	})

	t.Run("relative dest falls into download base dir", func(t *testing.T) {
		runner := &defaultActionRunner{}
		runner.SetDownloadBaseDir(dest)
		got := runner.resolveDownloadDest("logs/app.log", nil)
		if got != filepath.Join(dest, "logs/app.log") {
			t.Errorf("expected %q, got %q", filepath.Join(dest, "logs/app.log"), got)
		}
	})

	t.Run("trailing slash dest keeps base dir semantics", func(t *testing.T) {
		runner := &defaultActionRunner{}
		runner.SetDownloadBaseDir(dest)
		got := runner.resolveDownloadDest("./", nil)
		if got != filepath.Join(dest, ".") {
			t.Errorf("expected %q, got %q", filepath.Join(dest, "."), got)
		}
	})

	t.Run("no download base dir falls back to playbook base dir", func(t *testing.T) {
		runner := &defaultActionRunner{}
		runner.SetPlaybookBaseDir("/pb")
		got := runner.resolveDownloadDest("logs/app.log", nil)
		if got != filepath.Join("/pb", "logs/app.log") {
			t.Errorf("expected %q, got %q", filepath.Join("/pb", "logs/app.log"), got)
		}
	})

	t.Run("no base dirs at all keeps dest as-is", func(t *testing.T) {
		runner := &defaultActionRunner{}
		got := runner.resolveDownloadDest("logs/app.log", nil)
		if got != "logs/app.log" {
			t.Errorf("expected %q, got %q", "logs/app.log", got)
		}
	})
}



