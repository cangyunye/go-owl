package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func crudTestSetup(t *testing.T) (*sql.DB, *NodeHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY, name TEXT, address TEXT, port INTEGER DEFAULT 22,
		user TEXT, password TEXT, ssh_key TEXT, status TEXT DEFAULT 'unknown',
		groups TEXT DEFAULT '[]', labels TEXT DEFAULT '{}',
		proxy_jump TEXT DEFAULT '', created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO nodes (id, name, address, port, user, status) VALUES
		('existing-node', 'existing-node', '10.0.0.1', 22, 'root', 'online')`)
	require.NoError(t, err)

	return db, NewNodeHandler(db)
}

func injectRBAC(db *sql.DB, router *gin.Engine, method, path string, role model.Role, handler gin.HandlerFunc) {
	us := store.NewUserStore(db)
	as := service.NewAuthService("test-secret-32byte-long-string!!")
	ah := NewAuthHandler(us, as)
	router.Handle(method, path, ah.AuthMiddleware(), ah.RBACMiddleware(role), handler)
}

func authRequest(t *testing.T, router *gin.Engine, method, path string, body interface{}, role string) *httptest.ResponseRecorder {
	t.Helper()
	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, err := as.GenerateToken("testuser", role)
	require.NoError(t, err)

	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(method, path, &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(w, req)
	return w
}

func TestNodeCreate_Success(t *testing.T) {
	db, h := crudTestSetup(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	injectRBAC(db, router, "POST", "/api/v1/nodes", model.RoleEditor, h.Create)

	body := map[string]interface{}{
		"id":      "new-node",
		"name":    "New Node",
		"address": "10.0.0.100",
		"port":    2222,
		"user":    "deploy",
		"groups":  []string{"web", "staging"},
		"labels":  map[string]string{"env": "staging"},
		"status":  "online",
	}
	w := authRequest(t, router, "POST", "/api/v1/nodes", body, "editor")
	assert.Equal(t, 201, w.Code)

	var resp NodeResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "new-node", resp.ID)
	assert.Equal(t, "New Node", resp.Name)
	assert.Equal(t, "10.0.0.100", resp.Address)
	assert.Equal(t, 2222, resp.Port)
	assert.Equal(t, "deploy", resp.User)
	assert.Equal(t, []string{"web", "staging"}, resp.Groups)
	assert.Equal(t, map[string]string{"env": "staging"}, resp.Labels)

	var rawMap map[string]json.RawMessage
	json.Unmarshal(w.Body.Bytes(), &rawMap)
	assert.NotContains(t, rawMap, "password")
	assert.NotContains(t, rawMap, "ssh_key")
}

func TestNodeCreate_DuplicateID(t *testing.T) {
	db, h := crudTestSetup(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	injectRBAC(db, router, "POST", "/api/v1/nodes", model.RoleEditor, h.Create)

	body := map[string]interface{}{
		"id":      "existing-node",
		"address": "10.0.0.2",
		"user":    "root",
	}
	w := authRequest(t, router, "POST", "/api/v1/nodes", body, "editor")
	assert.Equal(t, 409, w.Code)
}

func TestNodeCreate_InvalidBody(t *testing.T) {
	db, h := crudTestSetup(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	injectRBAC(db, router, "POST", "/api/v1/nodes", model.RoleEditor, h.Create)

	w := authRequest(t, router, "POST", "/api/v1/nodes", map[string]interface{}{}, "editor")
	assert.Equal(t, 400, w.Code)
}

func TestNodeCreate_AsViewer_Forbidden(t *testing.T) {
	db, h := crudTestSetup(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	// Only editor+ can create, viewer should be rejected
	injectRBAC(db, router, "POST", "/api/v1/nodes", model.RoleEditor, h.Create)

	body := map[string]interface{}{"id": "x", "address": "1.2.3.4", "user": "root"}
	w := authRequest(t, router, "POST", "/api/v1/nodes", body, "viewer")
	assert.Equal(t, 403, w.Code)
}

func TestNodeCreate_NoAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	router := gin.New()
	us := store.NewUserStore(db)
	as := service.NewAuthService("test-secret-32byte-long-string!!")
	ah := NewAuthHandler(us, as)
	router.POST("/api/v1/nodes", ah.AuthMiddleware(), ah.RBACMiddleware(model.RoleEditor), (&NodeHandler{db: db}).Create)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/nodes", bytes.NewBufferString(`{"id":"x","address":"1.2.3.4","user":"root"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	assert.Equal(t, 401, w.Code)
}

func TestNodeUpdate_Success(t *testing.T) {
	db, h := crudTestSetup(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	injectRBAC(db, router, "PUT", "/api/v1/nodes/:id", model.RoleEditor, h.Update)

	body := map[string]interface{}{
		"name":   "Updated Node",
		"port":   2222,
		"groups": []string{"web", "prod"},
		"labels": map[string]string{"env": "prod"},
	}
	w := authRequest(t, router, "PUT", "/api/v1/nodes/existing-node", body, "editor")
	assert.Equal(t, 200, w.Code)

	var resp NodeResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "existing-node", resp.ID)
	assert.Equal(t, "Updated Node", resp.Name)
	assert.Equal(t, 2222, resp.Port)
	assert.Equal(t, "root", resp.User) // unchanged
	assert.Equal(t, []string{"web", "prod"}, resp.Groups)
	assert.Equal(t, map[string]string{"env": "prod"}, resp.Labels)
}

func TestNodeUpdate_NotFound(t *testing.T) {
	db, h := crudTestSetup(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	injectRBAC(db, router, "PUT", "/api/v1/nodes/:id", model.RoleEditor, h.Update)

	body := map[string]interface{}{"name": "Non Existent"}
	w := authRequest(t, router, "PUT", "/api/v1/nodes/nonexistent", body, "editor")
	assert.Equal(t, 404, w.Code)
}

func TestNodeUpdate_AsViewer_Forbidden(t *testing.T) {
	db, h := crudTestSetup(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	injectRBAC(db, router, "PUT", "/api/v1/nodes/:id", model.RoleEditor, h.Update)

	body := map[string]interface{}{"name": "Hack"}
	w := authRequest(t, router, "PUT", "/api/v1/nodes/existing-node", body, "viewer")
	assert.Equal(t, 403, w.Code)
}

func TestNodeDelete_Success(t *testing.T) {
	db, h := crudTestSetup(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	injectRBAC(db, router, "DELETE", "/api/v1/nodes/:id", model.RoleAdmin, h.Delete)

	w := authRequest(t, router, "DELETE", "/api/v1/nodes/existing-node", nil, "admin")
	assert.Equal(t, 200, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "deleted", resp["status"])

	// Verify it's actually gone
	var count int
	err := h.db.QueryRow("SELECT COUNT(*) FROM nodes WHERE id = 'existing-node'").Scan(&count)
	assert.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestNodeDelete_AsEditor_Forbidden(t *testing.T) {
	db, h := crudTestSetup(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	injectRBAC(db, router, "DELETE", "/api/v1/nodes/:id", model.RoleAdmin, h.Delete)

	w := authRequest(t, router, "DELETE", "/api/v1/nodes/existing-node", nil, "editor")
	assert.Equal(t, 403, w.Code)
}

func TestNodeDelete_NotFound(t *testing.T) {
	db, h := crudTestSetup(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	injectRBAC(db, router, "DELETE", "/api/v1/nodes/:id", model.RoleAdmin, h.Delete)

	w := authRequest(t, router, "DELETE", "/api/v1/nodes/nonexistent", nil, "admin")
	assert.Equal(t, 404, w.Code)
}

// Verify existing-node still exists after non-delete tests
func TestNodeCreate_WithPasswordAndSSHKey(t *testing.T) {
	db, h := crudTestSetup(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	injectRBAC(db, router, "POST", "/api/v1/nodes", model.RoleEditor, h.Create)

	body := map[string]interface{}{
		"id":       "secret-node",
		"address":  "10.0.0.200",
		"user":     "admin",
		"password": "my-secret-pass",
		"ssh_key":  "ssh-ed25519 AAA...",
	}
	w := authRequest(t, router, "POST", "/api/v1/nodes", body, "editor")
	assert.Equal(t, 201, w.Code)

	// Verify response doesn't include password/key
	var rawMap map[string]json.RawMessage
	json.Unmarshal(w.Body.Bytes(), &rawMap)
	assert.NotContains(t, rawMap, "password")
	assert.NotContains(t, rawMap, "ssh_key")

	// Verify they're stored in DB
	var pw, key string
	err := h.db.QueryRow("SELECT password, ssh_key FROM nodes WHERE id = 'secret-node'").Scan(&pw, &key)
	require.NoError(t, err)
	assert.Equal(t, "my-secret-pass", pw)
	assert.Equal(t, "ssh-ed25519 AAA...", key)
}

func TestNodeCreate_RecordsHistory(t *testing.T) {
	db, h := crudTestSetup(t)
	hs := store.NewHistoryStore(db)
	require.NoError(t, hs.Init(t.Context()))
	h.History = hs

	gin.SetMode(gin.TestMode)
	router := gin.New()
	injectRBAC(db, router, "POST", "/api/v1/nodes", model.RoleEditor, h.Create)

	body := map[string]interface{}{"id": "hist-node", "address": "10.0.0.9", "user": "root"}
	w := authRequest(t, router, "POST", "/api/v1/nodes", body, "editor")
	require.Equal(t, http.StatusCreated, w.Code)

	recs, total, err := hs.Query(t.Context(), &store.QueryOptions{OpType: "node_manage"})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	assert.Contains(t, recs[0].Operation.Command, "hist-node")
	assert.Equal(t, []string{"hist-node"}, recs[0].Operation.Targets)
}

func TestNodeUpdate_PartialUpdate(t *testing.T) {
	db, h := crudTestSetup(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	injectRBAC(db, router, "PUT", "/api/v1/nodes/:id", model.RoleEditor, h.Update)

	// Update only user and status
	body := map[string]interface{}{"user": "admin", "status": "offline"}
	w := authRequest(t, router, "PUT", "/api/v1/nodes/existing-node", body, "editor")
	assert.Equal(t, 200, w.Code)

	var resp NodeResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "admin", resp.User)
	assert.Equal(t, "offline", resp.Status)
	assert.Equal(t, "existing-node", resp.Name) // unchanged
}
