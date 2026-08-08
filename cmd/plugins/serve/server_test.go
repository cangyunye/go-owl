package serve

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

func TestServerInit_SyncsManualPlaybooksOnStartup(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	globalDir := filepath.Join(home, ".owl", "playbooks")
	require.NoError(t, os.MkdirAll(globalDir, 0755))

	name := fmt.Sprintf("sync-test-%d.yaml", time.Now().UnixNano())
	pbFile := filepath.Join(globalDir, name)
	require.NoError(t, os.WriteFile(pbFile, []byte("name: sync-test\ntasks: []\n"), 0644))
	t.Cleanup(func() { os.Remove(pbFile) })

	dir := t.TempDir()
	cfg := &Config{
		DBPath:     filepath.Join(dir, "owl.db"),
		ListenAddr: "127.0.0.1:0",
	}
	srv := NewServer(cfg)
	_, err = srv.Init()
	require.NoError(t, err)

	var count int
	require.NoError(t, srv.DB.QueryRow(`SELECT COUNT(*) FROM playbooks WHERE name = 'sync-test'`).Scan(&count))
	srv.DB.Close()
	assert.Equal(t, 1, count, "manually placed playbook must be synced into DB on startup")
}

func TestServerInit_CreatesAdminOnFirstRun(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "owl.db")
	cfg := &Config{
		DBPath:     dbPath,
		ListenAddr: "127.0.0.1:0",
	}

	srv := NewServer(cfg)
	creds, err := srv.Init()
	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "admin", creds.Username)
	assert.Len(t, creds.Password, 12)

	_, err = os.Stat(dbPath)
	assert.NoError(t, err)
}

func TestServerInit_NoDuplicateAdmin(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		DBPath:     filepath.Join(dir, "owl.db"),
		ListenAddr: "127.0.0.1:0",
	}

	srv := NewServer(cfg)
	creds1, err := srv.Init()
	require.NoError(t, err)
	require.NotNil(t, creds1)

	creds2, err := srv.Init()
	require.NoError(t, err)
	assert.Nil(t, creds2)
}

func TestServerInit_JWTSecretPersists(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "owl.db")

	cfg1 := &Config{DBPath: dbPath, ListenAddr: "127.0.0.1:0"}
	srv1 := NewServer(cfg1)
	_, err := srv1.Init()
	require.NoError(t, err)

	firstToken, _ := srv1.Auth.GenerateToken("admin", "admin")

	cfg2 := &Config{DBPath: dbPath, ListenAddr: "127.0.0.1:0"}
	srv2 := NewServer(cfg2)
	_, err = srv2.Init()
	require.NoError(t, err)

	_, err = srv2.Auth.ValidateToken(firstToken)
	assert.NoError(t, err, "token from first startup should be valid after restart")
}

func TestServerInit_LoginWorksAfterInit(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		DBPath:     filepath.Join(dir, "owl.db"),
		ListenAddr: "127.0.0.1:0",
	}

	srv := NewServer(cfg)
	creds, err := srv.Init()
	require.NoError(t, err)
	require.NotNil(t, creds)

	token, err := srv.Auth.GenerateToken(creds.Username, "admin")
	require.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestServer_ServesIndexHTML(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		DBPath:     filepath.Join(dir, "owl.db"),
		ListenAddr: "127.0.0.1:0",
	}
	srv := NewServer(cfg)
	_, err := srv.Init()
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)
	srv.Router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "OWL Console")
	assert.Contains(t, w.Body.String(), "static/js/app.js")
}

func TestServer_ServesStaticCSS(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		DBPath:     filepath.Join(dir, "owl.db"),
		ListenAddr: "127.0.0.1:0",
	}
	srv := NewServer(cfg)
	_, err := srv.Init()
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/static/css/app.css", nil)
	srv.Router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), ":root")
}

