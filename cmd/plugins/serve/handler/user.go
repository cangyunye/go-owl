package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	users         *store.UserStore
	auth          *service.AuthService
	OnUserCreated func(ctx context.Context, userID int64)
	// RevokeTokens 由 server 接线：改角色/改密码/删除账号后撤销该用户已签发的
	// token（JWT 内的角色是签发快照，不撤销则旧权限会保留到过期）。
	RevokeTokens func(ctx context.Context, username string)
}

func NewUserHandler(users *store.UserStore, auth *service.AuthService) *UserHandler {
	return &UserHandler{users: users, auth: auth}
}

type userResponse struct {
	ID          int64      `json:"id"`
	Username    string     `json:"username"`
	Role        model.Role `json:"role"`
	DisplayName string     `json:"display_name,omitempty"`
}

type createUserRequest struct {
	Username    string     `json:"username" binding:"required"`
	Password    string     `json:"password" binding:"required"`
	Role        model.Role `json:"role" binding:"required"`
	DisplayName string     `json:"display_name"`
	// NodeScope 节点/分组范围授权 JSON（{"groups":[],"nodes":[]}，空=不限）
	NodeScope string `json:"node_scope"`
}

type updateUserRequest struct {
	Password    string     `json:"password"`
	Role        model.Role `json:"role"`
	DisplayName string     `json:"display_name"`
	// NodeScope 传 null 或省略=不改；"{}"/空串=清除限制；JSON 对象=设置
	NodeScope *string `json:"node_scope"`
}

// validateNodeScope 校验 scope JSON 合法性（空/非法结构拒绝）。
func validateNodeScope(raw string) error {
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(raw) == "{}" {
		return nil
	}
	var ns NodeScope
	if err := json.Unmarshal([]byte(raw), &ns); err != nil {
		return fmt.Errorf("node_scope 必须是合法 JSON，如 {\"groups\":[\"web\"],\"nodes\":[]}")
	}
	return nil
}

var validRoles = map[model.Role]bool{
	model.RoleAdmin:    true,
	model.RoleOperator: true,
	model.RoleEditor:   true,
	model.RoleViewer:   true,
}

func toUserResponse(u *model.User) userResponse {
	return userResponse{
		ID:          u.ID,
		Username:    u.Username,
		Role:        u.Role,
		DisplayName: u.DisplayName,
	}
}

// isLastAdmin 判断目标用户是否为当前唯一的管理员：删掉/降级他系统将失去
// 全部管理入口，且应用内无法恢复（ensureAdmin 仅在用户表为空时补建）。
func (h *UserHandler) isLastAdmin(ctx context.Context, target *model.User) (bool, error) {
	if target == nil || target.Role != model.RoleAdmin {
		return false, nil
	}
	counts, err := h.users.CountByRole(ctx)
	if err != nil {
		return false, err
	}
	return counts[model.RoleAdmin] <= 1, nil
}

func (h *UserHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	keyword := strings.TrimSpace(c.Query("q"))
	role := strings.TrimSpace(c.Query("role"))
	if role != "" && !validRoles[model.Role(role)] {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid role"})
		return
	}

	users, total, err := h.users.ListPaged(c.Request.Context(), keyword, role, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "query error"})
		return
	}
	roleCounts, err := h.users.CountByRole(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "query error"})
		return
	}
	resp := make([]userResponse, 0, len(users))
	for _, u := range users {
		resp = append(resp, toUserResponse(u))
	}
	c.JSON(http.StatusOK, gin.H{
		"data": resp,
		"meta": gin.H{
			"total":       total,
			"page":        page,
			"page_size":   pageSize,
			"role_counts": roleCounts,
		},
	})
}

func (h *UserHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid user id"})
		return
	}

	user, err := h.users.FindByID(c.Request.Context(), id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "user not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "query error"})
		return
	}
	c.JSON(http.StatusOK, toUserResponse(user))
}

func (h *UserHandler) Create(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "username, password and role are required"})
		return
	}

	if !validRoles[req.Role] {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid role, must be admin/operator/editor/viewer"})
		return
	}

	hash, err := h.auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "internal error"})
		return
	}

	if err := validateNodeScope(req.NodeScope); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	user := &model.User{
		Username:     req.Username,
		PasswordHash: hash,
		Role:         req.Role,
		DisplayName:  req.DisplayName,
		NodeScope:    req.NodeScope,
	}
	if err := h.users.Create(c.Request.Context(), user); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "unique") {
			c.JSON(http.StatusConflict, gin.H{"code": 409, "message": "username already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "create failed"})
		return
	}

	if h.OnUserCreated != nil {
		h.OnUserCreated(c.Request.Context(), user.ID)
	}

	c.JSON(http.StatusCreated, toUserResponse(user))
}

func (h *UserHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid user id"})
		return
	}

	user, err := h.users.FindByID(c.Request.Context(), id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "user not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "query error"})
		return
	}

	var req updateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request body"})
		return
	}

	privilegesChanged := (req.Role != "" && req.Role != user.Role) || req.Password != ""
	if req.NodeScope != nil {
		if err := validateNodeScope(*req.NodeScope); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
			return
		}
		if *req.NodeScope != user.NodeScope {
			privilegesChanged = true // 范围变更同样撤销 token，立即生效
		}
	}
	if req.Role != "" {
		if !validRoles[req.Role] {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid role"})
			return
		}
		if req.Role != model.RoleAdmin {
			last, err := h.isLastAdmin(c.Request.Context(), user)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "query error"})
				return
			}
			if last {
				c.JSON(http.StatusConflict, gin.H{"code": 409, "message": "cannot demote the last admin"})
				return
			}
		}
		user.Role = req.Role
	}
	if req.DisplayName != "" {
		user.DisplayName = req.DisplayName
	}
	if req.Password != "" {
		hash, err := h.auth.HashPassword(req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "internal error"})
			return
		}
		user.PasswordHash = hash
	}
	if req.NodeScope != nil {
		scope := strings.TrimSpace(*req.NodeScope)
		if scope == "{}" {
			scope = "" // 显式清除限制
		}
		user.NodeScope = scope
	}

	if err := h.users.Update(c.Request.Context(), user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "update failed"})
		return
	}

	// 角色或密码变更后，此前签发的 token 立即失效（须重新登录换取新权限）
	if privilegesChanged && h.RevokeTokens != nil {
		h.RevokeTokens(c.Request.Context(), user.Username)
	}

	c.JSON(http.StatusOK, toUserResponse(user))
}

func (h *UserHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid user id"})
		return
	}

	user, err := h.users.FindByID(c.Request.Context(), id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "user not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "query error"})
		return
	}

	last, err := h.isLastAdmin(c.Request.Context(), user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "query error"})
		return
	}
	if last {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "message": "cannot delete the last admin"})
		return
	}

	if err := h.users.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "delete failed"})
		return
	}

	// 账号已删除：其已签发的 token 必须立即失效
	if h.RevokeTokens != nil {
		h.RevokeTokens(c.Request.Context(), user.Username)
	}

	okAction(c, "deleted")
}
