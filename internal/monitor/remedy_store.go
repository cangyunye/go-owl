package monitor

import (
	"database/sql"
	"fmt"
	"time"
)

// Remedy 对策：绑定告警类型的处置方案（脚本/剧本/人工指引）。
// 来源优先级：user > builtin > ai（ai 须审核后方可推荐/自动执行）。
type Remedy struct {
	ID           string `json:"id"`
	AlertTypeID  string `json:"alert_type_id"`
	Name         string `json:"name"`
	Kind         string `json:"kind"` // script | playbook | sop
	Content      string `json:"content"`
	Risk         string `json:"risk"` // low | medium | high
	Rollback     string `json:"rollback"`
	Source       string `json:"source"`       // builtin | ai | user
	Reviewed     bool   `json:"reviewed"`     // AI 生成须人工审核
	AutoApprove  bool   `json:"auto_approve"` // 该对策是否允许自动执行
	ExecCount    int    `json:"exec_count"`
	SuccessCount int    `json:"success_count"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
}

// CanAutoExecute 自动执行门槛：告警类型放行 AND 对策放行；AI 来源必须已审核。
func CanAutoExecute(at AlertType, r Remedy) bool {
	if !at.AutoApprove || !r.AutoApprove {
		return false
	}
	if r.Source == "ai" && !r.Reviewed {
		return false
	}
	return true
}

// EnsureRemedyTables 建对策表（幂等）。
func (s *Store) EnsureRemedyTables() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS remedies (
		id            TEXT PRIMARY KEY,
		alert_type_id TEXT NOT NULL,
		name          TEXT NOT NULL,
		kind          TEXT NOT NULL,
		content       TEXT NOT NULL,
		risk          TEXT NOT NULL,
		rollback      TEXT NOT NULL DEFAULT '',
		source        TEXT NOT NULL,
		reviewed      INTEGER NOT NULL DEFAULT 0,
		auto_approve  INTEGER NOT NULL DEFAULT 0,
		exec_count    INTEGER NOT NULL DEFAULT 0,
		success_count INTEGER NOT NULL DEFAULT 0,
		created_at    INTEGER NOT NULL,
		updated_at    INTEGER NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("monitor: 建对策表失败: %w", err)
	}
	return nil
}

// UpsertRemedy 写入或更新对策。
func (s *Store) UpsertRemedy(r Remedy) error {
	if err := s.EnsureRemedyTables(); err != nil {
		return err
	}
	now := time.Now().Unix()
	if r.CreatedAt == 0 {
		r.CreatedAt = now
	}
	r.UpdatedAt = now
	_, err := s.db.Exec(`INSERT INTO remedies
		(id, alert_type_id, name, kind, content, risk, rollback, source, reviewed, auto_approve,
		 exec_count, success_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, kind=excluded.kind, content=excluded.content,
			risk=excluded.risk, rollback=excluded.rollback, source=excluded.source,
			reviewed=excluded.reviewed, auto_approve=excluded.auto_approve,
			exec_count=excluded.exec_count, success_count=excluded.success_count,
			updated_at=excluded.updated_at`,
		r.ID, r.AlertTypeID, r.Name, r.Kind, r.Content, r.Risk, r.Rollback, r.Source,
		boolInt(r.Reviewed), boolInt(r.AutoApprove), r.ExecCount, r.SuccessCount,
		r.CreatedAt, r.UpdatedAt)
	if err != nil {
		return fmt.Errorf("monitor: 写入对策 %s 失败: %w", r.ID, err)
	}
	return nil
}

// GetRemedy 按 ID 获取对策。
func (s *Store) GetRemedy(id string) (Remedy, bool, error) {
	row := s.db.QueryRow(`SELECT id, alert_type_id, name, kind, content, risk, rollback, source,
		reviewed, auto_approve, exec_count, success_count, created_at, updated_at
		FROM remedies WHERE id = ?`, id)
	r, err := scanRemedy(row)
	if err == sql.ErrNoRows {
		return Remedy{}, false, nil
	}
	if err != nil {
		return Remedy{}, false, err
	}
	return r, true, nil
}

// ListRemedies 按告警类型列出对策。
func (s *Store) ListRemedies(alertTypeID string) ([]Remedy, error) {
	query := `SELECT id, alert_type_id, name, kind, content, risk, rollback, source,
		reviewed, auto_approve, exec_count, success_count, created_at, updated_at
		FROM remedies`
	var args []any
	if alertTypeID != "" {
		query += ` WHERE alert_type_id = ?`
		args = append(args, alertTypeID)
	}
	query += ` ORDER BY id`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []Remedy
	for rows.Next() {
		r, err := scanRemedy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteRemedy 删除对策；内置对策不可删除。
func (s *Store) DeleteRemedy(id string) error {
	r, exists, err := s.GetRemedy(id)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if r.Source == "builtin" {
		return fmt.Errorf("monitor: 内置对策 %s 不可删除", id)
	}
	if _, err := s.db.Exec(`DELETE FROM remedies WHERE id = ?`, id); err != nil {
		return fmt.Errorf("monitor: 删除对策失败: %w", err)
	}
	return nil
}

// RecordExecution 记录对策执行反馈（自愈成功/失败闭环）。
func (s *Store) RecordExecution(id string, success bool) error {
	res, err := s.db.Exec(`UPDATE remedies SET
		exec_count = exec_count + 1,
		success_count = success_count + ?,
		updated_at = ?
		WHERE id = ?`, boolInt(success), time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("monitor: 记录对策执行失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("monitor: 对策 %s 不存在", id)
	}
	return nil
}

// RecommendedRemedies 返回某告警类型的推荐对策（按用户 > 内置 > AI 排序，
// AI 未审核排除；同级来源内低风险优先）。供告警详情页/AI 处置使用。
func (s *Store) RecommendedRemedies(alertTypeID string) ([]Remedy, error) {
	all, err := s.ListRemedies(alertTypeID)
	if err != nil {
		return nil, err
	}
	sourceRank := map[string]int{"user": 0, "builtin": 1, "ai": 2}
	riskRank := map[string]int{"low": 0, "medium": 1, "high": 2}

	out := make([]Remedy, 0, len(all))
	for _, r := range all {
		if r.Source == "ai" && !r.Reviewed {
			continue // AI 未审核不入推荐
		}
		out = append(out, r)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0; j-- {
			a, b := out[j-1], out[j]
			if sourceRank[a.Source] > sourceRank[b.Source] ||
				(sourceRank[a.Source] == sourceRank[b.Source] && riskRank[a.Risk] > riskRank[b.Risk]) {
				out[j-1], out[j] = b, a
			}
		}
	}
	return out, nil
}

func scanRemedy(r rowScanner) (Remedy, error) {
	var rm Remedy
	var reviewed, autoApprove int
	err := r.Scan(&rm.ID, &rm.AlertTypeID, &rm.Name, &rm.Kind, &rm.Content, &rm.Risk,
		&rm.Rollback, &rm.Source, &reviewed, &autoApprove, &rm.ExecCount, &rm.SuccessCount,
		&rm.CreatedAt, &rm.UpdatedAt)
	if err != nil {
		return Remedy{}, err
	}
	rm.Reviewed = reviewed == 1
	rm.AutoApprove = autoApprove == 1
	return rm, nil
}
