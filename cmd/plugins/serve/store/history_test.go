package store

import (
	"context"
	"testing"
	"time"

	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistoryStore_InitAndRecordOperation(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	s := NewHistoryStore(db)
	require.NoError(t, s.Init(ctx))

	op := &Operation{
		TaskID:    "op-1",
		OpType:    "command",
		Command:   "uptime",
		Targets:   []string{"n1", "n2"},
		Status:    "running",
		CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, s.RecordOperation(ctx, op))

	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM operations WHERE task_id = 'op-1'`).Scan(&count))
	assert.Equal(t, 1, count)

	var targets string
	require.NoError(t, db.QueryRow(`SELECT targets FROM operations WHERE task_id = 'op-1'`).Scan(&targets))
	assert.JSONEq(t, `["n1","n2"]`, targets)
}

func TestHistoryStore_NilSafe(t *testing.T) {
	var s *HistoryStore
	assert.NoError(t, s.Init(context.Background()))
	assert.NoError(t, s.RecordOperation(context.Background(), &Operation{}))
}

func TestHistoryStore_RecordDetailsAndQuery(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := NewHistoryStore(db)
	require.NoError(t, s.Init(ctx))

	require.NoError(t, s.RecordOperation(ctx, &Operation{TaskID: "op-c", OpType: "command", Command: "uptime", Targets: []string{"n1"}, Status: "completed"}))
	require.NoError(t, s.RecordCommandExecution(ctx, &CommandExecution{TaskID: "op-c", NodeID: "n1", Command: "uptime", ExitCode: 0, Stdout: "ok", DurationMs: 12, Success: true}))
	require.NoError(t, s.RecordOperation(ctx, &Operation{TaskID: "op-f", OpType: "file_transfer", Command: "transfer a -> b", Targets: []string{"n1"}, Status: "completed"}))
	require.NoError(t, s.RecordFileTransfer(ctx, &FileTransfer{TaskID: "op-f", NodeID: "n1", FileName: "a.tar", FileSize: 100, TransferType: "push", Status: "completed"}))

	recs, total, err := s.Query(ctx, &QueryOptions{})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, recs, 2)

	byType, total, err := s.Query(ctx, &QueryOptions{OpType: "command"})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, byType, 1)
	assert.Equal(t, "op-c", byType[0].Operation.TaskID)
	require.Len(t, byType[0].CommandExecutions, 1)
	assert.Equal(t, "ok", byType[0].CommandExecutions[0].Stdout)

	byNode, total, err := s.Query(ctx, &QueryOptions{NodeID: "n1"})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, byNode, 2)

	rec, err := s.GetByTaskID(ctx, "op-f")
	require.NoError(t, err)
	require.Len(t, rec.Transfers, 1)
	assert.Equal(t, int64(100), rec.Transfers[0].FileSize)

	_, err = s.GetByTaskID(ctx, "missing")
	assert.Error(t, err)
}

func TestHistoryStore_QueryPaginationAndStatusFilter(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := NewHistoryStore(db)
	require.NoError(t, s.Init(ctx))

	for i := 0; i < 5; i++ {
		st := "completed"
		if i == 0 {
			st = "failed"
		}
		require.NoError(t, s.RecordOperation(ctx, &Operation{TaskID: "t" + string(rune('0'+i)), OpType: "command", Status: st}))
	}

	failed, total, err := s.Query(ctx, &QueryOptions{Status: "failed"})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, failed, 1)

	page, total, err := s.Query(ctx, &QueryOptions{Limit: 2, Offset: 0})
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	assert.Len(t, page, 2)
}

func TestHistoryStore_Cleanup(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := NewHistoryStore(db)
	require.NoError(t, s.Init(ctx))

	old := time.Now().UTC().AddDate(0, 0, -100)
	fresh := time.Now().UTC()
	require.NoError(t, s.RecordOperation(ctx, &Operation{TaskID: "old", OpType: "command", Status: "completed", CreatedAt: old}))
	require.NoError(t, s.RecordOperation(ctx, &Operation{TaskID: "fresh", OpType: "command", Status: "completed", CreatedAt: fresh}))
	require.NoError(t, s.RecordCommandExecution(ctx, &CommandExecution{TaskID: "old", NodeID: "n1", Success: true, CreatedAt: old}))

	deleted, err := s.Cleanup(ctx, 30)
	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted)

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM operations`).Scan(&count)
	assert.Equal(t, 1, count)
}

