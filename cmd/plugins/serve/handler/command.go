package handler

import (
	"net/http"
	"strconv"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
)

// ShortcutHandler 管理用户级快捷命令(/api/v1/shortcuts)。
type ShortcutHandler struct {
	commands *store.CommandStore
	users    *store.UserStore
}

func NewShortcutHandler(commands *store.CommandStore, users *store.UserStore) *ShortcutHandler {
	return &ShortcutHandler{commands: commands, users: users}
}

// currentUserID 从 JWT claims.username 反查当前用户 ID。
func (h *ShortcutHandler) currentUserID(c *gin.Context) (int64, bool) {
	claimsVal, ok := c.Get("claims")
	if !ok {
		return 0, false
	}
	claims, ok := claimsVal.(*service.Claims)
	if !ok {
		return 0, false
	}
	user, err := h.users.FindByUsername(c.Request.Context(), claims.Username)
	if err != nil {
		return 0, false
	}
	return user.ID, true
}

type shortcutRequest struct {
	Name    string `json:"name" binding:"required"`
	Command string `json:"command" binding:"required"`
}

func (h *ShortcutHandler) List(c *gin.Context) {
	userID, ok := h.currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "unauthorized"})
		return
	}
	list, err := h.commands.ListByUser(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "query failed"})
		return
	}
	if list == nil {
		list = []*model.UserCommand{}
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *ShortcutHandler) Create(c *gin.Context) {
	userID, ok := h.currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "unauthorized"})
		return
	}
	var req shortcutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "name and command are required"})
		return
	}
	cmd := &model.UserCommand{UserID: userID, Name: req.Name, Command: req.Command}
	if err := h.commands.Create(c.Request.Context(), cmd); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "create failed"})
		return
	}
	c.JSON(http.StatusCreated, cmd)
}

func (h *ShortcutHandler) Update(c *gin.Context) {
	userID, ok := h.currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "unauthorized"})
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid id"})
		return
	}
	var req shortcutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "name and command are required"})
		return
	}
	cmd := &model.UserCommand{ID: id, UserID: userID, Name: req.Name, Command: req.Command}
	result, err := h.commands.Update(c.Request.Context(), cmd)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "update failed"})
		return
	}
	if result == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "not found"})
		return
	}
	c.JSON(http.StatusOK, cmd)
}

func (h *ShortcutHandler) Delete(c *gin.Context) {
	userID, ok := h.currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "unauthorized"})
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid id"})
		return
	}
	result, err := h.commands.Delete(c.Request.Context(), id, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "delete failed"})
		return
	}
	if result == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "deleted"})
}

type reorderRequest struct {
	OrderedIDs []int64 `json:"ordered_ids" binding:"required"`
}

func (h *ShortcutHandler) Reorder(c *gin.Context) {
	userID, ok := h.currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "unauthorized"})
		return
	}
	var req reorderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "ordered_ids is required"})
		return
	}
	if err := h.commands.Reorder(c.Request.Context(), userID, req.OrderedIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "reorder failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok"})
}
