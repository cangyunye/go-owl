package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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
	auth.GET("/shortcuts/export", h.Export)
	auth.POST("/shortcuts", h.Create)
	auth.POST("/shortcuts/import", h.Import)
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

func TestShortcuts_Export(t *testing.T) {
	router, us := shortcutTestRouter(t)
	shortcutUser(t, us, "alice", "operator")
	token := testToken(t, "alice", "operator")

	sendShortcutRequest(t, router, "POST", "/api/v1/shortcuts", token,
		map[string]string{"name": "磁盘", "command": "df -h"})
	sendShortcutRequest(t, router, "POST", "/api/v1/shortcuts", token,
		map[string]string{"name": "内存", "command": "free -h"})

	w := sendShortcutRequest(t, router, "GET", "/api/v1/shortcuts/export", token, nil)
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Header().Get("Content-Disposition"), "attachment")
	require.Contains(t, w.Header().Get("Content-Type"), "application/json")

	var body struct {
		Version   string `json:"version"`
		Shortcuts []struct {
			Name    string `json:"name"`
			Command string `json:"command"`
		} `json:"shortcuts"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "1.0", body.Version)
	require.Len(t, body.Shortcuts, 2)
	assert.Equal(t, "磁盘", body.Shortcuts[0].Name)
	assert.Equal(t, "df -h", body.Shortcuts[0].Command)
	assert.Equal(t, "内存", body.Shortcuts[1].Name)
}

func TestShortcuts_Import_MergeDedup(t *testing.T) {
	router, us := shortcutTestRouter(t)
	shortcutUser(t, us, "alice", "operator")
	token := testToken(t, "alice", "operator")

	// 已有「磁盘」；导入文件里磁盘(同名→跳过) + 网络(新→追加)
	sendShortcutRequest(t, router, "POST", "/api/v1/shortcuts", token,
		map[string]string{"name": "磁盘", "command": "df -h"})

	w := sendShortcutRequest(t, router, "POST", "/api/v1/shortcuts/import", token,
		map[string]interface{}{
			"shortcuts": []map[string]string{
				{"name": "磁盘", "command": "df -hT"},
				{"name": "网络", "command": "ss -tulnp"},
			},
		})
	require.Equal(t, 200, w.Code)
	var res struct {
		Created int `json:"created"`
		Skipped int `json:"skipped"`
		Failed  int `json:"failed"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.Equal(t, 1, res.Created)
	assert.Equal(t, 1, res.Skipped)
	assert.Equal(t, 0, res.Failed)

	// 同名跳过 = 现有命令不被覆盖；新条目追加在末尾
	w = sendShortcutRequest(t, router, "GET", "/api/v1/shortcuts", token, nil)
	var list struct {
		Data []model.UserCommand `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list.Data, 2)
	assert.Equal(t, "磁盘", list.Data[0].Name)
	assert.Equal(t, "df -h", list.Data[0].Command)
	assert.Equal(t, "网络", list.Data[1].Name)
}

func TestShortcuts_Import_Idempotent(t *testing.T) {
	router, us := shortcutTestRouter(t)
	shortcutUser(t, us, "alice", "operator")
	token := testToken(t, "alice", "operator")

	payload := map[string]interface{}{
		"shortcuts": []map[string]string{
			{"name": "磁盘", "command": "df -h"},
			{"name": "内存", "command": "free -h"},
		},
	}
	sendShortcutRequest(t, router, "POST", "/api/v1/shortcuts/import", token, payload)
	w := sendShortcutRequest(t, router, "POST", "/api/v1/shortcuts/import", token, payload)
	var res struct {
		Created int `json:"created"`
		Skipped int `json:"skipped"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.Equal(t, 0, res.Created)
	assert.Equal(t, 2, res.Skipped)

	w = sendShortcutRequest(t, router, "GET", "/api/v1/shortcuts", token, nil)
	var list struct {
		Data []model.UserCommand `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	assert.Len(t, list.Data, 2)
}

func TestShortcuts_Import_Validation(t *testing.T) {
	router, us := shortcutTestRouter(t)
	shortcutUser(t, us, "alice", "operator")
	token := testToken(t, "alice", "operator")

	// 空 body / 缺字段 → 400
	w := sendShortcutRequest(t, router, "POST", "/api/v1/shortcuts/import", token, nil)
	assert.Equal(t, 400, w.Code)

	// 空字段条目计入 failed，不中断导入
	w = sendShortcutRequest(t, router, "POST", "/api/v1/shortcuts/import", token,
		map[string]interface{}{
			"shortcuts": []map[string]string{
				{"name": "", "command": "df -h"},
				{"name": "内存", "command": ""},
				{"name": "网络", "command": "ss -tulnp"},
			},
		})
	require.Equal(t, 200, w.Code)
	var res struct {
		Created int      `json:"created"`
		Skipped int      `json:"skipped"`
		Failed  int      `json:"failed"`
		Errors  []string `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.Equal(t, 1, res.Created)
	assert.Equal(t, 2, res.Failed)
	assert.Len(t, res.Errors, 2)

	// 超过条目上限 → 400
	many := make([]map[string]string, 501)
	for i := range many {
		many[i] = map[string]string{"name": fmt.Sprintf("c%d", i), "command": "x"}
	}
	w = sendShortcutRequest(t, router, "POST", "/api/v1/shortcuts/import", token,
		map[string]interface{}{"shortcuts": many})
	assert.Equal(t, 400, w.Code)
}

func TestShortcuts_ImportExport_RequireAuth(t *testing.T) {
	router, _ := shortcutTestRouter(t)
	w := sendShortcutRequest(t, router, "GET", "/api/v1/shortcuts/export", "", nil)
	assert.Equal(t, 401, w.Code)
	w = sendShortcutRequest(t, router, "POST", "/api/v1/shortcuts/import", "",
		map[string]interface{}{"shortcuts": []map[string]string{}})
	assert.Equal(t, 401, w.Code)
}
