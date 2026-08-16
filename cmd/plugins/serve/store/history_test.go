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

// TestHistoryStore_UsernameRoundtrip 验证 serve 侧操作的执行用户归属：
// 写入 Username 后 Query/GetByTaskID 能原样读回。
func TestHistoryStore_UsernameRoundtrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := NewHistoryStore(db)
	require.NoError(t, s.Init(ctx))

	require.NoError(t, s.RecordOperation(ctx, &Operation{TaskID: "op-u1", OpType: "command", Command: "uptime", Targets: []string{"n1"}, Status: "completed", Username: "alice"}))
	require.NoError(t, s.RecordOperation(ctx, &Operation{TaskID: "op-u2", OpType: "command", Command: "df -h", Targets: []string{"n2"}, Status: "failed", Username: "bob"}))

	rec, err := s.GetByTaskID(ctx, "op-u1")
	require.NoError(t, err)
	assert.Equal(t, "alice", rec.Operation.Username)

	recs, total, err := s.Query(ctx, &QueryOptions{})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, recs, 2)
	usernames := map[string]bool{}
	for _, r := range recs {
		usernames[r.Operation.Username] = true
	}
	assert.True(t, usernames["alice"], "alice's record should carry username")
	assert.True(t, usernames["bob"], "bob's record should carry username")
}

// TestHistoryStore_QueryUserFilter 验证按执行用户筛选：
// User 精确匹配，其他用户与空归属（CLI 存量/未归属）记录不命中。
func TestHistoryStore_QueryUserFilter(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := NewHistoryStore(db)
	require.NoError(t, s.Init(ctx))

	require.NoError(t, s.RecordOperation(ctx, &Operation{TaskID: "a1", OpType: "command", Command: "uptime", Status: "completed", Username: "alice"}))
	require.NoError(t, s.RecordOperation(ctx, &Operation{TaskID: "a2", OpType: "command", Command: "who", Status: "failed", Username: "alice"}))
	require.NoError(t, s.RecordOperation(ctx, &Operation{TaskID: "b1", OpType: "command", Command: "df -h", Status: "completed", Username: "bob"}))
	require.NoError(t, s.RecordOperation(ctx, &Operation{TaskID: "c1", OpType: "command", Command: "legacy", Status: "completed"}))

	byAlice, total, err := s.Query(ctx, &QueryOptions{User: "alice"})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, byAlice, 2)

	byBob, total, err := s.Query(ctx, &QueryOptions{User: "bob"})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, "b1", byBob[0].Operation.TaskID)

	none, total, err := s.Query(ctx, &QueryOptions{User: "nobody"})
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Len(t, none, 0)

	unassigned, total, err := s.Query(ctx, &QueryOptions{User: ""})
	require.NoError(t, err)
	assert.Equal(t, 4, total, "empty user filter should not filter anything")
	assert.Len(t, unassigned, 4)
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

func TestHistoryStore_QueryCommandFilter(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := NewHistoryStore(db)
	require.NoError(t, s.Init(ctx))

	require.NoError(t, s.RecordOperation(ctx, &Operation{TaskID: "c1", OpType: "command", Command: "uptime", Targets: []string{"n1"}, Status: "completed"}))
	require.NoError(t, s.RecordOperation(ctx, &Operation{TaskID: "c2", OpType: "command", Command: "systemctl status nginx", Targets: []string{"n1"}, Status: "completed"}))
	require.NoError(t, s.RecordOperation(ctx, &Operation{TaskID: "c3", OpType: "script", Command: "deploy.sh", Targets: []string{"n2"}, Status: "failed"}))

	byCmd, total, err := s.Query(ctx, &QueryOptions{Command: "systemctl"})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, byCmd, 1)
	assert.Equal(t, "c2", byCmd[0].Operation.TaskID)

	partial, total, err := s.Query(ctx, &QueryOptions{Command: "syst"})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, "c2", partial[0].Operation.TaskID)

	none, total, err := s.Query(ctx, &QueryOptions{Command: "no-such-cmd"})
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Len(t, none, 0)
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

