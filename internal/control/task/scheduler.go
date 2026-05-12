package task

import (
	"fmt"
	"sync"
	"time"

	"github.com/cangyunye/go-owl/internal/common/model"
)

type TaskType string

const (
	TaskTypeCommand      TaskType = "command"
	TaskTypeScript       TaskType = "script"
	TaskTypePlaybook     TaskType = "playbook"
	TaskTypeFileTransfer TaskType = "file_transfer"
)

type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusRunning    TaskStatus = "running"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
	TaskStatusCancelled  TaskStatus = "cancelled"
)

type TaskResult struct {
	NodeID    string
	ExitCode  int
	Output    string
	Error     error
	StartTime time.Time
	EndTime   time.Time
}

type TaskPayload interface{}

type CommandPayload struct {
	Command   string
	Timeout   time.Duration
	EnvVars   map[string]string
	WorkDir   string
}

type ScriptPayload struct {
	ScriptContent string
	ScriptName   string
	Args         []string
	Timeout      time.Duration
	EnvVars      map[string]string
	WorkDir      string
}

type PlaybookPayload struct {
	PlaybookContent string
	PlaybookName    string
	ExtraVars       map[string]interface{}
}

type FileTransferPayload struct {
	SourcePath      string
	DestinationPath string
	FileName        string
	FileSize        int64
	FileHash        string
	Direction       string
}

type Task struct {
	ID          string
	Type        TaskType
	Targets     []string
	Payload     TaskPayload
	Status      TaskStatus
	Results     map[string]*TaskResult
	CreatedAt   time.Time
	UpdatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	Error       string
}

func NewTask(id string, taskType TaskType, targets []string, payload TaskPayload) *Task {
	now := time.Now()
	return &Task{
		ID:        id,
		Type:      taskType,
		Targets:   targets,
		Payload:   payload,
		Status:    TaskStatusPending,
		Results:   make(map[string]*TaskResult),
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (t *Task) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("task ID is required")
	}
	if t.Type == "" {
		return fmt.Errorf("task type is required")
	}
	if len(t.Targets) == 0 {
		return fmt.Errorf("task targets cannot be empty")
	}
	return nil
}

func (t *Task) SetStatus(status TaskStatus) {
	t.Status = status
	t.UpdatedAt = time.Now()
	if status == TaskStatusRunning && t.StartedAt == nil {
		now := time.Now()
		t.StartedAt = &now
	}
	if status == TaskStatusCompleted || status == TaskStatusFailed || status == TaskStatusCancelled {
		now := time.Now()
		t.CompletedAt = &now
	}
}

func (t *Task) SetResult(nodeID string, result *TaskResult) {
	t.Results[nodeID] = result
	t.UpdatedAt = time.Now()
}

func (t *Task) IsCompleted() bool {
	return t.Status == TaskStatusCompleted || t.Status == TaskStatusFailed || t.Status == TaskStatusCancelled
}

func (t *Task) Progress() float64 {
	if len(t.Targets) == 0 {
		return 0
	}
	return float64(len(t.Results)) / float64(len(t.Targets))
}

func (t *Task) SuccessCount() int {
	count := 0
	for _, result := range t.Results {
		if result.ExitCode == 0 {
			count++
		}
	}
	return count
}

func (t *Task) FailureCount() int {
	count := 0
	for _, result := range t.Results {
		if result.ExitCode != 0 {
			count++
		}
	}
	return count
}

func (t *Task) Duration() time.Duration {
	if t.StartedAt == nil {
		return 0
	}
	end := t.CompletedAt
	if end == nil {
		end = &time.Time{}
		*end = time.Now()
	}
	return end.Sub(*t.StartedAt)
}

type TaskStore interface {
	Get(id string) (*Task, bool)
	Set(id string, task *Task)
	Delete(id string) bool
	GetAll() []*Task
}

type InMemoryTaskStore struct {
	mu    sync.RWMutex
	tasks map[string]*Task
}

