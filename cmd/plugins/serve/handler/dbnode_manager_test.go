package handler

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// serve 端 AI agent 原挂空内存节点管理器（server.go 曾传 NewInMemoryNodeStore
// 且从不加载），本地降级链的节点名/分组名参数提取拿不到数据——
// 「查询db的主机」等语料在 serve 端因 groupNames 为空而列出全部节点。
// DBNodeManager 以 nodes 表为数据源，List/GetByGroup 必须反映 DB 实况。

func dbNodeManagerSetup(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE nodes (
		id TEXT PRIMARY KEY, name TEXT, address TEXT, port INTEGER DEFAULT 22,
		user TEXT, password TEXT, ssh_key TEXT, status TEXT DEFAULT 'unknown',
		groups TEXT DEFAULT '[]', labels TEXT DEFAULT '{}',
		proxy_jump TEXT DEFAULT '', created_at TIMESTAMP, updated_at TIMESTAMP)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO nodes (id, name, address, status, groups) VALUES
		('node-1', 'web-01', '10.0.0.1', 'online', '["web","prod"]'),
		('node-2', 'db-01', '10.0.0.2', 'online', '["db"]'),
		('node-3', 'cache-01', '10.0.0.3', 'offline', '["cache"]')`)
	require.NoError(t, err)
	return db
}

func TestDBNodeManager_ListReflectsDB(t *testing.T) {
	db := dbNodeManagerSetup(t)
	m := NewDBNodeManager(db)

	nodes := m.List()
	require.Len(t, nodes, 3)

	byName := map[string]int{}
	for _, n := range nodes {
		byName[n.Name] = 1
		if n.Name == "cache-01" {
			require.Equal(t, "offline", string(n.Status), "DB status must be carried, not reset to online")
			require.Equal(t, []string{"cache"}, n.Groups)
		}
		if n.Name == "web-01" {
			require.ElementsMatch(t, []string{"web", "prod"}, n.Groups)
		}
	}
	require.Contains(t, byName, "web-01")
	require.Contains(t, byName, "db-01")
	require.Contains(t, byName, "cache-01")
}

func TestDBNodeManager_GetByGroup(t *testing.T) {
	db := dbNodeManagerSetup(t)
	m := NewDBNodeManager(db)

	web := m.GetByGroup("web")
	require.Len(t, web, 1)
	require.Equal(t, "web-01", web[0].Name)
	require.Empty(t, m.GetByGroup("nosuch"))
}

func TestDBNodeManager_ListPicksUpInserts(t *testing.T) {
	db := dbNodeManagerSetup(t)
	m := NewDBNodeManager(db)
	require.Len(t, m.List(), 3)

	_, err := db.Exec(`INSERT INTO nodes (id, name, address, status, groups) VALUES
		('node-4', 'worker-01', '10.0.0.4', 'online', '["worker"]')`)
	require.NoError(t, err)

	nodes := m.List()
	require.Len(t, nodes, 4, "List must re-sync from DB on each call")

	_, err = db.Exec(`DELETE FROM nodes WHERE id = 'node-1'`)
	require.NoError(t, err)
	require.Len(t, m.List(), 3, "removed nodes must disappear from List")
}
