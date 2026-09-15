package monitor

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
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
			builtin          INTEGER NOT NULL DEFAULT 1,
			check_cmd        TEXT NOT NULL DEFAULT '',
			check_mode       TEXT NOT NULL DEFAULT 'value',
			check_pattern    TEXT NOT NULL DEFAULT ''
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
	// 存量库迁移：自定义检查三列（列已存在时报 duplicate column，忽略）
	for _, alt := range []string{
		`ALTER TABLE alert_types ADD COLUMN check_cmd TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE alert_types ADD COLUMN check_mode TEXT NOT NULL DEFAULT 'value'`,
		`ALTER TABLE alert_types ADD COLUMN check_pattern TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := s.db.Exec(alt); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("monitor: 迁移 alert_types 失败: %w", err)
		}
	}
	return nil
}

// DeleteAlertType 删除非内置告警类型（builtin 类型拒绝），并级联删除其对策。
func (s *Store) DeleteAlertType(id string) error {
	if err := s.ensureAlertTables(); err != nil {
		return err
	}
	at, exists, err := s.GetAlertType(id)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("monitor: 告警类型 %s 不存在", id)
	}
	if at.Builtin {
		return fmt.Errorf("monitor: 内置告警类型 %s 不可删除", id)
	}
	if _, err := s.db.Exec(`DELETE FROM alert_types WHERE id = ?`, id); err != nil {
		return fmt.Errorf("monitor: 删除告警类型 %s 失败: %w", id, err)
	}
	if _, err := s.db.Exec(`DELETE FROM remedies WHERE alert_type_id = ?`, id); err != nil {
		return fmt.Errorf("monitor: 级联删除告警类型 %s 对策失败: %w", id, err)
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
		(id, category, name, description, default_severity, default_params, auto_approve, notifiable, enabled, builtin,
		 check_cmd, check_mode, check_pattern)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			category=excluded.category, name=excluded.name, description=excluded.description,
			default_severity=excluded.default_severity, default_params=excluded.default_params,
			auto_approve=excluded.auto_approve, notifiable=excluded.notifiable, enabled=excluded.enabled,
			check_cmd=excluded.check_cmd, check_mode=excluded.check_mode, check_pattern=excluded.check_pattern`,
		at.ID, at.Category, at.Name, at.Description, string(at.DefaultSeverity), string(params),
		boolInt(at.AutoApprove), boolInt(at.Notifiable), boolInt(at.Enabled), boolInt(at.Builtin),
		at.CheckCmd, at.CheckMode, at.CheckPattern)
	if err != nil {
		return fmt.Errorf("monitor: 写入告警类型 %s 失败: %w", at.ID, err)
	}
	return nil
}