func NewInMemoryTaskStore() *InMemoryTaskStore {
	return &InMemoryTaskStore{
		tasks: make(map[string]*Task),
	}
}

func (s *InMemoryTaskStore) Get(id string) (*Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	task, ok := s.tasks[id]
	return task, ok
}

func (s *InMemoryTaskStore) Set(id string, task *Task) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[id] = task
}

func (s *InMemoryTaskStore) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tasks[id]; ok {
		delete(s.tasks, id)
		return true
	}
	return false
}

func (s *InMemoryTaskStore) GetAll() []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tasks := make([]*Task, 0, len(s.tasks))
	for _, task := range s.tasks {
		tasks = append(tasks, task)
	}
	return tasks
}

type Scheduler interface {
	CreateTask(taskType TaskType, targets []string, payload TaskPayload) (*Task, error)
	GetTask(id string) (*Task, error)
	ListTasks() []*Task
	CancelTask(id string) error
	DispatchTask(id string, dispatcher func(*Task) error) error
}

type scheduler struct {
	store  TaskStore
	mu     sync.Mutex
	idGen  func() string
}

func NewScheduler(store TaskStore) Scheduler {
	return &scheduler{
		store: store,
		idGen: func() string {
			return fmt.Sprintf("task-%d", time.Now().UnixNano())
		},
	}
}

func (s *scheduler) CreateTask(taskType TaskType, targets []string, payload TaskPayload) (*Task, error) {
	task := NewTask(s.idGen(), taskType, targets, payload)
	if err := task.Validate(); err != nil {
		return nil, fmt.Errorf("invalid task: %w", err)
	}
	s.store.Set(task.ID, task)
	return task, nil
}

func (s *scheduler) GetTask(id string) (*Task, error) {
	task, ok := s.store.Get(id)
	if !ok {
		return nil, fmt.Errorf("task with ID '%s' not found", id)
	}
	return task, nil
}

func (s *scheduler) ListTasks() []*Task {
	return s.store.GetAll()
}

func (s *scheduler) CancelTask(id string) error {
	task, ok := s.store.Get(id)
	if !ok {
		return fmt.Errorf("task with ID '%s' not found", id)
	}
	if task.IsCompleted() {
		return fmt.Errorf("cannot cancel completed task")
	}
	task.SetStatus(TaskStatusCancelled)
	s.store.Set(id, task)
	return nil
}

func (s *scheduler) DispatchTask(id string, dispatcher func(*Task) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.store.Get(id)
	if !ok {
		return fmt.Errorf("task with ID '%s' not found", id)
	}

	if task.IsCompleted() {
		return fmt.Errorf("task already completed")
	}

	task.SetStatus(TaskStatusRunning)
	s.store.Set(id, task)

	go func() {
		err := dispatcher(task)
		s.mu.Lock()
		defer s.mu.Unlock()

		task, ok := s.store.Get(id)
		if !ok {
			return
		}

		if err != nil {
			task.SetStatus(TaskStatusFailed)
			task.Error = err.Error()
		} else if task.FailureCount() == 0 {
			task.SetStatus(TaskStatusCompleted)
		} else if task.SuccessCount() > 0 {
			task.SetStatus(TaskStatusCompleted)
		} else {
			task.SetStatus(TaskStatusFailed)
		}
		s.store.Set(id, task)
	}()

	return nil
}

type TaskExecutor interface {
	Execute(task *Task, nodeManager interface{ GetByID(string) (*model.Node, error) }) error
}

type ParallelismPolicy int

const (
	ParallelismAll     ParallelismPolicy = 0
	ParallelismOne     ParallelismPolicy = 1
	ParallelismPercent ParallelismPolicy = 2
)

type ExecutionOptions struct {
	Parallelism    int
	Policy         ParallelismPolicy
	FailureMode    string
	ContinueOnError bool
}

func DefaultExecutionOptions() *ExecutionOptions {
	return &ExecutionOptions{
		Parallelism:    10,
		Policy:         ParallelismAll,
		FailureMode:    "stop",
		ContinueOnError: false,
	}
}
