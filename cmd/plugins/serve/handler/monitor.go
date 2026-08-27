package handler

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	monitorSvc "github.com/cangyunye/go-owl/cmd/plugins/serve/monitor"
	owlmonitor "github.com/cangyunye/go-owl/internal/monitor"
	"github.com/gin-gonic/gin"
)

// MonitorHandler 监控 REST API：告警、告警类型、对策、通知渠道、指标查询、静默。
type MonitorHandler struct {
	db  *sql.DB
	svc *monitorSvc.Service
}

// NewMonitorHandler 创建监控 handler。
func NewMonitorHandler(db *sql.DB, svc *monitorSvc.Service) *MonitorHandler {
	return &MonitorHandler{db: db, svc: svc}
}

// AlertView 告警视图（附类型中文名与节点名）。
type AlertView struct {
	owlmonitor.Alert
	AlertTypeName string `json:"alert_type_name"`
	NodeName      string `json:"node_name"`
}

// ListAlerts 告警列表：status/severity/node_id 筛选 + 分页 + 总数。
func (h *MonitorHandler) ListAlerts(c *gin.Context) {
	filter := owlmonitor.AlertFilter{
		Status:   owlmonitor.AlertStatus(c.Query("status")),
		NodeID:   c.Query("node_id"),
		Severity: c.Query("severity"),
	}
	if v := c.Query("limit"); v != "" {
		filter.Limit, _ = strconv.Atoi(v)
	}
	if v := c.Query("offset"); v != "" {
		filter.Offset, _ = strconv.Atoi(v)
	}
	if filter.Limit <= 0 {
		filter.Limit = 50
	}

	alerts, err := h.svc.Store.ListAlerts(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "查询告警失败: " + err.Error()})
		return
	}
	total, err := h.svc.Store.CountAlerts(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "统计告警失败: " + err.Error()})
		return
	}
	names := h.nodeNames()

	items := make([]AlertView, 0, len(alerts))
	for _, a := range alerts {
		items = append(items, h.toView(a, names))
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total})
}

// GetAlert 告警详情（含推荐对策）。
func (h *MonitorHandler) GetAlert(c *gin.Context) {
	id := c.Param("id")
	a, exists, err := h.svc.Store.GetAlert(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "查询告警失败: " + err.Error()})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "告警不存在"})
		return
	}
	view := h.toView(*a, h.nodeNames())
	remedies, err := h.svc.Store.RecommendedRemedies(a.AlertTypeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "查询对策失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"alert": view, "remedies": remedies})
}

// AckAlert 人工确认告警（operator+）。
func (h *MonitorHandler) AckAlert(c *gin.Context) {
	al, err := h.svc.Manager.Ack(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"alert": h.toView(*al, h.nodeNames())})
}

// ResolveAlert 人工解决告警（operator+）。
func (h *MonitorHandler) ResolveAlert(c *gin.Context) {
	al, err := h.svc.Manager.Resolve(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"alert": h.toView(*al, h.nodeNames())})
}

// ListAlertTypes 告警类型列表（reader）。
func (h *MonitorHandler) ListAlertTypes(c *gin.Context) {
	types, err := h.svc.Store.ListAlertTypes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "查询告警类型失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": types})
}

// UpdateAlertType 更新告警类型（阈值/开关/放行，admin）。
func (h *MonitorHandler) UpdateAlertType(c *gin.Context) {
	var at owlmonitor.AlertType
	if err := c.ShouldBindJSON(&at); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请求体非法: " + err.Error()})
		return
	}
	at.ID = c.Param("id")
	if err := h.svc.Store.UpsertAlertType(at); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "更新告警类型失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": at})
}

// ListRemedies 对策列表（可按告警类型过滤，reader）。
func (h *MonitorHandler) ListRemedies(c *gin.Context) {
	recs, err := h.svc.Store.ListRemedies(c.Query("alert_type_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "查询对策失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": recs})
}

// UpsertRemedy 新建/更新对策（admin）。
func (h *MonitorHandler) UpsertRemedy(c *gin.Context) {
	var r owlmonitor.Remedy
	if err := c.ShouldBindJSON(&r); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请求体非法: " + err.Error()})
		return
	}
	if id := c.Param("id"); id != "" {
		r.ID = id
	}
	if r.ID == "" || r.AlertTypeID == "" || r.Name == "" || r.Kind == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "缺少必填字段(id/alert_type_id/name/kind)"})
		return
	}
	if err := h.svc.Store.UpsertRemedy(r); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "写入对策失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": r})
}

// DeleteRemedy 删除对策（admin；内置不可删）。
func (h *MonitorHandler) DeleteRemedy(c *gin.Context) {
	if err := h.svc.Store.DeleteRemedy(c.Param("id")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ListNotifyChannels 通知渠道列表（admin，含 SMTP 等敏感配置）。
func (h *MonitorHandler) ListNotifyChannels(c *gin.Context) {
	chs, err := h.svc.Store.ListNotifyChannels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "查询通知渠道失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": chs})
}

// UpsertNotifyChannel 新建/更新通知渠道（admin）。
func (h *MonitorHandler) UpsertNotifyChannel(c *gin.Context) {
	var ch owlmonitor.NotifyChannel
	if err := c.ShouldBindJSON(&ch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请求体非法: " + err.Error()})
		return
	}
	if id := c.Param("id"); id != "" {
		ch.ID = id
	}
	if ch.ID == "" || ch.Kind == "" || ch.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "缺少必填字段(id/kind/name)"})
		return
	}
	if err := h.svc.Store.UpsertNotifyChannel(ch); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "写入通知渠道失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": ch})
}

