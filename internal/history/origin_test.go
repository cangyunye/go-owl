//go:build !duckdb
// +build !duckdb

package history

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOriginDefaultCLI CLI 侧 operations 默认来源为 cli，显式值保留。
func TestOriginDefaultCLI(t *testing.T) {
	cfg := &Config{DBPath: t.TempDir() + "/owl.db"}
	db, err := NewDB(cfg)
	require.NoError(t, err)
	defer db.Close()
	SetGlobalDB(db)
	defer SetGlobalDB(nil)

	require.NoError(t, RecordOperation(&Operation{
		TaskID: "op-cli", OpType: "command", Command: "uptime",
		Targets: []string{"n1"}, Status: "completed",
	}))
	require.NoError(t, RecordOperation(&Operation{
		TaskID: "op-custom", OpType: "command", Command: "uptime",
		Targets: []string{"n1"}, Status: "completed", Origin: "custom",
	}))

	conn := db.Connection()
	var origin string
	require.NoError(t, conn.QueryRow(`SELECT origin FROM operations WHERE task_id = 'op-cli'`).Scan(&origin))
	assert.Equal(t, "cli", origin, "CLI 侧默认来源应为 cli")

	require.NoError(t, conn.QueryRow(`SELECT origin FROM operations WHERE task_id = 'op-custom'`).Scan(&origin))
	assert.Equal(t, "custom", origin)
}

// TestOriginColumnLegacyMigration 旧库补列 origin（幂等，兼容 serve 已建列）。
func TestOriginColumnLegacyMigration(t *testing.T) {
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
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`)
	require.NoError(t, err)

	require.NoError(t, db.EnsureOperationColumns())

	var n int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('operations') WHERE name = 'origin'`).Scan(&n))
	assert.Equal(t, 1, n, "origin column should be migrated")

	// 重复执行幂等
	require.NoError(t, db.EnsureOperationColumns())
}
