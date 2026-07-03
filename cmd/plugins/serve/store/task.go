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
	ID        string     `json:"id"`
	NodeID    string     `json:"node_id"`
	Command   string     `json:"command"`
	Status    TaskStatus `json:"status"`
	Output    string     `json:"output,omitempty"`
	ExitCode  *int       `json:"exit_code,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type TaskStore struct {
	db *sql.DB
}

func NewTaskStore(db *sql.DB) *TaskStore {
	return &TaskStore{db: db}
}

func (s *TaskStore) Init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			node_id TEXT NOT NULL,
			command TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'queued',
			output TEXT DEFAULT '',
			exit_code INTEGER,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			started_at TIMESTAMP,
			completed_at TIMESTAMP
		)
	`)
	return err
}

func (s *TaskStore) Create(ctx context.Context, nodeID, command string) (*Task, error) {
	task := &Task{
		ID:        uuid.New().String(),
		NodeID:    nodeID,
		Command:   command,
		Status:    TaskStatusQueued,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tasks (id, node_id, command, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		task.ID, task.NodeID, task.Command, task.Status, task.CreatedAt, task.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return task, nil
}

func (s *TaskStore) Get(ctx context.Context, id string) (*Task, error) {
	t := &Task{}
	var startedAt, completedAt sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT id, node_id, command, status, COALESCE(output, ''), exit_code, created_at, updated_at, started_at, completed_at FROM tasks WHERE id = ?`, id).
		Scan(&t.ID, &t.NodeID, &t.Command, &t.Status, &t.Output, &t.ExitCode, &t.CreatedAt, &t.UpdatedAt, &startedAt, &completedAt)
	if err != nil {
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

func (s *TaskStore) List(ctx context.Context, limit, offset int) ([]*Task, int, error) {
	var total int
	s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks`).Scan(&total)

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, node_id, command, status, COALESCE(output, ''), exit_code, created_at, updated_at, started_at, completed_at
		FROM tasks ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	tasks := make([]*Task, 0)
	for rows.Next() {
		t := &Task{}
		var startedAt, completedAt sql.NullTime
		if err := rows.Scan(&t.ID, &t.NodeID, &t.Command, &t.Status, &t.Output, &t.ExitCode, &t.CreatedAt, &t.UpdatedAt, &startedAt, &completedAt); err != nil {
			continue
		}
		if startedAt.Valid { t.StartedAt = &startedAt.Time }
		if completedAt.Valid { t.CompletedAt = &completedAt.Time }
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

func (s *TaskStore) ListByNode(ctx context.Context, nodeID string, status TaskStatus) ([]*Task, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, node_id, command, status, COALESCE(output, ''), exit_code, created_at, updated_at, started_at, completed_at
		FROM tasks WHERE node_id = ? AND status = ? ORDER BY created_at DESC`, nodeID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := make([]*Task, 0)
	for rows.Next() {
		t := &Task{}
		var startedAt, completedAt sql.NullTime
		if err := rows.Scan(&t.ID, &t.NodeID, &t.Command, &t.Status, &t.Output, &t.ExitCode, &t.CreatedAt, &t.UpdatedAt, &startedAt, &completedAt); err != nil {
			continue
		}
		if startedAt.Valid { t.StartedAt = &startedAt.Time }
		if completedAt.Valid { t.CompletedAt = &completedAt.Time }
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (s *TaskStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id)
	return err
}
