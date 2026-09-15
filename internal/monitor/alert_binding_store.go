package monitor

import (
	"database/sql"
	"fmt"
	"time"
)

// AlertBinding 告警实例专属处置指令绑定：对某条具体告警（按告警 ID）
// 指定 playbook 或脚本。告警重开（合并窗口）时绑定保留，
// 告警被保留期清理时级联删除。
type AlertBinding struct {
	ID        string `json:"id"`
	AlertID   string `json:"alert_id"`
	Kind      string `json:"kind"`      // script | playbook
	Name      string `json:"name"`
	Content   string `json:"content"`   // script: 命令文本；playbook: 剧本 ID
	Risk      string `json:"risk"`      // low | medium | high
	AutoExec  bool   `json:"auto_exec"` // 告警打开/重开时自动执行
	ExecMode  string `json:"exec_mode"` // sequential | concurrent（自动执行分组用）
	Seq       int    `json:"seq"`       // 执行顺序
	CreatedBy string `json:"created_by"`
	CreatedAt int64  `json:"created_at"`
}

// AlertBindingRun 绑定执行记录：关联一次 remedy_run（script）或
// playbook_run（playbook），供告警详情展示执行历史与状态。
type AlertBindingRun struct {
	ID         string `json:"id"`
	AlertID    string `json:"alert_id"`
	BindingID  string `json:"binding_id"`
	Kind       string `json:"kind"`
	RefID      string `json:"ref_id"` // remedy_run ID 或 playbook_run ID
	Mode       string `json:"mode"`   // sequential | concurrent
	Status     string `json:"status"` // pending | running | success | failed
	Err        string `json:"err,omitempty"`
	CreatedBy  string `json:"created_by"`
	CreatedAt  int64  `json:"created_at"`
	FinishedAt int64  `json:"finished_at,omitempty"`
}

