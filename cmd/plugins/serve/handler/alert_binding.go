// 告警绑定 REST API：对具体告警（按告警 ID）指定 playbook / 脚本指令。
// 查看 reader+；执行 operator+；新增/编辑/删除 admin（路由分组控制）。
package handler

import (
	"fmt"
	"net/http"
	"time"

	owlmonitor "github.com/cangyunye/go-owl/internal/monitor"
	"github.com/gin-gonic/gin"
)

type alertBindingRequest struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	Risk     string `json:"risk"`
	AutoExec bool   `json:"auto_exec"`
	ExecMode string `json:"exec_mode"`
	Seq      int    `json:"seq"`
}

type alertBindingUpdateRequest struct {
	Name     *string `json:"name"`
	Content  *string `json:"content"`
	Risk     *string `json:"risk"`
	AutoExec *bool   `json:"auto_exec"`
	ExecMode *string `json:"exec_mode"`
	Seq      *int    `json:"seq"`
}

// ListAlertBindings GET /alerts/:id/bindings 列出告警的专属指令绑定。
func (h *MonitorHandler) ListAlertBindings(c *gin.Context) {
	items, err := h.svc.Store.ListAlertBindings(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": internalErr("list alert bindings failed", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// CreateAlertBinding POST /alerts/:id/bindings 新增绑定（admin）。
func (h *MonitorHandler) CreateAlertBinding(c *gin.Context) {
	alertID := c.Param("id")
	if _, exists, err := h.svc.Store.GetAlert(alertID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": internalErr("query alert failed", err)})
		return
	} else if !exists {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "alert not found"})
		return
	}
	var req alertBindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": internalErr("invalid request body", err)})
		return
	}
	if req.Kind != "script" && req.Kind != "playbook" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "kind must be script or playbook"})
		return
	}
	if req.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "content is required"})
		return
	}
	name := req.Name
	if req.Kind == "playbook" {
		if h.playbooks == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "playbook store 未就绪"})
			return
		}
		pb, err := h.playbooks.Get(c.Request.Context(), req.Content)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "剧本不存在: " + req.Content})
			return
		}
		if name == "" {
			name = pb.Name
		}
	} else if name == "" {
		name = "自定义脚本"
	}
	if req.ExecMode != "concurrent" {
		req.ExecMode = "sequential"
	}
	if req.Risk == "" {
		req.Risk = "medium"
	}
	b := owlmonitor.AlertBinding{
		ID:        fmt.Sprintf("AB-%d", time.Now().UnixNano()),
		AlertID:   alertID,
		Kind:      req.Kind,
		Name:      name,
		Content:   req.Content,
		Risk:      req.Risk,
		AutoExec:  req.AutoExec,
		ExecMode:  req.ExecMode,
		Seq:       req.Seq,
		CreatedBy: c.GetString("username"),
	}
	if err := h.svc.Store.CreateAlertBinding(b); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": internalErr("create alert binding failed", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": b})
}

// UpdateAlertBinding PUT /alert-bindings/:id 部分更新绑定（admin）。
func (h *MonitorHandler) UpdateAlertBinding(c *gin.Context) {
	b, exists, err := h.svc.Store.GetAlertBinding(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": internalErr("query alert binding failed", err)})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "alert binding not found"})
		return
	}
	var req alertBindingUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": internalErr("invalid request body", err)})
		return
	}
	if req.Name != nil && *req.Name != "" {
		b.Name = *req.Name
	}
	if req.Content != nil && *req.Content != "" {
		if b.Kind == "playbook" {
			if h.playbooks == nil {
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "playbook store 未就绪"})
				return
			}
			if _, err := h.playbooks.Get(c.Request.Context(), *req.Content); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "剧本不存在: " + *req.Content})
				return
			}
		}
		b.Content = *req.Content
	}
	if req.Risk != nil && *req.Risk != "" {
		b.Risk = *req.Risk
	}
	if req.AutoExec != nil {
		b.AutoExec = *req.AutoExec
	}
	if req.ExecMode != nil {
		if *req.ExecMode != "sequential" && *req.ExecMode != "concurrent" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "exec_mode must be sequential or concurrent"})
			return
		}
		b.ExecMode = *req.ExecMode
	}
	if req.Seq != nil {
		b.Seq = *req.Seq
	}
	if err := h.svc.Store.UpdateAlertBinding(b); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": internalErr("update alert binding failed", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": b})
}

// DeleteAlertBinding DELETE /alert-bindings/:id 删除绑定（admin）。
func (h *MonitorHandler) DeleteAlertBinding(c *gin.Context) {
	if err := h.svc.Store.DeleteAlertBinding(c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": internalErr("delete alert binding failed", err)})
		return
	}
	okAction(c, "deleted")
}

// RunAlertBindings POST /alerts/:id/bindings/run 按给定顺序执行绑定（operator+）。
// body: {binding_ids: [..], mode: sequential|concurrent}
func (h *MonitorHandler) RunAlertBindings(c *gin.Context) {
	alertID := c.Param("id")
	a, exists, err := h.svc.Store.GetAlert(alertID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": internalErr("query alert failed", err)})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "alert not found"})
		return
	}
	var body struct {
		BindingIDs []string `json:"binding_ids"`
		Mode       string   `json:"mode"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": internalErr("invalid request body", err)})
		return
	}
	if body.Mode != "concurrent" {
		body.Mode = "sequential"
	}
	if len(body.BindingIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "binding_ids is required"})
		return
	}
	bindings := make([]owlmonitor.AlertBinding, 0, len(body.BindingIDs))
	for _, id := range body.BindingIDs {
		b, exists, err := h.svc.Store.GetAlertBinding(id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": internalErr("query alert binding failed", err)})
			return
		}
		if !exists {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "绑定不存在: " + id})
			return
		}
		if b.AlertID != alertID {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "绑定 " + id + " 不属于该告警"})
			return
		}
		bindings = append(bindings, b)
	}
	items, err := h.svc.RunAlertBindings(alertID, bindings, body.Mode, a.NodeID, c.GetString("username"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"items": items})
}

// ListAlertBindingRuns GET /alerts/:id/binding-runs 绑定执行历史（reader）。
func (h *MonitorHandler) ListAlertBindingRuns(c *gin.Context) {
	items, err := h.svc.Store.ListAlertBindingRuns(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": internalErr("list alert binding runs failed", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}
