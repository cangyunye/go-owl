package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/cangyunye/go-owl/internal/logfile"
	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

type mockExecutor struct {
	output   string
	exitCode int
	err      error
}

func (m *mockExecutor) Execute(_ context.Context, _, _ string) (string, int, error) {
	return m.output, m.exitCode, m.err
}

func (m *mockExecutor) ExecuteStream(_ context.Context, _, _ string, ch chan<- OutputLine) (int, error) {
	if m.err != nil {
		return -1, m.err
	}
	ch <- OutputLine{NodeID: "test-node", Line: "line1", Type: "stdout"}
	ch <- OutputLine{NodeID: "test-node", Line: "line2", Type: "stdout"}
	close(ch)
	return m.exitCode, nil
}

func TestExecStream_Success(t *testing.T) {
	_, h := execTestSetup(t)
	ch := make(chan OutputLine, 10)
	go func() {
		code, err := h.exec.(*mockExecutor).ExecuteStream(t.Context(), "test-node", "uptime", ch)
		require.NoError(t, err)
		require.Equal(t, 0, code)
	}()

	var lines []string
	for l := range ch {
		lines = append(lines, l.Line)
	}
	require.Equal(t, []string{"line1", "line2"}, lines)
}

func execTestSetup(t *testing.T) (*sql.DB, *ExecHandler) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
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
	db.SetMaxOpenConns(1)
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

func TestExecCreate_NoSelection_RunsOnAllNodes(t *testing.T) {
	_, h := execTestSetup(t)
	router := execRBACRouter(t, h)

	_, err := h.db.Exec(`INSERT INTO nodes (id, name, address, port, user, status) VALUES ('node2', 'node2', '10.0.0.2', 22, 'root', 'online')`)
	require.NoError(t, err)

	// 未选择任何节点、无 group/label 过滤 -> 等价于对全部节点执行
	w := execPOST(t, router, map[string]string{"command": "uptime"})
	require.Equal(t, 202, w.Code)
	var resp execResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	require.Len(t, resp.Tasks, 2)
	ids := []string{resp.Tasks[0].NodeID, resp.Tasks[1].NodeID}
	assert.Contains(t, ids, "test-node")
	assert.Contains(t, ids, "node2")
}

func TestExecCreate_EmptyDB_NoTargetNodes(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY, name TEXT, address TEXT, port INTEGER DEFAULT 22,
		user TEXT, password TEXT, ssh_key TEXT, status TEXT DEFAULT 'unknown',
		groups TEXT DEFAULT '[]', labels TEXT DEFAULT '{}',
		proxy_jump TEXT DEFAULT '', created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	require.NoError(t, err)

	ts := store.NewTaskStore(db)
	require.NoError(t, ts.Init(t.Context()))

	h := NewExecHandler(db, ts, nil)
	h.exec = &mockExecutor{output: "ok\n", exitCode: 0}
	router := execRBACRouter(t, h)

	w := execPOST(t, router, map[string]string{"command": "uptime"})
	assert.Equal(t, 400, w.Code)
}

