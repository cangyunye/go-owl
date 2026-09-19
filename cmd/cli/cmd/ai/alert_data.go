package ai

import (
	"fmt"
	"sync"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	ai "github.com/cangyunye/go-owl/internal/ai"
	owlmonitor "github.com/cangyunye/go-owl/internal/monitor"
)

// owlAlertData CLI/TUI 侧告警数据适配：与 owl-serve 共享同一 owl.db（WAL 并发安全），
// 惰性打开 monitor 存储（幂等建表/种子，serve 未运行时也能安全初始化），
// 节点名→ID 解析经 common.NodeStore。

type owlAlertData struct {
	nodeStore common.NodeStore
	dbPath    string

	mu    sync.Mutex
	store *owlmonitor.Store
}

func newOwlAlertData(nodeStore common.NodeStore, dbPath string) *owlAlertData {
	return &owlAlertData{nodeStore: nodeStore, dbPath: dbPath}
}

// monitorStore 惰性打开共享监控库。
func (d *owlAlertData) monitorStore() (*owlmonitor.Store, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.store != nil {
		return d.store, nil
	}
	st, err := owlmonitor.OpenStore(d.dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开本地监控库失败: %w", err)
	}
	d.store = st
	return st, nil
}

// Close 关闭惰性打开的监控库（进程退出/测试清理用）。
func (d *owlAlertData) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.store == nil {
		return nil
	}
	err := d.store.Close()
	d.store = nil
	return err
}

// ListAlerts 实现 ai.AlertData：告警码归一/模糊匹配 + 节点名解析 + 过滤查询。
func (d *owlAlertData) ListAlerts(p ai.AlertListParams) ([]ai.AlertRow, int, error) {
	st, err := d.monitorStore()
	if err != nil {
		return nil, 0, err
	}
	codes, err := d.matchAlertTypeIDs(st, p.AlertTypeID, p.Category)
	if err != nil {
		return nil, 0, err
	}

	filter := owlmonitor.AlertFilter{
		Status:       owlmonitor.AlertStatus(p.Status),
		Severity:     p.Severity,
		Group:        p.Group,
		AlertTypeIDs: codes,
	}
	if filter.Status == "" {
		filter.Status = "active"
	}
	if p.Node != "" {
		nodeID, err := d.resolveNodeRef(p.Node)
		if err != nil {
			return nil, 0, err
		}
		filter.NodeID = nodeID
	}
	if p.Limit > 0 && p.Limit <= 500 {
		filter.Limit = p.Limit
	} else {
		filter.Limit = 50
	}

	alerts, err := st.ListAlerts(filter)
	if err != nil {
		return nil, 0, fmt.Errorf("查询告警失败: %w", err)
	}
	total, err := st.CountAlerts(filter)
	if err != nil {
		return nil, 0, fmt.Errorf("统计告警失败: %w", err)
	}

	names := d.nodeNameMap()
	rows := make([]ai.AlertRow, 0, len(alerts))
	for _, a := range alerts {
		rows = append(rows, ai.AlertRow{
			ID: a.ID, TypeID: a.AlertTypeID, NodeID: a.NodeID,
			NodeName: nodeNameOrDefault(names, a.NodeID), Severity: string(a.Severity),
			Status: string(a.Status), Message: a.Message,
			FirstSeen: a.FirstSeen, LastSeen: a.LastSeen,
		})
	}
	return rows, total, nil
}

// ListAlertTypes 实现 ai.AlertData。
func (d *owlAlertData) ListAlertTypes() ([]ai.AlertTypeRow, error) {
	st, err := d.monitorStore()
	if err != nil {
		return nil, err
	}
	types, err := st.ListAlertTypes()
	if err != nil {
		return nil, fmt.Errorf("查询告警类型失败: %w", err)
	}
	rows := make([]ai.AlertTypeRow, 0, len(types))
	for _, at := range types {
		rows = append(rows, alertTypeRow(at))
	}
	return rows, nil
}

// ListRemedies 实现 ai.AlertData。
func (d *owlAlertData) ListRemedies(alertTypeID string) ([]ai.RemedyRow, error) {
	st, err := d.monitorStore()
	if err != nil {
		return nil, err
	}
	remedies, err := st.ListRemedies(alertTypeID)
	if err != nil {
		return nil, fmt.Errorf("查询告警对策失败: %w", err)
	}
	rows := make([]ai.RemedyRow, 0, len(remedies))
	for _, r := range remedies {
		rows = append(rows, ai.RemedyRow{
			ID: r.ID, Name: r.Name, Kind: r.Kind, Risk: r.Risk,
			Source: r.Source, Rollback: r.Rollback, Content: r.Content,
		})
	}
	return rows, nil
}

// matchAlertTypeIDs 告警码/类别 → 具体告警码列表。
func (d *owlAlertData) matchAlertTypeIDs(st *owlmonitor.Store, rawCode, category string) ([]string, error) {
	types, err := st.ListAlertTypes()
	if err != nil {
		return nil, fmt.Errorf("查询告警类型失败: %w", err)
	}
	rows := make([]ai.AlertTypeRow, 0, len(types))
	for _, at := range types {
		rows = append(rows, alertTypeRow(at))
	}
	return ai.MatchAlertTypeCodes(rows, rawCode, category)
}

// resolveNodeRef 按 ID 或名称解析节点。
func (d *owlAlertData) resolveNodeRef(ref string) (string, error) {
	if n, err := d.nodeStore.Get(ref); err == nil && n != nil {
		return n.ID, nil
	}
	nodes, err := d.nodeStore.List()
	if err != nil {
		return "", fmt.Errorf("解析节点 %s 失败: %w", ref, err)
	}
	for _, n := range nodes {
		if n.Name == ref {
			return n.ID, nil
		}
	}
	return "", fmt.Errorf("节点 %s 不存在", ref)
}

// nodeNameMap 本地节点 ID → 名称映射。
func (d *owlAlertData) nodeNameMap() map[string]string {
	out := map[string]string{}
	nodes, err := d.nodeStore.List()
	if err != nil {
		return out
	}
	for _, n := range nodes {
		out[n.ID] = n.Name
	}
	return out
}

func nodeNameOrDefault(names map[string]string, nodeID string) string {
	if n := names[nodeID]; n != "" {
		return n
	}
	return nodeID
}

func alertTypeRow(at owlmonitor.AlertType) ai.AlertTypeRow {
	return ai.AlertTypeRow{
		ID: at.ID, Category: at.Category, Name: at.Name,
		Description: at.Description, DefaultSeverity: string(at.DefaultSeverity),
		Rule: at.RuleDescription(), Enabled: at.Enabled,
	}
}
