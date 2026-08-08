package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedNodesDB(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS nodes (
			id TEXT PRIMARY KEY,
			name TEXT,
			address TEXT,
			port INTEGER DEFAULT 22,
			user TEXT,
			password TEXT,
			ssh_key TEXT,
			status TEXT DEFAULT 'unknown',
			groups TEXT DEFAULT '[]',
			labels TEXT DEFAULT '{}',
			proxy_jump TEXT DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO nodes (id, name, address, port, user, status, groups, labels) VALUES
		('web-01', 'web-01', '10.0.1.10', 22, 'root', 'online', '["web","prod"]', '{"env":"prod","tier":"frontend"}'),
		('web-02', 'web-02', '10.0.1.11', 22, 'deploy', 'online', '["web","prod"]', '{"env":"prod","tier":"frontend"}'),
		('db-01', 'db-01', '10.0.2.10', 22, 'root', 'offline', '["db","prod"]', '{"env":"prod","tier":"backend"}'),
		('dev-01', 'dev-01', '10.0.9.10', 22, 'dev', 'online', '["dev"]', '{"env":"staging"}')`)
	require.NoError(t, err)
}

func newTestNodeHandler(t *testing.T) (*NodeHandler, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	seedNodesDB(t, db)
	return NewNodeHandler(db), db
}

func TestNodeList(t *testing.T) {
	h, _ := newTestNodeHandler(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/nodes", h.List)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/nodes", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var resp struct {
		Data  []NodeResponse `json:"data"`
		Meta  struct {
			Total int `json:"total"`
		} `json:"meta"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Len(t, resp.Data, 4)
	assert.Equal(t, 4, resp.Meta.Total)
}

func TestNodeList_FilterByGroup(t *testing.T) {
	h, _ := newTestNodeHandler(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/nodes", h.List)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/nodes?group=web", nil)
	router.ServeHTTP(w, req)

	var resp struct {
		Data []NodeResponse `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Len(t, resp.Data, 2)
}

func TestNodeList_FilterByStatus(t *testing.T) {
	h, _ := newTestNodeHandler(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/nodes", h.List)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/nodes?status=offline", nil)
	router.ServeHTTP(w, req)

	var resp struct {
		Data []NodeResponse `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Len(t, resp.Data, 1)
	assert.Equal(t, "db-01", resp.Data[0].ID)
}

func TestNodeList_FilterByUser(t *testing.T) {
	h, _ := newTestNodeHandler(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/nodes", h.List)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/nodes?user=root", nil)
	router.ServeHTTP(w, req)

	var resp struct {
		Data []NodeResponse `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Len(t, resp.Data, 2)
}

func TestNodeList_FilterByLabel(t *testing.T) {
	h, _ := newTestNodeHandler(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/nodes", h.List)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/nodes?label=env:prod", nil)
	router.ServeHTTP(w, req)

	var resp struct {
		Data []NodeResponse `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Len(t, resp.Data, 3)
}

func TestNodeList_FilterByMultiLabel(t *testing.T) {
	h, _ := newTestNodeHandler(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/nodes", h.List)

	// 多 label 参数为 AND 语义:env=prod 且 tier=frontend -> web-01/web-02
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/nodes?label=env:prod&label=tier:frontend", nil)
	router.ServeHTTP(w, req)

	var resp struct {
		Data []NodeResponse `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	require.Len(t, resp.Data, 2)
	ids := []string{resp.Data[0].ID, resp.Data[1].ID}
	assert.Contains(t, ids, "web-01")
	assert.Contains(t, ids, "web-02")

	// env=prod 且 tier=backend -> 仅 db-01
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/v1/nodes?label=env:prod&label=tier:backend", nil)
	router.ServeHTTP(w2, req2)
	var resp2 struct {
		Data []NodeResponse `json:"data"`
	}
	json.Unmarshal(w2.Body.Bytes(), &resp2)
	require.Len(t, resp2.Data, 1)
	assert.Equal(t, "db-01", resp2.Data[0].ID)
}

func TestNodeList_CombinedFilters(t *testing.T) {
	h, _ := newTestNodeHandler(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/nodes", h.List)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/nodes?group=web&status=online", nil)
	router.ServeHTTP(w, req)

	var resp struct {
		Data []NodeResponse `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Len(t, resp.Data, 2)
}

func TestNodeList_Pagination(t *testing.T) {
	h, _ := newTestNodeHandler(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/nodes", h.List)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/nodes?page=1&page_size=2", nil)
	router.ServeHTTP(w, req)

	var resp struct {
		Data []NodeResponse `json:"data"`
		Meta struct {
			Total    int `json:"total"`
			Page     int `json:"page"`
			PageSize int `json:"page_size"`
		} `json:"meta"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Len(t, resp.Data, 2)
	assert.Equal(t, 4, resp.Meta.Total)
	assert.Equal(t, 1, resp.Meta.Page)
	assert.Equal(t, 2, resp.Meta.PageSize)
}

func TestNodeGet(t *testing.T) {
	h, _ := newTestNodeHandler(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/nodes/:id", h.Get)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/nodes/web-01", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var resp NodeResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "web-01", resp.ID)
	assert.Equal(t, "10.0.1.10", resp.Address)
	assert.Equal(t, "root", resp.User)
}

func TestNodeGet_NotFound(t *testing.T) {
	h, _ := newTestNodeHandler(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/nodes/:id", h.Get)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/nodes/notexist", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 404, w.Code)
}

func TestNodeSearch(t *testing.T) {
	h, _ := newTestNodeHandler(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/nodes/search", h.Search)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/nodes/search?q=web", nil)
	router.ServeHTTP(w, req)

	var resp struct {
		Data []NodeResponse `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Len(t, resp.Data, 2)
}

func TestNodeSearch_ByAddress(t *testing.T) {
	h, _ := newTestNodeHandler(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/nodes/search", h.Search)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/nodes/search?q=10.0.2", nil)
	router.ServeHTTP(w, req)

	var resp struct {
		Data []NodeResponse `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Len(t, resp.Data, 1)
	assert.Equal(t, "db-01", resp.Data[0].ID)
}

func TestNodeSearch_ByLabel(t *testing.T) {
	h, _ := newTestNodeHandler(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/nodes/search", h.Search)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/nodes/search?q=staging", nil)
	router.ServeHTTP(w, req)

	var resp struct {
		Data []NodeResponse `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Len(t, resp.Data, 1)
	assert.Equal(t, "dev-01", resp.Data[0].ID)
}

func TestNodeSearch_NoResults(t *testing.T) {
	h, _ := newTestNodeHandler(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/nodes/search", h.Search)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/nodes/search?q=nonexistent", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	var resp struct {
		Data []NodeResponse `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Len(t, resp.Data, 0)
}

func TestNodeGetSensitiveFieldsHidden(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE nodes (id TEXT PRIMARY KEY, name TEXT, address TEXT, port INTEGER DEFAULT 22, user TEXT, password TEXT, ssh_key TEXT, status TEXT, groups TEXT, labels TEXT, proxy_jump TEXT, created_at TIMESTAMP, updated_at TIMESTAMP)`)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO nodes (id, name, address, port, user, password, ssh_key, status, groups, labels) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"secret-box", "secret-box", "10.0.0.1", 22, "admin", "supersecret123", "my-ssh-private-key-data", "online", "[]", "{}")
	require.NoError(t, err)

	h := NewNodeHandler(db)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/nodes/:id", h.Get)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/nodes/secret-box", nil)
	router.ServeHTTP(w, req)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NotContains(t, resp, "password")
	assert.NotContains(t, resp, "ssh_key")
	assert.Equal(t, "secret-box", resp["id"])
}

func TestNodeFilters(t *testing.T) {
	h, _ := newTestNodeHandler(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/nodes/filters", h.Filters)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/nodes/filters", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var resp struct {
		Groups []string `json:"groups"`
		Users  []string `json:"users"`
		Labels []string `json:"labels"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.ElementsMatch(t, []string{"web", "prod", "db", "dev"}, resp.Groups)
	assert.ElementsMatch(t, []string{"root", "deploy", "dev"}, resp.Users)
	assert.ElementsMatch(t, []string{"env", "tier"}, resp.Labels)
}

func TestNodeStats(t *testing.T) {
	h, _ := newTestNodeHandler(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/nodes/stats", h.Stats)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/nodes/stats", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var resp struct {
		Total   int `json:"total"`
		Online  int `json:"online"`
		Offline int `json:"offline"`
		Warn    int `json:"warn"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, 4, resp.Total)
	assert.Equal(t, 3, resp.Online)
	assert.Equal(t, 1, resp.Offline)
	assert.Equal(t, 0, resp.Warn)
}

func TestNodeStats_OfflineIncludesUnknown(t *testing.T) {
	h, db := newTestNodeHandler(t)

	_, err := db.Exec(`INSERT INTO nodes (id, name, address, port, user, status) VALUES
		('unk-01', 'unk-01', '10.0.9.99', 22, 'root', 'unknown'),
		('warn-01', 'warn-01', '10.0.9.98', 22, 'root', 'warning')`)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/nodes/stats", h.Stats)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/nodes/stats", nil)
	router.ServeHTTP(w, req)

	var resp struct {
		Total   int `json:"total"`
		Online  int `json:"online"`
		Offline int `json:"offline"`
		Warn    int `json:"warn"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 6, resp.Total)
	assert.Equal(t, 3, resp.Online)
	assert.Equal(t, 2, resp.Offline)
	assert.Equal(t, 1, resp.Warn)
}