func TestExecCreate_NodeNotFound(t *testing.T) {
	_, h := execTestSetup(t)
	router := execRBACRouter(t, h)
	w := execPOST(t, router, map[string]string{"node_id": "nonexistent", "command": "uptime"})
	assert.Equal(t, 400, w.Code)
	assert.Contains(t, w.Body.String(), "节点不存在")
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

func TestExecCreate_GroupAndLabelIntersect(t *testing.T) {
	db, h := execTestSetup(t)
	router := execRBACRouter(t, h)

	db.Exec(`INSERT INTO nodes (id, name, address, port, user, status, groups, labels) VALUES
		('web-prod', 'web-prod', '10.0.1.1', 22, 'root', 'online', '["web","prod"]', '{"env":"prod"}')`)
	db.Exec(`INSERT INTO nodes (id, name, address, port, user, status, groups, labels) VALUES
		('web-stg', 'web-stg', '10.0.1.2', 22, 'root', 'online', '["web"]', '{"env":"stg"}')`)
	db.Exec(`INSERT INTO nodes (id, name, address, port, user, status, groups, labels) VALUES
		('db-prod', 'db-prod', '10.0.2.1', 22, 'root', 'online', '["db"]', '{"env":"prod"}')`)

	// 同时传 groups + labels 时取交集:web 组 且 env=prod -> 仅 web-prod
	w := execPOST(t, router, map[string]interface{}{
		"groups":  []string{"web"},
		"labels":  map[string]string{"env": "prod"},
		"command": "uptime",
	})

	require.Equal(t, 202, w.Code)
	var resp execResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	require.Len(t, resp.Tasks, 1)
	assert.Equal(t, "web-prod", resp.Tasks[0].NodeID)
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
	db.SetMaxOpenConns(1)
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

func TestExecCreate_RecordsHistory(t *testing.T) {
	db, h := execTestSetup(t)
	hs := store.NewHistoryStore(db)
	require.NoError(t, hs.Init(t.Context()))
	h.History = hs

	_, h2 := h, h
	_ = h2
	router := execRBACRouter(t, h)
	w := execPOST(t, router, map[string]string{"node_id": "test-node", "command": "uptime"})
	require.Equal(t, 202, w.Code)

	deadline := time.Now().Add(10 * time.Second)
	var total int
	for time.Now().Before(deadline) {
		recs, t2, _ := hs.Query(t.Context(), &store.QueryOptions{OpType: "command"})
		total = t2
		if len(recs) > 0 && recs[0].Operation.Status != "running" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	recs, total, err := hs.Query(t.Context(), &store.QueryOptions{OpType: "command"})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	assert.Equal(t, "uptime", recs[0].Operation.Command)
	assert.Equal(t, []string{"test-node"}, recs[0].Operation.Targets)
	assert.Equal(t, "completed", recs[0].Operation.Status)
	require.Len(t, recs[0].CommandExecutions, 1)
	assert.Equal(t, "test-node", recs[0].CommandExecutions[0].NodeID)
	assert.True(t, recs[0].CommandExecutions[0].Success)
}

func TestExecuteTask_WritesExecutionLog_SuccessAndFailure(t *testing.T) {
	t.Setenv("OWL_LOG_DIR", t.TempDir())
	_, h := execTestSetup(t)
	h.LogWriter = logfile.NewNodeLogWriter("")

	ctx := context.Background()

	// success (mock streams line1/line2)
	h.exec = &mockExecutor{output: "log-out-1\n", exitCode: 0}
	ts, err := h.task.CreateWithRecord(ctx, "test-node", "uptime", "op-succ")
	require.NoError(t, err)
	h.executeTask(ts.ID, ExecConfig{Command: "uptime", Retry: 0})

	succPath := filepath.Join(logfile.ExecutionsDir(), "op-succ", "test-node.log")
	succData, err := os.ReadFile(succPath)
	require.NoError(t, err, "success log should exist")
	require.Contains(t, string(succData), "COMMAND: uptime")
	require.Contains(t, string(succData), "line1")

	// failure: error message preserved
	h.exec = &mockExecutor{output: "partial\n", exitCode: 1, err: assert.AnError}
	tf, err := h.task.CreateWithRecord(ctx, "test-node", "bad-cmd", "op-fail")
	require.NoError(t, err)
	h.executeTask(tf.ID, ExecConfig{Command: "bad-cmd", Retry: 0, NoRetry: true})

	failPath := filepath.Join(logfile.ExecutionsDir(), "op-fail", "test-node.log")
	failData, err := os.ReadFile(failPath)
	require.NoError(t, err, "failure log should exist")
	require.Contains(t, string(failData), "ERROR:")
}

func TestBuildExecCommand_CommandMode(t *testing.T) {
	cfg := ExecConfig{Command: "uptime"}
	assert.Equal(t, "uptime", buildExecCommand("uptime", cfg))
}

func TestExecuteTask_StreamsOutputToWS(t *testing.T) {
	_, h := execTestSetup(t)

	hub := NewWSHub()
	h.hub = hub

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		require.NoError(t, err)
		hub.Subscribe(r.Context(), conn)
		<-r.Context().Done()
	}))
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, s.URL, nil)
	require.NoError(t, err)
	defer conn.CloseNow()
	time.Sleep(50 * time.Millisecond)

	h.exec = &mockExecutor{output: "line1\nline2\n", exitCode: 0}

	task, err := h.task.Create(ctx, "test-node", "uptime")
	require.NoError(t, err)

	go h.executeTask(task.ID, ExecConfig{Command: "uptime", Retry: 0})

	var seenOutput []string
	completed := false
	for !completed {
		var msg struct {
			Type string                 `json:"type"`
			Data map[string]interface{} `json:"data"`
		}
		err := wsjson.Read(ctx, conn, &msg)
		require.NoError(t, err)
		switch msg.Type {
		case "task_output":
			line, _ := msg.Data["line"].(string)
			seenOutput = append(seenOutput, line)
		case "task_update":
			if msg.Data["id"] == task.ID && msg.Data["status"] == "completed" {
				completed = true
			}
		}
	}

	require.Equal(t, []string{"line1", "line2"}, seenOutput)

	got, err := h.task.Get(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, store.TaskStatusCompleted, got.Status)
	require.Contains(t, got.Output, "line1")
	require.Contains(t, got.Output, "line2")
}

