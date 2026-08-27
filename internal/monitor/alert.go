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
	ID             string
	AlertTypeID    string
	NodeID         string
	Severity       Severity
	Status         AlertStatus
	Message        string
	MetricSnapshot string // JSON：触发时指标快照
	FirstSeen      int64
	LastSeen       int64
	ResolvedAt     int64
	RemedyID       string
}

// AlertFilter 告警列表筛选。
type AlertFilter struct {
	Status   AlertStatus
	NodeID   string
	Severity string
	Limit    int
}

// NewAlertID 生成可读告警实例 ID：AL-<unix>-<序号>。
func NewAlertID(now int64, seq int) string {
	return fmt.Sprintf("AL-%d-%d", now, seq)
}

// IsActive 是否处于活跃状态（open/acked）。
func (a *Alert) IsActive() bool {
	return a.Status != StatusResolved
}
