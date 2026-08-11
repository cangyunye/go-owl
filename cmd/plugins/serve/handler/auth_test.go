package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
