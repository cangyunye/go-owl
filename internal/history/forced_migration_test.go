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

	require.NoError(t, db.EnsureForcedColumn())

	require.NoError(t, GetDB().RecordOperation(&Operation{
		TaskID: "t1", OpType: "command", Command: "uptime",
		Targets: []string{"n1"}, Status: "running", Forced: true,
	}))

	var forced int
	require.NoError(t, conn.QueryRow(
		`SELECT forced FROM operations WHERE task_id = 't1'`).Scan(&forced))
	require.Equal(t, 1, forced)
}

func TestForcedColumnMigration_Idempotent(t *testing.T) {
	cfg := &Config{DBPath: t.TempDir() + "/owl.db"}
	db, err := NewDB(cfg)
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, db.EnsureForcedColumn())
	require.NoError(t, db.EnsureForcedColumn())

	require.NoError(t, GetDB().RecordOperation(&Operation{
		TaskID: "t2", OpType: "command", Command: "uptime",
		Targets: []string{"n1"}, Status: "running",
	}))

	var forced int
	require.NoError(t, db.Connection().QueryRow(
		`SELECT forced FROM operations WHERE task_id = 't2'`).Scan(&forced))
	require.Equal(t, 0, forced)
}
