package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		groups TEXT DEFAULT '[]', labels TEXT DEFAULT '{}'
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
