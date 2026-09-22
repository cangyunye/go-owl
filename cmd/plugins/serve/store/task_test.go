package store

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestTaskStore_CreateAndGet(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	s := NewTaskStore(db)
	require.NoError(t, s.Init(ctx))

	task, err := s.Create(ctx, "node-1", "uptime")
	require.NoError(t, err)
	assert.NotEmpty(t, task.ID)
	assert.Equal(t, "node-1", task.NodeID)
	assert.Equal(t, "uptime", task.Command)
	assert.Equal(t, TaskStatusQueued, task.Status)

	got, err := s.Get(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, task.ID, got.ID)
	assert.Equal(t, "uptime", got.Command)
}

func TestTaskStore_Get_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	s := NewTaskStore(db)
	require.NoError(t, s.Init(ctx))

	_, err := s.Get(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestTaskStore_List(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	s := NewTaskStore(db)
	require.NoError(t, s.Init(ctx))

	s.Create(ctx, "n1", "cmd1")
	s.Create(ctx, "n2", "cmd2")
	s.Create(ctx, "n3", "cmd3")

	tasks, total, err := s.List(ctx, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, tasks, 3)
}

func TestTaskStore_ListPagination(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	s := NewTaskStore(db)
	require.NoError(t, s.Init(ctx))

	for i := 0; i < 5; i++ {
		s.Create(ctx, "n1", "cmd")
	}

	tasks, total, err := s.List(ctx, 2, 0)
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	assert.Len(t, tasks, 2)
}

func TestTaskStore_UpdateStatus(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	s := NewTaskStore(db)
	require.NoError(t, s.Init(ctx))

	task, _ := s.Create(ctx, "n1", "uptime")

	// Transition through lifecycle: queued -> running -> completed
	err := s.UpdateStatus(ctx, task.ID, TaskStatusRunning, "", nil)
	require.NoError(t, err)

	exitCode := 0
	err = s.UpdateStatus(ctx, task.ID, TaskStatusCompleted, "ok\n", &exitCode)
	require.NoError(t, err)

	got, _ := s.Get(ctx, task.ID)
	assert.Equal(t, TaskStatusCompleted, got.Status)
	assert.Equal(t, "ok\n", got.Output)
	assert.Equal(t, 0, *got.ExitCode)
	assert.NotNil(t, got.StartedAt)
	assert.NotNil(t, got.CompletedAt)
}

func TestTaskStore_ListByNode(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	s := NewTaskStore(db)
	require.NoError(t, s.Init(ctx))

	s.Create(ctx, "n1", "cmd1")
	s.Create(ctx, "n1", "cmd2")
	s.Create(ctx, "n2", "cmd3")

	tasks, err := s.ListByNode(ctx, "n1", TaskStatusQueued)
	require.NoError(t, err)
	assert.Len(t, tasks, 2)
}

// TestTaskStore_InitCreatesIndexes 验证 tasks 表的查询索引：
// ListByRecord（按 record_id）、ListByNode（按 node_id+status）、updateOpStatus
// （按 record_id 只扫 status）都建立在裸表上，列表页与终态对账会全表扫描。
func TestTaskStore_InitCreatesIndexes(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	s := NewTaskStore(db)
	require.NoError(t, s.Init(ctx))
	// 幂等：对同一库重复 Init 不报错（CREATE INDEX IF NOT EXISTS）
	require.NoError(t, s.Init(ctx))

	for _, idx := range []string{"idx_tasks_record_id", "idx_tasks_node_id", "idx_tasks_status"} {
		var name string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`, idx).Scan(&name)
		require.NoError(t, err, "tasks 表缺少索引 %s", idx)
		assert.Equal(t, idx, name)
	}
}

// TestTaskStore_InitIndexesOnLegacyDB 验证存量库（表已存在、无索引）迁移后补齐索引。
func TestTaskStore_InitIndexesOnLegacyDB(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// 旧版建表：只有表，没有索引
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			node_id TEXT NOT NULL,
			command TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'queued',
			output TEXT DEFAULT '',
			exit_code INTEGER,
			record_id TEXT DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			started_at TIMESTAMP,
			completed_at TIMESTAMP
		)
	`)
	require.NoError(t, err)

	s := NewTaskStore(db)
	require.NoError(t, s.Init(ctx))

	for _, idx := range []string{"idx_tasks_record_id", "idx_tasks_node_id", "idx_tasks_status"} {
		var name string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`, idx).Scan(&name)
		require.NoError(t, err, "存量 tasks 表迁移后缺少索引 %s", idx)
	}
}

// TestTaskStore_ListOmitsOutput 列表路径不回传 output 大字段：
// 流式输出全程累积在 tasks.output，列表页 50 条会全量带回。列表 UI 只展示
// node/command/status（tasks.js renderTable），output 由详情 GET 与 WS 提供。
func TestTaskStore_ListOmitsOutput(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	s := NewTaskStore(db)
	require.NoError(t, s.Init(ctx))

	task, _ := s.Create(ctx, "n1", "cmd")
	big := strings.Repeat("x", 128*1024)
	exitCode := 0
	require.NoError(t, s.UpdateStatus(ctx, task.ID, TaskStatusRunning, big, &exitCode))

	tasks, total, err := s.List(ctx, 10, 0)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, 1, total)
	assert.Empty(t, tasks[0].Output, "列表路径不应回传 output 大字段")

	// 详情路径（Get）保留完整 output
	got, err := s.Get(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, big, got.Output)
}

func TestTaskStore_Delete(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	s := NewTaskStore(db)
	require.NoError(t, s.Init(ctx))

	task, _ := s.Create(ctx, "n1", "cmd")
	require.NoError(t, s.Delete(ctx, task.ID))

	_, err := s.Get(ctx, task.ID)
	assert.Error(t, err)
}
