package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
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