func TestBuildExecCommand_ScriptMode(t *testing.T) {
	content := "#!/bin/bash\necho hello"
	cfg := ExecConfig{
		ScriptContent: content,
		ScriptName:    "deploy.sh",
		ScriptDest:    "/tmp",
		ScriptKeep:    false,
	}
	cmd := buildExecCommand("script: deploy.sh", cfg)

	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	assert.Contains(t, cmd, "echo '"+encoded+"' | base64 -d > /tmp/deploy.sh")
	assert.Contains(t, cmd, "chmod +x /tmp/deploy.sh")
	assert.Contains(t, cmd, "&& /tmp/deploy.sh")
	assert.Contains(t, cmd, "rc=$?; rm -f /tmp/deploy.sh; exit $rc")
}

func TestBuildExecCommand_ScriptKeep(t *testing.T) {
	cfg := ExecConfig{
		ScriptContent: "echo hi",
		ScriptName:    "keep.sh",
		ScriptDest:    "/opt",
		ScriptKeep:    true,
	}
	cmd := buildExecCommand("script: keep.sh", cfg)
	assert.Contains(t, cmd, "> /opt/keep.sh")
	assert.NotContains(t, cmd, "rm -f")
}

func TestBuildExecCommand_ScriptArgs(t *testing.T) {
	cfg := ExecConfig{
		ScriptContent: "echo hi",
		ScriptName:    "run.sh",
		ScriptDest:    "/tmp",
		ScriptArgs:    "--env prod",
	}
	cmd := buildExecCommand("script: run.sh", cfg)
	assert.Contains(t, cmd, "&& /tmp/run.sh --env prod")
}

func TestResolveScriptContent_Inline(t *testing.T) {
	content, name, err := resolveScriptContent(execRequest{ScriptContent: "echo hi", ScriptName: "a.sh"}, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, "echo hi", content)
	assert.Equal(t, "a.sh", name)
}

func TestResolveScriptContent_InlineDefaultName(t *testing.T) {
	content, name, err := resolveScriptContent(execRequest{ScriptContent: "echo hi"}, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, "echo hi", content)
	assert.Equal(t, "script.sh", name)
}

func TestResolveScriptContent_Missing(t *testing.T) {
	_, _, err := resolveScriptContent(execRequest{}, t.TempDir())
	assert.Error(t, err)
}

func TestResolveScriptContent_StagingRef(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "deploy.sh"), []byte("#!/bin/bash\necho staging"), 0644))

	content, name, err := resolveScriptContent(execRequest{ScriptRef: "deploy.sh"}, dir)
	require.NoError(t, err)
	assert.Equal(t, "#!/bin/bash\necho staging", content)
	assert.Equal(t, "deploy.sh", name)
}

func TestResolveScriptContent_StagingRef_NotFound(t *testing.T) {
	_, _, err := resolveScriptContent(execRequest{ScriptRef: "missing.sh"}, t.TempDir())
	assert.ErrorContains(t, err, "script not found in staging")
}

func TestResolveScriptContent_StagingRef_RejectTraversal(t *testing.T) {
	_, _, err := resolveScriptContent(execRequest{ScriptRef: "../secret.sh"}, t.TempDir())
	assert.Error(t, err)
}

func TestResolveScriptContent_StagingRef_RejectSubpath(t *testing.T) {
	_, _, err := resolveScriptContent(execRequest{ScriptRef: "sub/dir.sh"}, t.TempDir())
	assert.Error(t, err)
}

