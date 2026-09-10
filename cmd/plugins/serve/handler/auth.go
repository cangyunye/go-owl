package handler

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	users *store.UserStore
	auth  *service.AuthService

	// limiter 限制连续失败的登录尝试（用户名+来源 IP）
	limiter *loginLimiter
	// revocations 为 nil 时不做撤销校验（测试与未启用撤销的构造路径）
	revocations *authRevocations
}

func NewAuthHandler(users *store.UserStore, auth *service.AuthService) *AuthHandler {
	return &AuthHandler{users: users, auth: auth, limiter: newLoginLimiter()}
}

// EnableRevocation 启用令牌撤销校验并从 settings 载入已有撤销记录。
// 启用后，改角色/改密码/删除账号会使此前的 token 立即失效。
func (h *AuthHandler) EnableRevocation(ctx context.Context, db *sql.DB) {
	h.revocations = newAuthRevocations(ctx, db)
}

// RevokeUserTokens 撤销指定用户名下已签发的全部 token。
func (h *AuthHandler) RevokeUserTokens(ctx context.Context, username string) {
	if h.revocations != nil {
		_ = h.revocations.Revoke(ctx, username)
	}
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

	limitKey := req.Username + "|" + c.ClientIP()
	if wait := h.limiter.RetryAfter(limitKey); wait > 0 {
		c.Header("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		c.JSON(http.StatusTooManyRequests, gin.H{"code": 429, "message": "too many failed attempts, try again later"})
		return
	}

	user, err := h.users.FindByUsername(c.Request.Context(), req.Username)
	if err == sql.ErrNoRows {
		h.limiter.Fail(limitKey)
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "invalid credentials"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "internal error"})
		return
	}

	if !h.auth.VerifyPassword(user.PasswordHash, req.Password) {
		h.limiter.Fail(limitKey)
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "invalid credentials"})
		return
	}
	h.limiter.Reset(limitKey)

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
		if h.revocations.isRevoked(claims.Username, tokenIssuedAt(claims)) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "token revoked"})
			return
		}

		c.Set("claims", claims)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Set("user_id", claims.Username)
		c.Next()
	}
}

// tokenIssuedAt 取 token 的签发时刻；缺失时返回零值（撤销校验会从严拒绝）。
func tokenIssuedAt(claims *service.Claims) time.Time {
	if claims.IssuedAt == nil {
		return time.Time{}
	}
	return claims.IssuedAt.Time
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
