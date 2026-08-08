package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func transferTestSetup(t *testing.T) (*sql.DB, *TransferHandler, *gin.Engine, string) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	ts := store.NewTaskStore(db)
	require.NoError(t, ts.Init(t.Context()))

	rs := store.NewTransferRecordStore(db)
	require.NoError(t, rs.Init(t.Context()))

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY, name TEXT, address TEXT, port INTEGER DEFAULT 22,
		user TEXT, password TEXT, ssh_key TEXT, status TEXT DEFAULT 'unknown',
		groups TEXT DEFAULT '[]', labels TEXT DEFAULT '{}',
		proxy_jump TEXT DEFAULT '', created_at TIMESTAMP, updated_at TIMESTAMP
	)`)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO nodes (id, address, port, user) VALUES ('node-1', '10.0.0.1', 22, 'test')`)
	require.NoError(t, err)

	us := store.NewUserStore(db)
	as := service.NewAuthService("test-secret-32byte-long-string!!")
	ah := NewAuthHandler(us, as)

	th := NewTransferHandler(db, ts, rs)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	auth := r.Group("/api/v1")
	auth.Use(ah.AuthMiddleware(), ah.RBACMiddleware(model.RoleOperator))
	auth.POST("/transfer", th.Create)

	opToken, _ := as.GenerateToken("operator", "operator")

	return db, th, r, opToken
}

// push 路径不存在时仍返回 202（SSH 阶段会失败）
func TestTransferPush_SourcePathNotFound(t *testing.T) {
	_, _, router, token := transferTestSetup(t)

	body := map[string]interface{}{
		"action":      "push",
		"node_ids":    []string{"node-1"},
		"source_path": "/nonexistent/file.txt",
		"dest_path":   "/tmp/",
		"direction":   "push",
	}
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/transfer", &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, 202, w.Code)
}