func TestHistoryStore_Stats(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := NewHistoryStore(db)
	require.NoError(t, s.Init(ctx))

	s.RecordOperation(ctx, &Operation{TaskID: "a", OpType: "command", Status: "completed"})
	s.RecordOperation(ctx, &Operation{TaskID: "b", OpType: "command", Status: "failed"})
	s.RecordOperation(ctx, &Operation{TaskID: "c", OpType: "playbook", Status: "completed"})

	st, err := s.Stats(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, st.Total)
	assert.Equal(t, 2, st.ByOpType["command"])
	assert.Equal(t, 1, st.ByOpType["playbook"])
	assert.Equal(t, 2, st.ByStatus["completed"])
	assert.Equal(t, 1, st.ByStatus["failed"])
}

func TestHistoryStore_ForcedColumnMigration_LegacyDB(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.Exec(`CREATE TABLE operations (
		id INTEGER PRIMARY KEY AUTOINCREMENT, task_id TEXT, op_type TEXT,
		command TEXT, targets TEXT, status TEXT,
		execution_mode TEXT DEFAULT '', playbook_path TEXT DEFAULT '',
		current_task_index INTEGER DEFAULT 0, current_task_phase TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`)
	require.NoError(t, err)

	s := NewHistoryStore(db)
	require.NoError(t, s.Init(ctx))

	require.NoError(t, s.RecordOperation(ctx, &Operation{TaskID: "t1", OpType: "command", Command: "uptime", Targets: []string{"n1"}, Status: "running", Forced: true}))

	var forced int
	require.NoError(t, db.QueryRow(`SELECT forced FROM operations WHERE task_id = 't1'`).Scan(&forced))
	assert.Equal(t, 1, forced)

	require.NoError(t, s.Init(ctx))

	require.NoError(t, s.RecordOperation(ctx, &Operation{TaskID: "t2", OpType: "command", Command: "uptime", Targets: []string{"n1"}, Status: "running"}))
	require.NoError(t, db.QueryRow(`SELECT forced FROM operations WHERE task_id = 't2'`).Scan(&forced))
	assert.Equal(t, 0, forced)
}

func TestHistoryStore_UpdateOperationStatus(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := NewHistoryStore(db)
	require.NoError(t, s.Init(ctx))

	require.NoError(t, s.RecordOperation(ctx, &Operation{TaskID: "op", OpType: "command", Status: "running"}))
	require.NoError(t, s.UpdateOperationStatus(ctx, "op", "completed"))

	rec, err := s.GetByTaskID(ctx, "op")
	require.NoError(t, err)
	assert.Equal(t, "completed", rec.Operation.Status)
}

// TestHistoryStore_ForcedColumnMigration_AlterRace 模拟 CLI 与 serve 并发迁移竞态：
// 另一进程已完成 ALTER（列已存在），本进程的 ALTER 步骤收到
// "duplicate column name: forced" 必须视为成功（参照 playbook_run.go 的容错模式）。
func TestHistoryStore_ForcedColumnMigration_AlterRace(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.Exec(`CREATE TABLE operations (
		id INTEGER PRIMARY KEY AUTOINCREMENT, task_id TEXT, op_type TEXT,
		command TEXT, targets TEXT, status TEXT,
		execution_mode TEXT DEFAULT '', playbook_path TEXT DEFAULT '',
		current_task_index INTEGER DEFAULT 0, current_task_phase TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`)
	require.NoError(t, err)

	// 另一进程先完成了迁移
	_, err = db.Exec(`ALTER TABLE operations ADD COLUMN forced INTEGER DEFAULT 0`)
	require.NoError(t, err)

	s := NewHistoryStore(db)
	// 晚到的进程：ALTER 步骤须容忍 duplicate column
	require.NoError(t, s.addForcedColumn(ctx))
	// 整体入口同样不报错（列已存在走前置检查短路）
	require.NoError(t, s.ensureForcedColumn(ctx))
}

func TestHistoryStore_QueryExposesForced(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := NewHistoryStore(db)
	require.NoError(t, s.Init(ctx))

	require.NoError(t, s.RecordOperation(ctx, &Operation{TaskID: "op-forced", OpType: "playbook", Command: "run deploy.yaml", Targets: []string{"n1"}, Status: "completed", Forced: true}))
	require.NoError(t, s.RecordOperation(ctx, &Operation{TaskID: "op-normal", OpType: "command", Command: "uptime", Targets: []string{"n1"}, Status: "completed"}))

	recs, _, err := s.Query(ctx, &QueryOptions{TaskID: "op-forced"})
	require.NoError(t, err)
	require.Len(t, recs, 1)
	assert.True(t, recs[0].Operation.Forced, "Query 读回的 forced 应为 true")

	recs, _, err = s.Query(ctx, &QueryOptions{TaskID: "op-normal"})
	require.NoError(t, err)
	require.Len(t, recs, 1)
	assert.False(t, recs[0].Operation.Forced)
}
