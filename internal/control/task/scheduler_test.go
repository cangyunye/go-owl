package task

import (
	"testing"
	"time"
)

func TestNewTask(t *testing.T) {
	payload := &CommandPayload{Command: "ls -la"}
	task := NewTask("task-1", TaskTypeCommand, []string{"node-1", "node-2"}, payload)

	if task.ID != "task-1" {
		t.Errorf("expected ID 'task-1', got '%s'", task.ID)
	}
	if task.Type != TaskTypeCommand {
		t.Errorf("expected Type 'command', got '%s'", task.Type)
	}
	if len(task.Targets) != 2 {
		t.Errorf("expected 2 targets, got %d", len(task.Targets))
	}
	if task.Status != TaskStatusPending {
		t.Errorf("expected Status 'pending', got '%s'", task.Status)
	}
	if len(task.Results) != 0 {
		t.Errorf("expected empty Results, got %d", len(task.Results))
	}
}

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
	task := NewTask("task-1", TaskTypeCommand, []string{"node-1"}, nil)

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
	task := NewTask("task-1", TaskTypeCommand, []string{"node-1", "node-2"}, nil)

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
	task := NewTask("task-1", TaskTypeCommand, []string{"node-1"}, nil)

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
	task := NewTask("task-1", TaskTypeCommand, []string{"node-1", "node-2", "node-3"}, nil)

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
	task := NewTask("task-1", TaskTypeCommand, []string{"node-1", "node-2", "node-3"}, nil)

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
	task := NewTask("task-1", TaskTypeCommand, []string{"node-1"}, nil)

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

func TestInMemoryTaskStore(t *testing.T) {
	store := NewInMemoryTaskStore()

	task := NewTask("task-1", TaskTypeCommand, []string{"node-1"}, nil)
	store.Set("task-1", task)

	retrieved, ok := store.Get("task-1")
	if !ok {
		t.Error("expected task to be found")
	}
	if retrieved.ID != task.ID {
		t.Errorf("expected ID '%s', got '%s'", task.ID, retrieved.ID)
	}

	all := store.GetAll()
	if len(all) != 1 {
		t.Errorf("expected 1 task, got %d", len(all))
	}

	deleted := store.Delete("task-1")
	if !deleted {
		t.Error("expected task to be deleted")
	}

	_, ok = store.Get("task-1")
	if ok {
		t.Error("expected task to not be found after deletion")
	}
}

func TestScheduler_CreateTask(t *testing.T) {
	store := NewInMemoryTaskStore()
	sched := NewScheduler(store)

	payload := &CommandPayload{Command: "ls -la"}
	task, err := sched.CreateTask(TaskTypeCommand, []string{"node-1", "node-2"}, payload)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if task == nil {
		t.Fatal("expected task, got nil")
	}
	if task.ID == "" {
		t.Error("expected task ID to be set")
	}
	if task.Status != TaskStatusPending {
		t.Errorf("expected Status 'pending', got '%s'", task.Status)
	}
}

func TestScheduler_CreateTask_WithIDGenerator(t *testing.T) {
	store := NewInMemoryTaskStore()
	idCounter := 0
	sched := &scheduler{
		store: store,
		idGen: func() string {
			idCounter++
			return "custom-task-" + string(rune('0'+idCounter))
		},
	}

	task1, _ := sched.CreateTask(TaskTypeCommand, []string{"node-1"}, nil)
	task2, _ := sched.CreateTask(TaskTypeCommand, []string{"node-1"}, nil)

	if task1.ID == task2.ID {
		t.Error("expected different task IDs")
	}
}

func TestScheduler_GetTask(t *testing.T) {
	store := NewInMemoryTaskStore()
	sched := NewScheduler(store)

	_, err := sched.GetTask("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent task")
	}

	task, _ := sched.CreateTask(TaskTypeCommand, []string{"node-1"}, nil)
	retrieved, err := sched.GetTask(task.ID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if retrieved.ID != task.ID {
		t.Errorf("expected ID '%s', got '%s'", task.ID, retrieved.ID)
	}
}

func TestScheduler_ListTasks(t *testing.T) {
	store := NewInMemoryTaskStore()
	sched := NewScheduler(store)

	tasks := sched.ListTasks()
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}

	sched.CreateTask(TaskTypeCommand, []string{"node-1"}, nil)
	sched.CreateTask(TaskTypeCommand, []string{"node-1"}, nil)
	sched.CreateTask(TaskTypeScript, []string{"node-1"}, nil)

	tasks = sched.ListTasks()
	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(tasks))
	}
}

