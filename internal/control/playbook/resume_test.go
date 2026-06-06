package playbook

import (
	"testing"

	"github.com/cangyunye/go-owl/internal/common/model"
)

func TestPlaybookExecutor_ResumeSkipsCompletedTasks(t *testing.T) {
	mockNodeMgr := &MockNodeManager{}
	mockCmd := &mockCmdExecutor{}
	executor := NewExecutor(mockNodeMgr, mockCmd, nil, nil)
	pbExecutor := executor.(*playbookExecutor)
	pbExecutor.SetResumeFrom("tasks", 2) // skip task 1 and task 2

	playbook := &ParsedPlaybook{
		Raw: &Playbook{
			Name:  "resume test",
			Hosts: []string{"web"},
		},
		ExecutionMode: ExecutionModeFailContinue,
		Variables:     make(map[string]interface{}),
		Tasks: []*ParsedTask{
			{
				Name:   "task 1",
				Action: "shell",
				Args:   map[string]interface{}{"cmd": "should be skipped 1"},
			},
			{
				Name:   "task 2",
				Action: "shell",
				Args:   map[string]interface{}{"cmd": "should be skipped 2"},
			},
			{
				Name:   "task 3",
				Action: "shell",
				Args:   map[string]interface{}{"cmd": "should run"},
			},
		},
	}

	exec, err := executor.Execute(playbook, []*model.Node{{ID: "node-1"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, task1Executed := exec.Results["task 1"]
	if task1Executed {
		t.Error("task 1 should have been skipped by resume")
	}
	_, task2Executed := exec.Results["task 2"]
	if task2Executed {
		t.Error("task 2 should have been skipped by resume")
	}
	_, task3Executed := exec.Results["task 3"]
	if !task3Executed {
		t.Error("task 3 should have been executed")
	}
	if len(mockCmd.executedCmds) != 1 {
		t.Errorf("expected 1 command executed, got %d: %v", len(mockCmd.executedCmds), mockCmd.executedCmds)
	}
}

func TestPlaybookExecutor_CheckpointCallback(t *testing.T) {
	mockNodeMgr := &MockNodeManager{}
	mockCmd := &mockCmdExecutor{
		failOnCall: 2,
	}
	executor := NewExecutor(mockNodeMgr, mockCmd, nil, nil)
	pbExecutor := executor.(*playbookExecutor)

	var capturedPhase string
	var capturedIndex int
	pbExecutor.SetCheckpointFunc(func(phase string, index int) {
		capturedPhase = phase
		capturedIndex = index
	})

	playbook := &ParsedPlaybook{
		Raw: &Playbook{
			Name:  "checkpoint test",
			Hosts: []string{"web"},
		},
		ExecutionMode: ExecutionModePipeline,
		Variables:     make(map[string]interface{}),
		Tasks: []*ParsedTask{
			{
				Name:   "task 1",
				Action: "shell",
				Args:   map[string]interface{}{"cmd": "ok"},
			},
			{
				Name:   "task 2",
				Action: "shell",
				Args:   map[string]interface{}{"cmd": "fail"},
			},
		},
	}

	executor.Execute(playbook, []*model.Node{{ID: "node-1"}}, nil)

	if capturedPhase != "tasks" {
		t.Errorf("expected checkpoint phase 'tasks', got '%s'", capturedPhase)
	}
	if capturedIndex != 1 {
		t.Errorf("expected checkpoint index 1 (task 2 failed), got %d", capturedIndex)
	}
}
