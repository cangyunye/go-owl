package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerInit_AdminGetsDefaultShortcuts(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{DBPath: filepath.Join(dir, "owl.db"), ListenAddr: "127.0.0.1:0"}
	srv := NewServer(cfg)
	creds, err := srv.Init()
	require.NoError(t, err)
	require.NotNil(t, creds)

	user, err := srv.Users.FindByUsername(context.Background(), "admin")
	require.NoError(t, err)
	cs := store.NewCommandStore(srv.DB)
	n, err := cs.CountByUser(context.Background(), user.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
}

func TestServerInit_RestartDoesNotReseed(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{DBPath: filepath.Join(dir, "owl.db"), ListenAddr: "127.0.0.1:0"}

	srv := NewServer(cfg)
	_, err := srv.Init()
	require.NoError(t, err)
	srv.DB.Close()

	srv2 := NewServer(cfg)
	_, err = srv2.Init()
	require.NoError(t, err)
	user, err := srv2.Users.FindByUsername(context.Background(), "admin")
	require.NoError(t, err)
	cs := store.NewCommandStore(srv2.DB)
	n, _ := cs.CountByUser(context.Background(), user.ID)
	assert.Equal(t, 3, n, "restart must not add more defaults")
}

func TestUserCreate_SeedsDefaults(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{DBPath: filepath.Join(dir, "owl.db"), ListenAddr: "127.0.0.1:0"}
	srv := NewServer(cfg)
	creds, err := srv.Init()
	require.NoError(t, err)
	require.NotNil(t, creds)

	adminToken := loginToken(t, srv, "admin", creds.Password)

	// 管理员建新用户
	body := `{"username":"bob","password":"secret123","role":"operator","display_name":"Bob"}`
	req, _ := http.NewRequest("POST", "/api/v1/users", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)
	assert.Equal(t, 201, w.Code)

	// 新用户登录,查自己的快捷命令应为 3 条默认
	bobToken := loginToken(t, srv, "bob", "secret123")
	req2, _ := http.NewRequest("GET", "/api/v1/shortcuts", nil)
	req2.Header.Set("Authorization", "Bearer "+bobToken)
	w2 := httptest.NewRecorder()
	srv.Router.ServeHTTP(w2, req2)
	assert.Equal(t, 200, w2.Code)
	var list struct {
		Data []struct {
			Name    string `json:"name"`
			Command string `json:"command"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &list))
	require.Len(t, list.Data, 3)
	assert.Equal(t, "df -h", list.Data[0].Command)
	assert.Equal(t, "ps -fu $LOGNAME", list.Data[1].Command)
	assert.Equal(t, "free -h", list.Data[2].Command)
}

func loginToken(t *testing.T, srv *Server, username, password string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req, _ := http.NewRequest("POST", "/api/v1/login", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
	var resp struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp.Token
}