// ListAlertTypes 列出全部告警类型。
func (s *Store) ListAlertTypes() ([]AlertType, error) {
	rows, err := s.db.Query(`SELECT id, category, name, description, default_severity, default_params,
		auto_approve, notifiable, enabled, builtin, check_cmd, check_mode, check_pattern FROM alert_types ORDER BY id`)
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
		auto_approve, notifiable, enabled, builtin, check_cmd, check_mode, check_pattern FROM alert_types WHERE id = ?`, id)
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
		auto_approve, notifiable, enabled, builtin, check_cmd, check_mode, check_pattern FROM alert_types WHERE enabled = 1 ORDER BY id`)
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
	var checkCmd, checkMode, checkPattern string
	err := r.Scan(&at.ID, &at.Category, &at.Name, &at.Description, &severity, &params,
		&autoApprove, &notifiable, &enabled, &builtin, &checkCmd, &checkMode, &checkPattern)
	if err != nil {
		return AlertType{}, err
	}
	at.DefaultSeverity = Severity(severity)
	at.AutoApprove = autoApprove == 1
	at.Notifiable = notifiable == 1
	at.Enabled = enabled == 1
	at.Builtin = builtin == 1
	at.CheckCmd = checkCmd
	at.CheckMode = checkMode
	at.CheckPattern = checkPattern
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
		severity=?, status=?, message=?, metric_snapshot=?, first_seen=?, last_seen=?, resolved_at=?, remedy_id=?
		WHERE id=?`,
		string(a.Severity), string(a.Status), a.Message, a.MetricSnapshot,
		a.FirstSeen, a.LastSeen, a.ResolvedAt, a.RemedyID, a.ID)
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

// GetLatestResolvedAlert 获取某节点某类型最近一次已解决的告警实例
//（重复告警合并窗口查找用）。
func (s *Store) GetLatestResolvedAlert(typeID, nodeID string) (*Alert, bool, error) {
	row := s.db.QueryRow(`SELECT id, alert_type_id, node_id, severity, status, message, metric_snapshot,
		first_seen, last_seen, resolved_at, remedy_id FROM alerts
		WHERE alert_type_id = ? AND node_id = ? AND status = 'resolved'
		ORDER BY resolved_at DESC LIMIT 1`, typeID, nodeID)
	a, err := scanAlert(row)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return a, true, nil
}

// alertFilterSQL 追加 AlertFilter 的 WHERE 条件（ListAlerts/CountAlerts 共用）。
func (s *Store) alertFilterSQL(f AlertFilter) (string, []any) {
	query := ` WHERE 1=1`
	var args []any
	if f.Status == "active" {
		query += ` AND status != 'resolved'`
	} else if f.Status != "" {
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
	if f.AlertTypeID != "" {
		query += ` AND alert_type_id = ?`
		args = append(args, f.AlertTypeID)
	}
	if groups := splitGroups(f.Group); len(groups) > 0 && s.hasNodesTable() {
		// 分组筛选：nodes.groups 为 JSON 数组，取与 nodes 页一致的 LIKE 匹配
		query += ` AND node_id IN (SELECT id FROM nodes WHERE 1=0`
		for _, g := range groups {
			query += ` OR groups LIKE ?`
			args = append(args, `%"`+g+`"%`)
		}
		query += `)`
	}
	return query, args
}

// splitGroups 拆分逗号分隔分组并去掉空白项。
func splitGroups(raw string) []string {
	var out []string
	for _, g := range strings.Split(raw, ",") {
		if g = strings.TrimSpace(g); g != "" {
			out = append(out, g)
		}
	}
	return out
}

// hasNodesTable 判断同库是否存在 nodes 表（独立打开 monitor 库时无此表，
// 分组筛选退化为不过滤）。
func (s *Store) hasNodesTable() bool {
	var name string
	err := s.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='nodes'`).Scan(&name)
	return err == nil && name == "nodes"
}

// ListAlerts 按筛选条件列出告警实例，级别降序 + 首次触发时间降序。
// Status 为 "active" 时筛选未解决实例；支持 Limit/Offset 分页。
func (s *Store) ListAlerts(f AlertFilter) ([]Alert, error) {
	where, args := s.alertFilterSQL(f)
	query := `SELECT id, alert_type_id, node_id, severity, status, message, metric_snapshot,
		first_seen, last_seen, resolved_at, remedy_id FROM alerts` + where
	query += ` ORDER BY CASE severity WHEN 'critical' THEN 0 WHEN 'warn' THEN 1 ELSE 2 END, first_seen DESC`
	if f.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, f.Limit)
		if f.Offset > 0 {
			query += ` OFFSET ?`
			args = append(args, f.Offset)
		}
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

// CountAlerts 按筛选条件统计告警数量（与 ListAlerts 同过滤语义）。
func (s *Store) CountAlerts(f AlertFilter) (int, error) {
	where, args := s.alertFilterSQL(f)
	query := `SELECT COUNT(*) FROM alerts` + where
	var n int
	if err := s.db.QueryRow(query, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// CleanupAlerts 清理已解决且解决时间超过 days 天的告警记录。
// 未解决（open/acked）告警永不删除；实例级绑定与执行记录级联清理。
func (s *Store) CleanupAlerts(days int) error {
	cutoff := time.Now().Unix() - int64(days)*86400
	_, err := s.db.Exec(`DELETE FROM alerts
		WHERE status = 'resolved' AND COALESCE(resolved_at, last_seen, first_seen) < ?`, cutoff)
	if err != nil {
		return fmt.Errorf("monitor: 清理过期告警记录失败: %w", err)
	}
	return s.cleanupOrphanAlertBindings()
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
