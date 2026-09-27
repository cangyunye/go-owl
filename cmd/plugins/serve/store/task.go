package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type TaskStatus string

const (
	TaskStatusQueued    TaskStatus = "queued"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

type Task struct {
	ID          string     `json:"id"`
	NodeID      string     `json:"node_id"`
	Command     string     `json:"command"`
	Status      TaskStatus `json:"status"`
	Output      string     `json:"output,omitempty"`
	// OutputLen 仅轻量列表（?light=1）填充：客户端据此判断需要补拉多少尾部
	OutputLen   int        `json:"output_len,omitempty"`
	ExitCode    *int       `json:"exit_code,omitempty"`
	RecordID    string     `json:"record_id,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type TaskStore struct {
	db *sql.DB
}

// taskScanner 兼容 *sql.Row 与 *sql.Rows 的最小扫描接口。
type taskScanner interface {
	Scan(dest ...interface{}) error
}

// taskFullColumns 详情/对账路径的列清单（含 output 大字段）。
const taskFullColumns = `id, node_id, command, status, COALESCE(output, ''), exit_code, COALESCE(record_id, ''), created_at, updated_at, started_at, completed_at`

// scanTaskFull 扫描含 output 的完整任务行。
func scanTaskFull(s taskScanner) (*Task, error) {
	t := &Task{}
	var startedAt, completedAt sql.NullTime
	if err := s.Scan(&t.ID, &t.NodeID, &t.Command, &t.Status, &t.Output, &t.ExitCode, &t.RecordID, &t.CreatedAt, &t.UpdatedAt, &startedAt, &completedAt); err != nil {
		return nil, err
	}
	if startedAt.Valid {
		t.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		t.CompletedAt = &completedAt.Time
	}
	return t, nil
}

func NewTaskStore(db *sql.DB) *TaskStore {
	return &TaskStore{db: db}
}

func (s *TaskStore) Init(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			node_id TEXT NOT NULL,
			command TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'queued',
			output TEXT DEFAULT '',
			exit_code INTEGER,
			record_id TEXT DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			started_at TIMESTAMP,
			completed_at TIMESTAMP
		)
	`); err != nil {
		return err
	}
	// tasks 表由命令执行/传输/监控共享，record_id/node_id/status 上的
	// 索引覆盖 ListByRecord（按 record 对账）、ListByNode、updateOpStatus
	// （按 record_id 聚合状态）与 ListByCommandPrefix 之外的常规过滤；
	// CREATE INDEX IF NOT EXISTS 对存量库幂等生效。
	for _, stmt := range []string{
		`CREATE INDEX IF NOT EXISTS idx_tasks_record_id ON tasks (record_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_node_id ON tasks (node_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks (status)`,
	} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *TaskStore) Create(ctx context.Context, nodeID, command string) (*Task, error) {
	return s.CreateWithRecord(ctx, nodeID, command, "")
}

