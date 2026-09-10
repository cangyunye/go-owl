package serve

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestServer_RoleChangeRevokesOldToken 覆盖生产接线：Server.Init 启用撤销校验后，
// 管理员改角色会使该用户已签发的 token 立即失效（修复前旧 token 在 24h 内仍按
// 原角色放行）。
func TestServer_RoleChangeRevokesOldToken(t *testing.T) {
	srv, adminToken := setupMonitorServer(t)

	w := authedPost(t, srv, adminToken, "/api/v1/users", map[string]string{
		"username": "carol", "password": "pass1234", "role": "viewer",
	})
	require.Equal(t, http.StatusCreated, w.Code)
	var created struct {
		ID int64 `json:"id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))

	userToken := login(t, srv, "carol", "pass1234")
	require.Equal(t, http.StatusOK, authedGet(t, srv, userToken, "/api/v1/nodes").Code)

	upd := authedPut(t, srv, adminToken, fmt.Sprintf("/api/v1/users/%d", created.ID), map[string]string{"role": "editor"})
	require.Equal(t, http.StatusOK, upd.Code)

	assert.Equal(t, http.StatusUnauthorized, authedGet(t, srv, userToken, "/api/v1/nodes").Code,
		"改角色后旧 token 必须失效")

	// 重新登录需跨过签发时间与撤销时间同秒的边界（IssuedAt 为秒级精度）
	time.Sleep(1100 * time.Millisecond)
	fresh := login(t, srv, "carol", "pass1234")
	assert.Equal(t, http.StatusOK, authedGet(t, srv, fresh, "/api/v1/nodes").Code,
		"重新登录后应恢复访问")
}

// TestServer_DeleteUserRevokesOldToken 删除账号后其旧 token 立即失效。
func TestServer_DeleteUserRevokesOldToken(t *testing.T) {
	srv, adminToken := setupMonitorServer(t)

	w := authedPost(t, srv, adminToken, "/api/v1/users", map[string]string{
		"username": "dave", "password": "pass1234", "role": "viewer",
	})
	require.Equal(t, http.StatusCreated, w.Code)
	var created struct {
		ID int64 `json:"id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))

	userToken := login(t, srv, "dave", "pass1234")
	require.Equal(t, http.StatusOK, authedGet(t, srv, userToken, "/api/v1/nodes").Code)

	rec := authedDelete(t, srv, adminToken, fmt.Sprintf("/api/v1/users/%d", created.ID))
	require.Equal(t, http.StatusOK, rec.Code)

	assert.Equal(t, http.StatusUnauthorized, authedGet(t, srv, userToken, "/api/v1/nodes").Code,
		"删除账号后旧 token 必须失效")
}
