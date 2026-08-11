package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTransferRecordStore(t *testing.T) *TransferRecordStore {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db")+"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	ts := NewTaskStore(db)
	require.NoError(t, ts.Init(ctx))
	s := NewTransferRecordStore(db)
	require.NoError(t, s.Init(ctx))
	return s
}

func TestTransferRecordStore_ConcurrentUpdateNodeResult(t *testing.T) {
	ctx := context.Background()
	s := newTransferRecordStore(t)

	rec, err := s.Create(ctx, "/tmp/src", "/opt/dst", "push", "")
	require.NoError(t, err)

	const nodeCount = 20
	require.NoError(t, s.SetNodeCount(ctx, rec.ID, nodeCount))

	var wg sync.WaitGroup
	for i := 0; i < nodeCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			require.NoError(t, s.UpdateNodeResult(ctx, rec.ID, true))
		}()
	}
	wg.Wait()

	got, err := s.Get(ctx, rec.ID)
	require.NoError(t, err)
	assert.Equal(t, TransferCompleted, got.Status)
	assert.Equal(t, nodeCount, got.SuccessCount)
	assert.Equal(t, 0, got.FailedCount)
	require.NotNil(t, got.CompletedAt)
}

func TestTransferRecordStore_UpdateNodeResult_AllFail(t *testing.T) {
	ctx := context.Background()
	s := newTransferRecordStore(t)

	rec, err := s.Create(ctx, "/tmp/src", "/opt/dst", "push", "")
	require.NoError(t, err)
	require.NoError(t, s.SetNodeCount(ctx, rec.ID, 3))

	for i := 0; i < 3; i++ {
		require.NoError(t, s.UpdateNodeResult(ctx, rec.ID, false))
	}

	got, err := s.Get(ctx, rec.ID)
	require.NoError(t, err)
	assert.Equal(t, TransferFailed, got.Status)
	assert.Equal(t, 3, got.FailedCount)
}

func TestTransferRecordStore_UpdateNodeResult_Partial(t *testing.T) {
	ctx := context.Background()
	s := newTransferRecordStore(t)

	rec, err := s.Create(ctx, "/tmp/src", "/opt/dst", "push", "")
	require.NoError(t, err)
	require.NoError(t, s.SetNodeCount(ctx, rec.ID, 3))

	require.NoError(t, s.UpdateNodeResult(ctx, rec.ID, true))
	require.NoError(t, s.UpdateNodeResult(ctx, rec.ID, false))
	require.NoError(t, s.UpdateNodeResult(ctx, rec.ID, true))

	got, err := s.Get(ctx, rec.ID)
	require.NoError(t, err)
	assert.Equal(t, TransferPartialSuccess, got.Status)
	assert.Equal(t, 2, got.SuccessCount)
	assert.Equal(t, 1, got.FailedCount)
}

var _ = sql.ErrNoRows

func TestTransferRecordStore_PayloadPersistAndRead(t *testing.T) {
	ctx := context.Background()
	s := newTransferRecordStore(t)

	payload := `{"action":"push","node_ids":["n1","n2"],"groups":["web"],"labels":{"env":"prod"},"source_path":"/a/b.txt","dest_path":"/tmp/","direction":"push","overwrite":true,"mode":"0644","parallel":true,"resume":true}`
	rec, err := s.Create(ctx, "/a/b.txt", "/tmp/", "push", payload)
	require.NoError(t, err)

	got, err := s.Get(ctx, rec.ID)
	require.NoError(t, err)
	assert.Equal(t, payload, got.Payload)

	recs, total, err := s.List(ctx, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, recs, 1)
	assert.Equal(t, payload, recs[0].Payload)

	require.NoError(t, s.SetPayload(ctx, rec.ID, `{"node_ids":["n3"]}`))
	got, err = s.Get(ctx, rec.ID)
	require.NoError(t, err)
	assert.Equal(t, `{"node_ids":["n3"]}`, got.Payload)
}
