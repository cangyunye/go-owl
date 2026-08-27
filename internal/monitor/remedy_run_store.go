package monitor

import (
	"database/sql"
	"fmt"
	"time"
)

// EnsureRemedyRunTables 建处置执行计划相关表（幂等）。
func (s *Store) EnsureRemedyRunTables() error {
	schemas := []string{
		`CREATE TABLE IF NOT EXISTS remedy_runs (
			id           TEXT PRIMARY KEY,
			alert_id     TEXT NOT NULL,
			node_id      TEXT NOT NULL,
			status       TEXT NOT NULL,
			stop_on_error INTEGER NOT NULL DEFAULT 1,
			created_by   TEXT NOT NULL DEFAULT '',
			created_at   INTEGER NOT NULL,
			updated_at   INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS remedy_run_steps (
			run_id      TEXT NOT NULL,
			step_order  INTEGER NOT NULL,
			remedy_id   TEXT NOT NULL,
			name        TEXT NOT NULL,
			kind        TEXT NOT NULL,
			content     TEXT NOT NULL,
			rollback    TEXT NOT NULL DEFAULT '',
			status      TEXT NOT NULL,
			started_at  INTEGER NOT NULL DEFAULT 0,
			finished_at INTEGER NOT NULL DEFAULT 0,
			exit_code   INTEGER NOT NULL DEFAULT -1,
			output      TEXT NOT NULL DEFAULT '',
			node_id     TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (run_id, step_order)
		)`,
	}
	for _, q := range schemas {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("monitor: 建处置计划表失败: %w", err)
		}
	}
	return nil
}

// CreateRemedyRun 写入执行计划（含步骤）。
func (s *Store) CreateRemedyRun(run *RemedyRun) error {
	if err := s.EnsureRemedyRunTables(); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`INSERT INTO remedy_runs
		(id, alert_id, node_id, status, stop_on_error, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.AlertID, run.NodeID, string(run.Status), boolInt(run.StopOnError),
		run.CreatedBy, run.CreatedAt, run.UpdatedAt); err != nil {
		return fmt.Errorf("monitor: 写入执行计划失败: %w", err)
	}
	for _, st := range run.Steps {
		if _, err := tx.Exec(`INSERT INTO remedy_run_steps
			(run_id, step_order, remedy_id, name, kind, content, rollback, status, started_at,
			 finished_at, exit_code, output, node_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			run.ID, st.Order, st.RemedyID, st.Name, st.Kind, st.Content, st.Rollback,
			string(st.Status), st.StartedAt, st.FinishedAt, st.ExitCode, st.Output, st.NodeID); err != nil {
			return fmt.Errorf("monitor: 写入执行步骤失败: %w", err)
		}
	}
	return tx.Commit()
}

// GetRemedyRun 整单读取执行计划（含步骤，按顺序）。
func (s *Store) GetRemedyRun(id string) (*RemedyRun, bool, error) {
	if err := s.EnsureRemedyRunTables(); err != nil {
		return nil, false, err
	}
	row := s.db.QueryRow(`SELECT id, alert_id, node_id, status, stop_on_error, created_by, created_at, updated_at
		FROM remedy_runs WHERE id = ?`, id)
	var run RemedyRun
	var status string
	var stopOnError int
	err := row.Scan(&run.ID, &run.AlertID, &run.NodeID, &status, &stopOnError,
		&run.CreatedBy, &run.CreatedAt, &run.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	run.Status = RemedyRunStatus(status)
	run.StopOnError = stopOnError == 1

	steps, err := s.listSteps(id)
	if err != nil {
		return nil, false, err
	}
	run.Steps = steps
	return &run, true, nil
}

// ListRemedyRunsByAlert 按告警 ID 列出执行历史（时间倒序）。
func (s *Store) ListRemedyRunsByAlert(alertID string) ([]RemedyRun, error) {
	if err := s.EnsureRemedyRunTables(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id, alert_id, node_id, status, stop_on_error, created_by, created_at, updated_at
		FROM remedy_runs WHERE alert_id = ? ORDER BY created_at DESC`, alertID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var runs []RemedyRun
	for rows.Next() {
		var run RemedyRun
		var status string
		var stopOnError int
		if err := rows.Scan(&run.ID, &run.AlertID, &run.NodeID, &status, &stopOnError,
			&run.CreatedBy, &run.CreatedAt, &run.UpdatedAt); err != nil {
			return nil, err
		}
		run.Status = RemedyRunStatus(status)
		run.StopOnError = stopOnError == 1
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (s *Store) listSteps(runID string) ([]RemedyStep, error) {
	rows, err := s.db.Query(`SELECT step_order, remedy_id, name, kind, content, rollback, status,
		started_at, finished_at, exit_code, output, node_id
		FROM remedy_run_steps WHERE run_id = ? ORDER BY step_order`, runID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var steps []RemedyStep
	for rows.Next() {
		var st RemedyStep
		var status string
		if err := rows.Scan(&st.Order, &st.RemedyID, &st.Name, &st.Kind, &st.Content, &st.Rollback,
			&status, &st.StartedAt, &st.FinishedAt, &st.ExitCode, &st.Output, &st.NodeID); err != nil {
			return nil, err
		}
		st.Status = RemedyStepStatus(status)
		steps = append(steps, st)
	}
	return steps, rows.Err()
}

// UpdateRemedyRunStatus 更新整单状态。
func (s *Store) UpdateRemedyRunStatus(id string, status RemedyRunStatus) error {
	if _, err := s.db.Exec(`UPDATE remedy_runs SET status = ?, updated_at = ? WHERE id = ?`,
		string(status), time.Now().Unix(), id); err != nil {
		return fmt.Errorf("monitor: 更新执行计划状态失败: %w", err)
	}
	return nil
}

// UpdateRemedyStep 按回调推进单步状态（读-改-写）。
func (s *Store) UpdateRemedyStep(runID string, order int, mutate func(*RemedyStep)) error {
	steps, err := s.listSteps(runID)
	if err != nil {
		return err
	}
	for i := range steps {
		if steps[i].Order != order {
			continue
		}
		mutate(&steps[i])
		st := steps[i]
		if _, err := s.db.Exec(`UPDATE remedy_run_steps SET
			status = ?, started_at = ?, finished_at = ?, exit_code = ?, output = ?, node_id = ?
			WHERE run_id = ? AND step_order = ?`,
			string(st.Status), st.StartedAt, st.FinishedAt, st.ExitCode, st.Output, st.NodeID,
			runID, st.Order); err != nil {
			return fmt.Errorf("monitor: 更新执行步骤失败: %w", err)
		}
		return nil
	}
	return fmt.Errorf("monitor: 执行步骤 %s/%d 不存在", runID, order)
}