func (s *TaskStore) CreateWithRecord(ctx context.Context, nodeID, command, recordID string) (*Task, error) {
	task := &Task{
		ID:        uuid.New().String(),
		NodeID:    nodeID,
		Command:   command,
		Status:    TaskStatusQueued,
		RecordID:  recordID,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tasks (id, node_id, command, status, record_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		task.ID, task.NodeID, task.Command, task.Status, task.RecordID, task.CreatedAt, task.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return task, nil
}

func (s *TaskStore) Get(ctx context.Context, id string) (*Task, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+taskFullColumns+` FROM tasks WHERE id = ?`, id)
	return scanTaskFull(row)
}

// List 任务列表：不回传 output 大字段（流式输出全程累积，列表页会全量带回）。
// 列表 UI 只展示 node/command/status；output 由详情 Get 与 WS 提供。
func (s *TaskStore) List(ctx context.Context, limit, offset int) ([]*Task, int, error) {
	var total int
	s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks`).Scan(&total)

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, node_id, command, status, exit_code, COALESCE(record_id, ''), created_at, updated_at, started_at, completed_at
		FROM tasks ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	tasks := make([]*Task, 0)
	for rows.Next() {
		t := &Task{}
		var startedAt, completedAt sql.NullTime
		if err := rows.Scan(&t.ID, &t.NodeID, &t.Command, &t.Status, &t.ExitCode, &t.RecordID, &t.CreatedAt, &t.UpdatedAt, &startedAt, &completedAt); err != nil {
			continue
		}
		if startedAt.Valid {
			t.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			t.CompletedAt = &completedAt.Time
		}
		tasks = append(tasks, t)
	}
	return tasks, total, nil
}

// ListByCommandPrefix 按 command 前缀过滤后再取最新 limit 条。
// 传输任务详情列表必须用它：tasks 表由命令执行/监控等共享，若先取全量
// 最新 N 条再在内存里过滤，前缀外的任务一多就会把目标前缀的任务完全
// 挤出结果（列表间歇性变空）。
// FailOrphaned 启动对账：服务重启会丢失所有执行 goroutine，把遗留的
// running/queued 任务标记为失败并写明原因（保留已采集的输出），避免任务
// 永久停留在"执行中"。返回受影响的 record id，供调用方同步 operation 状态。
func (s *TaskStore) FailOrphaned(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT COALESCE(record_id, '') FROM tasks WHERE status IN ('running', 'queued')`)
	if err != nil {
		return nil, err
	}
	recordIDs := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil && id != "" {
			recordIDs = append(recordIDs, id)
		}
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	const reason = "服务重启，执行中断"
	if _, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET status = ?,
			output = CASE WHEN COALESCE(output, '') = '' THEN ? ELSE output || char(10) || ? END,
			completed_at = ?, updated_at = ?
		WHERE status IN ('running', 'queued')`,
		TaskStatusFailed, reason, reason, now, now); err != nil {
		return nil, err
	}
	return recordIDs, nil
}

// ListByRecord 返回一次提交(record)下的全部任务。
// 执行页用它做终态对账兜底：WS 消息可能在断线窗口丢失，按 record 一次性
// 拉取全部任务状态，避免按节点发 N 次请求。
func (s *TaskStore) ListByRecord(ctx context.Context, recordID string) ([]*Task, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+taskFullColumns+` FROM tasks WHERE record_id = ? ORDER BY created_at`, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := make([]*Task, 0)
	for rows.Next() {
		t, err := scanTaskFull(rows)
		if err != nil {
			continue
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (s *TaskStore) ListByCommandPrefix(ctx context.Context, prefix string, limit, offset int) ([]*Task, int, error) {
	var total int
	s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE command LIKE ?`, prefix+"%").Scan(&total)

	rows, err := s.db.QueryContext(ctx,
		`SELECT `+taskFullColumns+` FROM tasks WHERE command LIKE ? ORDER BY created_at DESC LIMIT ? OFFSET ?`, prefix+"%", limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	tasks := make([]*Task, 0)
	for rows.Next() {
		t, err := scanTaskFull(rows)
		if err != nil {
			continue
		}
		tasks = append(tasks, t)
	}
	return tasks, total, nil
}

func (s *TaskStore) UpdateStatus(ctx context.Context, id string, status TaskStatus, output string, exitCode *int) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET status = ?, output = ?, exit_code = ?, updated_at = ?,
		started_at = CASE WHEN started_at IS NULL AND ? = 'running' THEN ? ELSE started_at END,
		completed_at = CASE WHEN ? IN ('completed','failed','cancelled') THEN ? ELSE completed_at END
		WHERE id = ?`,
		status, output, exitCode, now, status, now, status, now, id)
	return err
}

// UpdateStatusGuarded 与 UpdateStatus 相同，但当任务已处于 cancelled 时不做任何写入。
// 执行 goroutine 的进度与终态写入都必须走这里，否则会覆盖人工取消。
// 返回 applied=false 表示因任务已取消而跳过写入。
func (s *TaskStore) UpdateStatusGuarded(ctx context.Context, id string, status TaskStatus, output string, exitCode *int) (bool, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET status = ?, output = ?, exit_code = ?, updated_at = ?,
		started_at = CASE WHEN started_at IS NULL AND ? = 'running' THEN ? ELSE started_at END,
		completed_at = CASE WHEN ? IN ('completed','failed','cancelled') THEN ? ELSE completed_at END
		WHERE id = ? AND status <> 'cancelled'`,
		status, output, exitCode, now, status, now, status, now, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *TaskStore) ListByNode(ctx context.Context, nodeID string, status TaskStatus) ([]*Task, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+taskFullColumns+` FROM tasks WHERE node_id = ? AND status = ? ORDER BY created_at DESC`, nodeID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := make([]*Task, 0)
	for rows.Next() {
		t, err := scanTaskFull(rows)
		if err != nil {
			continue
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (s *TaskStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id)
	return err
}
