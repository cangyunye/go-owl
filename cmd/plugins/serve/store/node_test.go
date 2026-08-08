package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func openNodeTestDB(t *testing.T) *NodeStore {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`
		CREATE TABLE nodes (
			id TEXT PRIMARY KEY,
			name TEXT,
			address TEXT,
			port INTEGER DEFAULT 22,
			user TEXT,
			status TEXT DEFAULT 'unknown',
			groups TEXT DEFAULT '[]',
			labels TEXT DEFAULT '{}',
			proxy_jump TEXT DEFAULT '',
			created_at TIMESTAMP,
			updated_at TIMESTAMP
		)
	`)
	require.NoError(t, err)

	return NewNodeStore(db)
}

func insertNode(t *testing.T, s *NodeStore, id, groups string) {
	t.Helper()
	_, err := s.db.Exec(
		`INSERT INTO nodes (id, name, address, user, groups) VALUES (?, ?, ?, ?, ?)`,
		id, "node-"+id, "10.0.0.1", "root", groups,
	)
	require.NoError(t, err)
}

func TestNodeStore_ListByGroups_Empty(t *testing.T) {
	s := openNodeTestDB(t)
	ids, err := s.ListByGroups(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, ids)
}

func TestNodeStore_ListByGroups_SingleGroup(t *testing.T) {
	s := openNodeTestDB(t)
	insertNode(t, s, "n1", `["web","db"]`)
	insertNode(t, s, "n2", `["web"]`)
	insertNode(t, s, "n3", `["cache"]`)

	ids, err := s.ListByGroups(context.Background(), []string{"web"})
	require.NoError(t, err)
	assert.Len(t, ids, 2)
	assert.ElementsMatch(t, []string{"n1", "n2"}, ids)
}

func TestNodeStore_ListByGroups_MultipleGroups(t *testing.T) {
	s := openNodeTestDB(t)
	insertNode(t, s, "n1", `["web"]`)
	insertNode(t, s, "n2", `["db"]`)
	insertNode(t, s, "n3", `["cache"]`)

	ids, err := s.ListByGroups(context.Background(), []string{"web", "db"})
	require.NoError(t, err)
	assert.Len(t, ids, 2)
	assert.ElementsMatch(t, []string{"n1", "n2"}, ids)
}

func TestNodeStore_ListByGroups_NoMatch(t *testing.T) {
	s := openNodeTestDB(t)
	insertNode(t, s, "n1", `["web"]`)

	ids, err := s.ListByGroups(context.Background(), []string{"nonexistent"})
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestNodeStore_ListByGroups_NoDuplicates(t *testing.T) {
	s := openNodeTestDB(t)
	insertNode(t, s, "n1", `["web","db"]`)

	ids, err := s.ListByGroups(context.Background(), []string{"web", "db"})
	require.NoError(t, err)
	assert.Len(t, ids, 1)
	assert.Equal(t, []string{"n1"}, ids)
}

func TestNodeStore_ListByGroups_EmptyGroupsColumn(t *testing.T) {
	s := openNodeTestDB(t)
	insertNode(t, s, "n1", `[]`)

	ids, err := s.ListByGroups(context.Background(), []string{"web"})
	require.NoError(t, err)
	assert.Empty(t, ids)
}
