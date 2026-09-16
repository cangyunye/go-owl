package serve

import (
	"database/sql"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// 启动对账：服务重启后遗留 running 的任务标记失败，并同步其 operation 状态，
// 否则任务与历史都会永久停在"执行中"。
func TestReconcileOrphanedTasks(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	ctx := t.Context()
	ts := store.NewTaskStore(db)
	require.NoError(t, ts.Init(ctx))
	hs := store.NewHistoryStore(db)
	require.NoError(t, hs.Init(ctx))

	task, err := ts.CreateWithRecord(ctx, "node-1", "echo hi", "rec-1")
	require.NoError(t, err)
	require.NoError(t, ts.UpdateStatus(ctx, task.ID, store.TaskStatusRunning, "partial", nil))
	require.NoError(t, hs.RecordOperation(ctx, &store.Operation{
		TaskID: "rec-1", OpType: "command", Command: "echo hi", Targets: []string{"node-1"}, Status: "running",
	}))

	require.NoError(t, reconcileOrphanedTasks(ctx, ts, hs))

	got, err := ts.Get(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, store.TaskStatusFailed, got.Status)
	assert.Contains(t, got.Output, "服务重启")

	recs, total, err := hs.Query(ctx, &store.QueryOptions{OpType: "command"})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	assert.Equal(t, "failed", recs[0].Operation.Status, "operation 状态应同步为 failed")
}

// 无悬挂任务时不应报错，也不应改动任何数据
func TestReconcileOrphanedTasks_Noop(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	ctx := t.Context()
	ts := store.NewTaskStore(db)
	require.NoError(t, ts.Init(ctx))
	hs := store.NewHistoryStore(db)
	require.NoError(t, hs.Init(ctx))

	task, err := ts.Create(ctx, "node-1", "echo hi")
	require.NoError(t, err)
	require.NoError(t, ts.UpdateStatus(ctx, task.ID, store.TaskStatusCompleted, "ok", nil))

	require.NoError(t, reconcileOrphanedTasks(ctx, ts, hs))

	got, err := ts.Get(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, store.TaskStatusCompleted, got.Status)
}
