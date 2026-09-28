package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/google/uuid"
)

type PlaybookRunStore struct {
	db *sql.DB
}

func NewPlaybookRunStore(db *sql.DB) *PlaybookRunStore {
	return &PlaybookRunStore{db: db}
}

func (s *PlaybookRunStore) Init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS playbook_runs (
			id              TEXT PRIMARY KEY,
			playbook_id     TEXT NOT NULL,
			playbook_name   TEXT NOT NULL DEFAULT '',
			playbook_file   TEXT NOT NULL DEFAULT '',
			status          TEXT NOT NULL DEFAULT 'queued',
			target_nodes    TEXT NOT NULL DEFAULT '[]',
			extra_vars      TEXT DEFAULT '{}',
			tags            TEXT DEFAULT '',
			danger_confirmed INTEGER DEFAULT 0,
			error           TEXT DEFAULT '',
			results         TEXT DEFAULT '[]',
			created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			started_at      TIMESTAMP,
			completed_at    TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `ALTER TABLE playbook_runs ADD COLUMN danger_confirmed INTEGER DEFAULT 0`)
	// 列已存在属正常迁移幂等：SQLite 报 duplicate column name，DuckDB 报 already exists
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") &&
		!strings.Contains(err.Error(), "already exists") {
		return err
	}
	_, err = s.db.ExecContext(ctx, `ALTER TABLE playbook_runs ADD COLUMN total_steps INTEGER DEFAULT 0`)
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") &&
		!strings.Contains(err.Error(), "already exists") {
		return err
	}
	_, err = s.db.ExecContext(ctx, `ALTER TABLE playbook_runs ADD COLUMN warnings TEXT DEFAULT '[]'`)
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") &&
		!strings.Contains(err.Error(), "already exists") {
		return err
	}
	return nil
}

func (s *PlaybookRunStore) Create(ctx context.Context, playbookID, name, file string, targetNodes []string, extraVars map[string]string, tags string, dangerConfirmed bool) (*model.PlaybookRun, error) {
	nodesJSON, _ := json.Marshal(targetNodes)
	varsJSON, _ := json.Marshal(extraVars)
	run := &model.PlaybookRun{
		ID:              uuid.New().String(),
		PlaybookID:      playbookID,
		PlaybookName:    name,
		PlaybookFile:    file,
		Status:          model.RunStatusQueued,
		TargetNodes:     targetNodes,
		ExtraVars:       extraVars,
		Tags:            tags,
		DangerConfirmed: dangerConfirmed,
		CreatedAt:       time.Now().UTC(),
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO playbook_runs (id, playbook_id, playbook_name, playbook_file, status, target_nodes, extra_vars, tags, danger_confirmed, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.PlaybookID, run.PlaybookName, run.PlaybookFile, run.Status, string(nodesJSON), string(varsJSON), run.Tags, run.DangerConfirmed, run.CreatedAt)
	if err != nil {
		return nil, err
	}
	return run, nil
}

func (s *PlaybookRunStore) scanRow(scanner interface {
	Scan(dest ...interface{}) error
}) (*model.PlaybookRun, error) {
	run := &model.PlaybookRun{}
	var targetNodesStr, extraVarsStr, resultsStr, warningsStr string
	var startedAt, completedAt sql.NullTime
	var totalSteps int
	err := scanner.Scan(
		&run.ID, &run.PlaybookID, &run.PlaybookName, &run.PlaybookFile,
		&run.Status, &targetNodesStr, &extraVarsStr, &run.Tags, &run.DangerConfirmed,
		&run.Error, &resultsStr, &run.CreatedAt, &startedAt, &completedAt, &totalSteps,
		&warningsStr)
	if err != nil {
		return nil, err
	}
	run.TotalSteps = totalSteps
	json.Unmarshal([]byte(targetNodesStr), &run.TargetNodes)
	json.Unmarshal([]byte(extraVarsStr), &run.ExtraVars)
	json.Unmarshal([]byte(resultsStr), &run.Results)
	json.Unmarshal([]byte(warningsStr), &run.Warnings)
	if startedAt.Valid {
		run.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		run.CompletedAt = &completedAt.Time
	}
	if run.ExtraVars == nil {
		run.ExtraVars = map[string]string{}
	}
	return run, nil
}

func (s *PlaybookRunStore) Get(ctx context.Context, id string) (*model.PlaybookRun, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, playbook_id, playbook_name, playbook_file, status, target_nodes, extra_vars, tags, COALESCE(danger_confirmed,0), COALESCE(error,''), COALESCE(results,'[]'), created_at, started_at, completed_at, COALESCE(total_steps,0), COALESCE(warnings,'[]') FROM playbook_runs WHERE id = ?`, id)
	return s.scanRow(row)
}

func (s *PlaybookRunStore) List(ctx context.Context, limit, offset int) ([]*model.PlaybookRun, int, error) {
	var total int
	s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM playbook_runs`).Scan(&total)

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, playbook_id, playbook_name, playbook_file, status, target_nodes, extra_vars, tags, COALESCE(danger_confirmed,0), COALESCE(error,''), COALESCE(results,'[]'), created_at, started_at, completed_at, COALESCE(total_steps,0), COALESCE(warnings,'[]')
		FROM playbook_runs ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	runs := make([]*model.PlaybookRun, 0)
	for rows.Next() {
		run, err := s.scanRow(rows)
		if err != nil {
			continue
		}
		runs = append(runs, run)
	}
	return runs, total, nil
}