// EnsureAlertBindingTables 建告警绑定与执行记录表（幂等）。
func (s *Store) EnsureAlertBindingTables() error {
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS alert_bindings (
			id         TEXT PRIMARY KEY,
			alert_id   TEXT NOT NULL,
			kind       TEXT NOT NULL,
			name       TEXT NOT NULL,
			content    TEXT NOT NULL,
			risk       TEXT NOT NULL DEFAULT 'medium',
			auto_exec  INTEGER NOT NULL DEFAULT 0,
			exec_mode  TEXT NOT NULL DEFAULT 'sequential',
			seq        INTEGER NOT NULL DEFAULT 0,
			created_by TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_alert_bindings_alert ON alert_bindings(alert_id, seq)`,
		`CREATE TABLE IF NOT EXISTS alert_binding_runs (
			id          TEXT PRIMARY KEY,
			alert_id    TEXT NOT NULL,
			binding_id  TEXT NOT NULL,
			kind        TEXT NOT NULL,
			ref_id      TEXT NOT NULL DEFAULT '',
			mode        TEXT NOT NULL DEFAULT 'sequential',
			status      TEXT NOT NULL DEFAULT 'pending',
			err         TEXT NOT NULL DEFAULT '',
			created_by  TEXT NOT NULL DEFAULT '',
			created_at  INTEGER NOT NULL,
			finished_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_alert_binding_runs_alert ON alert_binding_runs(alert_id, created_at)`,
	} {
		if _, err := s.db.Exec(ddl); err != nil {
			return fmt.Errorf("monitor: 建告警绑定表失败: %w", err)
		}
	}
	return nil
}

// CreateAlertBinding 新增绑定。
func (s *Store) CreateAlertBinding(b AlertBinding) error {
	if err := s.EnsureAlertBindingTables(); err != nil {
		return err
	}
	if b.CreatedAt == 0 {
		b.CreatedAt = time.Now().Unix()
	}
	_, err := s.db.Exec(`INSERT INTO alert_bindings
		(id, alert_id, kind, name, content, risk, auto_exec, exec_mode, seq, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.ID, b.AlertID, b.Kind, b.Name, b.Content, b.Risk,
		boolInt(b.AutoExec), b.ExecMode, b.Seq, b.CreatedBy, b.CreatedAt)
	if err != nil {
		return fmt.Errorf("monitor: 写入告警绑定 %s 失败: %w", b.ID, err)
	}
	return nil
}

// UpdateAlertBinding 按 ID 全量更新可编辑字段（名称/内容/风险/自动执行/模式/顺序）。
func (s *Store) UpdateAlertBinding(b AlertBinding) error {
	if err := s.EnsureAlertBindingTables(); err != nil {
		return err
	}
	res, err := s.db.Exec(`UPDATE alert_bindings SET
		name=?, content=?, risk=?, auto_exec=?, exec_mode=?, seq=? WHERE id=?`,
		b.Name, b.Content, b.Risk, boolInt(b.AutoExec), b.ExecMode, b.Seq, b.ID)
	if err != nil {
		return fmt.Errorf("monitor: 更新告警绑定 %s 失败: %w", b.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("monitor: 告警绑定 %s 不存在", b.ID)
	}
	return nil
}

// DeleteAlertBinding 删除绑定。
func (s *Store) DeleteAlertBinding(id string) error {
	if err := s.EnsureAlertBindingTables(); err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM alert_bindings WHERE id = ?`, id); err != nil {
		return fmt.Errorf("monitor: 删除告警绑定 %s 失败: %w", id, err)
	}
	return nil
}

// GetAlertBinding 按 ID 获取绑定。
func (s *Store) GetAlertBinding(id string) (AlertBinding, bool, error) {
	if err := s.EnsureAlertBindingTables(); err != nil {
		return AlertBinding{}, false, err
	}
	row := s.db.QueryRow(`SELECT id, alert_id, kind, name, content, risk, auto_exec, exec_mode, seq, created_by, created_at
		FROM alert_bindings WHERE id = ?`, id)
	b, err := scanAlertBinding(row)
	if err == sql.ErrNoRows {
		return AlertBinding{}, false, nil
	}
	if err != nil {
		return AlertBinding{}, false, err
	}
	return *b, true, nil
}

// ListAlertBindings 列出某告警的全部绑定（按 seq 升序）。
func (s *Store) ListAlertBindings(alertID string) ([]AlertBinding, error) {
	if err := s.EnsureAlertBindingTables(); err != nil {
		return nil, err
	}
	return s.queryAlertBindings(`SELECT id, alert_id, kind, name, content, risk, auto_exec, exec_mode, seq, created_by, created_at
		FROM alert_bindings WHERE alert_id = ? ORDER BY seq, id`, alertID)
}

// ListAutoAlertBindings 列出某告警配置了自动执行的绑定（按 seq 升序）。
func (s *Store) ListAutoAlertBindings(alertID string) ([]AlertBinding, error) {
	if err := s.EnsureAlertBindingTables(); err != nil {
		return nil, err
	}
	return s.queryAlertBindings(`SELECT id, alert_id, kind, name, content, risk, auto_exec, exec_mode, seq, created_by, created_at
		FROM alert_bindings WHERE alert_id = ? AND auto_exec = 1 ORDER BY seq, id`, alertID)
}

func (s *Store) queryAlertBindings(query string, args ...any) ([]AlertBinding, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []AlertBinding
	for rows.Next() {
		b, err := scanAlertBinding(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

func scanAlertBinding(r rowScanner) (*AlertBinding, error) {
	var b AlertBinding
	var auto int
	err := r.Scan(&b.ID, &b.AlertID, &b.Kind, &b.Name, &b.Content, &b.Risk,
		&auto, &b.ExecMode, &b.Seq, &b.CreatedBy, &b.CreatedAt)
	if err != nil {
		return nil, err
	}
	b.AutoExec = auto == 1
	return &b, nil
}

// CreateAlertBindingRun 写入绑定执行记录（初始 pending/running）。
func (s *Store) CreateAlertBindingRun(r *AlertBindingRun) error {
	if err := s.EnsureAlertBindingTables(); err != nil {
		return err
	}
	if r.CreatedAt == 0 {
		r.CreatedAt = time.Now().Unix()
	}
	if r.Status == "" {
		r.Status = "pending"
	}
	_, err := s.db.Exec(`INSERT INTO alert_binding_runs
		(id, alert_id, binding_id, kind, ref_id, mode, status, err, created_by, created_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.AlertID, r.BindingID, r.Kind, r.RefID, r.Mode, r.Status,
		r.Err, r.CreatedBy, r.CreatedAt, r.FinishedAt)
	if err != nil {
		return fmt.Errorf("monitor: 写入绑定执行记录 %s 失败: %w", r.ID, err)
	}
	return nil
}

// UpdateAlertBindingRunRef 回填关联的 remedy_run / playbook_run ID 并置为 running。
func (s *Store) UpdateAlertBindingRunRef(id, refID string) error {
	if err := s.EnsureAlertBindingTables(); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE alert_binding_runs SET ref_id = ?, status = 'running' WHERE id = ?`, refID, id)
	if err != nil {
		return fmt.Errorf("monitor: 回写绑定运行 %s 失败: %w", id, err)
	}
	return nil
}

// FinishAlertBindingRun 将执行记录置为终态（success/failed）。
func (s *Store) FinishAlertBindingRun(id, status, errMsg string) error {
	if err := s.EnsureAlertBindingTables(); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE alert_binding_runs SET status = ?, err = ?, finished_at = ? WHERE id = ?`,
		status, errMsg, time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("monitor: 终结绑定运行 %s 失败: %w", id, err)
	}
	return nil
}

// FailStaleAlertBindingRuns 启动对账：服务重启导致执行 goroutine 丢失，
// 将悬挂的 pending/running 记录标记为失败，避免永久停留中间态。
func (s *Store) FailStaleAlertBindingRuns() error {
	if err := s.EnsureAlertBindingTables(); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE alert_binding_runs
		SET status = 'failed', err = '服务重启，执行中断', finished_at = ?
		WHERE status IN ('pending', 'running')`, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("monitor: 对账悬挂绑定运行失败: %w", err)
	}
	return nil
}

// ListAlertBindingRuns 列出某告警的绑定执行记录（按创建时间倒序）。
func (s *Store) ListAlertBindingRuns(alertID string) ([]AlertBindingRun, error) {
	if err := s.EnsureAlertBindingTables(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id, alert_id, binding_id, kind, ref_id, mode, status, err, created_by, created_at, finished_at
		FROM alert_binding_runs WHERE alert_id = ? ORDER BY created_at DESC, id`, alertID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []AlertBindingRun
	for rows.Next() {
		var r AlertBindingRun
		var finished int64
		if err := rows.Scan(&r.ID, &r.AlertID, &r.BindingID, &r.Kind, &r.RefID, &r.Mode,
			&r.Status, &r.Err, &r.CreatedBy, &r.CreatedAt, &finished); err != nil {
			return nil, err
		}
		r.FinishedAt = finished
		out = append(out, r)
	}
	return out, rows.Err()
}

// cleanupOrphanAlertBindings 级联清理已不存在的告警的绑定与执行记录。
func (s *Store) cleanupOrphanAlertBindings() error {
	if err := s.EnsureAlertBindingTables(); err != nil {
		return err
	}
	for _, tbl := range []string{"alert_bindings", "alert_binding_runs"} {
		if _, err := s.db.Exec(`DELETE FROM ` + tbl + ` WHERE alert_id NOT IN (SELECT id FROM alerts)`); err != nil {
			return fmt.Errorf("monitor: 级联清理 %s 失败: %w", tbl, err)
		}
	}
	return nil
}
