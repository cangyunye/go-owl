package handler

import (
	"context"
	"database/sql"
	"fmt"

	ai2 "github.com/cangyunye/go-owl/internal/ai"
	owlmonitor "github.com/cangyunye/go-owl/internal/monitor"
)

// WebExecutor 告警方法：经注入的 *owlmonitor.Store（与监控服务共享同一实例）
// 查询告警/告警类型/对策，渲染复用 internal/ai 的共享渲染函数，保证三端输出一致。

// SetMonitorStore 注入监控存储（server 装配时传入 monSvc.Store）。
func (e *WebExecutor) SetMonitorStore(st *owlmonitor.Store) {
	e.monitorStore = st
}

// ListAlerts 告警查询（只读，viewer 即可用；节点范围授权照常生效）。
func (e *WebExecutor) ListAlerts(ctx context.Context, p ai2.AlertListParams) (*ai2.AlertListResult, error) {
	if e.monitorStore == nil {
		return nil, fmt.Errorf("告警数据不可用：监控服务未启用")
	}
	codes, err := e.matchAlertTypeIDs(ctx, p.AlertTypeID, p.Category)
	if err != nil {
		return nil, err
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
		nodeID, err := e.nodeIDByRef(ctx, p.Node)
		if err != nil {
			return nil, err
		}
		filter.NodeID = nodeID
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	filter.Limit = limit

	alerts, err := e.monitorStore.ListAlerts(filter)
	if err != nil {
		return nil, fmt.Errorf("查询告警失败: %w", err)
	}
	total, err := e.monitorStore.CountAlerts(filter)
	if err != nil {
		return nil, fmt.Errorf("统计告警失败: %w", err)
	}

	rows, err := e.alertRowsForScope(ctx, alerts)
	if err != nil {
		return nil, err
	}
	return &ai2.AlertListResult{Text: ai2.RenderAlertList(rows, total)}, nil
}

// ListAlertTypes 列出全部告警码及触发规则。
func (e *WebExecutor) ListAlertTypes(ctx context.Context) (*ai2.AlertTypesResult, error) {
	if e.monitorStore == nil {
		return nil, fmt.Errorf("告警数据不可用：监控服务未启用")
	}
	types, err := e.monitorStore.ListAlertTypes()
	if err != nil {
		return nil, fmt.Errorf("查询告警类型失败: %w", err)
	}
	rows := make([]ai2.AlertTypeRow, 0, len(types))
	for _, at := range types {
		rows = append(rows, e.alertTypeRow(at))
	}
	return &ai2.AlertTypesResult{Text: ai2.RenderAlertTypes(rows)}, nil
}

// GetAlertRemedies 查询告警码的对策（SOP/脚本/剧本 + 回滚），只读不执行。
func (e *WebExecutor) GetAlertRemedies(ctx context.Context, p ai2.AlertRemedyParams) (*ai2.AlertRemedyResult, error) {
	if e.monitorStore == nil {
		return nil, fmt.Errorf("告警数据不可用：监控服务未启用")
	}
	remedies, err := e.monitorStore.ListRemedies(p.AlertTypeID)
	if err != nil {
		return nil, fmt.Errorf("查询告警对策失败: %w", err)
	}
	var typeRow *ai2.AlertTypeRow
	if at, ok, err := e.monitorStore.GetAlertType(p.AlertTypeID); err == nil && ok {
		row := e.alertTypeRow(at)
		typeRow = &row
	}
	remedyRows := make([]ai2.RemedyRow, 0, len(remedies))
	for _, r := range remedies {
		remedyRows = append(remedyRows, ai2.RemedyRow{
			ID: r.ID, Name: r.Name, Kind: r.Kind, Risk: r.Risk,
			Source: r.Source, Rollback: r.Rollback, Content: r.Content,
		})
	}
	return &ai2.AlertRemedyResult{Text: ai2.RenderRemedies(p.AlertTypeID, typeRow, remedyRows)}, nil
}

// matchAlertTypeIDs 将原始告警码/类别解析为具体告警码列表。
func (e *WebExecutor) matchAlertTypeIDs(ctx context.Context, rawCode, category string) ([]string, error) {
	types, err := e.monitorStore.ListAlertTypes()
	if err != nil {
		return nil, fmt.Errorf("查询告警类型失败: %w", err)
	}
	rows := make([]ai2.AlertTypeRow, 0, len(types))
	for _, at := range types {
		rows = append(rows, e.alertTypeRow(at))
	}
	return ai2.MatchAlertTypeCodes(rows, rawCode, category)
}

// nodeIDByRef 按 ID 或名称解析节点。
func (e *WebExecutor) nodeIDByRef(ctx context.Context, ref string) (string, error) {
	var id string
	err := e.db.QueryRowContext(ctx, `SELECT id FROM nodes WHERE id = ? OR name = ?`, ref, ref).Scan(&id)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("节点 %s 不存在", ref)
	}
	if err != nil {
		return "", fmt.Errorf("解析节点 %s 失败: %w", ref, err)
	}
	return id, nil
}

// alertRowsForScope 转 DTO 并套用用户节点范围授权（scope 内不可见的节点告警不展示）。
func (e *WebExecutor) alertRowsForScope(ctx context.Context, alerts []owlmonitor.Alert) ([]ai2.AlertRow, error) {
	ids := make([]string, 0, len(alerts))
	seen := map[string]bool{}
	for _, a := range alerts {
		if !seen[a.NodeID] {
			seen[a.NodeID] = true
			ids = append(ids, a.NodeID)
		}
	}
	allowed := NewScopeChecker(e.db).FilterNodeIDs(ctx, IdentityFromContext(ctx).Username, ids)
	allowedSet := make(map[string]bool, len(allowed))
	for _, id := range allowed {
		allowedSet[id] = true
	}
	names := e.nodeNames(ctx, ids)

	rows := make([]ai2.AlertRow, 0, len(alerts))
	for _, a := range alerts {
		if !allowedSet[a.NodeID] {
			continue
		}
		rows = append(rows, ai2.AlertRow{
			ID: a.ID, TypeID: a.AlertTypeID, NodeID: a.NodeID,
			NodeName: names[a.NodeID], Severity: string(a.Severity), Status: string(a.Status),
			Message: a.Message, FirstSeen: a.FirstSeen, LastSeen: a.LastSeen,
		})
	}
	return rows, nil
}

// nodeNames 批量取节点名（缺失时回退显示 ID）。
func (e *WebExecutor) nodeNames(ctx context.Context, ids []string) map[string]string {
	out := make(map[string]string, len(ids))
	for _, id := range ids {
		out[id] = id
	}
	stmt, err := e.db.PrepareContext(ctx, `SELECT name FROM nodes WHERE id = ?`)
	if err != nil {
		return out
	}
	defer func() { _ = stmt.Close() }()
	for _, id := range ids {
		var name string
		if err := stmt.QueryRowContext(ctx, id).Scan(&name); err == nil && name != "" {
			out[id] = name
		}
	}
	return out
}

func (e *WebExecutor) alertTypeRow(at owlmonitor.AlertType) ai2.AlertTypeRow {
	return ai2.AlertTypeRow{
		ID: at.ID, Category: at.Category, Name: at.Name,
		Description: at.Description, DefaultSeverity: string(at.DefaultSeverity),
		Rule: at.RuleDescription(), Enabled: at.Enabled,
	}
}