func TestServer_ServesJS(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		DBPath:     filepath.Join(dir, "owl.db"),
		ListenAddr: "127.0.0.1:0",
	}
	srv := NewServer(cfg)
	_, err := srv.Init()
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/static/js/app.js", nil)
	srv.Router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "renderLogin")
}

func TestServer_WebSocketUpgradeAndBroadcast(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		DBPath:     filepath.Join(dir, "owl.db"),
		ListenAddr: "127.0.0.1:0",
	}
	srv := NewServer(cfg)
	_, err := srv.Init()
	require.NoError(t, err)

	token, err := srv.Auth.GenerateToken("admin", "admin")
	require.NoError(t, err)

	s := httptest.NewServer(srv.Router)
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, s.URL+"/api/v1/ws?token="+token, nil)
	require.NoError(t, err)
	defer conn.CloseNow()

	time.Sleep(50 * time.Millisecond)

	srv.wsHub.BroadcastTaskUpdate(&struct {
		ID      string `json:"id"`
		Status  string `json:"status"`
		Command string `json:"command"`
	}{ID: "integration-task", Status: "running", Command: "test"})

	var msg struct {
		Type string `json:"type"`
		Data struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	err = wsjson.Read(ctx, conn, &msg)
	require.NoError(t, err)
	assert.Equal(t, "task_update", msg.Type)
	assert.Equal(t, "integration-task", msg.Data.ID)
	assert.Equal(t, "running", msg.Data.Status)
}

func TestServer_WebSocketRequiresAuth(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		DBPath:     filepath.Join(dir, "owl.db"),
		ListenAddr: "127.0.0.1:0",
	}
	srv := NewServer(cfg)
	_, err := srv.Init()
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/ws", nil)
	srv.Router.ServeHTTP(w, req)
	assert.Equal(t, 400, w.Code)

	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/v1/ws?token=badtoken", nil)
	srv.Router.ServeHTTP(w2, req2)
	assert.Equal(t, 401, w2.Code)
}

func TestServer_ServesMarkedJS(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{DBPath: filepath.Join(dir, "owl.db"), ListenAddr: "127.0.0.1:0"}
	srv := NewServer(cfg)
	_, err := srv.Init()
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/static/js/marked.min.js", nil)
	srv.Router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "marked")
	assert.Contains(t, w.Body.String(), "parse")
}

func TestServer_IndexHTML_HasMarkedScript(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{DBPath: filepath.Join(dir, "owl.db"), ListenAddr: "127.0.0.1:0"}
	srv := NewServer(cfg)
	_, err := srv.Init()
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)
	srv.Router.ServeHTTP(w, req)

	body := w.Body.String()
	assert.Contains(t, body, "static/js/marked.min.js", "index.html must include marked.min.js")
	assert.Contains(t, body, "static/js/storage.js", "marked should be after storage.js")
	assert.Contains(t, body, "static/js/app.js", "marked should be before app.js")

	storageIdx := indexOf(body, "static/js/storage.js")
	markedIdx := indexOf(body, "static/js/marked.min.js")
	appIdx := indexOf(body, "static/js/app.js")
	assert.True(t, storageIdx < markedIdx, "storage.js must load before marked.min.js")
	assert.True(t, markedIdx < appIdx, "marked.min.js must load before app.js")
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func TestServer_SPARouting(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		DBPath:     filepath.Join(dir, "owl.db"),
		ListenAddr: "127.0.0.1:0",
	}
	srv := NewServer(cfg)
	_, err := srv.Init()
	require.NoError(t, err)

	tests := []string{"/nodes", "/nodes/test-01", "/login", "/tasks", "/tasks/task-1", "/settings"}
	for _, p := range tests {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", p, nil)
		srv.Router.ServeHTTP(w, req)
		assert.Equal(t, 200, w.Code, "path %s should return 200", p)
		assert.Contains(t, w.Body.String(), "OWL Console", "path %s should serve index.html", p)
	}
}
