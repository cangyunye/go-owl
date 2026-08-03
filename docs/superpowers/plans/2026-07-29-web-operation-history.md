# Web 操作历史子系统 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 Web 控制台补齐 CLI `owl history` 对应的操作历史能力，记录 command/file_transfer/playbook/node_manage 操作并与 CLI 历史统一（同库同表）。

**Architecture:** serve module 内自建 `HistoryStore`（纯 Go modernc），写入与 `internal/history` 逐字相同 schema 的共享 `~/.owl/owl.db`；handler 通过 nil-safe 的导出字段 `History` 埋点记录；新增 `HistoryHandler` 提供查询/详情/导出/清理/统计；前端重写 `history.js`。

**Tech Stack:** Go (gin, modernc.org/sqlite, testify, google/uuid, gopkg.in/yaml.v3), 原生 JS SPA。

## Global Constraints

- **纯 Go 约束**：serve module（`cmd/plugins/serve`）不得引入任何 CGO 依赖（禁止 import `internal/history`、`mattn/go-sqlite3`、`duckdb`）。仅用 `modernc.org/sqlite`。`make build-serve` 必须保持无 CGO 交叉编译可用。
- **schema 同步约束**：`store/history.go` 的 4 张表 schema 必须与 `internal/history/db_sqlite3.go` 逐字一致（CLI 与 Web 共用 owl.db，`CREATE TABLE IF NOT EXISTS` 先建者生效）。
- **记录非阻断约束**：所有历史埋点失败只 `log.Printf`，绝不返回错误、绝不阻断主操作。
- **nil-safe 约束**：`HistoryStore` 所有方法须 nil-receiver-safe（`if s == nil { return ... }`）；handler 通过导出字段 `History *store.HistoryStore` 注入，未注入时埋点自动 no-op（保证现有测试不受影响）。
- **路由约束**：gin 不允许同级混用静态段与通配段，故详情路由用 `/history/detail/:task_id`（而非 `/history/:task_id`），避免与 `/history/export`、`/history/stats` 冲突。
- **代码风格**：不新增注释（除非必要，如 schema 同步说明）；遵循现有 handler/store 命名与响应格式（`gin.H{"code":..,"message":..}`、列表 `{"data":..,"meta":{"total":..}}`）。
- **TDD**：每个任务先写失败测试再实现。测试用 `openTestDB(t)`（store 包，`user_test.go:14`）或 in-memory sqlite + gin（handler 包，参考 `exec_test.go`）。

---

## File Structure

**Create:**
- `cmd/plugins/serve/store/history.go` — HistoryStore + 类型（Operation/CommandExecution/FileTransfer/NodeCommunication/Record/QueryOptions/Stats）+ Init/Record*/Query/GetByTaskID/Cleanup/Stats/UpdateOperationStatus
- `cmd/plugins/serve/store/history_test.go` — store 测试
- `cmd/plugins/serve/handler/history.go` — HistoryHandler（List/Get/Stats/Export/Clean）+ parseHistoryDuration
- `cmd/plugins/serve/handler/history_test.go` — handler 测试

**Modify:**
- `cmd/plugins/serve/server.go` — WAL pragmas、HistoryStore 初始化与注入、historyHandler、路由
- `cmd/plugins/serve/handler/ws.go` — `BroadcastHistoryUpdate()`
- `cmd/plugins/serve/handler/exec.go` — `History` 字段、command + command_executions 记录、状态聚合、广播
- `cmd/plugins/serve/handler/transfer.go` — `History`/`Hub` 字段、file_transfer + file_transfers 记录、状态聚合、广播
- `cmd/plugins/serve/handler/playbook.go` — `History` 字段、playbook + command_executions 记录、终态更新、广播
- `cmd/plugins/serve/handler/node.go` — `History`/`Hub` 字段、node_manage 记录、广播
- `cmd/plugins/serve/web/js/api.js` — history 系列方法
- `cmd/plugins/serve/web/js/pages/history.js` — 完整重写

---

## Task 1: HistoryStore — schema、类型、Init、RecordOperation

**Files:**
- Create: `cmd/plugins/serve/store/history.go`
- Test: `cmd/plugins/serve/store/history_test.go`

**Interfaces:**
- Produces: `NewHistoryStore(db *sql.DB) *HistoryStore`、`(*HistoryStore).Init(ctx) error`、`(*HistoryStore).RecordOperation(ctx, *Operation) error`、类型 `Operation`。后续任务依赖这些以及 `RecordCommandExecution`/`RecordFileTransfer`/`RecordNodeCommunication`/`Query`/`GetByTaskID`/`Cleanup`/`Stats`/`UpdateOperationStatus`（Task 2/3 定义）。

- [ ] **Step 1: 写失败测试**

Create `cmd/plugins/serve/store/history_test.go`:

```go
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
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./store/ -run TestHistoryStore -v`（在 `cmd/plugins/serve` 目录）
Expected: FAIL（`undefined: NewHistoryStore` 等）

- [ ] **Step 3: 实现 history.go（schema + 类型 + Init + RecordOperation）**

Create `cmd/plugins/serve/store/history.go`:

