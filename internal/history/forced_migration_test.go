//go:build !duckdb
// +build !duckdb

package history

import (
	"testing"

	_ "modernc.org/sqlite"

	"github.com/stretchr/testify/require"
)

func TestForcedColumnMigration_LegacyDB(t *testing.T) {
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
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`)
	require.NoError(t, err)

	require.NoError(t, db.EnsureOperationColumns())

	require.NoError(t, GetDB().RecordOperation(&Operation{
		TaskID: "t1", OpType: "command", Command: "uptime",
		Targets: []string{"n1"}, Status: "running", Forced: true,
	}))

	var forced int
	require.NoError(t, conn.QueryRow(
		`SELECT forced FROM operations WHERE task_id = 't1'`).Scan(&forced))
	require.Equal(t, 1, forced)
}

// TestOperationsColumnMigration_LegacyDB 覆盖最早期 CLI 建库场景：
// operations 表仅有 id/task_id/op_type/command/targets/status/created_at 七列，
// 缺失 execution_mode/playbook_path/current_task_index/current_task_phase/forced。
// 此时 RecordOperation 必须经迁移后仍能成功，否则报
// "has no column named execution_mode"。
func TestOperationsColumnMigration_LegacyDB(t *testing.T) {
	cfg := &Config{DBPath: t.TempDir() + "/owl.db"}
	db, err := NewDB(cfg)
	require.NoError(t, err)
	defer db.Close()

	conn := db.Connection()
	_, err = conn.Exec(`DROP TABLE operations`)
	require.NoError(t, err)
	_, err = conn.Exec(`CREATE TABLE operations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id TEXT, op_type TEXT, command TEXT, targets TEXT, status TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`)
	require.NoError(t, err)

	require.NoError(t, db.EnsureOperationColumns())

	for _, col := range []string{"execution_mode", "playbook_path", "current_task_index", "current_task_phase", "forced"} {
		var n int
		require.NoError(t, conn.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('operations') WHERE name = ?`, col).Scan(&n))
		require.Equal(t, 1, n, "column %s should be migrated", col)
	}

	require.NoError(t, GetDB().RecordOperation(&Operation{
		TaskID:           "t1",
		OpType:           "command",
		Command:          "uptime",
		Targets:          []string{"n1"},
		Status:           "running",
		ExecutionMode:    "pipeline",
		PlaybookPath:     "/tmp/a.yaml",
		CurrentTaskIndex: 1,
		CurrentTaskPhase: "run",
		Forced:           true,
	}))

	var em, pp string
	var cti int
	var ctp string
	var forced int
	require.NoError(t, conn.QueryRow(
		`SELECT execution_mode, playbook_path, current_task_index, current_task_phase, forced FROM operations WHERE task_id = 't1'`,
	).Scan(&em, &pp, &cti, &ctp, &forced))
	require.Equal(t, "pipeline", em)
	require.Equal(t, "/tmp/a.yaml", pp)
	require.Equal(t, 1, cti)
	require.Equal(t, "run", ctp)
	require.Equal(t, 1, forced)
}

func TestForcedColumnMigration_Idempotent(t *testing.T) {
	cfg := &Config{DBPath: t.TempDir() + "/owl.db"}
	db, err := NewDB(cfg)
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, db.EnsureOperationColumns())
	require.NoError(t, db.EnsureOperationColumns())

	require.NoError(t, GetDB().RecordOperation(&Operation{
		TaskID: "t2", OpType: "command", Command: "uptime",
		Targets: []string{"n1"}, Status: "running",
	}))

	var forced int
	require.NoError(t, db.Connection().QueryRow(
		`SELECT forced FROM operations WHERE task_id = 't2'`).Scan(&forced))
	require.Equal(t, 0, forced)
}
