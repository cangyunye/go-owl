package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openPlaybookRunTestDB(t *testing.T) *PlaybookRunStore {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	s := NewPlaybookRunStore(db)
	require.NoError(t, s.Init(context.Background()))
	return s
}

func TestPlaybookRunStore_CreateAndGet(t *testing.T) {
	s := openPlaybookRunTestDB(t)
	ctx := context.Background()

	run, err := s.Create(ctx, "abc123", "deploy-web", "/playbooks/deploy.yaml", []string{"node1", "node2"}, map[string]string{"env": "prod"}, "deploy")
	require.NoError(t, err)
	assert.NotEmpty(t, run.ID)
	assert.Equal(t, "abc123", run.PlaybookID)
	assert.Equal(t, "deploy-web", run.PlaybookName)
	assert.Equal(t, "/playbooks/deploy.yaml", run.PlaybookFile)
	assert.Equal(t, "queued", string(run.Status))

	got, err := s.Get(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, run.ID, got.ID)
	assert.Equal(t, "abc123", got.PlaybookID)
	assert.Equal(t, "deploy-web", got.PlaybookName)
	assert.Equal(t, "/playbooks/deploy.yaml", got.PlaybookFile)
	assert.Equal(t, []string{"node1", "node2"}, got.TargetNodes)
	assert.Equal(t, "prod", got.ExtraVars["env"])
	assert.Equal(t, "deploy", got.Tags)
}

func TestPlaybookRunStore_GetNotFound(t *testing.T) {
	s := openPlaybookRunTestDB(t)
	_, err := s.Get(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestPlaybookRunStore_List(t *testing.T) {
	s := openPlaybookRunTestDB(t)
	ctx := context.Background()

	_, err := s.Create(ctx, "pb1", "run1", "/a.yaml", nil, nil, "")
	require.NoError(t, err)
	_, err = s.Create(ctx, "pb2", "run2", "/b.yaml", nil, nil, "")
	require.NoError(t, err)
	_, err = s.Create(ctx, "pb3", "run3", "/c.yaml", nil, nil, "")
	require.NoError(t, err)

	runs, total, err := s.List(ctx, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, runs, 3)
	assert.Equal(t, "pb3", runs[0].PlaybookID)
	assert.Equal(t, "pb1", runs[2].PlaybookID)
}

func TestPlaybookRunStore_ListPagination(t *testing.T) {
	s := openPlaybookRunTestDB(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_, err := s.Create(ctx, "pb", "run", "/x.yaml", nil, nil, "")
		require.NoError(t, err)
	}

	runs, total, err := s.List(ctx, 2, 0)
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	assert.Len(t, runs, 2)

	runs, total, err = s.List(ctx, 2, 3)
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	assert.Len(t, runs, 2)
}

func TestPlaybookRunStore_UpdateStatus(t *testing.T) {
	s := openPlaybookRunTestDB(t)
	ctx := context.Background()

	run, _ := s.Create(ctx, "pb1", "run1", "/a.yaml", nil, nil, "")

	err := s.UpdateStatus(ctx, run.ID, "running", "")
	require.NoError(t, err)

	got, _ := s.Get(ctx, run.ID)
	assert.Equal(t, "running", string(got.Status))
	assert.NotNil(t, got.StartedAt)
	assert.Nil(t, got.CompletedAt)

	err = s.UpdateStatus(ctx, run.ID, "completed", "")
	require.NoError(t, err)

	got, _ = s.Get(ctx, run.ID)
	assert.Equal(t, "completed", string(got.Status))
	assert.NotNil(t, got.CompletedAt)
}

func TestPlaybookRunStore_UpdateStatusFailed(t *testing.T) {
	s := openPlaybookRunTestDB(t)
	ctx := context.Background()

	run, _ := s.Create(ctx, "pb1", "run1", "/a.yaml", nil, nil, "")

	err := s.UpdateStatus(ctx, run.ID, "failed", "something broke")
	require.NoError(t, err)

	got, _ := s.Get(ctx, run.ID)
	assert.Equal(t, "failed", string(got.Status))
	assert.Equal(t, "something broke", got.Error)
	assert.NotNil(t, got.CompletedAt)
}

func TestPlaybookRunStore_AppendResult(t *testing.T) {
	s := openPlaybookRunTestDB(t)
	ctx := context.Background()

	run, _ := s.Create(ctx, "pb1", "run1", "/a.yaml", nil, nil, "")

	step := &stepResult{TaskName: "task1", NodeID: "node1", Status: "completed", ExitCode: 0}
	err := s.AppendResult(ctx, run.ID, toStepResult(step))
	require.NoError(t, err)

	got, _ := s.Get(ctx, run.ID)
	require.Len(t, got.Results, 1)
	assert.Equal(t, "task1", got.Results[0].TaskName)
	assert.Equal(t, "node1", got.Results[0].NodeID)

	step2 := &stepResult{TaskName: "task2", NodeID: "node2", Status: "failed", ExitCode: 1}
	err = s.AppendResult(ctx, run.ID, toStepResult(step2))
	require.NoError(t, err)

	got, _ = s.Get(ctx, run.ID)
	require.Len(t, got.Results, 2)
}

func TestPlaybookRunStore_InitIdempotent(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	s := NewPlaybookRunStore(db)
	require.NoError(t, s.Init(context.Background()))
	require.NoError(t, s.Init(context.Background()))

	_, err = s.Create(context.Background(), "pb1", "run1", "/a.yaml", nil, nil, "")
	require.NoError(t, err)
}

func TestPlaybookRunStore_CreateWithEmptyOptionals(t *testing.T) {
	s := openPlaybookRunTestDB(t)
	ctx := context.Background()

	run, err := s.Create(ctx, "pb1", "run1", "/a.yaml", nil, nil, "")
	require.NoError(t, err)

	got, _ := s.Get(ctx, run.ID)
	assert.Empty(t, got.TargetNodes)
	assert.NotNil(t, got.ExtraVars)
	assert.Empty(t, got.Tags)
}

type stepResult struct {
	TaskName string
	NodeID   string
	Status   string
	ExitCode int
}

func toStepResult(s *stepResult) *model.StepResult {
	return &model.StepResult{
		TaskName: s.TaskName,
		NodeID:   s.NodeID,
		Status:   s.Status,
		ExitCode: s.ExitCode,
	}
}