```go
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// 表结构必须与 internal/history/db_sqlite3.go 保持逐字一致。
// CLI 与 Web 共用 ~/.owl/owl.db，两者以 CREATE TABLE IF NOT EXISTS 建同名表，
// 先建者生效；schema 不一致会导致读写错乱。

type Operation struct {
	ID               int64
	TaskID           string
	OpType           string
	Command          string
	Targets          []string
	Status           string
	ExecutionMode    string
	PlaybookPath     string
	CurrentTaskIndex int
	CurrentTaskPhase string
	CreatedAt        time.Time
}

type CommandExecution struct {
	ID         int64
	TaskID     string
	NodeID     string
	Command    string
	ExitCode   int
	Stdout     string
	Stderr     string
	DurationMs int64
	Success    bool
	CreatedAt  time.Time
}

type FileTransfer struct {
	ID           int64
	TaskID       string
	NodeID       string
	FileName     string
	FileSize     int64
	TransferType string
	Status       string
	Progress     float64
	Error        string
	CreatedAt    time.Time
}

type NodeCommunication struct {
	ID          int64
	TaskID      string
	NodeID      string
	NodeAddress string
	Direction   string
	MessageType string
	Payload     string
	Success     bool
	Error       string
	CreatedAt   time.Time
}

type Record struct {
	Operation         *Operation         `json:"operation"`
	CommandExecutions []*CommandExecution `json:"command_executions,omitempty"`
	Transfers         []*FileTransfer     `json:"transfers,omitempty"`
	Communications    []*NodeCommunication `json:"communications,omitempty"`
}

type QueryOptions struct {
	TaskID    string
	NodeID    string
	OpType    string
	Status    string
	StartTime time.Time
	EndTime   time.Time
	Limit     int
	Offset    int
}

type Stats struct {
	Total    int            `json:"total"`
	ByOpType map[string]int `json:"by_op_type"`
	ByStatus map[string]int `json:"by_status"`
}

type HistoryStore struct {
	db *sql.DB
}

func NewHistoryStore(db *sql.DB) *HistoryStore {
	return &HistoryStore{db: db}
}

func (s *HistoryStore) Init(ctx context.Context) error {
	if s == nil {
		return nil
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS operations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id TEXT,
			op_type TEXT,
			command TEXT,
			targets TEXT,
			status TEXT,
			execution_mode TEXT DEFAULT '',
			playbook_path TEXT DEFAULT '',
			current_task_index INTEGER DEFAULT 0,
			current_task_phase TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_operations_task_id ON operations (task_id)`,
		`CREATE INDEX IF NOT EXISTS idx_operations_op_type ON operations (op_type)`,
		`CREATE INDEX IF NOT EXISTS idx_operations_created_at ON operations (created_at)`,
		`CREATE TABLE IF NOT EXISTS command_executions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id TEXT,
			node_id TEXT,
			command TEXT,
			exit_code INTEGER,
			stdout TEXT,
			stderr TEXT,
			duration_ms INTEGER,
			success INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_executions_task_id ON command_executions (task_id)`,
		`CREATE INDEX IF NOT EXISTS idx_executions_node_id ON command_executions (node_id)`,
		`CREATE INDEX IF NOT EXISTS idx_executions_created_at ON command_executions (created_at)`,
		`CREATE TABLE IF NOT EXISTS file_transfers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id TEXT,
			node_id TEXT,
			file_name TEXT,
			file_size INTEGER,
			transfer_type TEXT,
			status TEXT,
			progress REAL,
			error TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_transfers_task_id ON file_transfers (task_id)`,
		`CREATE INDEX IF NOT EXISTS idx_transfers_node_id ON file_transfers (node_id)`,
		`CREATE INDEX IF NOT EXISTS idx_transfers_created_at ON file_transfers (created_at)`,
		`CREATE TABLE IF NOT EXISTS node_communications (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id TEXT,
			node_id TEXT,
			node_address TEXT,
			direction TEXT,
			message_type TEXT,
			payload TEXT,
			success INTEGER,
			error TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_communications_task_id ON node_communications (task_id)`,
		`CREATE INDEX IF NOT EXISTS idx_communications_node_id ON node_communications (node_id)`,
		`CREATE INDEX IF NOT EXISTS idx_communications_created_at ON node_communications (created_at)`,
	}
	for _, stmt := range statements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *HistoryStore) RecordOperation(ctx context.Context, op *Operation) error {
	if s == nil {
		return nil
	}
	if op.CreatedAt.IsZero() {
		op.CreatedAt = time.Now().UTC()
	}
	targetsJSON, _ := json.Marshal(op.Targets)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO operations (task_id, op_type, command, targets, status, execution_mode, playbook_path, current_task_index, current_task_phase, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, op.TaskID, op.OpType, op.Command, string(targetsJSON), op.Status, op.ExecutionMode, op.PlaybookPath, op.CurrentTaskIndex, op.CurrentTaskPhase, op.CreatedAt)
	return err
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./store/ -run TestHistoryStore -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add cmd/plugins/serve/store/history.go cmd/plugins/serve/store/history_test.go
git commit -m "feat(serve): 新增 HistoryStore schema 与 RecordOperation"
```

---

## Task 2: HistoryStore — 明细记录 + Query + GetByTaskID

**Files:**
- Modify: `cmd/plugins/serve/store/history.go`
- Test: `cmd/plugins/serve/store/history_test.go`

**Interfaces:**
- Consumes: Task 1 的类型与 Init/RecordOperation。
- Produces: `RecordCommandExecution(ctx, *CommandExecution) error`、`RecordFileTransfer(ctx, *FileTransfer) error`、`RecordNodeCommunication(ctx, *NodeCommunication) error`、`Query(ctx, *QueryOptions) ([]*Record, int, error)`、`GetByTaskID(ctx, taskID) (*Record, error)`。

- [ ] **Step 1: 写失败测试**（追加到 history_test.go）

```go
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
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./store/ -run 'TestHistoryStore_Record|TestHistoryStore_Query' -v`
Expected: FAIL（`s.RecordCommandExecution undefined` 等）

- [ ] **Step 3: 实现明细记录 + Query + GetByTaskID**（追加到 history.go）

```go
func (s *HistoryStore) RecordCommandExecution(ctx context.Context, exec *CommandExecution) error {
	if s == nil {
		return nil
	}
	if exec.CreatedAt.IsZero() {
		exec.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO command_executions (task_id, node_id, command, exit_code, stdout, stderr, duration_ms, success, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, exec.TaskID, exec.NodeID, exec.Command, exec.ExitCode, exec.Stdout, exec.Stderr, exec.DurationMs, exec.Success, exec.CreatedAt)
	return err
}

func (s *HistoryStore) RecordFileTransfer(ctx context.Context, tr *FileTransfer) error {
	if s == nil {
		return nil
	}
	if tr.CreatedAt.IsZero() {
		tr.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO file_transfers (task_id, node_id, file_name, file_size, transfer_type, status, progress, error, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, tr.TaskID, tr.NodeID, tr.FileName, tr.FileSize, tr.TransferType, tr.Status, tr.Progress, tr.Error, tr.CreatedAt)
	return err
}

func (s *HistoryStore) RecordNodeCommunication(ctx context.Context, c *NodeCommunication) error {
	if s == nil {
		return nil
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO node_communications (task_id, node_id, node_address, direction, message_type, payload, success, error, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.TaskID, c.NodeID, c.NodeAddress, c.Direction, c.MessageType, c.Payload, c.Success, c.Error, c.CreatedAt)
	return err
}

func (s *HistoryStore) Query(ctx context.Context, opts *QueryOptions) ([]*Record, int, error) {
	if s == nil {
		return nil, 0, nil
	}
	where := " WHERE 1=1"
	args := []interface{}{}
	if opts.TaskID != "" {
		where += " AND task_id = ?"
		args = append(args, opts.TaskID)
	}
	if opts.OpType != "" {
		where += " AND op_type = ?"
		args = append(args, opts.OpType)
	}
	if opts.Status != "" {
		where += " AND status = ?"
		args = append(args, opts.Status)
	}
	if opts.NodeID != "" {
		where += " AND targets LIKE ?"
		args = append(args, "%"+opts.NodeID+"%")
	}
	if !opts.StartTime.IsZero() {
		where += " AND created_at >= ?"
		args = append(args, opts.StartTime)
	}
	if !opts.EndTime.IsZero() {
		where += " AND created_at <= ?"
		args = append(args, opts.EndTime)
	}

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM operations"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := "SELECT id, task_id, op_type, command, targets, status, execution_mode, playbook_path, current_task_index, current_task_phase, created_at FROM operations" + where + " ORDER BY created_at DESC"
	listArgs := append([]interface{}{}, args...)
	if opts.Limit > 0 {
		query += " LIMIT ? OFFSET ?"
		listArgs = append(listArgs, opts.Limit, opts.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	records := []*Record{}
	for rows.Next() {
		var op Operation
		var targetsJSON string
		if err := rows.Scan(&op.ID, &op.TaskID, &op.OpType, &op.Command, &targetsJSON, &op.Status, &op.ExecutionMode, &op.PlaybookPath, &op.CurrentTaskIndex, &op.CurrentTaskPhase, &op.CreatedAt); err != nil {
			continue
		}
		json.Unmarshal([]byte(targetsJSON), &op.Targets)
		records = append(records, &Record{Operation: &op})
	}

	for _, rec := range records {
		rec.CommandExecutions, _ = s.executionsByTaskID(ctx, rec.Operation.TaskID)
		rec.Transfers, _ = s.transfersByTaskID(ctx, rec.Operation.TaskID)
		rec.Communications, _ = s.commsByTaskID(ctx, rec.Operation.TaskID)
	}
	return records, total, nil
}

func (s *HistoryStore) GetByTaskID(ctx context.Context, taskID string) (*Record, error) {
	recs, _, err := s.Query(ctx, &QueryOptions{TaskID: taskID})
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, sql.ErrNoRows
	}
	return recs[0], nil
}

func (s *HistoryStore) executionsByTaskID(ctx context.Context, taskID string) ([]*CommandExecution, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, task_id, node_id, command, exit_code, stdout, stderr, duration_ms, success, created_at
		FROM command_executions WHERE task_id = ? ORDER BY created_at`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []*CommandExecution{}
	for rows.Next() {
		var e CommandExecution
		if err := rows.Scan(&e.ID, &e.TaskID, &e.NodeID, &e.Command, &e.ExitCode, &e.Stdout, &e.Stderr, &e.DurationMs, &e.Success, &e.CreatedAt); err != nil {
			continue
		}
		results = append(results, &e)
	}
	return results, nil
}

func (s *HistoryStore) transfersByTaskID(ctx context.Context, taskID string) ([]*FileTransfer, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, task_id, node_id, file_name, file_size, transfer_type, status, progress, error, created_at
		FROM file_transfers WHERE task_id = ? ORDER BY created_at`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []*FileTransfer{}
	for rows.Next() {
		var tr FileTransfer
		if err := rows.Scan(&tr.ID, &tr.TaskID, &tr.NodeID, &tr.FileName, &tr.FileSize, &tr.TransferType, &tr.Status, &tr.Progress, &tr.Error, &tr.CreatedAt); err != nil {
			continue
		}
		results = append(results, &tr)
	}
	return results, nil
}

func (s *HistoryStore) commsByTaskID(ctx context.Context, taskID string) ([]*NodeCommunication, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, task_id, node_id, node_address, direction, message_type, payload, success, error, created_at
		FROM node_communications WHERE task_id = ? ORDER BY created_at`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []*NodeCommunication{}
	for rows.Next() {
		var c NodeCommunication
		if err := rows.Scan(&c.ID, &c.TaskID, &c.NodeID, &c.NodeAddress, &c.Direction, &c.MessageType, &c.Payload, &c.Success, &c.Error, &c.CreatedAt); err != nil {
			continue
		}
		results = append(results, &c)
	}
	return results, nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./store/ -run TestHistoryStore -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add cmd/plugins/serve/store/history.go cmd/plugins/serve/store/history_test.go
git commit -m "feat(serve): HistoryStore 明细记录与多维 Query/GetByTaskID"
```

---

## Task 3: HistoryStore — Cleanup、Stats、UpdateOperationStatus

**Files:**
- Modify: `cmd/plugins/serve/store/history.go`
- Test: `cmd/plugins/serve/store/history_test.go`

**Interfaces:**
- Produces: `Cleanup(ctx, retentionDays int) (int64, error)`、`Stats(ctx) (*Stats, error)`、`UpdateOperationStatus(ctx, taskID, status string) error`。

- [ ] **Step 1: 写失败测试**（追加）

```go
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
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./store/ -run 'TestHistoryStore_Cleanup|TestHistoryStore_Stats|TestHistoryStore_UpdateOperationStatus' -v`
Expected: FAIL（方法未定义）

- [ ] **Step 3: 实现**（追加到 history.go）

```go
func (s *HistoryStore) UpdateOperationStatus(ctx context.Context, taskID, status string) error {
	if s == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `UPDATE operations SET status = ? WHERE task_id = ?`, status, taskID)
	return err
}

func (s *HistoryStore) Cleanup(ctx context.Context, retentionDays int) (int64, error) {
	if s == nil {
		return 0, nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	var total int64
	for _, table := range []string{"operations", "command_executions", "file_transfers", "node_communications"} {
		res, err := s.db.ExecContext(ctx, "DELETE FROM "+table+" WHERE created_at < ?", cutoff)
		if err != nil {
			return total, err
		}
		if n, err := res.RowsAffected(); err == nil {
			total += n
		}
	}
	return total, nil
}

func (s *HistoryStore) Stats(ctx context.Context) (*Stats, error) {
	if s == nil {
		return &Stats{ByOpType: map[string]int{}, ByStatus: map[string]int{}}, nil
	}
	st := &Stats{ByOpType: map[string]int{}, ByStatus: map[string]int{}}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM operations`).Scan(&st.Total); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT op_type, COUNT(*) FROM operations GROUP BY op_type`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err == nil {
			st.ByOpType[k] = n
		}
	}
	rows.Close()

	rows2, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM operations GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var k string
		var n int
		if err := rows2.Scan(&k, &n); err == nil {
			st.ByStatus[k] = n
		}
	}
	return st, nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./store/ -v`
Expected: PASS（全部 store 测试）

- [ ] **Step 5: 提交**

```bash
git add cmd/plugins/serve/store/history.go cmd/plugins/serve/store/history_test.go
git commit -m "feat(serve): HistoryStore Cleanup/Stats/UpdateOperationStatus"
```

---

## Task 4: WSHub — BroadcastHistoryUpdate

**Files:**
- Modify: `cmd/plugins/serve/handler/ws.go`（在 `BroadcastTaskOutput` 之后，约 ws.go:87）

**Interfaces:**
- Produces: `(*WSHub).BroadcastHistoryUpdate()`，广播 `{type:"history_update"}`。

- [ ] **Step 1: 实现**（在 ws.go 的 `BroadcastTaskOutput` 方法后追加）

```go
func (h *WSHub) BroadcastHistoryUpdate() {
	h.Broadcast(WSMessage{Type: "history_update", Data: nil})
}
```

- [ ] **Step 2: 编译确认**

Run: `go build ./handler/`
Expected: 成功，无输出

- [ ] **Step 3: 提交**

```bash
git add cmd/plugins/serve/handler/ws.go
git commit -m "feat(serve): WSHub 增加 BroadcastHistoryUpdate"
```

---

## Task 5: ExecHandler — command 记录 + 明细 + 状态聚合

**Files:**
- Modify: `cmd/plugins/serve/handler/exec.go`
- Test: `cmd/plugins/serve/handler/exec_test.go`

**Interfaces:**
- Consumes: `store.HistoryStore`（Task 1-3）、`WSHub.BroadcastHistoryUpdate`（Task 4）、`store.TaskStore.CreateWithRecord`（已存在）。
- Produces: `ExecHandler.History *store.HistoryStore` 导出字段；Create 记录 op_type=command；executeTask 记录 command_executions 并聚合 operations 状态。

- [ ] **Step 1: 写失败测试**（追加到 exec_test.go）

```go
func TestExecCreate_RecordsHistory(t *testing.T) {
	db, h := execTestSetup(t)
	hs := store.NewHistoryStore(db)
	require.NoError(t, hs.Init(t.Context()))
	h.History = hs

	_, h2 := h, h
	_ = h2
	router := execRBACRouter(t, h)
	w := execPOST(t, router, map[string]string{"node_id": "test-node", "command": "uptime"})
	require.Equal(t, 202, w.Code)

	// 等待异步 executeTask 完成
	deadline := time.Now().Add(2 * time.Second)
	var total int
	for time.Now().Before(deadline) {
		recs, t2, _ := hs.Query(t.Context(), &store.QueryOptions{OpType: "command"})
		total = t2
		if len(recs) > 0 && recs[0].Operation.Status != "running" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	recs, total, err := hs.Query(t.Context(), &store.QueryOptions{OpType: "command"})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	assert.Equal(t, "uptime", recs[0].Operation.Command)
	assert.Equal(t, []string{"test-node"}, recs[0].Operation.Targets)
	assert.Equal(t, "completed", recs[0].Operation.Status)
	require.Len(t, recs[0].CommandExecutions, 1)
	assert.Equal(t, "test-node", recs[0].CommandExecutions[0].NodeID)
	assert.True(t, recs[0].CommandExecutions[0].Success)
}
```

> 注：exec_test.go 需补充 import `"time"`（若尚无）。

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./handler/ -run TestExecCreate_RecordsHistory -v`
Expected: FAIL（`h.History undefined`）

- [ ] **Step 3: 实现 exec.go 改动**

3a. 在 `ExecHandler` 结构体（exec.go:28-33）增加字段：

```go
type ExecHandler struct {
	db      *sql.DB
	task    *store.TaskStore
	exec    Executor
	hub     *WSHub
	History *store.HistoryStore
}
```

3b. 在 import 块（exec.go:3-15）补充 `"log"` 和 `"github.com/google/uuid"`。

3c. 修改 `Create`（exec.go:167-276）：在 `cfg` 构造之后、`var tasks []*store.Task` 之前插入 opID 与 targets 收集变量；将 createSingleTask 调用传入 opID；在返回前记录 operations。

将 `var tasks []*store.Task` / `isMerge := false` 一段替换为：

```go
	opID := uuid.New().String()
	var opTargets []string

	var tasks []*store.Task
	isMerge := false
```

并行分支（exec.go:219-241）中 `task, err := h.createSingleTask(c, nid, command, cfg.Force)` 改为：

```go
			task, err := h.createSingleTask(c, nid, command, cfg.Force, opID)
```

并在该分支 `tasks = append(tasks, task.task)`（非 merged 路径）之后、`if h.exec != nil {` 之前插入：

```go
			opTargets = append(opTargets, nid)
```

串行分支（exec.go:242-264）中 `task, err := h.createSingleTask(c, nid, command, cfg.Force)` 改为：

```go
			task, err := h.createSingleTask(c, nid, command, cfg.Force, opID)
```

并在 `if !task.merged {` 块内 `serialTasks = append(serialTasks, task.task)` 之后插入：

```go
				opTargets = append(opTargets, nid)
```

在 `if len(tasks) == 0 {` 之前插入记录逻辑：

```go
	if len(opTargets) > 0 {
		op := &store.Operation{TaskID: opID, OpType: "command", Command: command, Targets: opTargets, Status: "running", CreatedAt: time.Now().UTC()}
		if err := h.History.RecordOperation(c.Request.Context(), op); err != nil {
			log.Printf("record history: %v", err)
		}
		if h.hub != nil {
			h.hub.BroadcastHistoryUpdate()
		}
	}
```

3d. 修改 `createSingleTask` 签名（exec.go:288）与其内部 Create 调用（exec.go:307）：

```go
func (h *ExecHandler) createSingleTask(c *gin.Context, nid, command string, force bool, recordID string) (*taskResult, *apiError) {
```

将其内部：

```go
	task, err := h.task.Create(c.Request.Context(), nid, command)
```

改为：

```go
	task, err := h.task.CreateWithRecord(c.Request.Context(), nid, command, recordID)
```

3e. 修改 `executeTask`（exec.go:364-459）：在函数开头 `ctx := context.Background()` 之后插入 `start := time.Now()`；在失败返回前与成功返回前分别记录明细并聚合状态。

在 `ctx := context.Background()`（exec.go:368）后插入：

```go
	start := time.Now()
```

失败分支（exec.go:432-443）中，在 `h.task.UpdateStatus(ctx, taskID, store.TaskStatusFailed, errMsg, &exitCode)` 之后、`task, _ = h.task.Get(ctx, taskID)` 之前插入：

```go
		h.recordCommandExecution(ctx, task, exitCode, "", errMsg, time.Since(start).Milliseconds(), false)
		h.updateOpStatus(ctx, task.RecordID)
```

成功分支末尾（exec.go:454-458），在 `h.task.UpdateStatus(ctx, taskID, store.TaskStatusCompleted, outputStr, &exitCode)` 之后、`task, _ = h.task.Get(ctx, taskID)` 之前插入：

```go
	h.recordCommandExecution(ctx, task, exitCode, outputStr, "", time.Since(start).Milliseconds(), true)
	h.updateOpStatus(ctx, task.RecordID)
```

3f. 在 exec.go 末尾（`parseInt` 之前或之后）追加两个辅助方法：

```go
func (h *ExecHandler) recordCommandExecution(ctx context.Context, task *store.Task, exitCode int, stdout, stderr string, durationMs int64, success bool) {
	if task == nil || task.RecordID == "" {
		return
	}
	exec := &store.CommandExecution{TaskID: task.RecordID, NodeID: task.NodeID, Command: task.Command, ExitCode: exitCode, Stdout: stdout, Stderr: stderr, DurationMs: durationMs, Success: success, CreatedAt: time.Now().UTC()}
	if err := h.History.RecordCommandExecution(ctx, exec); err != nil {
		log.Printf("record command execution: %v", err)
	}
}

func (h *ExecHandler) updateOpStatus(ctx context.Context, opID string) {
	if opID == "" || h.History == nil {
		return
	}
	rows, err := h.db.QueryContext(ctx, `SELECT status FROM tasks WHERE record_id = ?`, opID)
	if err != nil {
		return
	}
	defer rows.Close()
	var statuses []string
	for rows.Next() {
		var st string
		if err := rows.Scan(&st); err == nil {
			statuses = append(statuses, st)
		}
	}
	if len(statuses) == 0 {
		return
	}
	allDone := true
	anyFail := false
	for _, st := range statuses {
		if st == "running" || st == "queued" || st == "pending" {
			allDone = false
		}
		if st == "failed" || st == "cancelled" {
			anyFail = true
		}
	}
	if !allDone {
		return
	}
	status := "completed"
	if anyFail {
		status = "failed"
	}
	if err := h.History.UpdateOperationStatus(ctx, opID, status); err != nil {
		log.Printf("update op status: %v", err)
	}
	if h.hub != nil {
		h.hub.BroadcastHistoryUpdate()
	}
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./handler/ -run TestExec -v`
Expected: PASS（含新增 RecordsHistory 与全部既有 exec 测试）

- [ ] **Step 5: 提交**

```bash
git add cmd/plugins/serve/handler/exec.go cmd/plugins/serve/handler/exec_test.go
git commit -m "feat(serve): ExecHandler 记录 command 操作历史与明细"
```

---

## Task 6: TransferHandler — file_transfer 记录 + 明细 + 状态聚合

**Files:**
- Modify: `cmd/plugins/serve/handler/transfer.go`
- Test: `cmd/plugins/serve/handler/transfer_test.go`

**Interfaces:**
- Consumes: `store.HistoryStore`、`WSHub.BroadcastHistoryUpdate`、`store.TransferRecordStore`。
- Produces: `TransferHandler.History`/`TransferHandler.Hub` 导出字段；Create 记录 op_type=file_transfer；goroutine 记录 file_transfers 并聚合状态。

- [ ] **Step 1: 写失败测试**（追加到 transfer_test.go；若该文件无 setup 辅助，参考其既有测试构造 TransferHandler）

```go
func TestTransferCreate_RecordsHistory(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY, name TEXT, address TEXT, port INTEGER DEFAULT 22,
		user TEXT, password TEXT, ssh_key TEXT, status TEXT DEFAULT 'unknown',
		groups TEXT DEFAULT '[]', labels TEXT DEFAULT '{}',
		proxy_jump TEXT DEFAULT '', created_at TIMESTAMP, updated_at TIMESTAMP)`)
	require.NoError(t, err)
	db.Exec(`INSERT INTO nodes (id, name, address, port, user, status) VALUES ('n1','n1','127.0.0.1',22,'root','online')`)

	ts := store.NewTaskStore(db)
	require.NoError(t, ts.Init(t.Context()))
	rs := store.NewTransferRecordStore(db)
	require.NoError(t, rs.Init(t.Context()))
	hs := store.NewHistoryStore(db)
	require.NoError(t, hs.Init(t.Context()))

	h := NewTransferHandler(db, ts, rs)
	h.History = hs

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/transfer", h.Create)

	body, _ := json.Marshal(map[string]interface{}{
		"node_ids": []string{"n1"}, "source_path": "/tmp/a.tar", "dest_path": "/opt/", "direction": "push",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/transfer", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusAccepted, w.Code)

	recs, total, err := hs.Query(t.Context(), &store.QueryOptions{OpType: "file_transfer"})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	assert.Equal(t, []string{"n1"}, recs[0].Operation.Targets)
	assert.Contains(t, recs[0].Operation.Command, "/tmp/a.tar")
}
```

> 注：transfer_test.go 需 import `bytes`/`encoding/json`/`net/http`/`net/http/httptest`/`github.com/gin-gonic/gin`（按现有缺失补充）。

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./handler/ -run TestTransferCreate_RecordsHistory -v`
Expected: FAIL（`h.History undefined`）

- [ ] **Step 3: 实现 transfer.go 改动**

3a. 结构体（transfer.go:15-19）增加字段：

```go
type TransferHandler struct {
	db          *sql.DB
	task        *store.TaskStore
	recordStore *store.TransferRecordStore
	History     *store.HistoryStore
	Hub         *WSHub
}
```

3b. import 块（transfer.go:3-13）补充 `"log"` 与 `"path/filepath"`。

3c. 在 `Create` 中 `h.recordStore.SetNodeCount(...)`（transfer.go:64）之后插入：

```go
	op := &store.Operation{TaskID: transferRec.ID, OpType: "file_transfer", Command: fmt.Sprintf("transfer %s -> %s", req.SourcePath, req.DestPath), Targets: req.NodeIDs, Status: "running", CreatedAt: time.Now().UTC()}
	if err := h.History.RecordOperation(c.Request.Context(), op); err != nil {
		log.Printf("record history: %v", err)
	}
	if h.Hub != nil {
		h.Hub.BroadcastHistoryUpdate()
	}
```

3d. 在 goroutine（transfer.go:83-98）内 `h.recordStore.UpdateNodeResult(bg, recordID, err == nil)` 之后插入：

```go
			ftStatus := "completed"
			if err != nil {
				ftStatus = "failed"
			}
			ft := &store.FileTransfer{TaskID: recordID, NodeID: nodeID, FileName: filepath.Base(src), TransferType: dir, Status: ftStatus, Error: errMsg, CreatedAt: time.Now().UTC()}
			if e := h.History.RecordFileTransfer(bg, ft); e != nil {
				log.Printf("record file transfer: %v", e)
			}
			h.updateOpStatus(bg, recordID)
```

3e. 在 transfer.go 末尾追加：

```go
func (h *TransferHandler) updateOpStatus(ctx context.Context, recordID string) {
	if recordID == "" || h.History == nil || h.recordStore == nil {
		return
	}
	rec, err := h.recordStore.Get(ctx, recordID)
	if err != nil {
		return
	}
	var status string
	switch rec.Status {
	case store.TransferCompleted:
		status = "completed"
	case store.TransferFailed, store.TransferPartialSuccess:
		status = "failed"
	default:
		return
	}
	if err := h.History.UpdateOperationStatus(ctx, recordID, status); err != nil {
		log.Printf("update op status: %v", err)
	}
	if h.Hub != nil {
		h.Hub.BroadcastHistoryUpdate()
	}
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./handler/ -run TestTransfer -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add cmd/plugins/serve/handler/transfer.go cmd/plugins/serve/handler/transfer_test.go
git commit -m "feat(serve): TransferHandler 记录 file_transfer 操作历史与明细"
```

---

## Task 7: PlaybookHandler — playbook 记录 + 明细 + 终态

**Files:**
- Modify: `cmd/plugins/serve/handler/playbook.go`
- Test: `cmd/plugins/serve/handler/playbook_test.go`

**Interfaces:**
- Consumes: `store.HistoryStore`、`model.StepResult`（字段 TaskName/NodeID/ExitCode/Output/Error/DurationMs）。
- Produces: `PlaybookHandler.History` 导出字段；Run 记录 op_type=playbook（含 playbook_path）；executePlaybookRun 记录每步 command_executions 并在终态更新 operations 状态。

- [ ] **Step 1: 写失败测试**（追加到 playbook_test.go；直接测 Run 的记录逻辑需构造 PlaybookStore/RunStore，参考既有 playbook 测试 setup。下方为聚焦 executePlaybookRun 记录的最小测试思路：用真实 store + 一个临时 playbook 文件）

```go
func TestPlaybookRun_RecordsHistory(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY, name TEXT, address TEXT, port INTEGER DEFAULT 22,
		user TEXT, password TEXT, ssh_key TEXT, status TEXT DEFAULT 'unknown',
		groups TEXT DEFAULT '[]', labels TEXT DEFAULT '{}',
		proxy_jump TEXT DEFAULT '', created_at TIMESTAMP, updated_at TIMESTAMP)`)
	require.NoError(t, err)

	ps := store.NewPlaybookStore(db)
	require.NoError(t, ps.Init(t.Context()))
	rs := store.NewPlaybookRunStore(db)
	require.NoError(t, rs.Init(t.Context()))
	ns := store.NewNodeStore(db)
	hs := store.NewHistoryStore(db)
	require.NoError(t, hs.Init(t.Context()))

	h := NewPlaybookHandler(db, ps, rs, ns, nil)
	h.History = hs

	run, err := rs.Create(t.Context(), "pb-1", "demo", "/nonexistent.yaml", []string{"n1"}, nil, "")
	require.NoError(t, err)

	// 直接记录一条 playbook operation 验证字段（Run 的 HTTP 路径在既有测试已覆盖）
	op := &store.Operation{TaskID: run.ID, OpType: "playbook", Command: "playbook run demo", Targets: []string{"n1"}, PlaybookPath: "/nonexistent.yaml", Status: "running"}
	require.NoError(t, hs.RecordOperation(t.Context(), op))

	recs, total, err := hs.Query(t.Context(), &store.QueryOptions{OpType: "playbook"})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	assert.Equal(t, "/nonexistent.yaml", recs[0].Operation.PlaybookPath)
}
```

> 注：playbook_test.go 需 import `database/sql`（按现有缺失补充）。`store.NewNodeStore`/`NewPlaybookStore`/`NewPlaybookRunStore` 签名以现有代码为准。

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./handler/ -run TestPlaybookRun_RecordsHistory -v`
Expected: FAIL（`h.History undefined`）

- [ ] **Step 3: 实现 playbook.go 改动**

3a. 结构体（playbook.go:18-24）增加字段：

```go
type PlaybookHandler struct {
	db        *sql.DB
	playbooks *store.PlaybookStore
	runs      *store.PlaybookRunStore
	nodes     *store.NodeStore
	hub       *WSHub
	History   *store.HistoryStore
}
```

3b. import 块补充 `"log"`（若尚无）。

3c. 在 `Run`（playbook.go:202-247）中 `go h.executePlaybookRun(run.ID)`（playbook.go:240）之前插入：

```go
	op := &store.Operation{TaskID: run.ID, OpType: "playbook", Command: "playbook run " + pb.Name, Targets: req.TargetNodes, PlaybookPath: pb.FilePath, Status: "running", CreatedAt: time.Now().UTC()}
	if err := h.History.RecordOperation(c.Request.Context(), op); err != nil {
		log.Printf("record history: %v", err)
	}
```

3d. 在 `executePlaybookRun`（playbook.go:308-400）的步骤循环中，`h.runs.AppendResult(ctx, runID, step)`（playbook.go:377）之后插入：

```go
			ce := &store.CommandExecution{TaskID: runID, NodeID: step.NodeID, Command: step.TaskName, ExitCode: step.ExitCode, Stdout: step.Output, Stderr: step.Error, DurationMs: step.DurationMs, Success: step.ExitCode == 0, CreatedAt: time.Now().UTC()}
			if e := h.History.RecordCommandExecution(ctx, ce); e != nil {
				log.Printf("record command execution: %v", e)
			}
```

3e. 在 `executePlaybookRun` 终态处（playbook.go:391-395，`h.runs.UpdateStatus(ctx, runID, finalStatus, "")` 之后）插入：

```go
	opStatus := "completed"
	if failed {
		opStatus = "failed"
	}
	if err := h.History.UpdateOperationStatus(ctx, runID, opStatus); err != nil {
		log.Printf("update op status: %v", err)
	}
	if h.hub != nil {
		h.hub.BroadcastHistoryUpdate()
	}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./handler/ -run TestPlaybook -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add cmd/plugins/serve/handler/playbook.go cmd/plugins/serve/handler/playbook_test.go
git commit -m "feat(serve): PlaybookHandler 记录 playbook 操作历史与步骤明细"
```

---

## Task 8: NodeHandler — node_manage 记录

**Files:**
- Modify: `cmd/plugins/serve/handler/node.go`
- Test: `cmd/plugins/serve/handler/node_crud_test.go`（或 node_test.go）

**Interfaces:**
- Consumes: `store.HistoryStore`、`WSHub.BroadcastHistoryUpdate`。
- Produces: `NodeHandler.History`/`NodeHandler.Hub` 导出字段；Create/Update/Delete/BatchGroups/Import 记录 op_type=node_manage。

- [ ] **Step 1: 写失败测试**（追加到 node_crud_test.go，复用其既有 setup；下方为示意，需匹配该文件已有的 router/setup 辅助函数）

```go
func TestNodeCreate_RecordsHistory(t *testing.T) {
	db, h, router := nodeCRUDTestSetup(t) // 以现有 node_crud_test.go 的 setup 为准
	hs := store.NewHistoryStore(db)
	require.NoError(t, hs.Init(t.Context()))
	h.History = hs

	body, _ := json.Marshal(map[string]interface{}{"id": "hist-node", "address": "10.0.0.9", "user": "root"})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/nodes", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken())
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	recs, total, err := hs.Query(t.Context(), &store.QueryOptions{OpType: "node_manage"})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	assert.Contains(t, recs[0].Operation.Command, "hist-node")
	assert.Equal(t, []string{"hist-node"}, recs[0].Operation.Targets)
}
```

> 实施者须先读 `node_crud_test.go` 确认 setup 辅助函数名与 router 构造方式，并据此调整测试。

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./handler/ -run TestNodeCreate_RecordsHistory -v`
Expected: FAIL（`h.History undefined`）

- [ ] **Step 3: 实现 node.go 改动**

3a. 结构体（node.go:62-64）增加字段：

```go
type NodeHandler struct {
	db      *sql.DB
	History *store.HistoryStore
	Hub     *WSHub
}
```

3b. import 块补充 `"context"`、`"log"`、`"github.com/google/uuid"`（按现有缺失补充；node.go 已 import `store`）。

3c. 在 node.go 中追加辅助方法：

```go
func (h *NodeHandler) recordNodeManage(ctx context.Context, action string, targets []string) {
	op := &store.Operation{TaskID: uuid.New().String(), OpType: "node_manage", Command: action, Targets: targets, Status: "completed", CreatedAt: time.Now().UTC()}
	if err := h.History.RecordOperation(ctx, op); err != nil {
		log.Printf("record history: %v", err)
	}
	if h.Hub != nil {
		h.Hub.BroadcastHistoryUpdate()
	}
}
```

3d. `Create`（node.go:283-349）：在 `c.JSON(http.StatusCreated, n)`（node.go:348）之前插入：

```go
	h.recordNodeManage(c.Request.Context(), "node create "+req.ID, []string{req.ID})
```

3e. `Update`（node.go:351-432）：在 `c.Request.URL.Path = "/api/v1/nodes/" + id`（node.go:430）之前插入：

```go
	h.recordNodeManage(c.Request.Context(), "node update "+id, []string{id})
```

3f. `Delete`（node.go:434-447）：在 `c.JSON(http.StatusOK, gin.H{"status": "deleted"})`（node.go:446）之前插入：

```go
	h.recordNodeManage(c.Request.Context(), "node delete "+id, []string{id})
```

3g. `BatchGroups`（node.go:463-532）：在最终 `c.JSON(http.StatusOK, gin.H{...})`（node.go:528）之前插入：

```go
	if updated > 0 {
		h.recordNodeManage(c.Request.Context(), fmt.Sprintf("node batch groups add=%v remove=%v", req.Add, req.Remove), req.NodeIDs)
	}
```

3h. `Import`（node.go:646-746）：在 `result := importResult{}`（node.go:673）之后插入 `var importedIDs []string`；在成功分支 `result.Success++`（node.go:741）之后插入 `importedIDs = append(importedIDs, node.ID)`；在最终 `c.JSON(http.StatusOK, result)`（node.go:745）之前插入：

```go
	if len(importedIDs) > 0 {
		h.recordNodeManage(c.Request.Context(), fmt.Sprintf("node import success=%d", result.Success), importedIDs)
	}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./handler/ -run TestNode -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add cmd/plugins/serve/handler/node.go cmd/plugins/serve/handler/node_crud_test.go
git commit -m "feat(serve): NodeHandler 记录 node_manage 操作历史"
```

---

## Task 9: HistoryHandler — List / Get / Stats

**Files:**
- Create: `cmd/plugins/serve/handler/history.go`
- Test: `cmd/plugins/serve/handler/history_test.go`

**Interfaces:**
- Consumes: `store.HistoryStore.Query/GetByTaskID/Stats`、`store.QueryOptions`、`store.Record`、`store.Stats`。
- Produces: `NewHistoryHandler(*store.HistoryStore) *HistoryHandler`、`List`/`Get`/`Stats`/`Export`/`Clean`（Export/Clean 在 Task 10）、`parseHistoryDuration`。

- [ ] **Step 1: 写失败测试**

Create `cmd/plugins/serve/handler/history_test.go`:

```go
package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func historyTestSetup(t *testing.T) (*store.HistoryStore, *gin.Engine) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	hs := store.NewHistoryStore(db)
	require.NoError(t, hs.Init(context.Background()))

	gin.SetMode(gin.TestMode)
	us := store.NewUserStore(db)
	as := service.NewAuthService("test-secret-32byte-long-string!!")
	ah := NewAuthHandler(us, as)
	hh := NewHistoryHandler(hs)

	r := gin.New()
	auth := r.Group("/api/v1")
	auth.Use(ah.AuthMiddleware())
	reader := auth.Group("", ah.RBACMiddleware(model.RoleViewer, model.RoleEditor, model.RoleOperator, model.RoleAdmin))
	reader.GET("/history", hh.List)
	reader.GET("/history/stats", hh.Stats)
	reader.GET("/history/export", hh.Export)
	reader.GET("/history/detail/:task_id", hh.Get)
	admin := auth.Group("", ah.RBACMiddleware(model.RoleAdmin))
	admin.DELETE("/history", hh.Clean)
	return hs, r
}

func historyGET(t *testing.T, r *gin.Engine, path, tok string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", path, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)
	return w
}

func TestHistoryList_AndFilters(t *testing.T) {
	hs, r := historyTestSetup(t)
	ctx := context.Background()
	hs.RecordOperation(ctx, &store.Operation{TaskID: "a", OpType: "command", Command: "uptime", Targets: []string{"n1"}, Status: "completed", CreatedAt: time.Now().UTC()})
	hs.RecordOperation(ctx, &store.Operation{TaskID: "b", OpType: "playbook", Command: "deploy", Targets: []string{"n2"}, Status: "failed", CreatedAt: time.Now().UTC()})

	w := historyGET(t, r, "/api/v1/history", adminToken())
	require.Equal(t, 200, w.Code)
	var resp struct {
		Data []store.Record `json:"data"`
		Meta struct{ Total int `json:"total"` } `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.Meta.Total)
	assert.Len(t, resp.Data, 2)

	w2 := historyGET(t, r, "/api/v1/history?op_type=command", adminToken())
	var resp2 struct {
		Data []store.Record `json:"data"`
		Meta struct{ Total int `json:"total"` } `json:"meta"`
	}
	json.Unmarshal(w2.Body.Bytes(), &resp2)
	assert.Equal(t, 1, resp2.Meta.Total)
	assert.Equal(t, "command", resp2.Data[0].Operation.OpType)

	w3 := historyGET(t, r, "/api/v1/history?status=failed", adminToken())
	var resp3 struct {
		Meta struct{ Total int `json:"total"` } `json:"meta"`
	}
	json.Unmarshal(w3.Body.Bytes(), &resp3)
	assert.Equal(t, 1, resp3.Meta.Total)
}

func TestHistoryGet_Detail(t *testing.T) {
	hs, r := historyTestSetup(t)
	ctx := context.Background()
	hs.RecordOperation(ctx, &store.Operation{TaskID: "op-x", OpType: "command", Command: "uptime", Targets: []string{"n1"}, Status: "completed"})
	hs.RecordCommandExecution(ctx, &store.CommandExecution{TaskID: "op-x", NodeID: "n1", Command: "uptime", ExitCode: 0, Stdout: "ok", Success: true})

	w := historyGET(t, r, "/api/v1/history/detail/op-x", adminToken())
	require.Equal(t, 200, w.Code)
	var rec store.Record
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rec))
	assert.Equal(t, "op-x", rec.Operation.TaskID)
	require.Len(t, rec.CommandExecutions, 1)

	w404 := historyGET(t, r, "/api/v1/history/detail/nope", adminToken())
	assert.Equal(t, 404, w404.Code)
}

func TestHistoryStats(t *testing.T) {
	hs, r := historyTestSetup(t)
	ctx := context.Background()
	hs.RecordOperation(ctx, &store.Operation{TaskID: "a", OpType: "command", Status: "completed"})
	hs.RecordOperation(ctx, &store.Operation{TaskID: "b", OpType: "command", Status: "failed"})

	w := historyGET(t, r, "/api/v1/history/stats", adminToken())
	require.Equal(t, 200, w.Code)
	var st store.Stats
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &st))
	assert.Equal(t, 2, st.Total)
	assert.Equal(t, 2, st.ByOpType["command"])
}

func TestHistoryList_AsViewer_Allowed(t *testing.T) {
	_, r := historyTestSetup(t)
	w := historyGET(t, r, "/api/v1/history", viewerToken())
	assert.Equal(t, 200, w.Code)
}

func TestParseHistoryDuration(t *testing.T) {
	d, err := parseHistoryDuration("24h")
	require.NoError(t, err)
	assert.Equal(t, 24*time.Hour, d)
	d2, err := parseHistoryDuration("7d")
	require.NoError(t, err)
	assert.Equal(t, 7*24*time.Hour, d2)
	d3, err := parseHistoryDuration("30m")
	require.NoError(t, err)
	assert.Equal(t, 30*time.Minute, d3)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./handler/ -run 'TestHistory|TestParseHistoryDuration' -v`
Expected: FAIL（`undefined: NewHistoryHandler` 等）

- [ ] **Step 3: 实现 history.go（List/Get/Stats + parseOptions + parseHistoryDuration）**

Create `cmd/plugins/serve/handler/history.go`:

```go
package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

type HistoryHandler struct {
	history *store.HistoryStore
}

func NewHistoryHandler(history *store.HistoryStore) *HistoryHandler {
	return &HistoryHandler{history: history}
}

func parseHistoryDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	suffix := s[len(s)-1]
	switch suffix {
	case 'h', 'H':
		hours, err := strconv.ParseFloat(s[:len(s)-1], 64)
		if err != nil {
			return 0, err
		}
		return time.Duration(hours * float64(time.Hour)), nil
	case 'd', 'D':
		days, err := strconv.ParseFloat(s[:len(s)-1], 64)
		if err != nil {
			return 0, err
		}
		return time.Duration(days * 24 * float64(time.Hour)), nil
	default:
		return time.ParseDuration(s)
	}
}

func (h *HistoryHandler) parseOptions(c *gin.Context) *store.QueryOptions {
	opts := &store.QueryOptions{
		TaskID: c.Query("task_id"),
		NodeID: c.Query("node_id"),
		OpType: c.Query("op_type"),
		Status: c.Query("status"),
	}
	if v := c.Query("last"); v != "" {
		if d, err := parseHistoryDuration(v); err == nil && d > 0 {
			opts.StartTime = time.Now().UTC().Add(-d)
		}
	}
	if v := c.Query("start_time"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			opts.StartTime = t
		}
	}
	if v := c.Query("end_time"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			opts.EndTime = t
		}
	}
	opts.Limit = 50
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 {
		if v > 1000 {
			v = 1000
		}
		opts.Limit = v
	}
	if v, err := strconv.Atoi(c.Query("offset")); err == nil && v >= 0 {
		opts.Offset = v
	}
	return opts
}

func (h *HistoryHandler) List(c *gin.Context) {
	opts := h.parseOptions(c)
	records, total, err := h.history.Query(c.Request.Context(), opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "query failed"})
		return
	}
	if records == nil {
		records = []*store.Record{}
	}
	c.JSON(http.StatusOK, gin.H{
		"data": records,
		"meta": gin.H{"total": total, "limit": opts.Limit, "offset": opts.Offset},
	})
}

func (h *HistoryHandler) Get(c *gin.Context) {
	taskID := c.Param("task_id")
	rec, err := h.history.GetByTaskID(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "record not found"})
		return
	}
	c.JSON(http.StatusOK, rec)
}

