//go:build duckdb
// +build duckdb

package history

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestOperationsColumnMigration_DuckDB_LegacyDB 验证 DuckDB 后端的
// EnsureOperationColumns 幂等补齐早期缺失列。
func TestOperationsColumnMigration_DuckDB_LegacyDB(t *testing.T) {
	cfg := &Config{DBPath: t.TempDir() + "/owl.db"}
	db, err := NewDB(cfg)
	require.NoError(t, err)
	defer db.Close()

	conn := db.Connection()
	_, err = conn.Exec(`DROP TABLE operations`)
	require.NoError(t, err)
	_, err = conn.Exec(`CREATE TABLE operations (
		id BIGINT PRIMARY KEY DEFAULT NEXTVAL('seq_operations_id'),
		task_id VARCHAR, op_type VARCHAR, command VARCHAR, targets VARCHAR, status VARCHAR,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP)`)
	require.NoError(t, err)

	require.NoError(t, db.EnsureOperationColumns())

	for _, col := range []string{"execution_mode", "playbook_path", "current_task_index", "current_task_phase", "forced"} {
		var n int
		require.NoError(t, conn.QueryRow(
			`SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'operations' AND column_name = ?`, col).Scan(&n))
		require.Equal(t, 1, n, "column %s should be migrated", col)
	}

	require.NoError(t, RecordOperation(&Operation{
		TaskID:        "d1",
		OpType:        "command",
		Command:       "uptime",
		Targets:       []string{"n1"},
		Status:        "running",
		ExecutionMode: "pipeline",
		Forced:        true,
	}))
}
