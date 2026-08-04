package handler

import (
	"context"
	"database/sql"
	"testing"

	nodeselect "github.com/cangyunye/go-owl/internal/node/select"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func nodeSelectTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`CREATE TABLE nodes (
		id TEXT PRIMARY KEY, name TEXT, address TEXT, port INTEGER DEFAULT 22,
		user TEXT, password TEXT, ssh_key TEXT, status TEXT DEFAULT 'unknown',
		groups TEXT DEFAULT '[]', labels TEXT DEFAULT '{}',
		proxy_jump TEXT DEFAULT ''
	)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO nodes (id, name, user, status, groups, labels) VALUES
		('n1', 'web-01', 'root', 'online', '["web","prod"]', '{"env":"prod"}'),
		('n2', 'web-k8s-01', 'root', 'online', '["web-k8s"]', '{"env":"prod","zone":"a"}'),
		('n3', 'db-01', 'admin', 'online', '["db"]', '{}')`)
	require.NoError(t, err)
	return db
}

func TestDBNodeSource_List(t *testing.T) {
	db := nodeSelectTestDB(t)
	src := &dbNodeSource{db: db}
	rows, err := src.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, rows, 3)
}

func TestResolveNodeIDs_GroupExact(t *testing.T) {
	db := nodeSelectTestDB(t)
	ids, err := resolveNodeIDs(context.Background(), db, execRequest{Groups: []string{"web"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"n1"}, ids, "web 不得命中 web-k8s")
}

func TestResolveNodeIDs_LabelExact(t *testing.T) {
	db := nodeSelectTestDB(t)
	ids, err := resolveNodeIDs(context.Background(), db, execRequest{
		Labels: map[string]string{"env": "prod", "zone": "a"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"n2"}, ids)
}

func TestResolveNodeIDs_UnknownNodeErrors(t *testing.T) {
	db := nodeSelectTestDB(t)
	_, err := resolveNodeIDs(context.Background(), db, execRequest{NodeIDs: []string{"ghost"}})
	require.Error(t, err)
}

func TestSelector_GroupExact_NoFalsePositive(t *testing.T) {
	db := nodeSelectTestDB(t)
	sel := nodeselect.NewSelector(&dbNodeSource{db: db})
	got, err := sel.Select(context.Background(), nodeselect.SelectOptions{Groups: []string{"web"}})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "n1", got[0].ID)
}
