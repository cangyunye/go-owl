package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"path/filepath"
	"net/http/httptest"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func settingsTestSetup(t *testing.T) (*sql.DB, *SettingsHandler) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY, value TEXT NOT NULL
	)`)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO settings (key, value) VALUES
		('ai_provider', 'openai'),
		('ai_model', 'gpt-4'),
		('theme', 'dark')
	`)
	require.NoError(t, err)

	return db, NewSettingsHandler(db)
}

func settingsRBACRouter(t *testing.T, handler *SettingsHandler) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	us := store.NewUserStore(db)
	as := service.NewAuthService("test-secret-32byte-long-string!!")
	ah := NewAuthHandler(us, as)

	r := gin.New()
	admin := r.Group("/api/v1")
	admin.Use(ah.AuthMiddleware(), ah.RBACMiddleware("admin"))
	{
		admin.GET("/settings", handler.List)
		admin.GET("/settings/:key", handler.Get)
		admin.PUT("/settings/:key", handler.Set)
	}
	return r
}

func TestSettingsList(t *testing.T) {
	_, h := settingsTestSetup(t)
	gin.SetMode(gin.TestMode)
	router := settingsRBACRouter(t, h)

	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, _ := as.GenerateToken("admin", "admin")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/settings", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	var resp struct {
		Data []SettingResponse `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Len(t, resp.Data, 3)
}

func TestSettingsGet(t *testing.T) {
	_, h := settingsTestSetup(t)
	gin.SetMode(gin.TestMode)
	router := settingsRBACRouter(t, h)

	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, _ := as.GenerateToken("admin", "admin")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/settings/ai_provider", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	var resp SettingResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "ai_provider", resp.Key)
	assert.Equal(t, "openai", resp.Value)
}

func TestSettingsGet_NotFound(t *testing.T) {
	_, h := settingsTestSetup(t)
	gin.SetMode(gin.TestMode)
	router := settingsRBACRouter(t, h)

	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, _ := as.GenerateToken("admin", "admin")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/settings/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)
	assert.Equal(t, 404, w.Code)
}

func TestSettingsSet(t *testing.T) {
	_, h := settingsTestSetup(t)
	gin.SetMode(gin.TestMode)
	router := settingsRBACRouter(t, h)

	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, _ := as.GenerateToken("admin", "admin")

	body := map[string]string{"value": "ollama"}
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/v1/settings/ai_provider", &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	var resp SettingResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "ai_provider", resp.Key)
	assert.Equal(t, "ollama", resp.Value)
}

func TestSettingsSet_NewKey(t *testing.T) {
	_, h := settingsTestSetup(t)
	gin.SetMode(gin.TestMode)
	router := settingsRBACRouter(t, h)

	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, _ := as.GenerateToken("admin", "admin")

	body := map[string]string{"value": "my-new-value"}
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/v1/settings/ai_endpoint", &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	var resp SettingResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "ai_endpoint", resp.Key)
	assert.Equal(t, "my-new-value", resp.Value)
}

func TestSettingsSet_InvalidBody(t *testing.T) {
	_, h := settingsTestSetup(t)
	gin.SetMode(gin.TestMode)
	router := settingsRBACRouter(t, h)

	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, _ := as.GenerateToken("admin", "admin")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/v1/settings/ai_provider", bytes.NewBufferString(`{}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	assert.Equal(t, 400, w.Code)
}

func TestSettingsSet_StagingDirValid(t *testing.T) {
	_, h := settingsTestSetup(t)
	gin.SetMode(gin.TestMode)
	router := settingsRBACRouter(t, h)

	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, _ := as.GenerateToken("admin", "admin")

	body := map[string]string{"value": filepath.Join(t.TempDir(), "staging")}
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/v1/settings/staging_dir", &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
}