// DeleteNotifyChannel 删除通知渠道（admin）。
func (h *MonitorHandler) DeleteNotifyChannel(c *gin.Context) {
	if err := h.svc.Store.DeleteNotifyChannel(c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// TestNotifyChannel 测试发送（admin）。
func (h *MonitorHandler) TestNotifyChannel(c *gin.Context) {
	ch, exists, err := h.svc.Store.GetNotifyChannel(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "渠道不存在"})
		return
	}
	ctx := c.Request.Context()
	webURL := h.webURL() + "/alerts/test"
	if err := h.svc.Dispatcher.SendTest(ctx, ch, webURL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// QueryMetrics 指标查询（reader）：node_id + metric + [from, to]。
func (h *MonitorHandler) QueryMetrics(c *gin.Context) {
	nodeID := c.Query("node_id")
	metric := c.Query("metric")
	if nodeID == "" || metric == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "缺少 node_id/metric"})
		return
	}
	from, _ := strconv.ParseInt(c.Query("from"), 10, 64)
	to, _ := strconv.ParseInt(c.Query("to"), 10, 64)
	if from == 0 {
		from = time.Now().Add(-time.Hour).Unix()
	}
	if to == 0 {
		to = time.Now().Unix()
	}
	samples, err := h.svc.Store.QuerySamples(nodeID, metric, from, to)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "查询指标失败: " + err.Error()})
		return
	}
	type point struct {
		TS    int64   `json:"ts"`
		Value float64 `json:"value"`
	}
	points := make([]point, 0, len(samples))
	for _, s := range samples {
		points = append(points, point{TS: s.TS, Value: s.Value})
	}
	c.JSON(http.StatusOK, gin.H{"metric": metric, "node_id": nodeID, "points": points})
}

// GetSilence 读取静默状态（admin）。
func (h *MonitorHandler) GetSilence(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"silence_until": h.svc.GetSilenceUntil()})
}

// SetSilence 设置静默截止时间（admin；0 = 取消静默）。
func (h *MonitorHandler) SetSilence(c *gin.Context) {
	var body struct {
		SilenceUntil int64 `json:"silence_until"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请求体非法: " + err.Error()})
		return
	}
	if err := h.svc.SetSilenceUntil(body.SilenceUntil); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"silence_until": body.SilenceUntil})
}

// --- 处置计划（Remedy Run）---

// CreateRemedyPlan 创建处置计划并异步执行（operator+）。
// body: {remedy_ids: [..], stop_on_error?: bool}
func (h *MonitorHandler) CreateRemedyPlan(c *gin.Context) {
	var body struct {
		RemedyIDs   []string `json:"remedy_ids"`
		StopOnError *bool    `json:"stop_on_error"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请求体非法: " + err.Error()})
		return
	}
	if len(body.RemedyIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请至少选择一条对策"})
		return
	}
	stopOnError := true
	if body.StopOnError != nil {
		stopOnError = *body.StopOnError
	}
	run, err := h.svc.StartRemedyRun(c.Param("id"), body.RemedyIDs, stopOnError, c.GetString("username"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"run": run})
}

// GetRemedyPlan 查询处置计划进度（reader）。
func (h *MonitorHandler) GetRemedyPlan(c *gin.Context) {
	run, exists, err := h.svc.Store.GetRemedyRun(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "处置计划不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"run": run})
}

// ListRemedyPlans 列出某告警的处置记录（reader）。
func (h *MonitorHandler) ListRemedyPlans(c *gin.Context) {
	runs, err := h.svc.Store.ListRemedyRunsByAlert(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": runs})
}

// StopRemedyPlan 停止处置计划（operator+）。
func (h *MonitorHandler) StopRemedyPlan(c *gin.Context) {
	run, exists, err := h.svc.Store.GetRemedyRun(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "处置计划不存在"})
		return
	}
	if run.IsTerminal() {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "计划已结束，无需停止"})
		return
	}
	if err := h.svc.Store.UpdateRemedyRunStatus(run.ID, owlmonitor.RunStopped); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ApproveRemedyPlan 批准处置计划中的待审批步骤并恢复执行（operator+）。
func (h *MonitorHandler) ApproveRemedyPlan(c *gin.Context) {
	run, exists, err := h.svc.Store.GetRemedyRun(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "处置计划不存在"})
		return
	}
	if run.Status != owlmonitor.RunWaitingApproval {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "计划不在待审批状态"})
		return
	}
	if err := h.svc.Store.ApproveRemedySteps(run.ID, time.Now().Unix()); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	h.svc.ExecuteRemedyRun(run.ID)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// RejectRemedyPlan 拒绝处置计划中的待审批步骤（operator+）。
func (h *MonitorHandler) RejectRemedyPlan(c *gin.Context) {
	run, exists, err := h.svc.Store.GetRemedyRun(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "处置计划不存在"})
		return
	}
	if run.Status != owlmonitor.RunWaitingApproval {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "计划不在待审批状态"})
		return
	}
	if err := h.svc.Store.RejectRemedySteps(run.ID, time.Now().Unix()); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	h.svc.ExecuteRemedyRun(run.ID)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// --- 内部辅助 ---

// nodeNames 节点 id → name 映射（列表页展示用）。
func (h *MonitorHandler) nodeNames() map[string]string {
	rows, err := h.db.Query(`SELECT id, name FROM nodes`)
	if err != nil {
		return map[string]string{}
	}
	defer func() { _ = rows.Close() }()

	names := make(map[string]string)
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		names[id] = name
	}
	return names
}

func (h *MonitorHandler) toView(a owlmonitor.Alert, names map[string]string) AlertView {
	at, _ := owlmonitor.FindAlertType(a.AlertTypeID)
	return AlertView{
		Alert:         a,
		AlertTypeName: at.Name,
		NodeName:      names[a.NodeID],
	}
}

func (h *MonitorHandler) webURL() string {
	return "http://" + h.svc.WebURL()
}
