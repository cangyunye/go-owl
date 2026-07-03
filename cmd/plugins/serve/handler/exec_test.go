package handler

import (
	"bytes"
	"context"
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

type mockExecutor struct {
	output   string
	exitCode int
	err      error
}

func (m *mockExecutor) Execute(_ context.Context, _, _ string) (string, int, error) {
	return m.output, m.exitCode, m.err
}

func execTestSetup(t *testing.T) (*sql.DB, *ExecHandler) {
	t.Helper()
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
		('test-node', 'test-node', '127.0.0.1', 22, 'root', 'online')`)
	require.NoError(t, err)

	ts := store.NewTaskStore(db)
	require.NoError(t, ts.Init(t.Context()))

	h := NewExecHandler(db, ts, nil)
	h.exec = &mockExecutor{output: "ok\n", exitCode: 0}
	return db, h
}

func execRBACRouter(t *testing.T, handler *ExecHandler) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	us := store.NewUserStore(db)
	as := service.NewAuthService("test-secret-32byte-long-string!!")
	ah := NewAuthHandler(us, as)

	r := gin.New()
	auth := r.Group("/api/v1")
	auth.Use(ah.AuthMiddleware())
	auth.POST("/exec", ah.RBACMiddleware(model.RoleOperator, model.RoleAdmin), handler.Create)
	auth.GET("/tasks", ah.RBACMiddleware(model.RoleViewer, model.RoleEditor, model.RoleOperator, model.RoleAdmin), handler.List)
	auth.GET("/tasks/:id", ah.RBACMiddleware(model.RoleViewer, model.RoleEditor, model.RoleOperator, model.RoleAdmin), handler.Get)
	auth.DELETE("/tasks/:id", ah.RBACMiddleware(model.RoleAdmin), handler.Cancel)
	return r
}

func adminToken() string {
	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, _ := as.GenerateToken("admin", "admin")
	return token
}

func viewerToken() string {
	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, _ := as.GenerateToken("viewer", "viewer")
	return token
}

func execPOST(t *testing.T, router *gin.Engine, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(body)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/exec", &buf)
	req.Header.Set("Authorization", "Bearer "+adminToken())
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	return w
}

type execResponse struct {
	Tasks []store.Task `json:"tasks"`
}

func TestExecCreate_Success(t *testing.T) {
	_, h := execTestSetup(t)
	router := execRBACRouter(t, h)
	w := execPOST(t, router, map[string]string{"node_id": "test-node", "command": "uptime"})

	assert.Equal(t, 202, w.Code)
	var resp execResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	require.Len(t, resp.Tasks, 1)
	assert.NotEmpty(t, resp.Tasks[0].ID)
	assert.Equal(t, "test-node", resp.Tasks[0].NodeID)
	assert.Equal(t, "uptime", resp.Tasks[0].Command)
	assert.Equal(t, store.TaskStatusQueued, resp.Tasks[0].Status)
}

func TestExecCreate_MissingNodeID(t *testing.T) {
	_, h := execTestSetup(t)
	router := execRBACRouter(t, h)
	w := execPOST(t, router, map[string]string{"command": "uptime"})
	assert.Equal(t, 400, w.Code)
}

func TestExecCreate_NodeNotFound(t *testing.T) {
	_, h := execTestSetup(t)
	router := execRBACRouter(t, h)
	w := execPOST(t, router, map[string]string{"node_id": "nonexistent", "command": "uptime"})
	assert.Equal(t, 404, w.Code)
}

func TestExecCreate_ScriptMode(t *testing.T) {
	_, h := execTestSetup(t)
	router := execRBACRouter(t, h)

	w := execPOST(t, router, map[string]interface{}{
		"node_ids":       []string{"test-node"},
		"script_content": "#!/bin/bash\necho hello",
		"script_name":    "test.sh",
	})

	require.Equal(t, 202, w.Code)
	var resp execResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	require.Len(t, resp.Tasks, 1)
	assert.Equal(t, "test-node", resp.Tasks[0].NodeID)
	assert.Equal(t, store.TaskStatusQueued, resp.Tasks[0].Status)
}

func TestExecCreate_ByGroup(t *testing.T) {
	db, h := execTestSetup(t)
	router := execRBACRouter(t, h)

	// Add nodes with groups
	db.Exec(`INSERT INTO nodes (id, name, address, port, user, status, groups) VALUES
		('web-1', 'web-1', '10.0.1.1', 22, 'root', 'online', '["web","prod"]')`)
	db.Exec(`INSERT INTO nodes (id, name, address, port, user, status, groups) VALUES
		('web-2', 'web-2', '10.0.1.2', 22, 'root', 'online', '["web","stg"]')`)
	db.Exec(`INSERT INTO nodes (id, name, address, port, user, status, groups) VALUES
		('db-1', 'db-1', '10.0.2.1', 22, 'root', 'online', '["db","prod"]')`)

	w := execPOST(t, router, map[string]interface{}{
		"group":   "web",
		"command": "uptime",
	})

	require.Equal(t, 202, w.Code)
	var resp execResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	require.Len(t, resp.Tasks, 2)
	ids := []string{resp.Tasks[0].NodeID, resp.Tasks[1].NodeID}
	assert.Contains(t, ids, "web-1")
	assert.Contains(t, ids, "web-2")
}

func TestExecCreate_ByLabel(t *testing.T) {
	db, h := execTestSetup(t)
	router := execRBACRouter(t, h)

	db.Exec(`INSERT INTO nodes (id, name, address, port, user, status, labels) VALUES
		('app-a', 'app-a', '10.0.1.1', 22, 'root', 'online', '{"env":"prod","tier":"frontend"}')`)
	db.Exec(`INSERT INTO nodes (id, name, address, port, user, status, labels) VALUES
		('app-b', 'app-b', '10.0.1.2', 22, 'root', 'online', '{"env":"stg","tier":"frontend"}')`)

	w := execPOST(t, router, map[string]interface{}{
		"labels": map[string]string{"env": "prod"},
		"command": "uptime",
	})

	require.Equal(t, 202, w.Code)
	var resp execResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	require.Len(t, resp.Tasks, 1)
	assert.Equal(t, "app-a", resp.Tasks[0].NodeID)
}

func TestExecCreate_ForceMultiNode(t *testing.T) {
	db, h := execTestSetup(t)
	router := execRBACRouter(t, h)

	ts := store.NewTaskStore(db)
	ts.Init(t.Context())
	runningTask, _ := ts.Create(t.Context(), "test-node", "sleep 60")
	h.task.UpdateStatus(t.Context(), runningTask.ID, store.TaskStatusRunning, "", nil)

	w := execPOST(t, router, map[string]interface{}{
		"node_ids": []string{"test-node", "test-node"},
		"command":  "uptime",
		"force":    "true",
	})

	require.Equal(t, 202, w.Code)
	var resp execResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	// Even with conflicts, force creates new tasks
	assert.Len(t, resp.Tasks, 2)
}

func TestExecCreate_MissingCommandAndScript(t *testing.T) {
	_, h := execTestSetup(t)
	router := execRBACRouter(t, h)

	w := execPOST(t, router, map[string]interface{}{
		"node_ids": []string{"test-node"},
	})

	assert.Equal(t, 400, w.Code)
}

func TestExecCreate_MultiNode(t *testing.T) {
	_, h := execTestSetup(t)
	router := execRBACRouter(t, h)

	// Add a second node
	_, err := h.db.Exec(`INSERT INTO nodes (id, name, address, port, user, status) VALUES ('node2', 'node2', '10.0.0.2', 22, 'root', 'online')`)
	require.NoError(t, err)
	h.task.Init(t.Context())

	w := execPOST(t, router, map[string]interface{}{
		"node_ids": []string{"test-node", "node2"},
		"command":  "uptime",
	})

	require.Equal(t, 202, w.Code)
	var resp execResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	require.Len(t, resp.Tasks, 2)
	assert.Equal(t, "test-node", resp.Tasks[0].NodeID)
	assert.Equal(t, "node2", resp.Tasks[1].NodeID)
	assert.Equal(t, store.TaskStatusQueued, resp.Tasks[0].Status)
	assert.Equal(t, "uptime", resp.Tasks[0].Command)
}

func TestExecCreate_SameCommandMerge(t *testing.T) {
	db, h := execTestSetup(t)
	router := execRBACRouter(t, h)

	// Create task directly (no goroutine since exec is set)
	ts := store.NewTaskStore(db)
	ts.Init(t.Context())
	runningTask, _ := ts.Create(t.Context(), "test-node", "uptime")
	h.task.UpdateStatus(t.Context(), runningTask.ID, store.TaskStatusRunning, "", nil)

	// Same command should merge
	w2 := execPOST(t, router, map[string]string{"node_id": "test-node", "command": "uptime"})
	assert.Equal(t, 200, w2.Code)

	var resp execResponse
	json.Unmarshal(w2.Body.Bytes(), &resp)
	require.Len(t, resp.Tasks, 1)
	assert.Equal(t, runningTask.ID, resp.Tasks[0].ID)
}

func TestExecCreate_DifferentCommandConflict(t *testing.T) {
	db, h := execTestSetup(t)
	router := execRBACRouter(t, h)

	ts := store.NewTaskStore(db)
	ts.Init(t.Context())
	runningTask, _ := ts.Create(t.Context(), "test-node", "sleep 30")
	h.task.UpdateStatus(t.Context(), runningTask.ID, store.TaskStatusRunning, "", nil)

	w2 := execPOST(t, router, map[string]string{"node_id": "test-node", "command": "uptime"})
	assert.Equal(t, 409, w2.Code)
}

func TestExecCreate_ForceOverridesConflict(t *testing.T) {
	db, h := execTestSetup(t)
	router := execRBACRouter(t, h)

	ts := store.NewTaskStore(db)
	ts.Init(t.Context())
	runningTask, _ := ts.Create(t.Context(), "test-node", "sleep 30")
	h.task.UpdateStatus(t.Context(), runningTask.ID, store.TaskStatusRunning, "", nil)

	w2 := execPOST(t, router, map[string]string{"node_id": "test-node", "command": "uptime", "force": "true"})
	assert.Equal(t, 202, w2.Code)
}

func TestExecCreate_AsViewer_Forbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	us := store.NewUserStore(db)
	as := service.NewAuthService("test-secret-32byte-long-string!!")
	ah := NewAuthHandler(us, as)

	_, h := execTestSetup(t)
	r := gin.New()
	r.POST("/api/v1/exec", ah.AuthMiddleware(), ah.RBACMiddleware(model.RoleOperator), h.Create)

	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(map[string]string{"node_id": "test-node", "command": "uptime"})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/exec", &buf)
	req.Header.Set("Authorization", "Bearer "+viewerToken())
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assert.Equal(t, 403, w.Code)
}

func TestTasksList(t *testing.T) {
	db, h := execTestSetup(t)
	router := execRBACRouter(t, h)

	// Create tasks directly to avoid goroutine race
	ts := store.NewTaskStore(db)
	ts.Init(t.Context())
	ts.Create(t.Context(), "test-node", "cmd1")
	ts.Create(t.Context(), "test-node", "cmd2")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/tasks", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken())
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	var resp struct {
		Data []store.Task `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Len(t, resp.Data, 2)
}

func TestTasksGet(t *testing.T) {
	db, h := execTestSetup(t)
	router := execRBACRouter(t, h)

	// Create task directly to avoid goroutine
	ts := store.NewTaskStore(db)
	ts.Init(t.Context())
	task, _ := ts.Create(t.Context(), "test-node", "uptime")

	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/v1/tasks/"+task.ID, nil)
	req2.Header.Set("Authorization", "Bearer "+adminToken())
	router.ServeHTTP(w2, req2)

	assert.Equal(t, 200, w2.Code)
	var got store.Task
	json.Unmarshal(w2.Body.Bytes(), &got)
	assert.Equal(t, task.ID, got.ID)
}

func TestTasksGet_NotFound(t *testing.T) {
	_, h := execTestSetup(t)
	router := execRBACRouter(t, h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/tasks/nonexistent-id", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken())
	router.ServeHTTP(w, req)
	assert.Equal(t, 404, w.Code)
}

func TestTasksCancel(t *testing.T) {
	db, h := execTestSetup(t)
	router := execRBACRouter(t, h)

	ts := store.NewTaskStore(db)
	ts.Init(t.Context())
	task, _ := ts.Create(t.Context(), "test-node", "sleep 60")
	h.task.UpdateStatus(t.Context(), task.ID, store.TaskStatusRunning, "", nil)

	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("DELETE", "/api/v1/tasks/"+task.ID, nil)
	req2.Header.Set("Authorization", "Bearer "+adminToken())
	router.ServeHTTP(w2, req2)
	assert.Equal(t, 200, w2.Code)

	updated, _ := h.task.Get(t.Context(), task.ID)
	assert.Equal(t, store.TaskStatusCancelled, updated.Status)
}

func unmarshalTask(t *testing.T, w *httptest.ResponseRecorder) store.Task {
	t.Helper()
	var task store.Task
	json.Unmarshal(w.Body.Bytes(), &task)
	return task
}
