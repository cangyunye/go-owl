package handler

import (
	"database/sql"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	run, err := rs.Create(t.Context(), "pb-1", "demo", "/nonexistent.yaml", []string{"n1"}, nil, "", false)
	require.NoError(t, err)

	op := &store.Operation{TaskID: run.ID, OpType: "playbook", Command: "playbook run demo", Targets: []string{"n1"}, PlaybookPath: "/nonexistent.yaml", Status: "running"}
	require.NoError(t, hs.RecordOperation(t.Context(), op))

	recs, total, err := hs.Query(t.Context(), &store.QueryOptions{OpType: "playbook"})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	assert.Equal(t, "/nonexistent.yaml", recs[0].Operation.PlaybookPath)
}
