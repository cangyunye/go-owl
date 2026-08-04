package model

import "time"

type Playbook struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Category    string   `json:"category,omitempty"`
	FilePath    string   `json:"file_path"`
	TasksCount  int      `json:"tasks_count"`
	TaskNames   []string `json:"task_names,omitempty"`
	FileExists  bool     `json:"file_exists"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
}

type PlaybookRunStatus string

const (
	RunStatusQueued    PlaybookRunStatus = "queued"
	RunStatusRunning   PlaybookRunStatus = "running"
	RunStatusCompleted PlaybookRunStatus = "completed"
	RunStatusFailed    PlaybookRunStatus = "failed"
	RunStatusCancelled PlaybookRunStatus = "cancelled"
)

type PlaybookRun struct {
	ID              string            `json:"id"`
	PlaybookID      string            `json:"playbook_id"`
	PlaybookName    string            `json:"playbook_name"`
	PlaybookFile    string            `json:"playbook_file"`
	Status          PlaybookRunStatus `json:"status"`
	TargetNodes     []string          `json:"target_nodes"`
	ExtraVars       map[string]string `json:"extra_vars,omitempty"`
	Tags            string            `json:"tags,omitempty"`
	DangerConfirmed bool              `json:"danger_confirmed,omitempty"`
	Error           string            `json:"error,omitempty"`
	Results         []*StepResult     `json:"results,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	StartedAt       *time.Time        `json:"started_at,omitempty"`
	CompletedAt     *time.Time        `json:"completed_at,omitempty"`
}

type StepResult struct {
	TaskName   string `json:"task_name"`
	NodeID     string `json:"node_id"`
	Action     string `json:"action,omitempty"`
	Status     string `json:"status"`
	ExitCode   int    `json:"exit_code"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"duration_ms"`
}
