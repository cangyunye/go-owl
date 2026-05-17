package async

import (
	"testing"
	"time"
)

func TestDefaultAsyncOptions(t *testing.T) {
	opts := DefaultAsyncOptions()

	if opts.Timeout != 1*time.Hour {
		t.Errorf("expected Timeout 1h, got %v", opts.Timeout)
	}

	if opts.PollInterval != 10*time.Second {
		t.Errorf("expected PollInterval 10s, got %v", opts.PollInterval)
	}

	if opts.MaxPollCount != 3600 {
		t.Errorf("expected MaxPollCount 3600, got %d", opts.MaxPollCount)
	}

	if opts.RemoteBaseDir != "/tmp/owl" {
		t.Errorf("expected RemoteBaseDir /tmp/owl, got %s", opts.RemoteBaseDir)
	}
}

func TestAsyncTask_IsCompleted(t *testing.T) {
	tests := []struct {
		name   string
		status AsyncTaskStatus
		want   bool
	}{
		{"pending", AsyncTaskStatusPending, false},
		{"running", AsyncTaskStatusRunning, false},
		{"success", AsyncTaskStatusSuccess, true},
		{"failed", AsyncTaskStatusFailed, true},
		{"timeout", AsyncTaskStatusTimeout, true},
		{"canceled", AsyncTaskStatusCanceled, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &AsyncTask{Status: tt.status}
			if got := task.IsCompleted(); got != tt.want {
				t.Errorf("IsCompleted() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAsyncTask_Duration(t *testing.T) {
	start := time.Now()
	time.Sleep(10 * time.Millisecond)

	task := &AsyncTask{
		StartTime: start,
		EndTime:   time.Now(),
	}

	dur := task.Duration()
	if dur < 10*time.Millisecond {
		t.Errorf("expected duration >= 10ms, got %v", dur)
	}
}

func TestNewAsyncTaskManager(t *testing.T) {
	manager := NewAsyncTaskManager(nil)

	if manager.remoteBaseDir != "/tmp/owl" {
		t.Errorf("expected remoteBaseDir /tmp/owl, got %s", manager.remoteBaseDir)
	}

	if manager.maxConcurrent != 100 {
		t.Errorf("expected maxConcurrent 100, got %d", manager.maxConcurrent)
	}

	if manager.cleanupAfter != 24*time.Hour {
		t.Errorf("expected cleanupAfter 24h, got %v", manager.cleanupAfter)
	}
}

func TestAsyncTaskManager_GetTask(t *testing.T) {
	manager := NewAsyncTaskManager(nil)

	task := &AsyncTask{
		ID:     "test-task",
		NodeID: "test-node",
	}

	manager.mu.Lock()
	manager.tasks["test-task"] = task
	manager.mu.Unlock()

	found := manager.GetTask("test-task")
	if found == nil {
		t.Error("expected task to be found")
	}
	if found.ID != "test-task" {
		t.Errorf("expected task ID test-task, got %s", found.ID)
	}

	notFound := manager.GetTask("nonexistent")
	if notFound != nil {
		t.Error("expected nil for nonexistent task")
	}
}

func TestAsyncTaskManager_ListTasks(t *testing.T) {
	manager := NewAsyncTaskManager(nil)

	manager.mu.Lock()
	manager.tasks["task1"] = &AsyncTask{ID: "task1"}
	manager.tasks["task2"] = &AsyncTask{ID: "task2"}
	manager.mu.Unlock()

	tasks := manager.ListTasks()
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestAsyncTaskManager_CancelTask(t *testing.T) {
	manager := NewAsyncTaskManager(nil)

	task := &AsyncTask{
		ID:     "cancel-test",
		NodeID: "test-node",
		Pid:    99999,
	}

	manager.mu.Lock()
	manager.tasks["cancel-test"] = task
	manager.mu.Unlock()

	err := manager.CancelTask("cancel-test")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	err = manager.CancelTask("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}