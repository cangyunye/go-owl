package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

func shortcutTestRouter(t *testing.T) (*gin.Engine, *store.UserStore) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	us := store.NewUserStore(db)
	require.NoError(t, us.Init(context.Background()))
	cs := store.NewCommandStore(db)
	require.NoError(t, cs.Init(context.Background()))

	ah := NewAuthHandler(us, service.NewAuthService("test-secret-32byte-long-string!!"))
	h := NewShortcutHandler(cs, us)

	r := gin.New()
	auth := r.Group("/api/v1")
	auth.Use(ah.AuthMiddleware())
	auth.GET("/shortcuts", h.List)
	auth.POST("/shortcuts", h.Create)
	auth.PUT("/shortcuts/reorder", h.Reorder)
	auth.PUT("/shortcuts/:id", h.Update)
	auth.DELETE("/shortcuts/:id", h.Delete)
	return r, us
}

func shortcutUser(t *testing.T, us *store.UserStore, username, role string) {
	t.Helper()
	require.NoError(t, us.Create(context.Background(),
		&model.User{Username: username, PasswordHash: "hash", Role: model.Role(role)}))
}

func sendShortcutRequest(t *testing.T, router *gin.Engine, method, path, token string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req, _ := http.NewRequest(method, path, &buf)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestShortcuts_CrudAndReorder(t *testing.T) {
	router, us := shortcutTestRouter(t)
	shortcutUser(t, us, "alice", "operator")
	token := testToken(t, "alice", "operator")

	w := sendShortcutRequest(t, router, "POST", "/api/v1/shortcuts", token,
		map[string]string{"name": "磁盘", "command": "df -h"})
	assert.Equal(t, 201, w.Code)
	var c1 model.UserCommand
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &c1))

	w = sendShortcutRequest(t, router, "POST", "/api/v1/shortcuts", token,
		map[string]string{"name": "内存", "command": "free -h"})
	var c2 model.UserCommand
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &c2))

	w = sendShortcutRequest(t, router, "GET", "/api/v1/shortcuts", token, nil)
	assert.Equal(t, 200, w.Code)
	var list struct {
		Data []model.UserCommand `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list.Data, 2)
	assert.Equal(t, "磁盘", list.Data[0].Name)

	w = sendShortcutRequest(t, router, "PUT", "/api/v1/shortcuts/reorder", token,
		map[string][]int64{"ordered_ids": {c2.ID, c1.ID}})
	assert.Equal(t, 200, w.Code)

	w = sendShortcutRequest(t, router, "GET", "/api/v1/shortcuts", token, nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	assert.Equal(t, []string{"内存", "磁盘"}, []string{list.Data[0].Name, list.Data[1].Name})

	w = sendShortcutRequest(t, router, "PUT", "/api/v1/shortcuts/"+itoa(c1.ID), token,
		map[string]string{"name": "磁盘占用", "command": "df -h"})
	assert.Equal(t, 200, w.Code)

	w = sendShortcutRequest(t, router, "DELETE", "/api/v1/shortcuts/"+itoa(c1.ID), token, nil)
	assert.Equal(t, 200, w.Code)

	w = sendShortcutRequest(t, router, "GET", "/api/v1/shortcuts", token, nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	assert.Len(t, list.Data, 1)
}

func TestShortcuts_RequireAuth(t *testing.T) {
	router, _ := shortcutTestRouter(t)
	w := sendShortcutRequest(t, router, "GET", "/api/v1/shortcuts", "", nil)
	assert.Equal(t, 401, w.Code)
}

func TestShortcuts_OwnershipIsolation(t *testing.T) {
	router, us := shortcutTestRouter(t)
	shortcutUser(t, us, "alice", "operator")
	shortcutUser(t, us, "mallory", "viewer")
	alice := testToken(t, "alice", "operator")
	mallory := testToken(t, "mallory", "viewer")

	w := sendShortcutRequest(t, router, "POST", "/api/v1/shortcuts", alice,
		map[string]string{"name": "秘密", "command": "echo hi"})
	var c1 model.UserCommand
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &c1))

	// mallory 看不到 alice 的
	w = sendShortcutRequest(t, router, "GET", "/api/v1/shortcuts", mallory, nil)
	var list struct {
		Data []model.UserCommand `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	assert.Empty(t, list.Data)

	// mallory 改/删 alice 的 → 404(不暴露存在性)
	w = sendShortcutRequest(t, router, "PUT", "/api/v1/shortcuts/"+itoa(c1.ID), mallory,
		map[string]string{"name": "x", "command": "x"})
	assert.Equal(t, 404, w.Code)
	w = sendShortcutRequest(t, router, "DELETE", "/api/v1/shortcuts/"+itoa(c1.ID), mallory, nil)
	assert.Equal(t, 404, w.Code)
}

func TestShortcuts_Validation(t *testing.T) {
	router, us := shortcutTestRouter(t)
	shortcutUser(t, us, "alice", "operator")
	token := testToken(t, "alice", "operator")

	w := sendShortcutRequest(t, router, "POST", "/api/v1/shortcuts", token,
		map[string]string{"name": "", "command": "df -h"})
	assert.Equal(t, 400, w.Code)
	w = sendShortcutRequest(t, router, "POST", "/api/v1/shortcuts", token,
		map[string]string{"name": "x", "command": ""})
	assert.Equal(t, 400, w.Code)
}
