package serve

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/handler"
	"github.com/cangyunye/go-owl/internal/logfile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func janitorSetup(t *testing.T) (*Server, *sql.DB) {
	t.Helper()
	t.Setenv("OWL_LOG_DIR", t.TempDir())
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "owl.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY, value TEXT NOT NULL
	)`)
	require.NoError(t, err)
	return &Server{DB: db}, db
}

func makeOldBatch(t *testing.T, opID string, ageDays int) string {
	t.Helper()
	dir := filepath.Join(logfile.ExecutionsDir(), opID)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "node-a.log"), []byte("log-body"), 0o644))
	old := time.Now().AddDate(0, 0, -ageDays)
	require.NoError(t, os.Chtimes(dir, old, old))
	return dir
}

// 定期清理：默认保留期 30 天，过期批次被删除。
func TestRunExecLogsCleanupOnce_DefaultRetention(t *testing.T) {
	s, _ := janitorSetup(t)
	drop := makeOldBatch(t, "op-old", 40)
	keep := makeOldBatch(t, "op-new", 1)

	s.runExecLogsCleanupOnce("test")

	_, err := os.Stat(drop)
	assert.True(t, os.IsNotExist(err), "40-day-old batch removed under default 30-day retention")
	_, err = os.Stat(keep)
	assert.NoError(t, err, "recent batch must stay")
}

// settings 显式保留期生效；0 = 关闭定期清理（文件不动）。
func TestRunExecLogsCleanupOnce_SettingOverrides(t *testing.T) {
	s, db := janitorSetup(t)
	batch := makeOldBatch(t, "op-1", 8)

	_, err := db.Exec(`INSERT INTO settings (key, value) VALUES ('logs.executions_retention_days', '7')`)
	require.NoError(t, err)
	s.runExecLogsCleanupOnce("test")
	_, err = os.Stat(batch)
	assert.True(t, os.IsNotExist(err), "removed under 7-day retention")

	batch2 := makeOldBatch(t, "op-2", 100)
	_, err = db.Exec(`UPDATE settings SET value = '0' WHERE key = 'logs.executions_retention_days'`)
	require.NoError(t, err)
	s.runExecLogsCleanupOnce("test")
	_, err = os.Stat(batch2)
	assert.NoError(t, err, "retention 0 must disable periodic cleanup")
}

// 端点冒烟：HTTP 层确认 handler 与 janitor 共用的保留期读取一致。
func TestExecLogsRetentionDays_Default(t *testing.T) {
	_, db := janitorSetup(t)
	assert.Equal(t, 30, handler.ExecLogsRetentionDays(db))
}
