package monitor

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// NotifyChannel 通知渠道：邮件或自定义 Webhook。
type NotifyChannel struct {
	ID          string       `json:"id"`
	Kind        string       `json:"kind"` // email | webhook
	Name        string       `json:"name"`
	Config      NotifyConfig `json:"config"`
	SeverityMin Severity     `json:"severity_min"` // 最低通知级别
	AlertTypes  string       `json:"alert_types"`  // 空=全部；逗号分隔告警 ID
	Enabled     bool         `json:"enabled"`
	CreatedAt   int64        `json:"created_at"`
}

// NotifyConfig 渠道配置（按 Kind 取对应子结构），JSON 结构与存储一致。
type NotifyConfig struct {
	Email   *EmailConfig   `json:"email,omitempty"`
	Webhook *WebhookConfig `json:"webhook,omitempty"`
}

// EmailConfig SMTP 邮件配置。
// Encryption：ssl（隐式 TLS，465）| starttls | none；空则按端口推断。
type EmailConfig struct {
	SMTPHost   string   `json:"smtp_host"`
	SMTPPort   int      `json:"smtp_port"`
	Encryption string   `json:"encryption,omitempty"`
	Username   string   `json:"username"`
	Password   string   `json:"password"`
	From       string   `json:"from"`
	To         []string `json:"to"`
}

// WebhookConfig 自定义 Webhook 配置。
type WebhookConfig struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

// NotifyPayload 通知载荷（固定 JSON 结构，设计文档 8.3）。
type NotifyPayload struct {
	AlertID        string      `json:"alert_id"`
	AlertType      string      `json:"alert_type"`
	AlertTypeName  string      `json:"alert_type_name"`
	NodeID         string      `json:"node_id"`
	NodeName       string      `json:"node_name"`
	Severity       Severity    `json:"severity"`
	Status         AlertStatus `json:"status"`
	Message        string      `json:"message"`
	MetricSnapshot string      `json:"metric_snapshot"`
	FirstSeen      int64       `json:"first_seen"`
	WebURL         string      `json:"web_url"`
	// 以下字段仅合成事件使用（verify_failed 等）
	Event  string `json:"event,omitempty"`
	Detail string `json:"detail,omitempty"`
	RunID  string `json:"run_id,omitempty"`
}

func (c NotifyConfig) marshal() ([]byte, error) {
	return json.Marshal(c)
}

func (c *NotifyConfig) unmarshal(data []byte) error {
	return json.Unmarshal(data, c)
}

// EnsureNotifyTables 建通知渠道表（幂等）。
func (s *Store) EnsureNotifyTables() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS notify_channels (
		id           TEXT PRIMARY KEY,
		kind         TEXT NOT NULL,
		name         TEXT NOT NULL,
		config       TEXT NOT NULL,
		severity_min TEXT NOT NULL DEFAULT 'warn',
		alert_types  TEXT NOT NULL DEFAULT '',
		enabled      INTEGER NOT NULL DEFAULT 1,
		created_at   INTEGER NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("monitor: 建通知渠道表失败: %w", err)
	}
	return nil
}

// UpsertNotifyChannel 写入或更新通知渠道。
func (s *Store) UpsertNotifyChannel(ch NotifyChannel) error {
	if err := s.EnsureNotifyTables(); err != nil {
		return err
	}
	if ch.CreatedAt == 0 {
		ch.CreatedAt = time.Now().Unix()
	}
	cfg, err := ch.Config.marshal()
	if err != nil {
		return fmt.Errorf("monitor: 序列化渠道配置失败: %w", err)
	}
	_, err = s.db.Exec(`INSERT INTO notify_channels
		(id, kind, name, config, severity_min, alert_types, enabled, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			kind=excluded.kind, name=excluded.name, config=excluded.config,
			severity_min=excluded.severity_min, alert_types=excluded.alert_types,
			enabled=excluded.enabled`,
		ch.ID, ch.Kind, ch.Name, string(cfg), string(ch.SeverityMin), ch.AlertTypes,
		boolInt(ch.Enabled), ch.CreatedAt)
	if err != nil {
		return fmt.Errorf("monitor: 写入通知渠道 %s 失败: %w", ch.ID, err)
	}
	return nil
}

// GetNotifyChannel 按 ID 获取通知渠道。
func (s *Store) GetNotifyChannel(id string) (NotifyChannel, bool, error) {
	row := s.db.QueryRow(`SELECT id, kind, name, config, severity_min, alert_types, enabled, created_at
		FROM notify_channels WHERE id = ?`, id)
	ch, err := scanNotifyChannel(row)
	if err == sql.ErrNoRows {
		return NotifyChannel{}, false, nil
	}
	if err != nil {
		return NotifyChannel{}, false, err
	}
	return ch, true, nil
}

// ListNotifyChannels 列出全部通知渠道。
func (s *Store) ListNotifyChannels() ([]NotifyChannel, error) {
	if err := s.EnsureNotifyTables(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id, kind, name, config, severity_min, alert_types, enabled, created_at
		FROM notify_channels ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []NotifyChannel
	for rows.Next() {
		ch, err := scanNotifyChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

// DeleteNotifyChannel 删除通知渠道。
func (s *Store) DeleteNotifyChannel(id string) error {
	if _, err := s.db.Exec(`DELETE FROM notify_channels WHERE id = ?`, id); err != nil {
		return fmt.Errorf("monitor: 删除通知渠道失败: %w", err)
	}
	return nil
}

func scanNotifyChannel(r rowScanner) (NotifyChannel, error) {
	var ch NotifyChannel
	var cfg string
	var enabled int
	err := r.Scan(&ch.ID, &ch.Kind, &ch.Name, &cfg, &ch.SeverityMin, &ch.AlertTypes,
		&enabled, &ch.CreatedAt)
	if err != nil {
		return NotifyChannel{}, err
	}
	ch.Enabled = enabled == 1
	if err := ch.Config.unmarshal([]byte(cfg)); err != nil {
		return NotifyChannel{}, fmt.Errorf("monitor: 解析渠道 %s 配置失败: %w", ch.ID, err)
	}
	return ch, nil
}

// ChannelMatches 判断渠道是否应接收某告警：启用、级别达标、类型白名单匹配。
func ChannelMatches(ch NotifyChannel, severity Severity, alertTypeID string) bool {
	if !ch.Enabled {
		return false
	}
	if SeverityRank(severity) < SeverityRank(ch.SeverityMin) {
		return false
	}
	if ch.AlertTypes == "" {
		return true
	}
	for _, id := range strings.Split(ch.AlertTypes, ",") {
		if strings.TrimSpace(id) == alertTypeID {
			return true
		}
	}
	return false
}