// Case 12: push 时源路径存在于中转站
func TestTransferPush_SourcePathExists(t *testing.T) {
	_, _, router, token := transferTestSetup(t)

	// Create the source file
	tmpFile := filepath.Join(t.TempDir(), "deploy.tar")
	os.WriteFile(tmpFile, []byte("mock content"), 0644)

	body := map[string]interface{}{
		"action":      "push",
		"node_ids":    []string{"node-1"},
		"source_path": tmpFile,
		"dest_path":   "/tmp/",
		"direction":   "push",
	}
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/transfer", &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	// 路径校验通过，但 SSH 连接失败（预期），返回 202 且状态为 failed
	assert.Equal(t, 202, w.Code)
	var resp struct {
		Transfers []transferResponse `json:"transfers"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	require.Len(t, resp.Transfers, 1)
	assert.Equal(t, "queued", resp.Transfers[0].Status)
}

// pull 时不需要源路径校验（源在远程节点）
func TestTransferPull_NoSourceCheck(t *testing.T) {
	_, _, router, token := transferTestSetup(t)

	body := map[string]interface{}{
		"action":      "pull",
		"node_ids":    []string{"node-1"},
		"source_path": "/remote/file.log", // 远程文件，服务器不校验
		"dest_path":   "/tmp/",
		"direction":   "pull",
	}
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/transfer", &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	// pull 不校验本地文件存在，应该返回 202
	assert.Equal(t, 202, w.Code)
}

// 未传 node_ids 时按 group/label 过滤解析目标节点
func TestTransferResolve_ByGroupAndLabel(t *testing.T) {
	db, _, router, token := transferTestSetup(t)

	_, err := db.Exec(`INSERT INTO nodes (id, name, address, port, user, status, groups, labels) VALUES
		('web-1', 'web-1', '10.0.1.1', 22, 'root', 'online', '["web","prod"]', '{"env":"prod"}')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO nodes (id, name, address, port, user, status, groups, labels) VALUES
		('web-2', 'web-2', '10.0.1.2', 22, 'root', 'online', '["web"]', '{"env":"stg"}')`)
	require.NoError(t, err)

	body := map[string]interface{}{
		"action":      "push",
		"groups":      []string{"web"},
		"labels":      map[string]string{"env": "prod"},
		"source_path": "/tmp/deploy.tar",
		"dest_path":   "/tmp/",
		"direction":   "push",
	}
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/transfer", &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	require.Equal(t, 202, w.Code)
	var resp struct {
		Transfers []transferResponse `json:"transfers"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	// 命中 web 组 + env=prod 的节点,且 node_ids 为空 -> 解析出 web-1
	require.Len(t, resp.Transfers, 1)
	assert.Equal(t, "web-1", resp.Transfers[0].NodeID)
}

func TestTransferCreate_RecordsHistory(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY, name TEXT, address TEXT, port INTEGER DEFAULT 22,
		user TEXT, password TEXT, ssh_key TEXT, status TEXT DEFAULT 'unknown',
		groups TEXT DEFAULT '[]', labels TEXT DEFAULT '{}',
		proxy_jump TEXT DEFAULT '', created_at TIMESTAMP, updated_at TIMESTAMP)`)
	require.NoError(t, err)
	db.Exec(`INSERT INTO nodes (id, name, address, port, user, status) VALUES ('n1','n1','127.0.0.1',22,'root','online')`)

	ts := store.NewTaskStore(db)
	require.NoError(t, ts.Init(t.Context()))
	rs := store.NewTransferRecordStore(db)
	require.NoError(t, rs.Init(t.Context()))
	hs := store.NewHistoryStore(db)
	require.NoError(t, hs.Init(t.Context()))

	h := NewTransferHandler(db, ts, rs)
	h.History = hs

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/transfer", h.Create)

	body, _ := json.Marshal(map[string]interface{}{
		"node_ids": []string{"n1"}, "source_path": "/tmp/a.tar", "dest_path": "/opt/", "direction": "push",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/transfer", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusAccepted, w.Code)

	recs, total, err := hs.Query(t.Context(), &store.QueryOptions{OpType: "file_transfer"})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	assert.Equal(t, []string{"n1"}, recs[0].Operation.Targets)
	assert.Contains(t, recs[0].Operation.Command, "/tmp/a.tar")
}

func TestParseFileMode(t *testing.T) {
	assert.Equal(t, os.FileMode(0644), parseFileMode("0644"))
	assert.Equal(t, os.FileMode(0755), parseFileMode("755"))
	assert.Equal(t, os.FileMode(0), parseFileMode(""))
	assert.Equal(t, os.FileMode(0), parseFileMode("invalid"))
}

func TestResolveLocalDest(t *testing.T) {
	dir := t.TempDir()
	assert.Equal(t, filepath.Join(dir, "f.txt"), resolveLocalDest(dir+"/", "f.txt"))
	assert.Equal(t, filepath.Join(dir, "f.txt"), resolveLocalDest(dir, "f.txt"))
	assert.Equal(t, "/some/file.txt", resolveLocalDest("/some/file.txt", "other.txt"))
}

func localhostNodeInfo(t *testing.T) *nodeSSHInfo {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	key, err := os.ReadFile(filepath.Join(home, ".ssh", "id_rsa"))
	if err != nil {
		t.Skip("no ~/.ssh/id_rsa, skipping localhost SFTP e2e")
	}
	user := os.Getenv("USER")
	if user == "" {
		user = "root"
	}
	info := &nodeSSHInfo{Address: "127.0.0.1", Port: 22, User: user, SSHKey: string(key)}
	client, sshClient, err := dialSFTP(info)
	if err != nil {
		t.Skipf("localhost SSH unavailable: %v", err)
	}
	client.Close()
	sshClient.Close()
	return info
}

func remoteCleanup(info *nodeSSHInfo, path string) {
	c, sc, err := dialSFTP(info)
	if err != nil {
		return
	}
	defer sc.Close()
	defer c.Close()
	c.Remove(path)
}

func TestSFTPTransfer_PushE2E(t *testing.T) {
	info := localhostNodeInfo(t)

	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "owl_sftp_test.txt")
	content := "hello sftp transfer\nline2\n"
	require.NoError(t, os.WriteFile(src, []byte(content), 0644))

	remotePath := fmt.Sprintf("/tmp/owl_sftp_push_%d.txt", os.Getpid())
	remoteCleanup(info, remotePath)
	defer remoteCleanup(info, remotePath)

	opts := transferOptions{Overwrite: true, Mode: 0600}
	require.NoError(t, sftpTransfer(info, src, remotePath, "push", opts))

	c, sc, err := dialSFTP(info)
	require.NoError(t, err)
	defer sc.Close()
	defer c.Close()
	fi, err := c.Stat(remotePath)
	require.NoError(t, err)
	assert.Equal(t, int64(len(content)), fi.Size())
	assert.Equal(t, os.FileMode(0600), fi.Mode().Perm())
	rf, err := c.Open(remotePath)
	require.NoError(t, err)
	data, _ := io.ReadAll(rf)
	rf.Close()
	assert.Equal(t, content, string(data))
}

func TestSFTPTransfer_OverwriteE2E(t *testing.T) {
	info := localhostNodeInfo(t)

	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "f.txt")
	remotePath := fmt.Sprintf("/tmp/owl_sftp_ow_%d.txt", os.Getpid())
	remoteCleanup(info, remotePath)
	defer remoteCleanup(info, remotePath)

	require.NoError(t, os.WriteFile(src, []byte("v1"), 0644))
	require.NoError(t, sftpTransfer(info, src, remotePath, "push", transferOptions{Overwrite: true}))

	require.NoError(t, os.WriteFile(src, []byte("v2-longer-content"), 0644))
	err := sftpTransfer(info, src, remotePath, "push", transferOptions{Overwrite: false})
	assert.Error(t, err, "expected error when overwrite disabled and file exists")

	require.NoError(t, sftpTransfer(info, src, remotePath, "push", transferOptions{Overwrite: true}))

	c, sc, err := dialSFTP(info)
	require.NoError(t, err)
	defer sc.Close()
	defer c.Close()
	rf, err := c.Open(remotePath)
	require.NoError(t, err)
	data, _ := io.ReadAll(rf)
	rf.Close()
	assert.Equal(t, "v2-longer-content", string(data))
}

func TestSFTPTransfer_ResumeE2E(t *testing.T) {
	info := localhostNodeInfo(t)

	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "f.txt")
	full := "0123456789ABCDEF"
	require.NoError(t, os.WriteFile(src, []byte(full), 0644))

	remotePath := fmt.Sprintf("/tmp/owl_sftp_resume_%d.txt", os.Getpid())
	remoteCleanup(info, remotePath)
	defer remoteCleanup(info, remotePath)

	c, sc, err := dialSFTP(info)
	require.NoError(t, err)
	pf, err := c.Create(remotePath)
	require.NoError(t, err)
	_, err = pf.Write([]byte(full[:8]))
	require.NoError(t, err)
	pf.Close()
	c.Close()
	sc.Close()

	require.NoError(t, sftpTransfer(info, src, remotePath, "push", transferOptions{Resume: true}))

	c2, sc2, err := dialSFTP(info)
	require.NoError(t, err)
	defer sc2.Close()
	defer c2.Close()
	rf, err := c2.Open(remotePath)
	require.NoError(t, err)
	data, _ := io.ReadAll(rf)
	rf.Close()
	assert.Equal(t, full, string(data))
}
