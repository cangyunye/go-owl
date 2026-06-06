package playbook

import (
	"fmt"
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

func TestNewExecutor(t *testing.T) {
	executor := NewExecutor(nil, nil, nil, nil)
	if executor == nil {
		t.Fatal("expected executor to be created")
	}
}

func TestNewDefaultActionRunner(t *testing.T) {
	runner := NewDefaultActionRunner(nil, nil)
	if runner == nil {
		t.Fatal("expected runner to be created")
	}
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

func TestNewParallelExecutor(t *testing.T) {
	executor := NewParallelExecutor(nil, 5)
	if executor == nil {
		t.Fatal("expected parallel executor to be created")
	}
	if executor.maxParallelism != 5 {
		t.Errorf("expected maxParallelism 5, got %d", executor.maxParallelism)
	}
}

func TestNewParallelExecutor_DefaultParallelism(t *testing.T) {
	executor := NewParallelExecutor(nil, 0)
	if executor.maxParallelism != 10 {
		t.Errorf("expected default maxParallelism 10, got %d", executor.maxParallelism)
	}
}

func TestParallelExecutor_ExecuteAsync(t *testing.T) {
	mockExec := &MockExecutor{}
	executor := NewParallelExecutor(mockExec, 1)
	_, err := executor.ExecuteAsync(nil, nil, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
}

func TestParallelExecutor_GetExecution(t *testing.T) {
	mockExec := &MockExecutor{}
	executor := NewParallelExecutor(mockExec, 1)

	execID, _ := executor.ExecuteAsync(nil, nil, nil)

	time.Sleep(10 * time.Millisecond)
	exec, ok := executor.GetExecution(execID)
	if !ok {
		t.Error("expected execution to be found")
	}
	if exec == nil {
		t.Error("expected non-nil execution")
	}

	_, ok = executor.GetExecution("nonexistent")
	if ok {
		t.Error("expected no execution for nonexistent ID")
	}
}

func TestParallelExecutor_ListExecutions(t *testing.T) {
	mockExec := &MockExecutor{}
	executor := NewParallelExecutor(mockExec, 1)

	execs := executor.ListExecutions()
	if len(execs) != 0 {
		t.Errorf("expected 0 executions initially, got %d", len(execs))
	}

	executor.ExecuteAsync(nil, nil, nil)
	executor.ExecuteAsync(nil, nil, nil)

	time.Sleep(10 * time.Millisecond)
	execs = executor.ListExecutions()
	if len(execs) != 2 {
		t.Errorf("expected 2 executions, got %d", len(execs))
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
	executor := NewExecutor(nil, nil, nil, nil)
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
	executor := NewExecutor(mockNodeMgr, nil, nil, nil)

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
		runner := NewDefaultActionRunner(nil, nil)
		result, err := runner.RunAction("command", map[string]interface{}{"cmd": "echo hello"}, "node-1", nil, nil)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result.Action != "command" {
			t.Errorf("expected Action 'command', got '%s'", result.Action)
		}
	})

	t.Run("with command arg", func(t *testing.T) {
		runner := NewDefaultActionRunner(nil, nil)
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
	executor := NewExecutor(mockNodeMgr, nil, nil, nil)

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
	executor := NewExecutor(mockNodeMgr, mockCmd, nil, nil)

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
	executor := NewExecutor(mockNodeMgr, mockCmd, nil, nil)

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
