package monitor

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// ensureAlertTables 建告警相关表（幂等）。
func (s *Store) ensureAlertTables() error {
	schemas := []string{
		`CREATE TABLE IF NOT EXISTS alert_types (
			id               TEXT PRIMARY KEY,
			category         TEXT NOT NULL,
			name             TEXT NOT NULL,
			description      TEXT NOT NULL DEFAULT '',
			default_severity TEXT NOT NULL,
			default_params   TEXT NOT NULL DEFAULT '{}',
			auto_approve     INTEGER NOT NULL DEFAULT 0,
			notifiable       INTEGER NOT NULL DEFAULT 1,
			enabled          INTEGER NOT NULL DEFAULT 1,
			builtin          INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE TABLE IF NOT EXISTS alerts (
			id              TEXT PRIMARY KEY,
			alert_type_id   TEXT NOT NULL,
			node_id         TEXT NOT NULL,
			severity        TEXT NOT NULL,
			status          TEXT NOT NULL DEFAULT 'open',
			message         TEXT NOT NULL,
			metric_snapshot TEXT NOT NULL DEFAULT '{}',
			first_seen      INTEGER NOT NULL,
			last_seen       INTEGER NOT NULL,
			resolved_at     INTEGER NOT NULL DEFAULT 0,
			remedy_id       TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_alerts_active
			ON alerts(alert_type_id, node_id) WHERE status != 'resolved'`,
	}
	for _, q := range schemas {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("monitor: 建告警表失败: %w", err)
		}
	}
	return nil
}

// SeedAlertTypesIfEmpty 首次打开时写入内置告警类型注册表（幂等）。
func (s *Store) SeedAlertTypesIfEmpty() error {
	if err := s.ensureAlertTables(); err != nil {
		return err
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM alert_types`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	for _, at := range BuiltinAlertTypes() {
		if err := s.UpsertAlertType(at); err != nil {
			return err
		}
	}
	return nil
}

// UpsertAlertType 写入或更新告警类型（含阈值/开关/放行配置）。
func (s *Store) UpsertAlertType(at AlertType) error {
	params, err := json.Marshal(at.DefaultParams)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO alert_types
		(id, category, name, description, default_severity, default_params, auto_approve, notifiable, enabled, builtin)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			category=excluded.category, name=excluded.name, description=excluded.description,
			default_severity=excluded.default_severity, default_params=excluded.default_params,
			auto_approve=excluded.auto_approve, notifiable=excluded.notifiable, enabled=excluded.enabled`,
		at.ID, at.Category, at.Name, at.Description, string(at.DefaultSeverity), string(params),
		boolInt(at.AutoApprove), boolInt(at.Notifiable), boolInt(at.Enabled), boolInt(at.Builtin))
	if err != nil {
		return fmt.Errorf("monitor: 写入告警类型 %s 失败: %w", at.ID, err)
	}
	return nil
}

