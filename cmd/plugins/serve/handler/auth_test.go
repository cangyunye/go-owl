package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func newTestAuth(t *testing.T) (*AuthHandler, *store.UserStore, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	us := store.NewUserStore(db)
	require.NoError(t, us.Init(context.Background()))

	authSvc := service.NewAuthService("test-secret-32-bytes-long-for-testing!")
	h := NewAuthHandler(us, authSvc)
	return h, us, db
}

func TestLogin_Success(t *testing.T) {
	h, us, _ := newTestAuth(t)
	ctx := context.Background()

	hash, _ := h.auth.HashPassword("secret123")
	us.Create(ctx, &model.User{
		Username: "admin", PasswordHash: hash, Role: model.RoleAdmin,
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/login", h.Login)

	body := `{"username":"admin","password":"secret123"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var resp struct {
		Token string     `json:"token"`
		User  model.User `json:"user"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Token)
	assert.Equal(t, "admin", resp.User.Username)
	assert.Equal(t, model.RoleAdmin, resp.User.Role)
	assert.Empty(t, resp.User.PasswordHash)
}

func TestLogin_WrongPassword(t *testing.T) {
	h, us, _ := newTestAuth(t)
	ctx := context.Background()

	hash, _ := h.auth.HashPassword("secret123")
	us.Create(ctx, &model.User{
		Username: "admin", PasswordHash: hash, Role: model.RoleAdmin,
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/login", h.Login)

	body := `{"username":"admin","password":"wrongpass"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}

func TestLogin_UserNotFound(t *testing.T) {
	h, _, _ := newTestAuth(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/login", h.Login)

	body := `{"username":"nonexistent","password":"anything"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}

func TestLogin_InvalidJSON(t *testing.T) {
	h, _, _ := newTestAuth(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/login", h.Login)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/login", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, 400, w.Code)
}

func TestMeEndpoint(t *testing.T) {
	h, us, _ := newTestAuth(t)
	ctx := context.Background()

	hash, _ := h.auth.HashPassword("secret123")
	us.Create(ctx, &model.User{
		Username: "admin", PasswordHash: hash, Role: model.RoleAdmin,
	})

	token, _ := h.auth.GenerateToken("admin", "admin")

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/me", h.AuthMiddleware(), h.Me)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var resp model.User
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "admin", resp.Username)
	assert.Equal(t, model.RoleAdmin, resp.Role)
}

func TestAuthMiddleware_SetsUserID(t *testing.T) {
	h, _, _ := newTestAuth(t)

	token, _ := h.auth.GenerateToken("alice", "operator")

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/t", h.AuthMiddleware(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"user_id": c.GetString("user_id"),
			"role":    c.GetString("role"),
		})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/t", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "alice", resp["user_id"])
	assert.Equal(t, "operator", resp["role"])
}

func TestMeEndpoint_NoToken(t *testing.T) {
	h, _, _ := newTestAuth(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/me", h.AuthMiddleware(), h.Me)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/me", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}

// loginRouter 构造登录路由（含限流）。
func loginRouter(t *testing.T, h *AuthHandler) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/login", h.Login)
	return r
}

func doLogin(t *testing.T, r *gin.Engine, username, password string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	body := `{"username":"` + username + `","password":"` + password + `"}`
	req, _ := http.NewRequest("POST", "/api/v1/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

// TestLogin_RateLimitedAfterRepeatedFailures 连续失败达到阈值后限流：
// 即使随后提交正确密码也返回 429（修复前可无限次暴力尝试）。
func TestLogin_RateLimitedAfterRepeatedFailures(t *testing.T) {
	h, us, _ := newTestAuth(t)
	hash, _ := h.auth.HashPassword("secret123")
	require.NoError(t, us.Create(context.Background(), &model.User{
		Username: "admin", PasswordHash: hash, Role: model.RoleAdmin,
	}))
	r := loginRouter(t, h)

	for i := 0; i < 5; i++ {
		require.Equalf(t, http.StatusUnauthorized, doLogin(t, r, "admin", "wrong").Code, "第 %d 次失败应为 401", i+1)
	}

	w := doLogin(t, r, "admin", "secret123")
	assert.Equal(t, http.StatusTooManyRequests, w.Code, "达到阈值后正确密码也应被限流")
	assert.NotEmpty(t, w.Header().Get("Retry-After"))
}

// TestLogin_SuccessResetsFailures 登录成功清零失败计数。
func TestLogin_SuccessResetsFailures(t *testing.T) {
	h, us, _ := newTestAuth(t)
	hash, _ := h.auth.HashPassword("secret123")
	require.NoError(t, us.Create(context.Background(), &model.User{
		Username: "admin", PasswordHash: hash, Role: model.RoleAdmin,
	}))
	r := loginRouter(t, h)

	for i := 0; i < 4; i++ {
		require.Equal(t, http.StatusUnauthorized, doLogin(t, r, "admin", "wrong").Code)
	}
	require.Equal(t, http.StatusOK, doLogin(t, r, "admin", "secret123").Code)

	// 清零后再失败 4 次仍未达阈值
	for i := 0; i < 4; i++ {
		assert.Equal(t, http.StatusUnauthorized, doLogin(t, r, "admin", "wrong").Code)
	}
}

// TestLogin_RateLimitIsPerUsername 限流按用户名隔离，不牵连其他账号。
func TestLogin_RateLimitIsPerUsername(t *testing.T) {
	h, us, _ := newTestAuth(t)
	ctx := context.Background()
	hash, _ := h.auth.HashPassword("secret123")
	require.NoError(t, us.Create(ctx, &model.User{Username: "alice", PasswordHash: hash, Role: model.RoleViewer}))
	require.NoError(t, us.Create(ctx, &model.User{Username: "bob", PasswordHash: hash, Role: model.RoleViewer}))
	r := loginRouter(t, h)

	for i := 0; i < 5; i++ {
		require.Equal(t, http.StatusUnauthorized, doLogin(t, r, "alice", "wrong").Code)
	}
	require.Equal(t, http.StatusTooManyRequests, doLogin(t, r, "alice", "secret123").Code)
	assert.Equal(t, http.StatusOK, doLogin(t, r, "bob", "secret123").Code, "其他账号不受影响")
}

// TestLoginLimiter_BackoffGrows 退避时长随失败次数指数增长并有上限。
func TestLoginLimiter_BackoffGrows(t *testing.T) {
	l := newLoginLimiter()
	now := time.Now()
	l.now = func() time.Time { return now }

	for i := 0; i < loginFailThreshold; i++ {
		l.Fail("k")
	}
	assert.Equal(t, loginBaseBackoff, l.RetryAfter("k"))

	l.Fail("k")
	assert.Equal(t, 2*loginBaseBackoff, l.RetryAfter("k"))

	for i := 0; i < 20; i++ {
		l.Fail("k")
	}
	assert.Equal(t, loginMaxBackoff, l.RetryAfter("k"), "退避应有上限")

	now = now.Add(loginMaxBackoff + time.Second)
	assert.Zero(t, l.RetryAfter("k"), "退避窗口结束后应放行")
}