func (h *HistoryHandler) Stats(c *gin.Context) {
	st, err := h.history.Stats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "stats failed"})
		return
	}
	c.JSON(http.StatusOK, st)
}
```

> 注：`Export`/`Clean` 在 Task 10 实现；本任务先让 history.go 可编译（history_test.go 中 Export/Clean 路由引用 `hh.Export`/`hh.Clean` 会编译失败）。为避免 Task 9 测试编译失败，**先在 history.go 末尾加上 Task 10 的两个方法 stub 不可行**（stub 会让 Task 10 测试失真）。因此：**将 history_test.go 中 `historyTestSetup` 的 `reader.GET("/history/export", hh.Export)` 与 `admin.DELETE("/history", hh.Clean)` 两行暂时注释，Task 10 再取消注释**。或：将 Task 9 与 Task 10 合并为一次实现。推荐合并实现 Export/Clean（见 Task 10），并在 Task 9 测试中先注释这两行路由。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./handler/ -run 'TestHistory|TestParseHistoryDuration' -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add cmd/plugins/serve/handler/history.go cmd/plugins/serve/handler/history_test.go
git commit -m "feat(serve): HistoryHandler List/Get/Stats 与时间解析"
```

---

## Task 10: HistoryHandler — Export / Clean

**Files:**
- Modify: `cmd/plugins/serve/handler/history.go`
- Test: `cmd/plugins/serve/handler/history_test.go`

