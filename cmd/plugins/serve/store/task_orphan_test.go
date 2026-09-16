package store

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func newTaskStoreForTest(t *testing.T) (*sql.DB, *TaskStore) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	ts := NewTaskStore(db)
	require.NoError(t, ts.Init(t.Context()))
	return db, ts
}

// 服务重启会丢失执行 goroutine：遗留的 running/queued 任务必须被对账标记
// 为失败并写明原因，否则任务永久停在"执行中"，用户永远等不到结束。
func TestTaskStore_FailOrphaned(t *testing.T) {
	_, ts := newTaskStoreForTest(t)
	ctx := t.Context()

	running, err := ts.CreateWithRecord(ctx, "node-1", "echo r", "rec-running")
	require.NoError(t, err)
	require.NoError(t, ts.UpdateStatus(ctx, running.ID, TaskStatusRunning, "partial out", nil))

	queued, err := ts.CreateWithRecord(ctx, "node-2", "echo q", "rec-queued")
	require.NoError(t, err)

	done, err := ts.CreateWithRecord(ctx, "node-3", "echo d", "rec-done")
	require.NoError(t, err)
	require.NoError(t, ts.UpdateStatus(ctx, done.ID, TaskStatusCompleted, "ok", nil))

	recIDs, err := ts.FailOrphaned(ctx)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"rec-running", "rec-queued"}, recIDs,
		"应返回受影响的 record（供调用方同步 operation 状态）")

	got, err := ts.Get(ctx, running.ID)
	require.NoError(t, err)
	assert.Equal(t, TaskStatusFailed, got.Status)
	assert.Contains(t, got.Output, "服务重启")
	assert.Contains(t, got.Output, "partial out", "已采集的输出应保留")
	assert.NotNil(t, got.CompletedAt, "中断任务应写入完成时间")

	gotQ, err := ts.Get(ctx, queued.ID)
	require.NoError(t, err)
	assert.Equal(t, TaskStatusFailed, gotQ.Status)
	assert.Contains(t, gotQ.Output, "服务重启")

	gotD, err := ts.Get(ctx, done.ID)
	require.NoError(t, err)
	assert.Equal(t, TaskStatusCompleted, gotD.Status, "已完成任务不得被改动")
	assert.Equal(t, "ok", gotD.Output)

	// 幂等：没有悬挂任务时返回空且不报错
	recIDs2, err := ts.FailOrphaned(ctx)
	require.NoError(t, err)
	assert.Empty(t, recIDs2)
}
