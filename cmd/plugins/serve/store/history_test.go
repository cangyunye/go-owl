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
