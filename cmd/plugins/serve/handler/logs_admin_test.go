package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/cangyunye/go-owl/internal/logfile"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func execLogsTestSetup(t *testing.T) (*ExecLogsAdminHandler, *gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	execDir := t.TempDir()
	t.Setenv("OWL_LOG_DIR", execDir)

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY, value TEXT NOT NULL
	)`)
	require.NoError(t, err)

	h := NewExecLogsAdminHandler(db)

	us := store.NewUserStore(db)
	as := service.NewAuthService("test-secret-32byte-long-string!!")
	ah := NewAuthHandler(us, as)
	token, _ := as.GenerateToken("admin", "admin")

	r := gin.New()
	admin := r.Group("/api/v1", ah.AuthMiddleware(), ah.RBACMiddleware("admin"))
	{
		admin.GET("/logs/executions", h.Summary)
		admin.DELETE("/logs/executions", h.Cleanup)
	}
	return h, r, token
}

// makeBatch 直接按目录布局造批次（opID/节点.log + manifest）。
func makeBatch(t *testing.T, opID string, logFiles int, body string) string {
	t.Helper()
	dir := filepath.Join(logfile.ExecutionsDir(), opID)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for i := 0; i < logFiles; i++ {
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, string(rune('a'+i))+".log"), []byte(body), 0o644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("{}"), 0o644))
	return dir
}

func TestExecLogsAdmin_Summary(t *testing.T) {
	_, r, token := execLogsTestSetup(t)
	makeBatch(t, "op-1", 2, "hello-logs")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/logs/executions", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Data struct {
			TotalBatches   int   `json:"total_batches"`
			TotalSizeBytes int64 `json:"total_size_bytes"`
			Batches        []struct {
				OpID      string `json:"op_id"`
				Files     int    `json:"files"`
				SizeBytes int64  `json:"size_bytes"`
			} `json:"batches"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.Data.TotalBatches)
	assert.Greater(t, resp.Data.TotalSizeBytes, int64(0))
	require.Len(t, resp.Data.Batches, 1)
	assert.Equal(t, "op-1", resp.Data.Batches[0].OpID)
	assert.Equal(t, 2, resp.Data.Batches[0].Files)
}

// 清理按 older_than_days 删除过期批次，保留期内不动。
func TestExecLogsAdmin_CleanupWithOlderThanDays(t *testing.T) {
	_, r, token := execLogsTestSetup(t)
	dropDir := makeBatch(t, "op-drop", 2, "old")
	keepDir := makeBatch(t, "op-keep", 2, "new")
	old := time.Now().AddDate(0, 0, -40)
	require.NoError(t, os.Chtimes(dropDir, old, old))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/api/v1/logs/executions?older_than_days=30", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Data struct {
			Removed        int   `json:"removed"`
			ReclaimedBytes int64 `json:"reclaimed_bytes"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.Data.Removed)
	assert.Greater(t, resp.Data.ReclaimedBytes, int64(0))
	_, err := os.Stat(dropDir)
	assert.True(t, os.IsNotExist(err), "expired batch should be removed")
	_, err = os.Stat(keepDir)
	assert.NoError(t, err, "recent batch must stay")
}

// 无查询参数时读取 settings 的 logs.executions_retention_days 作为保留期。
func TestExecLogsAdmin_CleanupUsesSettingDefault(t *testing.T) {
	h, r, token := execLogsTestSetup(t)
	_, err := h.db.Exec(`INSERT INTO settings (key, value) VALUES ('logs.executions_retention_days', '7')`)
	require.NoError(t, err)

	dropDir := makeBatch(t, "op-drop", 1, "old")
	old := time.Now().AddDate(0, 0, -8)
	require.NoError(t, os.Chtimes(dropDir, old, old))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/api/v1/logs/executions", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	_, statErr := os.Stat(dropDir)
	assert.True(t, os.IsNotExist(statErr), "8-day-old batch removed under 7-day retention")
}

// all=1 全量清空（无视保留期）。
func TestExecLogsAdmin_CleanupAll(t *testing.T) {
	_, r, token := execLogsTestSetup(t)
	makeBatch(t, "op-1", 1, "x")
	makeBatch(t, "op-2", 1, "y")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/api/v1/logs/executions?all=1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	entries, err := os.ReadDir(logfile.ExecutionsDir())
	require.NoError(t, err)
	assert.Empty(t, entries, "all batches should be wiped")
}

// 非法 older_than_days 拒绝。
func TestExecLogsAdmin_CleanupInvalidParam(t *testing.T) {
	_, r, token := execLogsTestSetup(t)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/api/v1/logs/executions?older_than_days=-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
