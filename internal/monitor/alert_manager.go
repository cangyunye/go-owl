package monitor

import (
	"encoding/json"
	"fmt"
	"time"
)

// AlertEventType 告警生命周期事件（供通知等下游消费）。
type AlertEventType string

const (
	EventOpened    AlertEventType = "opened"
	EventResolved  AlertEventType = "resolved"
	EventEscalated AlertEventType = "escalated"
)

// AlertEvent 一次告警状态变化。
type AlertEvent struct {
	Type  AlertEventType
	Alert *Alert
}

// AlertManager 告警生命周期管理器：开/刷新（去重）/自动恢复/升级/失联。
// 规则评估由 RuleEngine 完成，管理器负责持久化与状态转换。
type AlertManager struct {
	store            *Store
	engine           *RuleEngine
	recoverCounts    map[string]int // node|typeID → 连续未命中次数
	failCounts       map[string]int // nodeID → 连续采集失败次数
	recoverThreshold int            // 恢复所需连续未命中采样数，默认 3
	escalateAfter    time.Duration  // warn 未处理升级时长，默认 1h
	failThreshold    int            // 失联阈值（连续失败次数），默认 3
	now              func() int64
	seq              int
}

// NewAlertManager 创建告警管理器。
func NewAlertManager(store *Store) *AlertManager {
	return &AlertManager{
		store:            store,
		engine:           NewRuleEngine(),
		recoverCounts:    make(map[string]int),
		failCounts:       make(map[string]int),
		recoverThreshold: 3,
		escalateAfter:    time.Hour,
		failThreshold:    3,
		now:              func() int64 { return time.Now().Unix() },
	}
}

// Tick 对单节点推进一轮：评估规则 → 开/刷新告警 → 恢复检测 → 升级检查。
// types 为当前启用的告警类型（调用方从存储加载）。
func (m *AlertManager) Tick(nodeID string, samples []Sample, types []AlertType) ([]AlertEvent, error) {
	out := m.engine.Tick(nodeID, samples, types)
	now := m.now()
	var events []AlertEvent

	typeByID := make(map[string]AlertType, len(types))
	for _, at := range types {
		typeByID[at.ID] = at
	}

	// 1. 触发 → 开/刷新（活跃去重）
	for _, hit := range out.Triggered {
		at, ok := typeByID[hit.AlertTypeID]
		if !ok {
			continue
		}
		key := ruleKey(nodeID, at.ID)
		existing, exists, err := m.store.GetActiveAlert(at.ID, nodeID)
		if err != nil {
			return events, err
		}
		if exists {
			existing.LastSeen = now
			existing.Message = buildAlertMessage(at, hit)
			existing.MetricSnapshot = snapshotJSON(hit)
			if err := m.store.UpdateAlert(existing); err != nil {
				return events, err
			}
			delete(m.recoverCounts, key)
			continue
		}
		al := &Alert{
			ID:             NewAlertID(now, m.seq),
			AlertTypeID:    at.ID,
			NodeID:         nodeID,
			Severity:       at.DefaultSeverity,
			Status:         StatusOpen,
			Message:        buildAlertMessage(at, hit),
			MetricSnapshot: snapshotJSON(hit),
			FirstSeen:      now,
			LastSeen:       now,
		}
		m.seq++
		if err := m.store.InsertAlert(al); err != nil {
			return events, err
		}
		events = append(events, AlertEvent{Type: EventOpened, Alert: al})
	}

	// 2. 恢复检测：活跃告警的类型当前未满足 → 计数；达阈值 → 解决
	all, err := m.store.ListAlerts(AlertFilter{NodeID: nodeID})
	if err != nil {
		return events, err
	}
	for i := range all {
		a := &all[i]
		if !a.IsActive() {
			continue
		}
		at, ok := typeByID[a.AlertTypeID]
		if !ok || !at.Enabled {
			continue // 类型被禁用：不自动恢复
		}
		key := ruleKey(nodeID, a.AlertTypeID)
		if out.Matched[a.AlertTypeID] {
			delete(m.recoverCounts, key)
			continue
		}
		m.recoverCounts[key]++
		if m.recoverCounts[key] < m.recoverThreshold {
			continue
		}
		a.Status = StatusResolved
		a.ResolvedAt = now
		if err := m.store.UpdateAlert(a); err != nil {
			return events, err
		}
		delete(m.recoverCounts, key)
		events = append(events, AlertEvent{Type: EventResolved, Alert: a})
	}

	// 3. 升级检查：open 的 warn 告警超过 escalateAfter 未处理 → critical
	for i := range all {
		a := &all[i]
		if a.Status != StatusOpen || a.Severity != SeverityWarning {
			continue
		}
		if now-a.FirstSeen > int64(m.escalateAfter.Seconds()) {
			a.Severity = SeverityCritical
			if err := m.store.UpdateAlert(a); err != nil {
				return events, err
			}
			events = append(events, AlertEvent{Type: EventEscalated, Alert: a})
		}
	}

	return events, nil
}