**Interfaces:**
- Consumes: `store.HistoryStore.Query/Cleanup`、`gopkg.in/yaml.v3`。
- Produces: `Export`（json/yaml 下载）、`Clean`（admin，days 校验）。

- [ ] **Step 1: 写失败测试**（追加到 history_test.go，并取消 Task 9 注释的 export/clean 路由）

```go
func TestHistoryExport_JSON(t *testing.T) {
	hs, r := historyTestSetup(t)
	hs.RecordOperation(context.Background(), &store.Operation{TaskID: "a", OpType: "command", Command: "uptime", Status: "completed"})

	w := historyGET(t, r, "/api/v1/history/export?format=json", adminToken())
	require.Equal(t, 200, w.Code)
	assert.Contains(t, w.Header().Get("Content-Disposition"), "history-")
	assert.Contains(t, w.Header().Get("Content-Disposition"), ".json")
	var recs []store.Record
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &recs))
	assert.Len(t, recs, 1)
}

func TestHistoryExport_YAML(t *testing.T) {
	hs, r := historyTestSetup(t)
	hs.RecordOperation(context.Background(), &store.Operation{TaskID: "a", OpType: "command", Command: "uptime", Status: "completed"})

	w := historyGET(t, r, "/api/v1/history/export?format=yaml", adminToken())
	require.Equal(t, 200, w.Code)
	assert.Contains(t, w.Header().Get("Content-Disposition"), ".yaml")
	assert.Contains(t, w.Body.String(), "uptime")
}

func TestHistoryClean_AdminOnly(t *testing.T) {
	hs, r := historyTestSetup(t)
	ctx := context.Background()
	hs.RecordOperation(ctx, &store.Operation{TaskID: "old", OpType: "command", Status: "completed", CreatedAt: time.Now().UTC().AddDate(0, 0, -100)})
	hs.RecordOperation(ctx, &store.Operation{TaskID: "new", OpType: "command", Status: "completed", CreatedAt: time.Now().UTC()})

	// viewer 无权清理
	wv := httptest.NewRecorder()
	reqv, _ := http.NewRequest("DELETE", "/api/v1/history?days=30", nil)
	reqv.Header.Set("Authorization", "Bearer "+viewerToken())
	r.ServeHTTP(wv, reqv)
	assert.Equal(t, 403, wv.Code)

	// days 非法
	wbad := httptest.NewRecorder()
	reqbad, _ := http.NewRequest("DELETE", "/api/v1/history?days=0", nil)
	reqbad.Header.Set("Authorization", "Bearer "+adminToken())
	r.ServeHTTP(wbad, reqbad)
	assert.Equal(t, 400, wbad.Code)

	// admin 正常清理
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/api/v1/history?days=30", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken())
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
	var resp struct {
		Deleted int64 `json:"deleted"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, int64(1), resp.Deleted)
}
```

并在 `historyTestSetup` 中取消注释（确保存在）：

```go
	reader.GET("/history/export", hh.Export)
	...
	admin.DELETE("/history", hh.Clean)
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./handler/ -run 'TestHistoryExport|TestHistoryClean' -v`
Expected: FAIL（`hh.Export undefined`）

- [ ] **Step 3: 实现 Export / Clean**（追加到 history.go）

```go
func (h *HistoryHandler) Export(c *gin.Context) {
	opts := h.parseOptions(c)
	opts.Limit = 1000
	opts.Offset = 0
	records, _, err := h.history.Query(c.Request.Context(), opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "export failed"})
		return
	}
	if records == nil {
		records = []*store.Record{}
	}
	ts := time.Now().UTC().Format("20060102-150405")
	switch c.DefaultQuery("format", "json") {
	case "yaml", "yml":
		data, err := yaml.Marshal(records)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "marshal failed"})
			return
		}
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=history-%s.yaml", ts))
		c.Data(http.StatusOK, "application/x-yaml", data)
	default:
		data, err := json.MarshalIndent(records, "", "  ")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "marshal failed"})
			return
		}
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=history-%s.json", ts))
		c.Data(http.StatusOK, "application/json", data)
	}
}

