package handler

import (
	"bytes"
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strconv"
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

const revTestSecret = "test-secret-32byte-long-string!!"

// revocationTestSetup 构造启用撤销校验的 AuthHandler 与 /me 路由。
func revocationTestSetup(t *testing.T) (*sql.DB, *store.UserStore, *service.AuthService, *AuthHandler, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`)
	require.NoError(t, err)

	us := store.NewUserStore(db)
	require.NoError(t, us.Init(context.Background()))

	as := service.NewAuthService(revTestSecret)
	ah := NewAuthHandler(us, as)
	ah.EnableRevocation(context.Background(), db)

	r := gin.New()
	r.GET("/api/v1/me", ah.AuthMiddleware(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"username": c.GetString("username")})
	})
	return db, us, as, ah, r
}

// revMe 以给定 token 请求 /api/v1/me，返回状态码。
func revMe(r *gin.Engine, token string) int {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	return w.Code
}

func revMutate(r *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

// TestAuthRevocation_OldTokenRejected 撤销后旧 token 立即失效，重新登录后恢复。
func TestAuthRevocation_OldTokenRejected(t *testing.T) {
	_, _, as, ah, r := revocationTestSetup(t)

	token, err := as.GenerateToken("alice", "viewer")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, revMe(r, token))

	ah.RevokeUserTokens(context.Background(), "alice")
	assert.Equal(t, http.StatusUnauthorized, revMe(r, token), "撤销后旧 token 必须被拒")

	// 撤销后重新登录（跨过同一秒边界）应恢复访问
	time.Sleep(1100 * time.Millisecond)
	refreshed, err := as.GenerateToken("alice", "viewer")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, revMe(r, refreshed))
}

// TestAuthRevocation_PersistsAcrossReload 撤销记录持久化在 settings，重建实例后仍生效。
func TestAuthRevocation_PersistsAcrossReload(t *testing.T) {
	db, _, as, ah, r := revocationTestSetup(t)

	token, err := as.GenerateToken("bob", "editor")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, revMe(r, token))
	ah.RevokeUserTokens(context.Background(), "bob")

	// 模拟进程重启：同一库上重建 handler
	ah2 := NewAuthHandler(store.NewUserStore(db), service.NewAuthService(revTestSecret))
	ah2.EnableRevocation(context.Background(), db)
	r2 := gin.New()
	r2.GET("/api/v1/me", ah2.AuthMiddleware(), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) })

	assert.Equal(t, http.StatusUnauthorized, revMe(r2, token), "重启后撤销记录仍应生效")
}

// TestUserUpdate_RoleChangeRevokesOldToken 改角色后该用户旧 token 立即失效。
func TestUserUpdate_RoleChangeRevokesOldToken(t *testing.T) {
	_, us, as, ah, _ := revocationTestSetup(t)

	alice := &model.User{Username: "alice", PasswordHash: "x", Role: model.RoleEditor}
	require.NoError(t, us.Create(context.Background(), alice))

	uh := NewUserHandler(us, as)
	uh.RevokeTokens = ah.RevokeUserTokens

	r := gin.New()
	r.GET("/api/v1/me", ah.AuthMiddleware(), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) })
	r.PUT("/api/v1/users/:id", ah.AuthMiddleware(), ah.RBACMiddleware(model.RoleAdmin), uh.Update)

	aliceToken, _ := as.GenerateToken("alice", "editor")
	adminTok, _ := as.GenerateToken("root", "admin")
	require.Equal(t, http.StatusOK, revMe(r, aliceToken))

	w := revMutate(r, "PUT", "/api/v1/users/"+strconv.FormatInt(alice.ID, 10), `{"role":"viewer"}`, adminTok)
	require.Equal(t, http.StatusOK, w.Code)

	assert.Equal(t, http.StatusUnauthorized, revMe(r, aliceToken), "角色变更后旧 token 必须失效")
}

// TestUserUpdate_PasswordChangeRevokesOldToken 改密码后旧 token 立即失效。
func TestUserUpdate_PasswordChangeRevokesOldToken(t *testing.T) {
	_, us, as, ah, _ := revocationTestSetup(t)

	alice := &model.User{Username: "alice", PasswordHash: "x", Role: model.RoleViewer}
	require.NoError(t, us.Create(context.Background(), alice))

	uh := NewUserHandler(us, as)
	uh.RevokeTokens = ah.RevokeUserTokens

	r := gin.New()
	r.GET("/api/v1/me", ah.AuthMiddleware(), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) })
	r.PUT("/api/v1/users/:id", ah.AuthMiddleware(), ah.RBACMiddleware(model.RoleAdmin), uh.Update)

	aliceToken, _ := as.GenerateToken("alice", "viewer")
	require.Equal(t, http.StatusOK, revMe(r, aliceToken))

	w := revMutate(r, "PUT", "/api/v1/users/"+strconv.FormatInt(alice.ID, 10), `{"password":"newpass123"}`, adminToken())
	require.Equal(t, http.StatusOK, w.Code)

	assert.Equal(t, http.StatusUnauthorized, revMe(r, aliceToken), "改密后旧 token 必须失效")
}

// TestUserDelete_RevokesOldToken 删除账号后旧 token 立即失效。
func TestUserDelete_RevokesOldToken(t *testing.T) {
	_, us, as, ah, _ := revocationTestSetup(t)

	alice := &model.User{Username: "alice", PasswordHash: "x", Role: model.RoleViewer}
	require.NoError(t, us.Create(context.Background(), alice))

	uh := NewUserHandler(us, as)
	uh.RevokeTokens = ah.RevokeUserTokens

	r := gin.New()
	r.GET("/api/v1/me", ah.AuthMiddleware(), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) })
	r.DELETE("/api/v1/users/:id", ah.AuthMiddleware(), ah.RBACMiddleware(model.RoleAdmin), uh.Delete)

	aliceToken, _ := as.GenerateToken("alice", "viewer")
	require.Equal(t, http.StatusOK, revMe(r, aliceToken))

	w := revMutate(r, "DELETE", "/api/v1/users/"+strconv.FormatInt(alice.ID, 10), "", adminToken())
	require.Equal(t, http.StatusOK, w.Code)

	assert.Equal(t, http.StatusUnauthorized, revMe(r, aliceToken), "删除账号后旧 token 必须失效")
}

// TestAuthRevocation_DisabledByDefault 未启用撤销的构造路径（测试常用）行为不变。
func TestAuthRevocation_DisabledByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	us := store.NewUserStore(db)
	require.NoError(t, us.Init(context.Background()))
	as := service.NewAuthService(revTestSecret)
	ah := NewAuthHandler(us, as)

	r := gin.New()
	r.GET("/api/v1/me", ah.AuthMiddleware(), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) })

	token, _ := as.GenerateToken("ghost", "admin")
	assert.Equal(t, http.StatusOK, revMe(r, token))
}
