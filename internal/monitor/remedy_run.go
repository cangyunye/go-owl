package monitor

// RemedyRunStatus 处置执行计划状态。
type RemedyRunStatus string

const (
	RunPending RemedyRunStatus = "pending" // 待执行
	RunRunning RemedyRunStatus = "running" // 执行中
	RunDone    RemedyRunStatus = "done"    // 全部完成
	RunStopped RemedyRunStatus = "stopped" // 用户中止
	RunFailed  RemedyRunStatus = "failed"  // 失败中止
)

// RemedyStepStatus 执行步骤状态。
type RemedyStepStatus string

const (
	StepPending RemedyStepStatus = "pending"
	StepRunning RemedyStepStatus = "running"
	StepSuccess RemedyStepStatus = "success"
	StepFailed  RemedyStepStatus = "failed"
	StepSkipped RemedyStepStatus = "skipped" // 人工步骤（sop）或未执行
)

// RemedyStep 一条执行步骤（对策内容在执行时快照，避免对策被改后执行旧内容）。
type RemedyStep struct {
	Order      int              `json:"order"`
	RemedyID   string           `json:"remedy_id"`
	Name       string           `json:"name"`
	Kind       string           `json:"kind"` // script | playbook | sop
	Content    string           `json:"content"`
	Rollback   string           `json:"rollback"`
	Status     RemedyStepStatus `json:"status"`
	StartedAt  int64            `json:"started_at"`
	FinishedAt int64            `json:"finished_at"`
	ExitCode   int              `json:"exit_code"` // -1 = 未执行
	Output     string           `json:"output"`
	NodeID     string           `json:"node_id"`
}

// RemedyRun 处置执行计划：一组按序执行的处置步骤，绑定告警实例。
type RemedyRun struct {
	ID          string          `json:"id"`
	AlertID     string          `json:"alert_id"`
	NodeID      string          `json:"node_id"`
	Status      RemedyRunStatus `json:"status"`
	StopOnError bool            `json:"stop_on_error"` // 失败即停
	CreatedBy   string          `json:"created_by"`
	CreatedAt   int64           `json:"created_at"`
	UpdatedAt   int64           `json:"updated_at"`
	Steps       []RemedyStep    `json:"steps"`
}

// IsTerminal 是否已结束（不再执行后续步骤）。
func (r *RemedyRun) IsTerminal() bool {
	return r.Status == RunDone || r.Status == RunStopped || r.Status == RunFailed
}