func TestSettingsSet_StagingDirRelativePath(t *testing.T) {
	_, h := settingsTestSetup(t)
	gin.SetMode(gin.TestMode)
	router := settingsRBACRouter(t, h)

	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, _ := as.GenerateToken("admin", "admin")

	body := map[string]string{"value": "relative/staging"}
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/v1/settings/staging_dir", &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, 400, w.Code)
}

func TestSettingsSet_StagingMinFreeValid(t *testing.T) {
	_, h := settingsTestSetup(t)
	gin.SetMode(gin.TestMode)
	router := settingsRBACRouter(t, h)

	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, _ := as.GenerateToken("admin", "admin")

	body := map[string]string{"value": "20"}
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/v1/settings/staging_min_free", &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
}

func TestSettingsSet_StagingMinFreeNegative(t *testing.T) {
	_, h := settingsTestSetup(t)
	gin.SetMode(gin.TestMode)
	router := settingsRBACRouter(t, h)

	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, _ := as.GenerateToken("admin", "admin")

	body := map[string]string{"value": "-5"}
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/v1/settings/staging_min_free", &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, 400, w.Code)
}

func TestSettingsSet_StagingMinFreeNotNumber(t *testing.T) {
	_, h := settingsTestSetup(t)
	gin.SetMode(gin.TestMode)
	router := settingsRBACRouter(t, h)

	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, _ := as.GenerateToken("admin", "admin")

	body := map[string]string{"value": "ten"}
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/v1/settings/staging_min_free", &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, 400, w.Code)
}

func TestSettings_NonAdminForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	h := NewSettingsHandler(db)
	us := store.NewUserStore(db)
	as := service.NewAuthService("test-secret-32byte-long-string!!")
	ah := NewAuthHandler(us, as)
	token, _ := as.GenerateToken("viewer", "viewer")

	r := gin.New()
	r.GET("/api/v1/settings", ah.AuthMiddleware(), ah.RBACMiddleware("admin"), h.List)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/settings", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	assert.Equal(t, 403, w.Code)
}

// TestSettings_JWTSecretNotAccessible 验证 jwt_secret 不经 API 读出或写入：
// 它是会话令牌的签名密钥，读出来可离线伪造任意账号，写进去会使全部 token 失效。
func TestSettings_JWTSecretNotAccessible(t *testing.T) {
	db, h := settingsTestSetup(t)
	gin.SetMode(gin.TestMode)
	router := settingsRBACRouter(t, h)

	_, err := db.Exec(`INSERT INTO settings (key, value) VALUES ('jwt_secret', 'super-secret')`)
	require.NoError(t, err)

	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, _ := as.GenerateToken("admin", "admin")
	auth := "Bearer " + token

	t.Run("List 不返回", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/settings", nil)
		req.Header.Set("Authorization", auth)
		router.ServeHTTP(w, req)
		require.Equal(t, 200, w.Code)
		var resp struct {
			Data []SettingResponse `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		for _, s := range resp.Data {
			assert.NotEqual(t, "jwt_secret", s.Key, "签名密钥不得出现在设置列表")
		}
		assert.NotContains(t, w.Body.String(), "super-secret")
	})

	t.Run("GET 拒绝", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/settings/jwt_secret", nil)
		req.Header.Set("Authorization", auth)
		router.ServeHTTP(w, req)
		assert.Equal(t, 403, w.Code)
		assert.NotContains(t, w.Body.String(), "super-secret")
	})

	t.Run("PUT 拒绝且值不变", func(t *testing.T) {
		var buf bytes.Buffer
		json.NewEncoder(&buf).Encode(map[string]string{"value": "attacker-controlled"})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("PUT", "/api/v1/settings/jwt_secret", &buf)
		req.Header.Set("Authorization", auth)
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		assert.Equal(t, 403, w.Code)

		var got string
		require.NoError(t, db.QueryRow(`SELECT value FROM settings WHERE key = 'jwt_secret'`).Scan(&got))
		assert.Equal(t, "super-secret", got, "签名密钥不得被覆写")
	})
}