func (h *HistoryHandler) Clean(c *gin.Context) {
	days, err := strconv.Atoi(c.Query("days"))
	if err != nil || days <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "days must be a positive integer"})
		return
	}
	deleted, err := h.history.Cleanup(c.Request.Context(), days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "cleanup failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./handler/ -run 'TestHistory' -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add cmd/plugins/serve/handler/history.go cmd/plugins/serve/handler/history_test.go
git commit -m "feat(serve): HistoryHandler Export(json/yaml) 与 Clean(admin)"
```

---

## Task 11: server.go — WAL、HistoryStore 初始化与注入、路由

**Files:**
- Modify: `cmd/plugins/serve/server.go`

**Interfaces:**
- Consumes: 全部前置任务（HistoryStore、各 handler 的 History/Hub 字段、HistoryHandler、BroadcastHistoryUpdate）。
- Produces: Server 启动时启用 WAL/busy_timeout、初始化并注入 HistoryStore、注册 history 路由。

- [ ] **Step 1: 修改 Server 结构体**（server.go:43-64）增加字段：

```go
	historyHandler   *handler.HistoryHandler
	History          *store.HistoryStore
```

（加在 `wsHub *handler.WSHub` 附近）

- [ ] **Step 2: 在 Init 中启用 WAL pragmas**

在 `s.DB = db`（server.go:78）之后插入：

```go
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.ExecContext(context.Background(), pragma); err != nil {
			return nil, fmt.Errorf("set pragma: %w", err)
		}
	}