func (s *PlaybookRunStore) UpdateStatus(ctx context.Context, id string, status model.PlaybookRunStatus, errMsg string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		UPDATE playbook_runs SET status = ?, error = ?,
		started_at = CASE WHEN started_at IS NULL AND ? = 'running' THEN ? ELSE started_at END,
		completed_at = CASE WHEN ? IN ('completed','failed','cancelled') THEN ? ELSE completed_at END
		WHERE id = ?`,
		status, errMsg, status, now, status, now, id)
	return err
}

// CancelIfActive 原子取消：仅当 run 尚处终态之前时改为 cancelled。
// 单条条件 UPDATE 消除「Get 见 running → 引擎完成 → UpdateStatus 把
// 终态改写为 cancelled」的竞态；返回是否确实取消了。
func (s *PlaybookRunStore) CancelIfActive(ctx context.Context, id string, errMsg string) (bool, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE playbook_runs SET status = ?, error = ?, completed_at = ?
		WHERE id = ? AND status NOT IN ('completed','failed','cancelled')`,
		model.RunStatusCancelled, errMsg, now, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// SetWarnings 持久化预检警告（JSON 数组列），Get/List 读回，
// 刷新页面后运行详情仍能看到警告（R9）。
func (s *PlaybookRunStore) SetWarnings(ctx context.Context, id string, warnings []string) error {
	data, err := json.Marshal(warnings)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE playbook_runs SET warnings = ? WHERE id = ?`, string(data), id)
	return err
}

func (s *PlaybookRunStore) SetTotalSteps(ctx context.Context, id string, total int) error {
	_, err := s.db.ExecContext(ctx, `UPDATE playbook_runs SET total_steps = ? WHERE id = ?`, total, id)
	return err
}

func (s *PlaybookRunStore) AppendResult(ctx context.Context, id string, step *model.StepResult) error {
	resultsJSON, err := s.getResultsJSON(ctx, id)
	if err != nil {
		return err
	}
	var results []*model.StepResult
	json.Unmarshal([]byte(resultsJSON), &results)
	results = append(results, step)
	newJSON, _ := json.Marshal(results)
	_, err = s.db.ExecContext(ctx, `UPDATE playbook_runs SET results = ? WHERE id = ?`, string(newJSON), id)
	return err
}

func (s *PlaybookRunStore) getResultsJSON(ctx context.Context, id string) (string, error) {
	var r string
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(results,'[]') FROM playbook_runs WHERE id = ?`, id).Scan(&r)
	return r, err
}
