package history

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
)

type PlaybookRun struct {
	ID             string
	PlaybookName   string
	PlaybookHash   string
	Nodes          []string
	Status         string
	StartedAt      time.Time
	FinishedAt     *time.Time
	TotalSteps     int
	CompletedSteps int
	FailedSteps    int
}

type PlaybookStepState struct {
	ID         int64
	RunID      string
	NodeID     string
	StepIndex  int
	StepName   string
	Action     string
	Status     string
	StartedAt  *time.Time
	FinishedAt *time.Time
	DurationMs int64
	ExitCode   int
	Stdout     string
	Stderr     string
	Error      string
	RetryCount int
}

func ComputePlaybookHash(playbookContent string, nodes []string) string {
	nodesJSON, _ := json.Marshal(nodes)
	h := sha256.New()
	h.Write([]byte(playbookContent))
	h.Write(nodesJSON)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func CreatePlaybookRun(run *PlaybookRun) error {
	if GetGlobalDB() == nil {
		return nil
	}
	nodesJSON, _ := json.Marshal(run.Nodes)
	_, err := GetGlobalDB().Connection().Exec(`
		INSERT INTO playbook_runs (id, playbook_name, playbook_hash, nodes, status, started_at, total_steps, completed_steps, failed_steps)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, run.ID, run.PlaybookName, run.PlaybookHash, nodesJSON, run.Status, run.StartedAt, run.TotalSteps, run.CompletedSteps, run.FailedSteps)
	return err
}

func FinishPlaybookRun(runID string, status string, completedSteps, failedSteps int) error {
	if GetGlobalDB() == nil {
		return nil
	}
	now := time.Now()
	_, err := GetGlobalDB().Connection().Exec(`
		UPDATE playbook_runs SET status = ?, finished_at = ?, completed_steps = ?, failed_steps = ? WHERE id = ?
	`, status, now, completedSteps, failedSteps, runID)
	return err
}

func UpsertStepState(step *PlaybookStepState) error {
	if GetGlobalDB() == nil {
		return nil
	}
	_, err := GetGlobalDB().Connection().Exec(`
		INSERT INTO playbook_step_states (run_id, node_id, step_index, step_name, action, status, started_at, finished_at, duration_ms, exit_code, stdout, stderr, error, retry_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(run_id, node_id, step_index) DO UPDATE SET
			status = excluded.status,
			started_at = excluded.started_at,
			finished_at = excluded.finished_at,
			duration_ms = excluded.duration_ms,
			exit_code = excluded.exit_code,
			stdout = excluded.stdout,
			stderr = excluded.stderr,
			error = excluded.error,
			retry_count = excluded.retry_count
	`, step.RunID, step.NodeID, step.StepIndex, step.StepName, step.Action, step.Status,
		step.StartedAt, step.FinishedAt, step.DurationMs, step.ExitCode,
		truncateForDB(step.Stdout), truncateForDB(step.Stderr), step.Error, step.RetryCount)
	return err
}

func ListPlaybookRuns(playbookName string, status string, limit int) ([]*PlaybookRun, error) {
	if GetGlobalDB() == nil {
		return nil, nil
	}
	query := `SELECT id, playbook_name, playbook_hash, nodes, status, started_at, finished_at, total_steps, completed_steps, failed_steps FROM playbook_runs WHERE 1=1`
	var params []interface{}

	if playbookName != "" {
		query += ` AND playbook_name = ?`
		params = append(params, playbookName)
	}
	if status != "" {
		query += ` AND status = ?`
		params = append(params, status)
	}
	query += ` ORDER BY started_at DESC`
	if limit > 0 {
		query += ` LIMIT ?`
		params = append(params, limit)
	}

	rows, err := GetGlobalDB().Connection().Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []*PlaybookRun
	for rows.Next() {
		var run PlaybookRun
		var nodesJSON string
		var finishedAt *time.Time
		err := rows.Scan(&run.ID, &run.PlaybookName, &run.PlaybookHash, &nodesJSON,
			&run.Status, &run.StartedAt, &finishedAt, &run.TotalSteps, &run.CompletedSteps, &run.FailedSteps)
		if err != nil {
			continue
		}
		run.FinishedAt = finishedAt
		json.Unmarshal([]byte(nodesJSON), &run.Nodes)
		runs = append(runs, &run)
	}
	return runs, nil
}

func GetPlaybookRun(runID string) (*PlaybookRun, error) {
	if GetGlobalDB() == nil {
		return nil, nil
	}
	row := GetGlobalDB().Connection().QueryRow(`
		SELECT id, playbook_name, playbook_hash, nodes, status, started_at, finished_at, total_steps, completed_steps, failed_steps
		FROM playbook_runs WHERE id = ?
	`, runID)

	var run PlaybookRun
	var nodesJSON string
	var finishedAt *time.Time
	err := row.Scan(&run.ID, &run.PlaybookName, &run.PlaybookHash, &nodesJSON,
		&run.Status, &run.StartedAt, &finishedAt, &run.TotalSteps, &run.CompletedSteps, &run.FailedSteps)
	if err != nil {
		return nil, err
	}
	run.FinishedAt = finishedAt
	json.Unmarshal([]byte(nodesJSON), &run.Nodes)
	return &run, nil
}

func GetStepStates(runID string, nodeID string, statusFilter string) ([]*PlaybookStepState, error) {
	if GetGlobalDB() == nil {
		return nil, nil
	}
	query := `SELECT id, run_id, node_id, step_index, step_name, action, status, started_at, finished_at, duration_ms, exit_code, stdout, stderr, error, retry_count
		FROM playbook_step_states WHERE run_id = ?`
	var params []interface{}
	params = append(params, runID)

	if nodeID != "" {
		query += ` AND node_id = ?`
		params = append(params, nodeID)
	}
	if statusFilter == "incomplete" {
		query += ` AND status IN ('failed', 'pending')`
	} else if statusFilter != "" {
		query += ` AND status = ?`
		params = append(params, statusFilter)
	}
	query += ` ORDER BY node_id, step_index`

	rows, err := GetGlobalDB().Connection().Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var steps []*PlaybookStepState
	for rows.Next() {
		var step PlaybookStepState
		var startedAt, finishedAt *time.Time
		var stdout, stderr, errMsg *string
		err := rows.Scan(&step.ID, &step.RunID, &step.NodeID, &step.StepIndex, &step.StepName,
			&step.Action, &step.Status, &startedAt, &finishedAt, &step.DurationMs,
			&step.ExitCode, &stdout, &stderr, &errMsg, &step.RetryCount)
		if err != nil {
			continue
		}
		step.StartedAt = startedAt
		step.FinishedAt = finishedAt
		if stdout != nil {
			step.Stdout = *stdout
		}
		if stderr != nil {
			step.Stderr = *stderr
		}
		if errMsg != nil {
			step.Error = *errMsg
		}
		steps = append(steps, &step)
	}
	return steps, nil
}

func FindLastFailedRunByPlaybookName(playbookName string) (*PlaybookRun, error) {
	if GetGlobalDB() == nil {
		return nil, nil
	}
	row := GetGlobalDB().Connection().QueryRow(`
		SELECT id, playbook_name, playbook_hash, nodes, status, started_at, finished_at, total_steps, completed_steps, failed_steps
		FROM playbook_runs
		WHERE playbook_name = ? AND status = 'failed'
		ORDER BY started_at DESC LIMIT 1
	`, playbookName)

	var run PlaybookRun
	var nodesJSON string
	var finishedAt *time.Time
	err := row.Scan(&run.ID, &run.PlaybookName, &run.PlaybookHash, &nodesJSON,
		&run.Status, &run.StartedAt, &finishedAt, &run.TotalSteps, &run.CompletedSteps, &run.FailedSteps)
	if err != nil {
		return nil, err
	}
	run.FinishedAt = finishedAt
	json.Unmarshal([]byte(nodesJSON), &run.Nodes)
	return &run, nil
}

func truncateForDB(s string) string {
	const maxLen = 4096
	if len(s) <= maxLen {
		return s
	}
	head := s[:2048]
	tail := s[len(s)-2048:]
	return head + "\n...[truncated]...\n" + tail
}