// TestHistoryStore_OperationsColumnMigration_LegacyDB 覆盖早期 CLI 建库场景：
// operations 表仅有 id/task_id/op_type/command/targets/status/created_at 七列，
// 缺失 execution_mode/playbook_path/current_task_index/current_task_phase/forced。
// CREATE TABLE IF NOT EXISTS 对存量表不生效，必须逐列 ALTER 补齐，
// 否则前端执行命令时 RecordOperation 报 "has no column named execution_mode"。
func TestHistoryStore_OperationsColumnMigration_LegacyDB(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.Exec(`CREATE TABLE operations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id TEXT,
		op_type TEXT,
		command TEXT,
		targets TEXT,
		status TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`)
	require.NoError(t, err)

	s := NewHistoryStore(db)
	require.NoError(t, s.Init(ctx))

	for _, col := range []string{"execution_mode", "playbook_path", "current_task_index", "current_task_phase", "forced"} {
		var n int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('operations') WHERE name = ?`, col).Scan(&n))
		assert.Equal(t, 1, n, "column %s should be migrated into legacy operations table", col)
	}

	op := &Operation{
		TaskID:           "legacy-1",
		OpType:           "command",
		Command:          "uptime",
		Targets:          []string{"n1"},
		Status:           "running",
		ExecutionMode:    "serial",
		PlaybookPath:     "/tmp/demo.yaml",
		CurrentTaskIndex: 3,
		CurrentTaskPhase: "run",
		Forced:           true,
		CreatedAt:        time.Now().UTC(),
	}
	require.NoError(t, s.RecordOperation(ctx, op))

	rec, err := s.GetByTaskID(ctx, "legacy-1")
	require.NoError(t, err)
	assert.Equal(t, "serial", rec.Operation.ExecutionMode)
	assert.Equal(t, "/tmp/demo.yaml", rec.Operation.PlaybookPath)
	assert.Equal(t, 3, rec.Operation.CurrentTaskIndex)
	assert.Equal(t, "run", rec.Operation.CurrentTaskPhase)
	assert.True(t, rec.Operation.Forced)

	require.NoError(t, s.Init(ctx))
	require.NoError(t, s.RecordOperation(ctx, &Operation{TaskID: "legacy-2", OpType: "command", Status: "running"}))
}

// TestHistoryStore_UsernameColumnMigration_LegacyDB 覆盖旧库场景：
// 存量 operations 表无 username 列（早期 CLI 建库），serve 启动 Init 后
// 必须 ALTER 补齐，否则带 Username 的 RecordOperation 报
// "has no column named username"。
func TestHistoryStore_UsernameColumnMigration_LegacyDB(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.Exec(`CREATE TABLE operations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id TEXT,
		op_type TEXT,
		command TEXT,
		targets TEXT,
		status TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`)
	require.NoError(t, err)

	s := NewHistoryStore(db)
	require.NoError(t, s.Init(ctx))

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('operations') WHERE name = 'username'`).Scan(&n))
	assert.Equal(t, 1, n, "username column should be migrated into legacy operations table")

	require.NoError(t, s.RecordOperation(ctx, &Operation{TaskID: "legacy-u1", OpType: "command", Command: "uptime", Targets: []string{"n1"}, Status: "completed", Username: "alice"}))

	rec, err := s.GetByTaskID(ctx, "legacy-u1")
	require.NoError(t, err)
	assert.Equal(t, "alice", rec.Operation.Username)
}

// TestHistoryStore_UsernameMigration_Idempotent 验证列已存在时重复 Init
// 不报错（serve 与 CLI 可能并发迁移同一旧库）。
func TestHistoryStore_UsernameMigration_Idempotent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := NewHistoryStore(db)
	require.NoError(t, s.Init(ctx))
	require.NoError(t, s.Init(ctx))
	require.NoError(t, s.Init(ctx))
	require.NoError(t, s.RecordOperation(ctx, &Operation{TaskID: "idem-1", OpType: "command", Status: "completed", Username: "admin"}))
}

// TestHistoryStore_UsernameColumn_CLICompatInsert 验证 CLI（internal/history）
// 风格 INSERT：显式列清单不含 username。serve 建表后该语句必须仍成功，
// 未归属记录读回 Username 为空串——保证 CLI 与 serve 共用 owl.db 不互相破坏。
func TestHistoryStore_UsernameColumn_CLICompatInsert(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := NewHistoryStore(db)
	require.NoError(t, s.Init(ctx))

	_, err := db.ExecContext(ctx, `
		INSERT INTO operations (task_id, op_type, command, targets, status, execution_mode, playbook_path, current_task_index, current_task_phase, forced, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "cli-write-1", "command", "uptime", `["n1"]`, "completed", "", "", 0, "", 0, time.Now().UTC())
	require.NoError(t, err)

	rec, err := s.GetByTaskID(ctx, "cli-write-1")
	require.NoError(t, err)
	assert.Equal(t, "", rec.Operation.Username)
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
	// 另一进程已先完成迁移：晚到的进程须容忍 duplicate column，整体入口不报错
	require.NoError(t, s.ensureOperationColumns(ctx))
	require.NoError(t, s.ensureOperationColumns(ctx))
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
