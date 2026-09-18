package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	ai2 "github.com/cangyunye/go-owl/internal/ai"
	"github.com/gin-gonic/gin"
)

// AIApprovalHandler AI 高危操作审批单 API：确认门挂起的高危写操作
// 可持久化审批（会话关闭后仍可从列表批准/拒绝）。
type AIApprovalHandler struct {
	store *store.AIApprovalStore
	agent *ai2.Agent // 兜底 agent（含完整工具注册表），批准时重放执行
}

func NewAIApprovalHandler(s *store.AIApprovalStore, agent *ai2.Agent) *AIApprovalHandler {
	return &AIApprovalHandler{store: s, agent: agent}
}

// List GET /ai/approvals：admin 看全部，普通用户看本人；?status=pending 过滤。
func (h *AIApprovalHandler) List(c *gin.Context) {
	requestCtx := c.Request.Context()
	isAdmin := c.GetString("role") == "admin"
	items, err := h.store.List(requestCtx, c.GetString("user_id"), isAdmin, c.Query("status"), 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": internalErr("list approvals failed", err)})
		return
	}
	if items == nil {
		items = []*store.AIApproval{}
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// Approve POST /ai/approvals/:id/approve：以请求者原始身份重放工具调用
// （走内核黑名单 + 节点范围授权），结果写回审批单。
func (h *AIApprovalHandler) Approve(c *gin.Context) {
	h.decide(c, true)
}

// Reject POST /ai/approvals/:id/reject。
func (h *AIApprovalHandler) Reject(c *gin.Context) {
	h.decide(c, false)
}

func (h *AIApprovalHandler) decide(c *gin.Context, approve bool) {
	requestCtx := c.Request.Context()
	rec, err := h.store.Get(requestCtx, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "approval not found"})
		return
	}
	if rec.Status != store.AIApprovalPending {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "message": "该审批单已处理"})
		return
	}
	// 权限：本人或 admin
	if rec.UserID != c.GetString("user_id") && c.GetString("role") != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "只能处理自己的审批单"})
		return
	}

	decidedBy := c.GetString("username")
	if !approve {
		if err := h.store.Decide(requestCtx, rec.ID, store.AIApprovalRejected, decidedBy, "已拒绝"); err != nil {
			c.JSON(http.StatusConflict, gin.H{"code": 409, "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": store.AIApprovalRejected})
		return
	}

	// 重放执行：身份用审批单原始请求人（scope/审计与发起时一致）
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(rec.ArgumentsJSON), &args); err != nil {
		args = map[string]interface{}{}
	}
	execCtx := WithIdentity(context.Background(), ExecIdentity{Username: rec.Username})
	result, execErr := h.agent.ExecuteToolCall(execCtx, ai2.ToolCall{Name: rec.ToolName, Arguments: args})

	status := store.AIApprovalExecuted
	msg := result
	if execErr != nil {
		status = store.AIApprovalFailed
		msg = execErr.Error()
	}
	if err := h.store.Decide(requestCtx, rec.ID, status, decidedBy, truncateApprovalResult(msg, 4096)); err != nil {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": status, "result": msg})
}

// truncateApprovalResult 截断执行结果（防超长写入）。
func truncateApprovalResult(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…(截断)"
}
