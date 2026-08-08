//go:build !duckdb
// +build !duckdb

package history

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestForcedColumnMigration_ConcurrentAlterRace 模拟 CLI 与 serve 并发迁移竞态：
// 另一进程已完成全部 ALTER（operations 所需列已存在），
// 本进程的 EnsureOperationColumns 不得因 duplicate column name 而报错。
func TestForcedColumnMigration_ConcurrentAlterRace(t *testing.T) {
	cfg := &Config{DBPath: t.TempDir() + "/owl.db"}
	db, err := NewDB(cfg)
	require.NoError(t, err)
	defer db.Close()

	conn := db.Connection()
	_, err = conn.Exec(`DROP TABLE operations`)
	require.NoError(t, err)
	_, err = conn.Exec(`CREATE TABLE operations (
		id INTEGER PRIMARY KEY AUTOINCREMENT, task_id TEXT, op_type TEXT,
		command TEXT, targets TEXT, status TEXT,
		execution_mode TEXT DEFAULT '', playbook_path TEXT DEFAULT '',
		current_task_index INTEGER DEFAULT 0, current_task_phase TEXT DEFAULT '',
		forced INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`)
	require.NoError(t, err)

	// 晚到的进程：所有列已存在，整体入口不报错
	require.NoError(t, db.EnsureOperationColumns())
	require.NoError(t, db.EnsureOperationColumns())
}