func TestScheduler_CancelTask(t *testing.T) {
	store := NewInMemoryTaskStore()
	sched := NewScheduler(store)

	task, _ := sched.CreateTask(TaskTypeCommand, []string{"node-1"}, nil)

	err := sched.CancelTask(task.ID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	cancelled, _ := sched.GetTask(task.ID)
	if cancelled.Status != TaskStatusCancelled {
		t.Errorf("expected Status 'cancelled', got '%s'", cancelled.Status)
	}
}

func TestScheduler_CancelTask_NotFound(t *testing.T) {
	store := NewInMemoryTaskStore()
	sched := NewScheduler(store)

	err := sched.CancelTask("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestScheduler_CancelTask_AlreadyCompleted(t *testing.T) {
	store := NewInMemoryTaskStore()
	sched := NewScheduler(store)

	task, _ := sched.CreateTask(TaskTypeCommand, []string{"node-1"}, nil)
	task.SetStatus(TaskStatusCompleted)
	store.Set(task.ID, task)

	err := sched.CancelTask(task.ID)
	if err == nil {
		t.Error("expected error for already completed task")
	}
}

func TestScheduler_DispatchTask(t *testing.T) {
	store := NewInMemoryTaskStore()
	sched := NewScheduler(store)

	task, _ := sched.CreateTask(TaskTypeCommand, []string{"node-1"}, nil)

	dispatchCalled := false
	err := sched.DispatchTask(task.ID, func(t *Task) error {
		dispatchCalled = true
		t.SetResult("node-1", &TaskResult{ExitCode: 0})
		return nil
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	updated, _ := sched.GetTask(task.ID)
	if updated.Status != TaskStatusRunning {
		t.Errorf("expected Status 'running' immediately after dispatch, got '%s'", updated.Status)
	}

	time.Sleep(10 * time.Millisecond)

	updated, _ = sched.GetTask(task.ID)
	if updated.Status != TaskStatusCompleted {
		t.Errorf("expected Status 'completed', got '%s'", updated.Status)
	}

	if !dispatchCalled {
		t.Error("expected dispatcher to be called")
	}
}

func TestScheduler_DispatchTask_WithFailure(t *testing.T) {
	store := NewInMemoryTaskStore()
	sched := NewScheduler(store)

	task, _ := sched.CreateTask(TaskTypeCommand, []string{"node-1"}, nil)

	err := sched.DispatchTask(task.ID, func(t *Task) error {
		t.SetResult("node-1", &TaskResult{ExitCode: 1})
		return nil
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	updated, _ := sched.GetTask(task.ID)
	if updated.Status != TaskStatusFailed {
		t.Errorf("expected Status 'failed' when all results have non-zero exit code, got '%s'", updated.Status)
	}
}

func TestScheduler_DispatchTask_NotFound(t *testing.T) {
	store := NewInMemoryTaskStore()
	sched := NewScheduler(store)

	err := sched.DispatchTask("nonexistent", func(t *Task) error {
		return nil
	})
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestScheduler_DispatchTask_AlreadyCompleted(t *testing.T) {
	store := NewInMemoryTaskStore()
	sched := NewScheduler(store)

	task, _ := sched.CreateTask(TaskTypeCommand, []string{"node-1"}, nil)
	task.SetStatus(TaskStatusCompleted)
	store.Set(task.ID, task)

	err := sched.DispatchTask(task.ID, func(t *Task) error {
		return nil
	})
	if err == nil {
		t.Error("expected error for already completed task")
	}
}

func TestDefaultExecutionOptions(t *testing.T) {
	opts := DefaultExecutionOptions()

	if opts.Parallelism != 10 {
		t.Errorf("expected Parallelism 10, got %d", opts.Parallelism)
	}
	if opts.Policy != ParallelismAll {
		t.Errorf("expected Policy ParallelismAll, got %v", opts.Policy)
	}
	if opts.FailureMode != "stop" {
		t.Errorf("expected FailureMode 'stop', got '%s'", opts.FailureMode)
	}
	if opts.ContinueOnError {
		t.Error("expected ContinueOnError to be false")
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
