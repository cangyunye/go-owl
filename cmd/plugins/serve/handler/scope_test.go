package handler

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
)

func scopeTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY, name TEXT, address TEXT, port INTEGER DEFAULT 22,
		user TEXT, password TEXT, ssh_key TEXT, status TEXT DEFAULT 'unknown',
		groups TEXT DEFAULT '[]', labels TEXT DEFAULT '{}',
		proxy_jump TEXT DEFAULT '', created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP, last_check_at DATETIME)`)
	require.NoError(t, err)
	for _, n := range []struct{ id, name, groups string }{
		{"n-web1", "web-01", `["web","prod"]`},
		{"n-web2", "web-02", `["web","staging"]`},
		{"n-db1", "db-01", `["db","prod"]`},
	} {
		_, err := db.Exec(`INSERT INTO nodes (id, name, groups) VALUES (?,?,?)`, n.id, n.name, n.groups)
		require.NoError(t, err)
	}
	users := store.NewUserStore(db)
	require.NoError(t, users.Init(context.Background()))
	return db
}

func TestParseNodeScope(t *testing.T) {
	require.Nil(t, ParseNodeScope(""), "空=不限")
	require.Nil(t, ParseNodeScope("  "))
	ns := ParseNodeScope(`{"groups":["web"],"nodes":["n-db1"]}`)
	require.NotNil(t, ns)
	require.Equal(t, []string{"web"}, ns.Groups)
	require.Equal(t, []string{"n-db1"}, ns.Nodes)
	require.False(t, ns.IsUnrestricted())
	// 全部为空的 JSON 视为不限
	require.Nil(t, ParseNodeScope(`{"groups":[],"nodes":[]}`))
}

func TestScopeChecker_FilterNodeIDs(t *testing.T) {
	db := scopeTestDB(t)
	users := store.NewUserStore(db)
	ctx := context.Background()

	require.NoError(t, users.Create(ctx, &model.User{
		Username: "limited", PasswordHash: "x", Role: model.RoleOperator,
		NodeScope: `{"groups":["web"],"nodes":["n-db1"]}`,
	}))
	require.NoError(t, users.Create(ctx, &model.User{
		Username: "adminuser", PasswordHash: "x", Role: model.RoleAdmin,
		NodeScope: `{"groups":["web"]}`, // admin 恒不限
	}))

	sc := NewScopeChecker(db)

	// 受限用户：组内保留 + 显式节点保留，其余剔除
	got := sc.FilterNodeIDs(ctx, "limited", []string{"n-web1", "n-web2", "n-db1", "n-unknown"})
	require.Equal(t, []string{"n-web1", "n-web2", "n-db1"}, got)

	// admin：scope 不生效
	got = sc.FilterNodeIDs(ctx, "adminuser", []string{"n-db1"})
	require.Equal(t, []string{"n-db1"}, got)

	// 无 scope 用户：不限
	require.NoError(t, users.Create(ctx, &model.User{Username: "free", PasswordHash: "x", Role: model.RoleOperator}))
	got = sc.FilterNodeIDs(ctx, "free", []string{"n-db1"})
	require.Equal(t, []string{"n-db1"}, got)

	// 未知用户（不存在的库用户）：保守不限（登录 token 已由中间件验证）
	got = sc.FilterNodeIDs(ctx, "ghost", []string{"n-db1"})
	require.Equal(t, []string{"n-db1"}, got)
}

func TestUserScope_RoundTrip(t *testing.T) {
	db := scopeTestDB(t)
	users := store.NewUserStore(db)
	ctx := context.Background()

	u := &model.User{Username: "scoped", PasswordHash: "x", Role: model.RoleOperator,
		NodeScope: `{"groups":["web"]}`}
	require.NoError(t, users.Create(ctx, u))

	got, err := users.FindByUsername(ctx, "scoped")
	require.NoError(t, err)
	require.Equal(t, `{"groups":["web"]}`, got.NodeScope)

	got.NodeScope = `{"nodes":["n-db1"]}`
	require.NoError(t, users.Update(ctx, got))
	again, err := users.FindByUsername(ctx, "scoped")
	require.NoError(t, err)
	require.Equal(t, `{"nodes":["n-db1"]}`, again.NodeScope)
}
