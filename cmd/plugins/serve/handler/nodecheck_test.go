package handler

import (
	"database/sql"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	ai2 "github.com/cangyunye/go-owl/internal/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func nodeCheckTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`CREATE TABLE nodes (
		id TEXT PRIMARY KEY, name TEXT, address TEXT, port INTEGER DEFAULT 22,
		user TEXT, password TEXT, ssh_key TEXT, status TEXT DEFAULT 'unknown',
		groups TEXT DEFAULT '[]', labels TEXT DEFAULT '{}',
		proxy_jump TEXT DEFAULT '', created_at TIMESTAMP, updated_at TIMESTAMP
	)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO nodes (id, name, address, user, status, groups) VALUES
		('n1', 'web-01', '127.0.0.1', 'root', 'online', '["web"]'),
		('n2', 'web-k8s-01', '127.0.0.1', 'root', 'online', '["web-k8s"]'),
		('n3', 'db-01', '127.0.0.1', 'root', 'online', '["db"]'),
		('n4', 'upper-01', '127.0.0.1', 'root', 'online', '["WEB"]')`)
	require.NoError(t, err)
	return db
}

func nodeCheckTestExecutor(t *testing.T) (*WebExecutor, *sql.DB) {
	t.Helper()
	db := nodeCheckTestDB(t)

	ts := store.NewTaskStore(db)
	require.NoError(t, ts.Init(t.Context()))
	trs := store.NewTransferRecordStore(db)
	require.NoError(t, trs.Init(t.Context()))
	prs := store.NewPlaybookRunStore(db)
	require.NoError(t, prs.Init(t.Context()))
	ns := store.NewNodeStore(db)
	pbs := store.NewPlaybookStore(db)
	require.NoError(t, pbs.Init(t.Context()))
	audit := store.NewAIAuditStore(db)
	require.NoError(t, audit.Init(t.Context()))

	e := NewWebExecutor(db, ts, trs, prs, ns, pbs, audit, NewKeyManager(), false)
	e.userRole = "admin"
	return e, db
}

func TestWebExecutor_NodeCheck_GroupSelectsExactMatchOnly(t *testing.T) {
	e, _ := nodeCheckTestExecutor(t)

	res, err := e.NodeCheck(t.Context(), ai2.NodeCheckParams{Group: "web", Timeout: 1})
	require.NoError(t, err)
	assert.Contains(t, res.Text, "检查 1 个节点", "group=web 精确匹配只应命中 n1")
	assert.Contains(t, res.Text, "[n1]")
	assert.NotContains(t, res.Text, "[n2]", "web 不得命中 web-k8s")
	assert.NotContains(t, res.Text, "[n3]")
	assert.NotContains(t, res.Text, "[n4]", "不得大小写不敏感命中 WEB")
}

func TestWebExecutor_NodeCheck_GroupUpdateOnlySelectedNode(t *testing.T) {
	e, db := nodeCheckTestExecutor(t)

	res, err := e.NodeCheck(t.Context(), ai2.NodeCheckParams{Group: "web", Update: true, Timeout: 1})
	require.NoError(t, err)
	require.Contains(t, res.Text, "[n1]")

	var status string
	var updatedAt sql.NullString
	require.NoError(t, db.QueryRow(`SELECT status, updated_at FROM nodes WHERE id = 'n1'`).Scan(&status, &updatedAt))
	assert.Equal(t, "offline", status, "n1 无凭据不可达，应被写为 offline")
	assert.True(t, updatedAt.Valid, "n1 updated_at 应被更新")

	for _, id := range []string{"n2", "n3", "n4"} {
		require.NoError(t, db.QueryRow(`SELECT status, updated_at FROM nodes WHERE id = ?`, id).Scan(&status, &updatedAt))
		assert.Equal(t, "online", status, "%s 不应被触碰", id)
		assert.False(t, updatedAt.Valid, "%s updated_at 不应被触碰", id)
	}
}
