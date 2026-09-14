package monitor

import "fmt"

// AlertStatus 告警实例状态。
type AlertStatus string

const (
	StatusOpen     AlertStatus = "open"
	StatusAcked    AlertStatus = "acked"
	StatusResolved AlertStatus = "resolved"
)

// Alert 告警实例：某节点上某告警类型的一次实际告警。
type Alert struct {
	ID             string      `json:"id"`
	AlertTypeID    string      `json:"alert_type_id"`
	NodeID         string      `json:"node_id"`
	Severity       Severity    `json:"severity"`
	Status         AlertStatus `json:"status"`
	Message        string      `json:"message"`
	MetricSnapshot string      `json:"metric_snapshot"` // JSON：触发时指标快照
	FirstSeen      int64       `json:"first_seen"`
	LastSeen       int64       `json:"last_seen"`
	ResolvedAt     int64       `json:"resolved_at"`
	RemedyID       string      `json:"remedy_id"`
}

// AlertFilter 告警列表筛选。Status 为 "active" 时表示活跃（未解决）。
// Group 为逗号分隔的节点分组（任一命中），依赖同库 nodes 表。
type AlertFilter struct {
	Status      AlertStatus
	NodeID      string
	Severity    string
	AlertTypeID string
	Group       string
	Limit       int
	Offset      int
}

// NewAlertID 生成可读告警实例 ID：AL-<unix>-<序号>。
func NewAlertID(now int64, seq int) string {
	return fmt.Sprintf("AL-%d-%d", now, seq)
}

// IsActive 是否处于活跃状态（open/acked）。
func (a *Alert) IsActive() bool {
	return a.Status != StatusResolved
}
