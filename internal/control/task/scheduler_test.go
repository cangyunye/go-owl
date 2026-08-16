package task

import (
	"testing"
	"time"
)

func TestTask_Validate(t *testing.T) {
	tests := []struct {
		name    string
		task    *Task
		wantErr bool
	}{
		{
			name: "valid task",
			task: &Task{
				ID:      "task-1",
				Type:    TaskTypeCommand,
				Targets: []string{"node-1"},
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			task: &Task{
				Type:    TaskTypeCommand,
				Targets: []string{"node-1"},
			},
			wantErr: true,
		},
		{
			name: "missing type",
			task: &Task{
				ID:      "task-1",
				Targets: []string{"node-1"},
			},
			wantErr: true,
		},
		{
			name: "empty targets",
			task: &Task{
				ID:   "task-1",
				Type: TaskTypeCommand,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.task.Validate()
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestTask_SetStatus(t *testing.T) {
	task := &Task{ID: "task-1", Type: TaskTypeCommand, Targets: []string{"node-1"}, Results: make(map[string]*TaskResult)}

	task.SetStatus(TaskStatusRunning)
	if task.Status != TaskStatusRunning {
		t.Errorf("expected Status 'running', got '%s'", task.Status)
	}
	if task.StartedAt == nil {
		t.Error("StartedAt should be set when status is running")
	}

	task.SetStatus(TaskStatusCompleted)
	if task.Status != TaskStatusCompleted {
		t.Errorf("expected Status 'completed', got '%s'", task.Status)
	}
	if task.CompletedAt == nil {
		t.Error("CompletedAt should be set when status is completed")
	}
}

func TestTask_SetResult(t *testing.T) {
	task := &Task{ID: "task-1", Type: TaskTypeCommand, Targets: []string{"node-1", "node-2"}, Results: make(map[string]*TaskResult)}

	result := &TaskResult{
		NodeID:   "node-1",
		ExitCode: 0,
		Output:   "success",
	}

	task.SetResult("node-1", result)

	if len(task.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(task.Results))
	}

	if task.Results["node-1"].Output != "success" {
		t.Errorf("expected output 'success', got '%s'", task.Results["node-1"].Output)
	}
}

func TestTask_IsCompleted(t *testing.T) {
	task := &Task{ID: "task-1", Type: TaskTypeCommand, Targets: []string{"node-1"}, Results: make(map[string]*TaskResult)}

	if task.IsCompleted() {
		t.Error("expected IsCompleted() to be false for pending task")
	}

	task.SetStatus(TaskStatusRunning)
	if task.IsCompleted() {
		t.Error("expected IsCompleted() to be false for running task")
	}

	task.SetStatus(TaskStatusCompleted)
	if !task.IsCompleted() {
		t.Error("expected IsCompleted() to be true for completed task")
	}
}

func TestTask_Progress(t *testing.T) {
	task := &Task{ID: "task-1", Type: TaskTypeCommand, Targets: []string{"node-1", "node-2", "node-3"}, Results: make(map[string]*TaskResult)}

	progress := task.Progress()
	if progress != 0 {
		t.Errorf("expected progress 0, got %f", progress)
	}

	task.SetResult("node-1", &TaskResult{ExitCode: 0})
	progress = task.Progress()
	if progress != 1.0/3.0 {
		t.Errorf("expected progress %f, got %f", 1.0/3.0, progress)
	}

	task.SetResult("node-2", &TaskResult{ExitCode: 0})
	progress = task.Progress()
	if progress != 2.0/3.0 {
		t.Errorf("expected progress %f, got %f", 2.0/3.0, progress)
	}
}

func TestTask_SuccessFailureCount(t *testing.T) {
	task := &Task{ID: "task-1", Type: TaskTypeCommand, Targets: []string{"node-1", "node-2", "node-3"}, Results: make(map[string]*TaskResult)}

	task.SetResult("node-1", &TaskResult{ExitCode: 0})
	task.SetResult("node-2", &TaskResult{ExitCode: 1})
	task.SetResult("node-3", &TaskResult{ExitCode: 0})

	if task.SuccessCount() != 2 {
		t.Errorf("expected 2 successes, got %d", task.SuccessCount())
	}
	if task.FailureCount() != 1 {
		t.Errorf("expected 1 failure, got %d", task.FailureCount())
	}
}

func TestTask_Duration(t *testing.T) {
	task := &Task{ID: "task-1", Type: TaskTypeCommand, Targets: []string{"node-1"}, Results: make(map[string]*TaskResult)}

	duration := task.Duration()
	if duration != 0 {
		t.Errorf("expected duration 0 for pending task, got %v", duration)
	}

	now := time.Now()
	task.StartedAt = &now
	task.CompletedAt = &now

	duration = task.Duration()
	if duration != 0 {
		t.Errorf("expected duration 0 for completed task with same start/end time, got %v", duration)
	}
}

func TestCommandPayload(t *testing.T) {
	payload := &CommandPayload{
		Command: "ls -la",
		Timeout: 30 * time.Second,
		EnvVars: map[string]string{"PATH": "/usr/local/bin"},
		WorkDir: "/tmp",
	}

	if payload.Command != "ls -la" {
		t.Errorf("expected Command 'ls -la', got '%s'", payload.Command)
	}
	if payload.Timeout != 30*time.Second {
		t.Errorf("expected Timeout 30s, got %v", payload.Timeout)
	}
}

func TestScriptPayload(t *testing.T) {
	payload := &ScriptPayload{
		ScriptContent: "#!/bin/bash\necho hello",
		ScriptName:    "test.sh",
		Args:          []string{"arg1", "arg2"},
		Timeout:       60 * time.Second,
	}

	if payload.ScriptName != "test.sh" {
		t.Errorf("expected ScriptName 'test.sh', got '%s'", payload.ScriptName)
	}
	if len(payload.Args) != 2 {
		t.Errorf("expected 2 args, got %d", len(payload.Args))
	}
}

func TestFileTransferPayload(t *testing.T) {
	payload := &FileTransferPayload{
		SourcePath:      "/local/file.txt",
		DestinationPath: "/remote/file.txt",
		FileName:        "file.txt",
		FileSize:        1024,
		FileHash:        "abc123",
		Direction:       "upload",
	}

	if payload.FileSize != 1024 {
		t.Errorf("expected FileSize 1024, got %d", payload.FileSize)
	}
	if payload.Direction != "upload" {
		t.Errorf("expected Direction 'upload', got '%s'", payload.Direction)
	}
}
