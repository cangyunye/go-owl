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
	Warnings        []string          `json:"warnings,omitempty"`
	// TotalSteps 为预估总步数（任务数×节点数），运行中前端据此算进度百分比；
	// loop 展开的步骤可能超出该值。
	TotalSteps int `json:"total_steps,omitempty"`
	// RunningSteps 为正在执行中的步骤快照（仅内存、不落库），步骤开始即
	// 推送，完成后转入 Results 并从本字段移除——长任务执行期间用户也能
	// 看到当前卡在哪一步。
	RunningSteps []*StepResult `json:"running_steps,omitempty"`
	Results      []*StepResult `json:"results,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
	StartedAt    *time.Time    `json:"started_at,omitempty"`
	CompletedAt  *time.Time    `json:"completed_at,omitempty"`
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
