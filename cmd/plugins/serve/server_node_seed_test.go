package serve

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestServer_SeedRequiresAdmin 覆盖生产路由表：/nodes/seed 是批量造数工具，
// 只有 admin 可调用（此前放在 writer 组，editor 也能注入 50 条 mock 节点）。
func TestServer_SeedRequiresAdmin(t *testing.T) {
	srv, adminToken := setupMonitorServer(t)

	w := authedPost(t, srv, adminToken, "/api/v1/users", map[string]string{
		"username": "ed", "password": "pass1234", "role": "editor",
	})
	require.Equal(t, http.StatusCreated, w.Code)
	edToken := login(t, srv, "ed", "pass1234")

	assert.Equal(t, http.StatusForbidden, authedPost(t, srv, edToken, "/api/v1/nodes/seed", nil).Code,
		"editor 不得调用 seed")
	assert.Equal(t, http.StatusOK, authedPost(t, srv, adminToken, "/api/v1/nodes/seed", nil).Code,
		"admin 可调用 seed")

	// editor 仍可做常规单条写入（权限未被整体收紧）
	assert.Equal(t, http.StatusCreated, authedPost(t, srv, edToken, "/api/v1/nodes", map[string]string{
		"id": "ed-node", "address": "10.0.9.9", "user": "root",
	}).Code)
}
