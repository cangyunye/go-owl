package handler

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	users *store.UserStore
	auth  *service.AuthService
}

func NewAuthHandler(users *store.UserStore, auth *service.AuthService) *AuthHandler {
	return &AuthHandler{users: users, auth: auth}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request body"})
		return
	}

	user, err := h.users.FindByUsername(c.Request.Context(), req.Username)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "invalid credentials"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "internal error"})
		return
	}

	if !h.auth.VerifyPassword(user.PasswordHash, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "invalid credentials"})
		return
	}

	token, err := h.auth.GenerateToken(user.Username, string(user.Role))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user": gin.H{
			"id":           user.ID,
			"username":     user.Username,
			"role":         user.Role,
			"display_name": user.DisplayName,
		},
	})
}

func (h *AuthHandler) Me(c *gin.Context) {
	claims := c.MustGet("claims").(*service.Claims)

	user, err := h.users.FindByUsername(c.Request.Context(), claims.Username)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "user not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":           user.ID,
		"username":     user.Username,
		"role":         user.Role,
		"display_name": user.DisplayName,
	})
}

func (h *AuthHandler) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "missing or invalid token"})
			return
		}
		tokenStr := strings.TrimPrefix(auth, "Bearer ")

		claims, err := h.auth.ValidateToken(tokenStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "invalid or expired token"})
			return
		}

		c.Set("claims", claims)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Next()
	}
}

var roleHierarchy = map[string]int{
	"viewer":   0,
	"editor":   1,
	"operator": 2,
	"admin":    3,
}

// RBACMiddleware 要求用户角色等级不低于 allowedRoles 中最低等级的角色，
// 即该分组对列表中的每个角色及其以上等级的角色放行。
func (h *AuthHandler) RBACMiddleware(allowedRoles ...model.Role) gin.HandlerFunc {
	minLevel := -1
	for _, r := range allowedRoles {
		if lvl, ok := roleHierarchy[string(r)]; ok && (minLevel == -1 || lvl < minLevel) {
			minLevel = lvl
		}
	}
	if minLevel == -1 {
		minLevel = 0
	}
	return func(c *gin.Context) {
		role := c.GetString("role")
		userLevel, ok := roleHierarchy[role]
		if !ok || userLevel < minLevel {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "message": "insufficient permissions"})
			return
		}
		c.Next()
	}
}