// ListAlertTypes 列出全部告警类型。
func (s *Store) ListAlertTypes() ([]AlertType, error) {
	rows, err := s.db.Query(`SELECT id, category, name, description, default_severity, default_params,
		auto_approve, notifiable, enabled, builtin FROM alert_types ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []AlertType
	for rows.Next() {
		at, err := scanAlertType(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, at)
	}
	return out, rows.Err()
}

// GetAlertType 按 ID 获取告警类型。
func (s *Store) GetAlertType(id string) (AlertType, bool, error) {
	row := s.db.QueryRow(`SELECT id, category, name, description, default_severity, default_params,
		auto_approve, notifiable, enabled, builtin FROM alert_types WHERE id = ?`, id)
	at, err := scanAlertType(row)
	if err == sql.ErrNoRows {
		return AlertType{}, false, nil
	}
	if err != nil {
		return AlertType{}, false, err
	}
	return at, true, nil
}

// ListEnabledAlertTypes 列出启用的告警类型。
func (s *Store) ListEnabledAlertTypes() ([]AlertType, error) {
	rows, err := s.db.Query(`SELECT id, category, name, description, default_severity, default_params,
		auto_approve, notifiable, enabled, builtin FROM alert_types WHERE enabled = 1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []AlertType
	for rows.Next() {
		at, err := scanAlertType(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, at)
	}
	return out, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAlertType(r rowScanner) (AlertType, error) {
	var at AlertType
	var severity, params string
	var autoApprove, notifiable, enabled, builtin int
	err := r.Scan(&at.ID, &at.Category, &at.Name, &at.Description, &severity, &params,
		&autoApprove, &notifiable, &enabled, &builtin)
	if err != nil {
		return AlertType{}, err
	}
	at.DefaultSeverity = Severity(severity)
	at.AutoApprove = autoApprove == 1
	at.Notifiable = notifiable == 1
	at.Enabled = enabled == 1
	at.Builtin = builtin == 1
	if err := json.Unmarshal([]byte(params), &at.DefaultParams); err != nil {
		return AlertType{}, fmt.Errorf("monitor: 解析 %s 规则参数失败: %w", at.ID, err)
	}
	return at, nil
}

// InsertAlert 插入告警实例。同 (类型, 节点) 已有活跃实例时违反唯一索引返回错误。
func (s *Store) InsertAlert(a *Alert) error {
	_, err := s.db.Exec(`INSERT INTO alerts
		(id, alert_type_id, node_id, severity, status, message, metric_snapshot,
		 first_seen, last_seen, resolved_at, remedy_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.AlertTypeID, a.NodeID, string(a.Severity), string(a.Status), a.Message,
		a.MetricSnapshot, a.FirstSeen, a.LastSeen, a.ResolvedAt, a.RemedyID)
	if err != nil {
		return fmt.Errorf("monitor: 插入告警失败: %w", err)
	}
	return nil
}

// UpdateAlert 按 ID 更新告警实例（状态/级别/时间/快照）。
func (s *Store) UpdateAlert(a *Alert) error {
	res, err := s.db.Exec(`UPDATE alerts SET
		severity=?, status=?, message=?, metric_snapshot=?, last_seen=?, resolved_at=?, remedy_id=?
		WHERE id=?`,
		string(a.Severity), string(a.Status), a.Message, a.MetricSnapshot,
		a.LastSeen, a.ResolvedAt, a.RemedyID, a.ID)
	if err != nil {
		return fmt.Errorf("monitor: 更新告警失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("monitor: 告警 %s 不存在", a.ID)
	}
	return nil
}

// GetAlert 按 ID 获取告警实例。
func (s *Store) GetAlert(id string) (*Alert, bool, error) {
	row := s.db.QueryRow(`SELECT id, alert_type_id, node_id, severity, status, message, metric_snapshot,
		first_seen, last_seen, resolved_at, remedy_id FROM alerts WHERE id = ?`, id)
	a, err := scanAlert(row)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return a, true, nil
}

// GetActiveAlert 获取某节点某类型的活跃告警实例（去重查找）。
func (s *Store) GetActiveAlert(typeID, nodeID string) (*Alert, bool, error) {
	row := s.db.QueryRow(`SELECT id, alert_type_id, node_id, severity, status, message, metric_snapshot,
		first_seen, last_seen, resolved_at, remedy_id FROM alerts
		WHERE alert_type_id = ? AND node_id = ? AND status != 'resolved'`, typeID, nodeID)
	a, err := scanAlert(row)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return a, true, nil
}

// ListAlerts 按筛选条件列出告警实例，级别降序 + 首次触发时间降序。
func (s *Store) ListAlerts(f AlertFilter) ([]Alert, error) {
	query := `SELECT id, alert_type_id, node_id, severity, status, message, metric_snapshot,
		first_seen, last_seen, resolved_at, remedy_id FROM alerts WHERE 1=1`
	var args []any
	if f.Status != "" {
		query += ` AND status = ?`
		args = append(args, string(f.Status))
	}
	if f.NodeID != "" {
		query += ` AND node_id = ?`
		args = append(args, f.NodeID)
	}
	if f.Severity != "" {
		query += ` AND severity = ?`
		args = append(args, f.Severity)
	}
	query += ` ORDER BY CASE severity WHEN 'critical' THEN 0 WHEN 'warn' THEN 1 ELSE 2 END, first_seen DESC`
	if f.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, f.Limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []Alert
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func scanAlert(r rowScanner) (*Alert, error) {
	var a Alert
	var severity, status string
	err := r.Scan(&a.ID, &a.AlertTypeID, &a.NodeID, &severity, &status, &a.Message,
		&a.MetricSnapshot, &a.FirstSeen, &a.LastSeen, &a.ResolvedAt, &a.RemedyID)
	if err != nil {
		return nil, err
	}
	a.Severity = Severity(severity)
	a.Status = AlertStatus(status)
	return &a, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