func TestExecCreate_ScriptMode_RecordsHistory(t *testing.T) {
	db, h := execTestSetup(t)
	hs := store.NewHistoryStore(db)
	require.NoError(t, hs.Init(t.Context()))
	h.History = hs

	router := execRBACRouter(t, h)
	w := execPOST(t, router, map[string]interface{}{
		"mode":           "script",
		"node_ids":       []string{"test-node"},
		"script_content": "#!/bin/bash\necho hello",
		"script_name":    "deploy.sh",
	})
	require.Equal(t, 202, w.Code)

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		recs, total, _ := hs.Query(t.Context(), &store.QueryOptions{OpType: "script"})
		if total > 0 && recs[0].Operation.Status != "running" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	recs, total, err := hs.Query(t.Context(), &store.QueryOptions{OpType: "script"})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	assert.Equal(t, "script", recs[0].Operation.OpType)
	assert.Equal(t, "script: deploy.sh", recs[0].Operation.Command)
	assert.Equal(t, []string{"test-node"}, recs[0].Operation.Targets)
}

func TestExecCreate_ScriptMode_MissingContent(t *testing.T) {
	_, h := execTestSetup(t)
	router := execRBACRouter(t, h)
	w := execPOST(t, router, map[string]interface{}{
		"mode":     "script",
		"node_ids": []string{"test-node"},
	})
	assert.Equal(t, 400, w.Code)
	assert.True(t, strings.Contains(w.Body.String(), "script_content, script_url or script_ref is required"))
}

func TestExecCreate_ScriptMode_StagingRef(t *testing.T) {
	_, h := execTestSetup(t)
	router := execRBACRouter(t, h)

	_, err := h.db.Exec("CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT)")
	require.NoError(t, err)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "deploy.sh"), []byte("#!/bin/bash\necho from-staging"), 0644))
	_, err = h.db.Exec(`INSERT INTO settings (key, value) VALUES ('staging_dir', ?)`, dir)
	require.NoError(t, err)

	w := execPOST(t, router, map[string]interface{}{
		"mode":       "script",
		"node_ids":   []string{"test-node"},
		"script_ref": "deploy.sh",
	})

	require.Equal(t, 202, w.Code)
	var resp execResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	require.Len(t, resp.Tasks, 1)
	assert.Equal(t, "script: deploy.sh", resp.Tasks[0].Command)
}

func TestExecCreate_ScriptMode_StagingRef_Blacklist(t *testing.T) {
	_, h := execTestSetup(t)
	router := execRBACRouter(t, h)

	_, err := h.db.Exec("CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT)")
	require.NoError(t, err)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "danger.sh"), []byte("#!/bin/bash\nrm -rf /\n"), 0644))
	_, err = h.db.Exec(`INSERT INTO settings (key, value) VALUES ('staging_dir', ?)`, dir)
	require.NoError(t, err)

	w := execPOST(t, router, map[string]interface{}{
		"mode":       "script",
		"node_ids":   []string{"test-node"},
		"script_ref": "danger.sh",
	})

	require.Equal(t, http.StatusForbidden, w.Code)
	assert.True(t, strings.Contains(w.Body.String(), "危险命令已被黑名单拦截"))
}

func TestResolveStagingScriptRef_RejectDotPaths(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.sh"), []byte("echo hi"), 0644))

	_, _, err := resolveStagingScriptRef(dir, ".")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid script_ref")
	_, _, err = resolveStagingScriptRef(dir, "..")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid script_ref")
}

func TestResolveStagingScriptRef_RejectEmptyFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "empty.sh"), nil, 0644))

	_, _, err := resolveStagingScriptRef(dir, "empty.sh")
	assert.ErrorContains(t, err, "is empty")
}

func TestExecCreate_ScriptMode_StagingRef_MissingFile(t *testing.T) {
	_, h := execTestSetup(t)
	router := execRBACRouter(t, h)

	_, err := h.db.Exec("CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT)")
	require.NoError(t, err)

	_, err = h.db.Exec(`INSERT INTO settings (key, value) VALUES ('staging_dir', ?)`, t.TempDir())
	require.NoError(t, err)

	w := execPOST(t, router, map[string]interface{}{
		"mode":       "script",
		"node_ids":   []string{"test-node"},
		"script_ref": "ghost.sh",
	})
	assert.Equal(t, 400, w.Code)
	assert.True(t, strings.Contains(w.Body.String(), "script not found in staging"))
}
