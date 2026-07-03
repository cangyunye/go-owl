package store

import (
	"context"
	"testing"

	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