```

- [ ] **Step 3: 初始化 HistoryStore 并注入 handler**

在 `s.setupRoutes()`（server.go:165）之前插入：

```go
	s.History = store.NewHistoryStore(db)
	if err := s.History.Init(context.Background()); err != nil {
		return nil, fmt.Errorf("init history store: %w", err)
	}

	s.nodeHandler.History = s.History
	s.nodeHandler.Hub = s.wsHub
	s.execHandler.History = s.History
	s.transferHandler.History = s.History
	s.transferHandler.Hub = s.wsHub
	s.playbookHandler.History = s.History
	s.historyHandler = handler.NewHistoryHandler(s.History)
```

- [ ] **Step 4: 注册路由**（setupRoutes，server.go:170-247）

在 reader 组（server.go:183-199 区域，与 `/tasks` 等同级）追加：

```go
		reader.GET("/history", s.historyHandler.List)
		reader.GET("/history/stats", s.historyHandler.Stats)
		reader.GET("/history/export", s.historyHandler.Export)
		reader.GET("/history/detail/:task_id", s.historyHandler.Get)
```

在 admin 组（server.go:230-246）追加：

```go
			admin.DELETE("/history", s.historyHandler.Clean)
```

- [ ] **Step 5: 编译并运行 serve 包测试**

Run: `go build ./... && go test ./... -run 'TestServer' -v`（在 `cmd/plugins/serve`）
Expected: 编译成功；Server 测试 PASS

- [ ] **Step 6: 提交**

```bash
git add cmd/plugins/serve/server.go
git commit -m "feat(serve): 启用 WAL、初始化注入 HistoryStore 并注册 history 路由"
```

---

## Task 12: api.js — history 系列方法

**Files:**
- Modify: `cmd/plugins/serve/web/js/api.js`

**Interfaces:**
- Produces: `api.historyList(params)`、`api.historyStats()`、`api.historyGet(taskId)`、`api.historyExport(params, format)`、`api.historyClean(days)`。

- [ ] **Step 1: 在 api.js 的 `transferRecord` 之后（api.js:239 附近）追加方法**

```js
  historyList: (params = {}) => {
    const q = new URLSearchParams();
    for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== null && v !== '') q.set(k, v);
    return request('GET', `/history?${q}`);
  },

  historyStats: () =>
    request('GET', '/history/stats'),

  historyGet: (taskId) =>
    request('GET', `/history/detail/${encodeURIComponent(taskId)}`),

  historyClean: (days) =>
    request('DELETE', `/history?days=${encodeURIComponent(days)}`),

  historyExport: async (params = {}, format = 'json') => {
    const t = token();
    const q = new URLSearchParams();
    for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== null && v !== '') q.set(k, v);
    q.set('format', format);
    const res = await fetch(`${API_BASE}/history/export?${q}`, {
      method: 'GET',
      headers: { 'Authorization': `Bearer ${t}` },
    });
    if (res.status === 401) {
      localStorage.removeItem('token');
      localStorage.removeItem('user');
      window.location.href = '/login';
      throw new Error('Unauthorized');
    }
    if (!res.ok) throw new Error('Export failed');
    const disposition = res.headers.get('Content-Disposition') || '';
    const match = disposition.match(/filename=(.+)/);
    const filename = match ? match[1] : `history.${format === 'yaml' ? 'yaml' : 'json'}`;
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  },
```

- [ ] **Step 2: 语法检查**

Run: `node --check cmd/plugins/serve/web/js/api.js`
Expected: 无输出（语法正确）

- [ ] **Step 3: 提交**

```bash
git add cmd/plugins/serve/web/js/api.js
git commit -m "feat(web): api.js 增加 history 系列方法"
```

---

## Task 13: history.js — 完整重写

**Files:**
- Modify: `cmd/plugins/serve/web/js/pages/history.js`（整体替换）

**Interfaces:**
- Consumes: `api.historyList/historyStats/historyGet/historyExport/historyClean`、`api.connectWebSocket`、`shell.setPanelContent/setPanelTitle`、`render(html, afterRender)`。
- Produces: `renderHistory(render, navigate, user, api, shell)`，实现操作类型 tab、状态/时间/节点过滤、列表、详情钻取、导出、清理（admin）、分页、WS 刷新。

- [ ] **Step 1: 整体替换 history.js**

```js
export function renderHistory(render, navigate, user, api, shell) {
  const isAdmin = user && user.role === 'admin';
  const pageSize = 50;
  const state = {
    opType: '', status: '', nodeId: '', last: '', page: 1, total: 0, records: [], stats: null, wsCleanup: null,
  };

  function esc(s) { return String(s == null ? '' : s).replace(/[&<>"]/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m])); }
  function timeAgo(t) { if (!t) return '-'; const s = Math.floor((Date.now() - new Date(t).getTime())/1000); if (s<60) return s+'秒前'; if (s<3600) return Math.floor(s/60)+'分钟前'; if (s<86400) return Math.floor(s/3600)+'小时前'; return Math.floor(s/86400)+'天前'; }

  const OP_LABELS = { command: '命令', script: '脚本', file_transfer: '文件传输', playbook: '剧本', node_manage: '节点' };
  const OP_ICON = { command: 'terminal', script: 'terminal', file_transfer: 'upload', playbook: 'scroll', node_manage: 'nodes' };
  const STATUS_TEXT = { completed: '成功', failed: '失败', running: '进行中', cancelled: '已取消', pending: '等待中' };

  function buildParams() {
    return {
      op_type: state.opType, status: state.status, node_id: state.nodeId, last: state.last,
      limit: pageSize, offset: (state.page - 1) * pageSize,
    };
  }

  function renderPanel() {
    const st = state.stats || { total: 0, by_op_type: {} };
    const item = (key, label) => {
      const count = key === '' ? st.total : (st.by_op_type[key] || 0);
      const active = state.opType === key ? 'active' : '';
      return `<li class="panel-item ${active}" data-op="${key}"><span class="dot" style="background:var(--accent)"></span>${label} <span class="count">${count}</span></li>`;
    };
    shell.setPanelContent(
      item('', '全部') +
      item('command', '命令') +
      item('script', '脚本') +
      item('file_transfer', '文件传输') +
      item('playbook', '剧本') +
      item('node_manage', '节点')
    );
    document.querySelectorAll('#panelList .panel-item[data-op]').forEach(el => {
      el.addEventListener('click', () => { state.opType = el.dataset.op; state.page = 1; load(); });
    });
  }

  function statusBadge(s) {
    const cls = s === 'completed' ? 'success' : (s === 'failed' || s === 'cancelled') ? 'fail' : 'pending';
    return `<span class="hi-status ${cls}">${STATUS_TEXT[s] || esc(s)}</span>`;
  }

  function renderList() {
    const list = document.getElementById('history-list');
    if (!state.records.length) {
      list.innerHTML = '<div class="view-empty" style="padding:40px"><div class="empty-title">暂无历史记录</div></div>';
    } else {
      list.innerHTML = state.records.map(r => {
        const op = r.operation || {};
        const icon = OP_ICON[op.op_type] || 'clock';
        const targets = (op.targets || []).map(t => `<span class="chip">${esc(t)}</span>`).join(' ');
        return `<li class="history-item" data-task="${esc(op.task_id)}" style="cursor:pointer">
          <div class="hi-icon"><svg width="16" height="16" aria-hidden="true"><use href="#icon-${icon}"/></svg></div>
          <div class="hi-info">
            <div class="hi-name">${esc(op.command || '')}</div>
            <div class="hi-meta">${OP_LABELS[op.op_type] || esc(op.op_type)} · ${targets || '无目标'} · ${timeAgo(op.created_at)}</div>
          </div>
          ${statusBadge(op.status)}
          <div class="hi-action"><svg width="14" height="14" aria-hidden="true"><use href="#icon-chevron-right"/></svg></div>
        </li>`;
      }).join('');
      list.querySelectorAll('.history-item[data-task]').forEach(el => {
        el.addEventListener('click', () => openDetail(el.dataset.task));
      });
    }
    const totalPages = Math.max(1, Math.ceil(state.total / pageSize));
    document.getElementById('page-info').textContent = `共 ${state.total} 条记录 · 第 ${state.page}/${totalPages} 页`;
    document.getElementById('prev-btn').disabled = state.page <= 1;
    document.getElementById('next-btn').disabled = state.page >= totalPages;
  }

  async function load() {
    renderPanel();
    try {
      const [listRes, statsRes] = await Promise.all([api.historyList(buildParams()), api.historyStats()]);
      state.records = listRes.data || [];
      state.total = listRes.meta?.total || 0;
      state.stats = statsRes;
    } catch { state.records = []; state.total = 0; }
    renderPanel();
    renderList();
  }

  async function openDetail(taskId) {
    let rec;
    try { rec = await api.historyGet(taskId); } catch { alert('加载详情失败'); return; }
    const op = rec.operation || {};
    const execRows = (rec.command_executions || []).map(e =>
      `<tr><td>${esc(e.node_id)}</td><td>${e.exit_code}</td><td>${e.duration_ms}ms</td><td>${e.success ? '✅' : '❌'}</td><td>${esc(e.command)}</td></tr>`).join('');
    const tfRows = (rec.transfers || []).map(f =>
      `<tr><td>${esc(f.node_id)}</td><td>${esc(f.file_name)}</td><td>${f.file_size || '-'}</td><td>${esc(f.transfer_type)}</td><td>${esc(f.status)}</td></tr>`).join('');
    const commRows = (rec.communications || []).map(cm =>
      `<tr><td>${esc(cm.node_id)}</td><td>${esc(cm.direction)}</td><td>${esc(cm.message_type)}</td><td>${cm.success ? '✅' : '❌'}</td></tr>`).join('');

    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    overlay.innerHTML = `<div class="modal" style="max-width:760px;max-height:80vh;overflow:auto">
      <div class="modal-header"><h3>${esc(op.command || '操作详情')}</h3>
        <button class="btn btn-ghost btn-icon" id="detail-close"><svg width="16" height="16"><use href="#icon-x"/></svg></button></div>
      <div class="modal-body">
        <p style="color:var(--muted);font-size:12px">类型: ${OP_LABELS[op.op_type] || esc(op.op_type)} · 状态: ${STATUS_TEXT[op.status] || esc(op.status)} · 时间: ${esc(op.created_at)}</p>
        <p style="color:var(--muted);font-size:12px">目标: ${(op.targets || []).map(esc).join(', ') || '无'}</p>
        ${execRows ? `<h4>命令执行</h4><table class="table"><thead><tr><th>节点</th><th>退出码</th><th>耗时</th><th>状态</th><th>命令</th></tr></thead><tbody>${execRows}</tbody></table>` : ''}
        ${tfRows ? `<h4>文件传输</h4><table class="table"><thead><tr><th>节点</th><th>文件</th><th>大小</th><th>类型</th><th>状态</th></tr></thead><tbody>${tfRows}</tbody></table>` : ''}
        ${commRows ? `<h4>节点通信</h4><table class="table"><thead><tr><th>节点</th><th>方向</th><th>类型</th><th>状态</th></tr></thead><tbody>${commRows}</tbody></table>` : ''}
        ${(!execRows && !tfRows && !commRows) ? '<p style="color:var(--muted)">无明细数据</p>' : ''}
      </div>
    </div>`;
    document.body.appendChild(overlay);
    overlay.querySelector('#detail-close').addEventListener('click', () => overlay.remove());
    overlay.addEventListener('click', (e) => { if (e.target === overlay) overlay.remove(); });
  }

  render(`
    <div class="history-filters" style="display:flex;gap:8px;align-items:center;flex-wrap:wrap">
      <select class="select" id="status-filter">
        <option value="">全部状态</option>
        <option value="completed">成功</option>
        <option value="failed">失败</option>
        <option value="running">进行中</option>
        <option value="cancelled">已取消</option>
      </select>
      <select class="select" id="time-filter">
        <option value="">全部时间</option>
        <option value="1h">最近 1 小时</option>
        <option value="24h">最近 24 小时</option>
        <option value="7d">最近 7 天</option>
        <option value="30d">最近 30 天</option>
      </select>
      <input class="input" id="node-filter" placeholder="按节点过滤" style="max-width:160px" />
      <div class="spacer" style="flex:1"></div>
      <button class="btn btn-ghost btn-sm" id="export-json">导出 JSON</button>
      <button class="btn btn-ghost btn-sm" id="export-yaml">导出 YAML</button>
      ${isAdmin ? '<button class="btn btn-ghost btn-sm" id="clean-btn">清理</button>' : ''}
      <span style="font-size:12px;color:var(--muted)" id="page-info">加载中…</span>
    </div>
    <div class="card" style="overflow:auto">
      <ul class="history-list" id="history-list" style="padding:0 18px">
        <div class="view-loading">加载中…</div>
      </ul>
    </div>
    <div style="display:flex;justify-content:center;gap:6px;padding:4px 0">
      <button class="btn btn-ghost btn-sm" id="prev-btn" disabled>‹</button>
      <button class="btn btn-ghost btn-sm" id="next-btn">›</button>
    </div>
  `, () => {
    load();

    document.getElementById('status-filter').addEventListener('change', (e) => { state.status = e.target.value; state.page = 1; load(); });
    document.getElementById('time-filter').addEventListener('change', (e) => { state.last = e.target.value; state.page = 1; load(); });
    let nodeTimer = null;
    document.getElementById('node-filter').addEventListener('input', (e) => {
      clearTimeout(nodeTimer);
      nodeTimer = setTimeout(() => { state.nodeId = e.target.value.trim(); state.page = 1; load(); }, 300);
    });
    document.getElementById('prev-btn').addEventListener('click', () => { if (state.page > 1) { state.page--; load(); } });
    document.getElementById('next-btn').addEventListener('click', () => { const tp = Math.ceil(state.total / pageSize); if (state.page < tp) { state.page++; load(); } });
    document.getElementById('export-json').addEventListener('click', () => api.historyExport(buildParams(), 'json').catch(() => alert('导出失败')));
    document.getElementById('export-yaml').addEventListener('click', () => api.historyExport(buildParams(), 'yaml').catch(() => alert('导出失败')));
    if (isAdmin) {
      document.getElementById('clean-btn').addEventListener('click', async () => {
        const days = prompt('清理多少天之前的历史记录？', '30');
        if (!days) return;
        const n = parseInt(days, 10);
        if (!n || n <= 0) { alert('请输入正整数天数'); return; }
        if (!confirm(`确认清理 ${n} 天之前的历史记录？此操作不可撤销。`)) return;
        try { const res = await api.historyClean(n); alert(`已清理 ${res.deleted} 条记录`); load(); }
        catch { alert('清理失败'); }
      });
    }

    state.wsCleanup = api.connectWebSocket(msg => {
      if (msg.type === 'history_update' || msg.type === 'task_update' || msg.type === 'playbook_run_update') load();
    });

    return () => { if (state.wsCleanup) state.wsCleanup.close(); };
  });
}
```

- [ ] **Step 2: 语法检查**

Run: `node --check cmd/plugins/serve/web/js/pages/history.js`
Expected: 无输出

- [ ] **Step 3: 提交**

```bash
git add cmd/plugins/serve/web/js/pages/history.js
git commit -m "feat(web): 重写历史页面，支持多维过滤/详情钻取/导出/清理"
```

---

## Task 14: E2E 验证 + 纯 Go 构建检查

**Files:**
- 无新增（验证任务）

**Interfaces:**
- Consumes: 全部前置任务。
- Produces: 验证 `make build-serve` 纯 Go 可用、serve 测试全绿、手工 E2E 通过、CLI 与 Web 历史统一。

- [ ] **Step 1: 纯 Go 构建检查**

Run: `cd cmd/plugins/serve && CGO_ENABLED=0 go build ./...`
Expected: 成功（证明未引入 CGO 依赖）

Run: `make build-serve`
Expected: 成功生成 `build/owl-serve`

- [ ] **Step 2: 全量测试**

Run: `cd cmd/plugins/serve && go test ./... -v`
Expected: 全部 PASS

Run: `go vet ./...`（在 cmd/plugins/serve）
Expected: 无输出

- [ ] **Step 3: 手工 E2E（遵循 AGENTS.md）**

```bash
./build/owl-serve --reset-admin --port 8080   # 记录打印的 admin 密码
# 另一终端：
TOKEN="$(curl -s http://localhost:8080/api/v1/login -X POST -H 'Content-Type: application/json' -d '{"username":"admin","password":"<password>"}' | python3 -c 'import sys,json; print(json.load(sys.stdin)["token"])')"
curl -X POST http://localhost:8080/api/v1/nodes/seed -H "Authorization: Bearer $TOKEN"
# 触发一次命令执行（用任意在线/可达节点，或接受失败状态）：
curl -X POST http://localhost:8080/api/v1/exec -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"node_ids":["<seed-node-id>"],"command":"uptime"}'
# 验证历史 API：
curl -s "http://localhost:8080/api/v1/history?op_type=command" -H "Authorization: Bearer $TOKEN"
curl -s "http://localhost:8080/api/v1/history/stats" -H "Authorization: Bearer $TOKEN"
```

Expected: `/api/v1/history` 返回包含刚执行命令的 record；`/history/stats` 计数正确。

- [ ] **Step 4: CLI 与 Web 历史统一性验证**

```bash
# Web 触发的操作写入 ~/.owl/owl.db 的 operations 表，CLI 应能读到：
go run ./cmd/cli/main.go history --op-type command --last 1h
# 反向：CLI 执行一次操作后，Web 历史也应可见
go run ./cmd/cli/main.go exec run "echo hi" --nodes <node-id>   # 视实际节点配置
curl -s "http://localhost:8080/api/v1/history" -H "Authorization: Bearer $TOKEN"
```

Expected: CLI `owl history` 与 Web `/api/v1/history` 看到同一批 operations 记录（同库同表统一）。

- [ ] **Step 5: 浏览器手工验证前端**

打开 `http://localhost:8080`，登录后进入「任务历史」：
- 操作类型 tab 计数正确、可切换过滤
- 状态/时间/节点过滤生效
- 点击行弹出详情（命令执行/文件传输/通信表）
- 导出 JSON/YAML 下载成功
- admin 可见「清理」按钮，清理后列表更新
- 触发新操作时列表经 WS 自动刷新

- [ ] **Step 6: E2E 成功后原子提交（如有遗留改动）**

```bash
git status   # 确认无未提交的实现改动；若有，按模块原子提交
```

---

## Self-Review 结论

- **Spec 覆盖**：数据层(§1)→Task1-3；WAL(§2)→Task11；记录埋点(§3)→Task5-8；API(§4)→Task9-11；WS(§5)→Task4+各埋点；前端(§6)→Task12-13；错误处理(§7)→各任务 nil-safe/log；测试(§8)→各任务 TDD + Task14；构建(§9)→Task14；范围外(§10) 未纳入。✅
- **占位符扫描**：无 TBD/TODO；Task8/Task9 测试明确标注"须按现有 setup 调整"，属必要的实施者裁量提示而非占位。✅
- **类型一致性**：`HistoryStore` 方法名、`Operation`/`CommandExecution`/`FileTransfer`/`Record`/`Stats`/`QueryOptions` 字段在各任务一致；handler 导出字段统一为 `History`（transfer/node 额外 `Hub`）；路由 `/history/detail/:task_id` 前后端一致。✅
- **已知偏差**：详情路由由 spec 的 `/history/:task_id` 调整为 `/history/detail/:task_id`（规避 gin 路由冲突），已在 Global Constraints 与 Task9/12/13 体现。
