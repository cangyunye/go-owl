package handler

import (
	"context"
	"database/sql"
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
}

type updateUserRequest struct {
	Password    string     `json:"password"`
	Role        model.Role `json:"role"`
	DisplayName string     `json:"display_name"`
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

	users, total, err := h.users.ListPaged(c.Request.Context(), keyword, page, pageSize)
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
			"total":     total,
			"page":      page,
			"page_size": pageSize,
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

	user := &model.User{
		Username:     req.Username,
		PasswordHash: hash,
		Role:         req.Role,
		DisplayName:  req.DisplayName,
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

	if req.Role != "" {
		if !validRoles[req.Role] {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid role"})
			return
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

	if err := h.users.Update(c.Request.Context(), user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "update failed"})
		return
	}

	c.JSON(http.StatusOK, toUserResponse(user))
}

func (h *UserHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid user id"})
		return
	}

	if err := h.users.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "delete failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "deleted"})
}