// MarkCollectFail 节点采集失败一次；连续失败达阈值时开 OWL-OSS-001 失联告警。
func (m *AlertManager) MarkCollectFail(nodeID string) ([]AlertEvent, error) {
	m.failCounts[nodeID]++
	if m.failCounts[nodeID] < m.failThreshold {
		return nil, nil
	}
	_, exists, err := m.store.GetActiveAlert("OWL-OSS-001", nodeID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, nil // 已处于失联告警
	}
	al := &Alert{
		ID:             NewAlertID(m.now(), m.seq),
		AlertTypeID:    "OWL-OSS-001",
		NodeID:         nodeID,
		Severity:       SeverityCritical,
		Status:         StatusOpen,
		Message:        fmt.Sprintf("节点 SSH 连续 %d 次采集失败（失联）", m.failCounts[nodeID]),
		MetricSnapshot: "{}",
		FirstSeen:      m.now(),
		LastSeen:       m.now(),
	}
	m.seq++
	if err := m.store.InsertAlert(al); err != nil {
		return nil, err
	}
	return []AlertEvent{{Type: EventOpened, Alert: al}}, nil
}

// MarkCollectOK 节点采集成功：清除失败计数并解决失联告警。
func (m *AlertManager) MarkCollectOK(nodeID string) ([]AlertEvent, error) {
	delete(m.failCounts, nodeID)
	m.engine.Reset(nodeID)

	existing, exists, err := m.store.GetActiveAlert("OWL-OSS-001", nodeID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	existing.Status = StatusResolved
	existing.ResolvedAt = m.now()
	if err := m.store.UpdateAlert(existing); err != nil {
		return nil, err
	}
	return []AlertEvent{{Type: EventResolved, Alert: existing}}, nil
}

// Ack 人工确认告警（open → acked）。
func (m *AlertManager) Ack(id string) (*Alert, error) {
	a, exists, err := m.store.GetAlert(id)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("monitor: 告警 %s 不存在", id)
	}
	if a.Status == StatusResolved {
		return nil, fmt.Errorf("monitor: 已解决的告警不能确认")
	}
	a.Status = StatusAcked
	if err := m.store.UpdateAlert(a); err != nil {
		return nil, err
	}
	return a, nil
}

// Resolve 人工解决告警（→ resolved）。
func (m *AlertManager) Resolve(id string) (*Alert, error) {
	a, exists, err := m.store.GetAlert(id)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("monitor: 告警 %s 不存在", id)
	}
	a.Status = StatusResolved
	a.ResolvedAt = m.now()
	if err := m.store.UpdateAlert(a); err != nil {
		return nil, err
	}
	return a, nil
}

// buildAlertMessage 构造中文告警描述。
func buildAlertMessage(at AlertType, hit EvalResult) string {
	if at.DefaultParams.Op == "" {
		return fmt.Sprintf("%s：%s 当前 %.2f", at.Name, hit.Metric, hit.Value)
	}
	return fmt.Sprintf("%s：%s 当前 %.2f（规则 %s %v）", at.Name, hit.Metric, hit.Value,
		at.DefaultParams.Op, at.DefaultParams.Value)
}

// snapshotJSON 构造指标快照 JSON。
func snapshotJSON(hit EvalResult) string {
	data, _ := json.Marshal(map[string]float64{hit.Metric: hit.Value})
	return string(data)
}
