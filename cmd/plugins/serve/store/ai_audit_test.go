package store

import (
	"context"
	"testing"

	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAIAuditStore_CreateAndGet(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	s := NewAIAuditStore(db)
	require.NoError(t, s.Init(ctx))

	rec := &AIAuditRecord{
		UserID:         "user-1",
		Intent:         "deploy",
		Tool:           "run_command",
		ParamsSnapshot: `{"cmd":"uptime"}`,
		Result:         "success",
		TargetType:     "node",
		TargetIDs:      `["node-1"]`,
		PromptText:     "run uptime on node-1",
		ReplyText:      "done",
		LLMModel:       "gpt-4",
		LLMDurationMs:  1500,
	}

	err := s.Create(ctx, rec)
	require.NoError(t, err)
	assert.NotEmpty(t, rec.ID)
	assert.False(t, rec.CreatedAt.IsZero())

	got, err := s.Get(ctx, rec.ID)
	require.NoError(t, err)
	assert.Equal(t, rec.ID, got.ID)
	assert.Equal(t, "user-1", got.UserID)
	assert.Equal(t, "deploy", got.Intent)
	assert.Equal(t, "run_command", got.Tool)
	assert.Equal(t, `{"cmd":"uptime"}`, got.ParamsSnapshot)
	assert.Equal(t, "gpt-4", got.LLMModel)
	assert.Equal(t, int64(1500), got.LLMDurationMs)
}

func TestAIAuditStore_Get_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	s := NewAIAuditStore(db)
	require.NoError(t, s.Init(ctx))

	_, err := s.Get(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestAIAuditStore_List(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	s := NewAIAuditStore(db)
	require.NoError(t, s.Init(ctx))

	recs := []*AIAuditRecord{
		{UserID: "u1", Tool: "tool1", ParamsSnapshot: "{}", LLMModel: "gpt-4"},
		{UserID: "u2", Tool: "tool2", ParamsSnapshot: "{}", LLMModel: "gpt-4"},
		{UserID: "u3", Tool: "tool3", ParamsSnapshot: "{}", LLMModel: "gpt-4"},
	}
	for _, r := range recs {
		require.NoError(t, s.Create(ctx, r))
	}

	records, total, err := s.List(ctx, "", 0, 10)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, records, 3)
}

func TestAIAuditStore_List_UserFilter(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	s := NewAIAuditStore(db)
	require.NoError(t, s.Init(ctx))

	for _, u := range []string{"alice", "bob", "alice", "charlie"} {
		require.NoError(t, s.Create(ctx, &AIAuditRecord{
			UserID:         u,
			Tool:           "cmd",
			ParamsSnapshot: "{}",
			LLMModel:       "gpt-4",
		}))
	}

	records, total, err := s.List(ctx, "alice", 0, 10)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, records, 2)
	for _, r := range records {
		assert.Equal(t, "alice", r.UserID)
	}
}

func TestAIAuditStore_ListPagination(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	s := NewAIAuditStore(db)
	require.NoError(t, s.Init(ctx))

	for i := 0; i < 5; i++ {
		require.NoError(t, s.Create(ctx, &AIAuditRecord{
			UserID:         "u1",
			Tool:           "cmd",
			ParamsSnapshot: "{}",
			LLMModel:       "gpt-4",
		}))
	}

	records, total, err := s.List(ctx, "", 0, 2)
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	assert.Len(t, records, 2)
}
