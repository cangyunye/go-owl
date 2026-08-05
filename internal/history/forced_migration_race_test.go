//go:build !duckdb
// +build !duckdb

package history

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestForcedColumnMigration_ConcurrentAlterRace 模拟 CLI 与 serve 并发迁移竞态：
// 另一进程已完成 ALTER TABLE operations ADD COLUMN forced，
// 本进程的 ALTER 收到 "duplicate column name: forced" 必须视为成功。
func TestForcedColumnMigration_ConcurrentAlterRace(t *testing.T) {
	cfg := &Config{DBPath: t.TempDir() + "/owl.db"}
	db, err := NewDB(cfg)
	require.NoError(t, err)
	defer db.Close()

	sqlite, ok := db.(*SQLite3)
	require.True(t, ok)

	conn := db.Connection()
	_, err = conn.Exec(`DROP TABLE operations`)
	require.NoError(t, err)
	_, err = conn.Exec(`CREATE TABLE operations (
		id INTEGER PRIMARY KEY AUTOINCREMENT, task_id TEXT, op_type TEXT,
		command TEXT, targets TEXT, status TEXT,
		execution_mode TEXT DEFAULT '', playbook_path TEXT DEFAULT '',
		current_task_index INTEGER DEFAULT 0, current_task_phase TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`)
	require.NoError(t, err)

	// 另一进程先完成了迁移
	_, err = conn.Exec(`ALTER TABLE operations ADD COLUMN forced INTEGER DEFAULT 0`)
	require.NoError(t, err)

	// 晚到的进程：ALTER 步骤须容忍 duplicate column
	require.NoError(t, sqlite.addForcedColumn())
	// 整体入口同样不报错
	require.NoError(t, db.EnsureForcedColumn())
}
