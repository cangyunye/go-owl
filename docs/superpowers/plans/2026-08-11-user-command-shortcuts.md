# 用户级快捷命令 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在「命令执行」页为每个用户提供可增/删/改/拖拽排序的快捷命令,新用户建号时经 Hook 触发层自动获得 3 条默认指令。

**Architecture:** 新表 `user_commands`(user_id 外键级联删除)+ `CommandStore` + `ShortcutHandler`(REST /api/v1/shortcuts,当前用户经 JWT username 反查 id)。新建用户路径(`UserHandler.Create` 与首跑 `ensureAdmin`)统一走 `UserCreatedHook` 注册表播种默认指令(见 ADR-0001)。前端 exec.js 渲染横向 chip 条,点击填入命令输入框,HTML5 拖拽排序。

**Tech Stack:** Go 1.26 / gin / modernc.org/sqlite / 原生 ES module 前端(无框架)/ HTML5 drag & drop。

## Global Constraints

- 表名 `user_commands`;字段 `id/user_id/name/command/position/created_at/updated_at` 一字不差。
- 默认指令仅 3 条:`磁盘占用`=`df -h`、`我的进程`=`ps -fu $LOGNAME`、`内存`=`free -h`。`$LOGNAME` 原样存储、前端不替换。
- 权限:所有已登录用户可管理自己的快捷命令(reader 级);`/exec` 执行权限仍 operator+ 不变。
- 名称与命令必填非空;允许重名。
- 归属隔离:写操作必须 `WHERE id=? AND user_id=?`;越权返回 404(不暴露存在性)。
- Hook 只在建号时触发、失败仅记日志不阻塞建号;`ResetAdmin` 不触发 Hook。
- 测试命令:`go test ./...`(serve 模块:`cd cmd/plugins/serve && go test ./...`);前端语法 `node --check web/js/pages/exec.js`。
- 复用现有测试辅助:`openTestDB`(store 包)、`testToken`(handler 包)、`srv.Init()` 模式(serve 包)。

---

### Task 1: CommandStore(model + store + 单测)

**Files:**
- Create: `cmd/plugins/serve/model/command.go`
- Create: `cmd/plugins/serve/store/command_store.go`
- Create: `cmd/plugins/serve/store/command_store_test.go`

**Interfaces:**
- Consumes: `model.User`(已存在)、`store.NewUserStore`(已存在)。
- Produces: `model.UserCommand{ID,UserID,Name,Command,Position int64/string/int}`;`store.CommandStore` 的方法 `Init/ListByUser/Create/Update/Delete/Reorder/CountByUser`(签名见 Task 3/4 使用)。

- [ ] **Step 1: 写 model**

`cmd/plugins/serve/model/command.go`:

```go
package model

// UserCommand 用户级快捷命令:用户拥有、命名过的命令模板(见 CONTEXT.md Shortcut Command)。
type UserCommand struct {
	ID       int64  `json:"id"`
	UserID   int64  `json:"user_id"`
	Name     string `json:"name"`
	Command  string `json:"command"`
	Position int    `json:"position"`
}
```

- [ ] **Step 2: 写失败单测**

`cmd/plugins/serve/store/command_store_test.go`:

```go
package store

import (
	"context"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandStore_CreateAppendsPosition(t *testing.T) {
	db := openTestDB(t)
	s := NewCommandStore(db)
	ctx := context.Background()
	require.NoError(t, s.Init(ctx))

	c1 := &model.UserCommand{UserID: 1, Name: "磁盘", Command: "df -h"}
	require.NoError(t, s.Create(ctx, c1))
	assert.Equal(t, 0, c1.Position)

	c2 := &model.UserCommand{UserID: 1, Name: "内存", Command: "free -h"}
	require.NoError(t, s.Create(ctx, c2))
	assert.Equal(t, 1, c2.Position)

	c3 := &model.UserCommand{UserID: 2, Name: "x", Command: "echo x"}
	require.NoError(t, s.Create(ctx, c3))
	assert.Equal(t, 0, c3.Position, "position 按用户独立计数")
}

func TestCommandStore_ListByUser_OrdersByPosition(t *testing.T) {
	db := openTestDB(t)
	s := NewCommandStore(db)
	ctx := context.Background()
	require.NoError(t, s.Init(ctx))

	require.NoError(t, s.Create(ctx, &model.UserCommand{UserID: 1, Name: "a", Command: "echo a"}))
	require.NoError(t, s.Create(ctx, &model.UserCommand{UserID: 2, Name: "other", Command: "echo o"}))
	require.NoError(t, s.Create(ctx, &model.UserCommand{UserID: 1, Name: "c", Command: "echo c"}))

	list, err := s.ListByUser(ctx, 1)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, []string{"a", "c"}, []string{list[0].Name, list[1].Name})
}

func TestCommandStore_UpdateDelete_RespectOwnership(t *testing.T) {
	db := openTestDB(t)
	s := NewCommandStore(db)
	ctx := context.Background()
	require.NoError(t, s.Init(ctx))

	cmd := &model.UserCommand{UserID: 1, Name: "a", Command: "echo a"}
	require.NoError(t, s.Create(ctx, cmd))

	// 其他用户改不了
	affected, err := s.Update(ctx, &model.UserCommand{ID: cmd.ID, UserID: 2, Name: "hack", Command: "hack"})
	require.NoError(t, err)
	assert.Equal(t, int64(0), affected)
	affected, err = s.Delete(ctx, cmd.ID, 2)
	require.NoError(t, err)
	assert.Equal(t, int64(0), affected)
	list, _ := s.ListByUser(ctx, 1)
	require.Len(t, list, 1)
	assert.Equal(t, "a", list[0].Name)

	// 本人可改可删
	affected, err = s.Update(ctx, &model.UserCommand{ID: cmd.ID, UserID: 1, Name: "改名", Command: "echo b"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), affected)
	list, _ = s.ListByUser(ctx, 1)
	require.Len(t, list, 1)
	assert.Equal(t, "改名", list[0].Name)

	affected, err = s.Delete(ctx, cmd.ID, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(1), affected)
	list, _ = s.ListByUser(ctx, 1)
	assert.Empty(t, list)
}

func TestCommandStore_Reorder(t *testing.T) {
	db := openTestDB(t)
	s := NewCommandStore(db)
	ctx := context.Background()
	require.NoError(t, s.Init(ctx))

	c1 := &model.UserCommand{UserID: 1, Name: "1", Command: "c1"}
	c2 := &model.UserCommand{UserID: 1, Name: "2", Command: "c2"}
	c3 := &model.UserCommand{UserID: 1, Name: "3", Command: "c3"}
	for _, c := range []*model.UserCommand{c1, c2, c3} {
		require.NoError(t, s.Create(ctx, c))
	}

	require.NoError(t, s.Reorder(ctx, 1, []int64{c3.ID, c1.ID, c2.ID}))
	list, err := s.ListByUser(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, []string{"3", "1", "2"}, []string{list[0].Name, list[1].Name, list[2].Name})
}

func TestCommandStore_CountByUser(t *testing.T) {
	db := openTestDB(t)
	s := NewCommandStore(db)
	ctx := context.Background()
	require.NoError(t, s.Init(ctx))

	n, err := s.CountByUser(ctx, 7)
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	require.NoError(t, s.Create(ctx, &model.UserCommand{UserID: 7, Name: "a", Command: "echo a"}))
	n, _ = s.CountByUser(ctx, 7)
	assert.Equal(t, 1, n)
}

func TestCommandStore_CascadeOnUserDelete(t *testing.T) {
	db := openTestDB(t)
	_, err := db.Exec("PRAGMA foreign_keys=ON")
	require.NoError(t, err)
	ctx := context.Background()

	us := NewUserStore(db)
	require.NoError(t, us.Init(ctx))
	user := &model.User{Username: "u1", PasswordHash: "h", Role: "viewer"}
	require.NoError(t, us.Create(ctx, user))

	s := NewCommandStore(db)
	require.NoError(t, s.Init(ctx))
	require.NoError(t, s.Create(ctx, &model.UserCommand{UserID: user.ID, Name: "a", Command: "echo a"}))

	require.NoError(t, us.Delete(ctx, user.ID))
	n, err := s.CountByUser(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}
```

- [ ] **Step 3: 跑测试确认失败**

Run: `cd cmd/plugins/serve && go test ./store/ -run TestCommandStore -count=1`
Expected: 编译失败 `undefined: NewCommandStore`。

- [ ] **Step 4: 写 store 实现**

`cmd/plugins/serve/store/command_store.go`:

```go
package store

import (
	"context"
	"database/sql"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
)

// CommandStore 管理用户级快捷命令(user_commands 表)。
type CommandStore struct {
	db *sql.DB
}

func NewCommandStore(db *sql.DB) *CommandStore {
	return &CommandStore{db: db}
}

func (s *CommandStore) Init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS user_commands (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id    INTEGER NOT NULL REFERENCES web_users(id) ON DELETE CASCADE,
			name       TEXT NOT NULL,
			command    TEXT NOT NULL,
			position   INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_user_commands_user ON user_commands(user_id, position);
	`)
	return err
}

func (s *CommandStore) ListByUser(ctx context.Context, userID int64) ([]*model.UserCommand, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, name, command, position FROM user_commands WHERE user_id = ? ORDER BY position, id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*model.UserCommand, 0)
	for rows.Next() {
		cmd := &model.UserCommand{}
		if err := rows.Scan(&cmd.ID, &cmd.UserID, &cmd.Name, &cmd.Command, &cmd.Position); err != nil {
			return nil, err
		}
		list = append(list, cmd)
	}
	return list, rows.Err()
}

func (s *CommandStore) Create(ctx context.Context, cmd *model.UserCommand) error {
	var max int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(position), -1) FROM user_commands WHERE user_id = ?`, cmd.UserID).Scan(&max)
	cmd.Position = max + 1

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO user_commands (user_id, name, command, position) VALUES (?, ?, ?, ?)`,
		cmd.UserID, cmd.Name, cmd.Command, cmd.Position)
	if err != nil {
		return err
	}
	cmd.ID, err = result.LastInsertId()
	return err
}

func (s *CommandStore) Update(ctx context.Context, cmd *model.UserCommand) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE user_commands SET name = ?, command = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ?`,
		cmd.Name, cmd.Command, cmd.ID, cmd.UserID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *CommandStore) Delete(ctx context.Context, id, userID int64) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM user_commands WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *CommandStore) Reorder(ctx context.Context, userID int64, orderedIDs []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, id := range orderedIDs {
		if _, err := tx.ExecContext(ctx,
			`UPDATE user_commands SET position = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ?`,
			i, id, userID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *CommandStore) CountByUser(ctx context.Context, userID int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_commands WHERE user_id = ?`, userID).Scan(&n)
	return n, err
}
```

- [ ] **Step 5: 跑测试确认通过**

Run: `cd cmd/plugins/serve && go test ./store/ -run TestCommandStore -count=1`
Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add cmd/plugins/serve/model/command.go cmd/plugins/serve/store/command_store.go cmd/plugins/serve/store/command_store_test.go
git commit -m "feat(store): CommandStore 用户级快捷命令 CRUD/排序/级联删除"
```

---

### Task 2: ShortcutHandler(REST API + 单测)

**Files:**
- Create: `cmd/plugins/serve/handler/command.go`
- Create: `cmd/plugins/serve/handler/command_test.go`

**Interfaces:**
- Consumes: `store.CommandStore`(Task 1)、`store.UserStore`、`service.Claims`、`testToken`(rbac_test.go)。
- Produces: `handler.ShortcutHandler`(NewShortcutHandler + List/Create/Update/Delete/Reorder)。

- [ ] **Step 1: 写失败单测**

`cmd/plugins/serve/handler/command_test.go`:

```go
package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func shortcutRequest(t *testing.T, router *gin.Engine, method, path, token string, body interface{}) *httptest.ResponseRecorder {
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

	w := shortcutRequest(t, router, "POST", "/api/v1/shortcuts", token,
		map[string]string{"name": "磁盘", "command": "df -h"})
	assert.Equal(t, 201, w.Code)
	var c1 model.UserCommand
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &c1))

	w = shortcutRequest(t, router, "POST", "/api/v1/shortcuts", token,
		map[string]string{"name": "内存", "command": "free -h"})
	var c2 model.UserCommand
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &c2))

	w = shortcutRequest(t, router, "GET", "/api/v1/shortcuts", token, nil)
	assert.Equal(t, 200, w.Code)
	var list struct {
		Data []model.UserCommand `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list.Data, 2)
	assert.Equal(t, "磁盘", list.Data[0].Name)

	w = shortcutRequest(t, router, "PUT", "/api/v1/shortcuts/reorder", token,
		map[string][]int64{"ordered_ids": {c2.ID, c1.ID}})
	assert.Equal(t, 200, w.Code)

	w = shortcutRequest(t, router, "GET", "/api/v1/shortcuts", token, nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	assert.Equal(t, []string{"内存", "磁盘"}, []string{list.Data[0].Name, list.Data[1].Name})

	w = shortcutRequest(t, router, "PUT", "/api/v1/shortcuts/"+itoa(c1.ID), token,
		map[string]string{"name": "磁盘占用", "command": "df -h"})
	assert.Equal(t, 200, w.Code)

	w = shortcutRequest(t, router, "DELETE", "/api/v1/shortcuts/"+itoa(c1.ID), token, nil)
	assert.Equal(t, 200, w.Code)

	w = shortcutRequest(t, router, "GET", "/api/v1/shortcuts", token, nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	assert.Len(t, list.Data, 1)
}

func TestShortcuts_RequireAuth(t *testing.T) {
	router, _ := shortcutTestRouter(t)
	w := shortcutRequest(t, router, "GET", "/api/v1/shortcuts", "", nil)
	assert.Equal(t, 401, w.Code)
}

func TestShortcuts_OwnershipIsolation(t *testing.T) {
	router, us := shortcutTestRouter(t)
	shortcutUser(t, us, "alice", "operator")
	shortcutUser(t, us, "mallory", "viewer")
	alice := testToken(t, "alice", "operator")
	mallory := testToken(t, "mallory", "viewer")

	w := shortcutRequest(t, router, "POST", "/api/v1/shortcuts", alice,
		map[string]string{"name": "秘密", "command": "echo hi"})
	var c1 model.UserCommand
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &c1))

	// mallory 看不到 alice 的
	w = shortcutRequest(t, router, "GET", "/api/v1/shortcuts", mallory, nil)
	var list struct {
		Data []model.UserCommand `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	assert.Empty(t, list.Data)

	// mallory 改/删 alice 的 → 404(不暴露存在性)
	w = shortcutRequest(t, router, "PUT", "/api/v1/shortcuts/"+itoa(c1.ID), mallory,
		map[string]string{"name": "x", "command": "x"})
	assert.Equal(t, 404, w.Code)
	w = shortcutRequest(t, router, "DELETE", "/api/v1/shortcuts/"+itoa(c1.ID), mallory, nil)
	assert.Equal(t, 404, w.Code)
}

func TestShortcuts_Validation(t *testing.T) {
	router, us := shortcutTestRouter(t)
	shortcutUser(t, us, "alice", "operator")
	token := testToken(t, "alice", "operator")

	w := shortcutRequest(t, router, "POST", "/api/v1/shortcuts", token,
		map[string]string{"name": "", "command": "df -h"})
	assert.Equal(t, 400, w.Code)
	w = shortcutRequest(t, router, "POST", "/api/v1/shortcuts", token,
		map[string]string{"name": "x", "command": ""})
	assert.Equal(t, 400, w.Code)
}
```

(在 `command_test.go` 顶部加入小工具函数;若 `itoa` 已存在于 handler 包测试中则直接复用,否则新增:)

```go
func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd cmd/plugins/serve && go test ./handler/ -run TestShortcuts -count=1`
Expected: 编译失败 `undefined: NewShortcutHandler`。

- [ ] **Step 3: 写 handler 实现**

`cmd/plugins/serve/handler/command.go`:

```go
package handler

import (
	"net/http"
	"strconv"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
)

// ShortcutHandler 管理用户级快捷命令(/api/v1/shortcuts)。
type ShortcutHandler struct {
	commands *store.CommandStore
	users    *store.UserStore
}

func NewShortcutHandler(commands *store.CommandStore, users *store.UserStore) *ShortcutHandler {
	return &ShortcutHandler{commands: commands, users: users}
}

// currentUserID 从 JWT claims.username 反查当前用户 ID。
func (h *ShortcutHandler) currentUserID(c *gin.Context) (int64, bool) {
	claims, ok := c.Get("claims")
	if !ok {
		return 0, false
	}
	user, err := h.users.FindByUsername(c.Request.Context(), claims.(*service.Claims).Username)
	if err != nil {
		return 0, false
	}
	return user.ID, true
}

type shortcutRequest struct {
	Name    string `json:"name" binding:"required"`
	Command string `json:"command" binding:"required"`
}

func (h *ShortcutHandler) List(c *gin.Context) {
	userID, ok := h.currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "unauthorized"})
		return
	}
	list, err := h.commands.ListByUser(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "query failed"})
		return
	}
	if list == nil {
		list = []*model.UserCommand{}
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *ShortcutHandler) Create(c *gin.Context) {
	userID, ok := h.currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "unauthorized"})
		return
	}
	var req shortcutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "name and command are required"})
		return
	}
	cmd := &model.UserCommand{UserID: userID, Name: req.Name, Command: req.Command}
	if err := h.commands.Create(c.Request.Context(), cmd); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "create failed"})
		return
	}
	c.JSON(http.StatusCreated, cmd)
}

func (h *ShortcutHandler) Update(c *gin.Context) {
	userID, ok := h.currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "unauthorized"})
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid id"})
		return
	}
	var req shortcutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "name and command are required"})
		return
	}
	cmd := &model.UserCommand{ID: id, UserID: userID, Name: req.Name, Command: req.Command}
	if err := h.commands.Update(c.Request.Context(), cmd); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "update failed"})
		return
	}
	if cmd.ID == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "not found"})
		return
	}
	c.JSON(http.StatusOK, cmd)
}

func (h *ShortcutHandler) Delete(c *gin.Context) {
	userID, ok := h.currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "unauthorized"})
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid id"})
		return
	}
	result, err := h.commands.Delete(c.Request.Context(), id, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "delete failed"})
		return
	}
	if result == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "deleted"})
}

type reorderRequest struct {
	OrderedIDs []int64 `json:"ordered_ids" binding:"required"`
}

func (h *ShortcutHandler) Reorder(c *gin.Context) {
	userID, ok := h.currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "unauthorized"})
		return
	}
	var req reorderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "ordered_ids is required"})
		return
	}
	if err := h.commands.Reorder(c.Request.Context(), userID, req.OrderedIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "reorder failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok"})
}
```

注意:`Update`/`Delete` 返回受影响行数,handler 据此区分「越权/不存在」(404)与成功(Task 1 已实现该签名)。

- [ ] **Step 4: 跑测试确认通过**

Run: `cd cmd/plugins/serve && go test ./store/ ./handler/ -run "TestCommandStore|TestShortcuts" -count=1`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add cmd/plugins/serve/handler/command.go cmd/plugins/serve/handler/command_test.go cmd/plugins/serve/store/command_store.go cmd/plugins/serve/store/command_store_test.go
git commit -m "feat(handler): ShortcutHandler 快捷命令 REST API + 归属隔离 + 404 语义"
```

---

### Task 3: 用户创建 Hook 注册表 + 默认指令播种 + 后端接线

**Files:**
- Create: `cmd/plugins/serve/user_hooks.go`
- Create: `cmd/plugins/serve/user_hooks_test.go`
- Modify: `cmd/plugins/serve/server.go`(字段、Init 接线、ensureAdmin 签名)
- Modify: `cmd/plugins/serve/handler/user.go`(`OnUserCreated` 字段 + Create 内调用)

**Interfaces:**
- Consumes: `store.CommandStore.Create`(Task 1)、`s.users/s.auth`。
- Produces: `serve.UserCreatedHook` 类型、`(*Server).RegisterUserCreatedHook`、`(*Server).runUserCreatedHooks`、`seedDefaultShortcuts`、`defaultShortcutCommands`;`handler.UserHandler.OnUserCreated` 字段。

- [ ] **Step 1: 写失败单测**

`cmd/plugins/serve/user_hooks_test.go`:

```go
package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerInit_AdminGetsDefaultShortcuts(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{DBPath: filepath.Join(dir, "owl.db"), ListenAddr: "127.0.0.1:0"}
	srv := NewServer(cfg)
	creds, err := srv.Init()
	require.NoError(t, err)
	require.NotNil(t, creds)

	user, err := srv.Users.FindByUsername(context.Background(), "admin")
	require.NoError(t, err)
	cs := store.NewCommandStore(srv.DB)
	n, err := cs.CountByUser(context.Background(), user.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
}

func TestServerInit_RestartDoesNotReseed(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{DBPath: filepath.Join(dir, "owl.db"), ListenAddr: "127.0.0.1:0"}

	srv := NewServer(cfg)
	_, err := srv.Init()
	require.NoError(t, err)
	srv.DB.Close()

	srv2 := NewServer(cfg)
	_, err = srv2.Init()
	require.NoError(t, err)
	user, err := srv2.Users.FindByUsername(context.Background(), "admin")
	require.NoError(t, err)
	cs := store.NewCommandStore(srv2.DB)
	n, _ := cs.CountByUser(context.Background(), user.ID)
	assert.Equal(t, 3, n, "restart must not add more defaults")
}

func TestUserCreate_SeedsDefaults(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{DBPath: filepath.Join(dir, "owl.db"), ListenAddr: "127.0.0.1:0"}
	srv := NewServer(cfg)
	creds, err := srv.Init()
	require.NoError(t, err)
	require.NotNil(t, creds)

	adminToken := loginToken(t, srv, "admin", creds.Password)

	// 管理员建新用户
	body := `{"username":"bob","password":"secret123","role":"operator","display_name":"Bob"}`
	req, _ := http.NewRequest("POST", "/api/v1/users", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)
	assert.Equal(t, 201, w.Code)

	// 新用户登录,查自己的快捷命令应为 3 条默认
	bobToken := loginToken(t, srv, "bob", "secret123")
	req2, _ := http.NewRequest("GET", "/api/v1/shortcuts", nil)
	req2.Header.Set("Authorization", "Bearer "+bobToken)
	w2 := httptest.NewRecorder()
	srv.Router.ServeHTTP(w2, req2)
	assert.Equal(t, 200, w2.Code)
	var list struct {
		Data []struct {
			Name    string `json:"name"`
			Command string `json:"command"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &list))
	require.Len(t, list.Data, 3)
	assert.Equal(t, "df -h", list.Data[0].Command)
	assert.Equal(t, "ps -fu $LOGNAME", list.Data[1].Command)
	assert.Equal(t, "free -h", list.Data[2].Command)
}

func loginToken(t *testing.T, srv *Server, username, password string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req, _ := http.NewRequest("POST", "/api/v1/login", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
	var resp struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp.Token
}
```

注意:此测试依赖 Task 4 的路由(shortcuts 已注册)才能通过 GET `/api/v1/shortcuts`。若按顺序执行,Task 3 的 `TestUserCreate_SeedsDefaults` 会因 404 失败——请在 Task 4 完成后再跑该测试;Task 3 其余两个测试不依赖路由。

- [ ] **Step 2: 写 Hook 实现**

`cmd/plugins/serve/user_hooks.go`:

```go
package serve

import (
	"context"
	"log"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
)

// UserCreatedHook 用户创建成功后的供给钩子(见 ADR-0001)。
// 失败只记日志,不阻塞用户创建。
type UserCreatedHook func(ctx context.Context, userID int64) error

// defaultShortcutCommands 新用户默认快捷命令(New-User Defaults)。
var defaultShortcutCommands = []model.UserCommand{
	{Name: "磁盘占用", Command: "df -h"},
	{Name: "我的进程", Command: "ps -fu $LOGNAME"},
	{Name: "内存", Command: "free -h"},
}

// seedDefaultShortcuts 为新用户播种默认快捷命令。
func seedDefaultShortcuts(ctx context.Context, cs *store.CommandStore, userID int64) error {
	for _, d := range defaultShortcutCommands {
		cmd := d
		cmd.UserID = userID
		if err := cs.Create(ctx, &cmd); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) runUserCreatedHooks(ctx context.Context, userID int64) {
	for _, h := range s.userCreatedHooks {
		if err := h(ctx, userID); err != nil {
			log.Printf("user-created hook failed for user %d: %v", userID, err)
		}
	}
}
```

- [ ] **Step 3: 接线 server.go**

`cmd/plugins/serve/server.go` 修改:

3a. `Server` struct 增加字段(在 `Users`/`Tasks` 附近):

```go
	commands        *store.CommandStore
	userCreatedHooks []UserCreatedHook
	shortcutHandler  *handler.ShortcutHandler
```

3b. `Init()` 中,在 `ensureAdmin` 之前建 `commands` 并注册播种 hook(把第 126 行 `creds, err := ensureAdmin(...)` 改为传入 hook 执行器):

在 Tasks store Init 之后(紧邻 `s.Tasks.Init`)插入:

```go
	s.commands = store.NewCommandStore(db)
	if err := s.commands.Init(context.Background()); err != nil {
		return nil, fmt.Errorf("init command store: %w", err)
	}
	s.RegisterUserCreatedHook(func(ctx context.Context, userID int64) error {
		return seedDefaultShortcuts(ctx, s.commands, userID)
	})
```

将 `ensureAdmin` 调用改为:

```go
	creds, err := ensureAdmin(context.Background(), s.Users, s.Auth, s.runUserCreatedHooks)
```

3c. 在 `s.userHandler = handler.NewUserHandler(...)` 之后加:

```go
	s.userHandler.OnUserCreated = s.runUserCreatedHooks
	s.shortcutHandler = handler.NewShortcutHandler(s.commands, s.Users)
```

3d. `setupRoutes()` 的 `auth` group 内(第 226 行之后)加:

```go
		// 快捷命令:所有已登录用户可管理自己的(个人数据)
		auth.GET("/shortcuts", s.shortcutHandler.List)
		auth.POST("/shortcuts", s.shortcutHandler.Create)
		auth.PUT("/shortcuts/reorder", s.shortcutHandler.Reorder)
		auth.PUT("/shortcuts/:id", s.shortcutHandler.Update)
		auth.DELETE("/shortcuts/:id", s.shortcutHandler.Delete)
```

3e. 新增 `RegisterUserCreatedHook` 方法(放在 `runUserCreatedHooks` 同文件即可):

```go
func (s *Server) RegisterUserCreatedHook(h UserCreatedHook) {
	s.userCreatedHooks = append(s.userCreatedHooks, h)
}
```

3f. `ensureAdmin` 签名改为:

```go
func ensureAdmin(ctx context.Context, users *store.UserStore, auth *service.AuthService, onCreated func(context.Context, int64)) (*AdminCredentials, error) {
```

在 `users.Create(...)` 成功之后、`return creds` 之前:

```go
	if onCreated != nil {
		onCreated(ctx, user.ID)
	}
```

(`ResetAdmin` 中的 `ensureAdmin` 不涉及——ResetAdmin 内部自建用户、不调用 ensureAdmin,保持不触发 Hook。)

- [ ] **Step 4: 接线 handler/user.go**

`cmd/plugins/serve/handler/user.go`:

4a. `UserHandler` struct 增加字段:

```go
type UserHandler struct {
	users        *store.UserStore
	auth         *service.AuthService
	OnUserCreated func(ctx context.Context, userID int64)
}
```

(需在 import 中加 `"context"`。)

4b. `Create` 方法内,`h.users.Create(...)` 成功之后、`c.JSON(http.StatusCreated, ...)` 之前:

```go
	if h.OnUserCreated != nil {
		h.OnUserCreated(c.Request.Context(), user.ID)
	}
```

- [ ] **Step 5: 跑测试确认通过**

Run: `cd cmd/plugins/serve && go test ./... -run "TestServerInit|TestUserCreate_SeedsDefaults|TestCommandStore|TestShortcuts" -count=1`
Expected: 除 `TestUserCreate_SeedsDefaults`(依赖 Task 4 路由,此时应 FAIL 于 shortcuts 404)外全部 PASS。

- [ ] **Step 6: Commit**

```bash
git add cmd/plugins/serve/user_hooks.go cmd/plugins/serve/user_hooks_test.go cmd/plugins/serve/server.go cmd/plugins/serve/handler/user.go
git commit -m "feat(serve): 用户创建 Hook 注册表 + 新用户默认快捷命令播种"
```

---

### Task 4: 前端 API 客户端(api.js)

**Files:**
- Modify: `cmd/plugins/serve/web/js/api.js`

**Interfaces:**
- Consumes: 后端 `/api/v1/shortcuts`(Task 2/3)。
- Produces: `api.shortcuts()/createShortcut/updateShortcut/deleteShortcut/reorderShortcuts`。

- [ ] **Step 1: 加 API 方法**

`cmd/plugins/serve/web/js/api.js`,在 `filters: () => ...` 之后(或任一相邻位置)加:

```js
  shortcuts: () =>
    request('GET', '/shortcuts'),

  createShortcut: (data) =>
    request('POST', '/shortcuts', data),

  updateShortcut: (id, data) =>
    request('PUT', `/shortcuts/${encodeURIComponent(id)}`, data),

  deleteShortcut: (id) =>
    request('DELETE', `/shortcuts/${encodeURIComponent(id)}`),

  reorderShortcuts: (orderedIds) =>
    request('PUT', '/shortcuts/reorder', { ordered_ids: orderedIds }),
```

- [ ] **Step 2: 语法校验**

Run: `node --check cmd/plugins/serve/web/js/api.js`
Expected: 无输出(成功)。

- [ ] **Step 3: 后端路由确认 + 冒烟**

Run: `cd cmd/plugins/serve && go build ./...`
Expected: 编译成功。

此时 Task 3 的 `TestUserCreate_SeedsDefaults` 应可通过:

Run: `cd cmd/plugins/serve && go test ./... -run TestUserCreate_SeedsDefaults -count=1`
Expected: PASS。

- [ ] **Step 4: Commit**

```bash
git add cmd/plugins/serve/web/js/api.js
git commit -m "feat(web): api.js 增加 shortcuts CRUD/reorder 方法"
```

---

### Task 5: 前端快捷命令条(exec.js + CSS + 静态断言)

**Files:**
- Modify: `cmd/plugins/serve/web/js/pages/exec.js`
- Modify: `cmd/plugins/serve/web/css/app.css`
- Modify: `cmd/plugins/serve/filesjs_test.go`

**Interfaces:**
- Consumes: `api.shortcuts()` 等(Task 4)、`switchExecMode/updateExecButton/esc`(已有)。
- Produces: 快捷命令条 UI、点击填入、增删改弹窗、拖拽排序。

- [ ] **Step 1: 写静态断言测试(先红)**

`cmd/plugins/serve/filesjs_test.go` 追加:

```go
func TestExecJS_ShortcutBar(t *testing.T) {
	src := readWebFile(t, "web/js/pages/exec.js")

	assert.True(t, strings.Contains(src, "id=\"shortcut-chips\""), "exec.js must render a shortcut chip container")
	assert.True(t, strings.Contains(src, "id=\"add-shortcut-btn\""), "exec.js must expose an add-shortcut button")
	assert.True(t, strings.Contains(src, "api.shortcuts()"), "exec.js must load shortcuts via api")
	assert.True(t, strings.Contains(src, "reorderShortcuts"), "exec.js must persist drag-drop order")
	assert.True(t, strings.Contains(src, "draggable=\"true\""), "exec.js must make chips draggable")
	assert.True(t, strings.Contains(src, "switchExecMode('command')"), "exec.js must switch to command mode when a chip is clicked")
	assert.True(t, strings.Contains(src, "openShortcutModal"), "exec.js must support add/edit modal")
	assert.True(t, strings.Contains(src, "deleteShortcut"), "exec.js must support delete")
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd cmd/plugins/serve && go test . -run TestExecJS_ShortcutBar -count=1`
Expected: FAIL(assert 至少一条不满足)。

- [ ] **Step 3: exec.js 状态与加载**

`cmd/plugins/serve/web/js/pages/exec.js`:

3a. 状态变量(`let commandContent = ...` 附近)加:

```js
  let shortcuts = [];
  let editingShortcutID = null;
```

3b. 初始化回调 `render(..., () => { loadNodes(); loadFilters(); ... })` 里加:

```js
    loadShortcuts();
```

3c. 新增函数(放在 `buildExecPayload` 之前):

```js
  async function loadShortcuts() {
    try {
      const res = await api.shortcuts();
      shortcuts = res.data || [];
    } catch { shortcuts = []; }
    renderShortcutChips();
  }

  function renderShortcutChips() {
    const c = document.getElementById('shortcut-chips');
    if (!c) return;
    if (shortcuts.length === 0) {
      c.innerHTML = '<span style="color:var(--muted);font-size:12px">暂无快捷命令,点 + 添加</span>';
      return;
    }
    c.innerHTML = shortcuts.map((s, i) => `
      <span class="shortcut-chip" draggable="true" data-index="${i}">
        <span class="shortcut-chip-label">${esc(s.name)}</span>
        <span class="shortcut-chip-actions">
          <button class="shortcut-chip-btn" data-act="edit" title="编辑">✎</button>
          <button class="shortcut-chip-btn" data-act="del" title="删除">×</button>
        </span>
      </span>`).join('');

    c.querySelectorAll('.shortcut-chip').forEach(chip => {
      const idx = parseInt(chip.dataset.index);
      chip.addEventListener('click', e => {
        if (e.target.closest('.shortcut-chip-actions')) return;
        const s = shortcuts[idx];
        switchExecMode('command');
        document.getElementById('cmd-input').value = s.command;
        updateExecButton();
      });
      chip.querySelectorAll('.shortcut-chip-btn').forEach(btn => {
        btn.addEventListener('click', e => {
          e.stopPropagation();
          const s = shortcuts[idx];
          if (btn.dataset.act === 'edit') openShortcutModal(s);
          else if (btn.dataset.act === 'del' && confirm(`删除快捷命令「${s.name}」?`)) deleteShortcut(s.id);
        });
      });
      chip.addEventListener('dragstart', e => {
        chip.classList.add('dragging');
        e.dataTransfer.setData('text/plain', String(idx));
        e.dataTransfer.effectAllowed = 'move';
      });
      chip.addEventListener('dragend', () => chip.classList.remove('dragging'));
      chip.addEventListener('dragover', e => e.preventDefault());
      chip.addEventListener('drop', e => {
        e.preventDefault();
        const from = parseInt(e.dataTransfer.getData('text/plain'));
        if (from === idx) return;
        const arr = shortcuts.slice();
        const [moved] = arr.splice(from, 1);
        arr.splice(idx, 0, moved);
        shortcuts = arr;
        renderShortcutChips();
        api.reorderShortcuts(shortcuts.map(x => x.id)).catch(() => loadShortcuts());
      });
    });
  }

  function openShortcutModal(s) {
    editingShortcutID = s ? s.id : null;
    document.getElementById('shortcut-modal-title').textContent = s ? '编辑快捷命令' : '新增快捷命令';
    document.getElementById('shortcut-name').value = s ? s.name : '';
    document.getElementById('shortcut-command').value = s ? s.command : '';
    document.getElementById('shortcut-error').textContent = '';
    document.getElementById('shortcut-modal').classList.add('open');
  }

  async function saveShortcut() {
    const name = document.getElementById('shortcut-name').value.trim();
    const command = document.getElementById('shortcut-command').value.trim();
    const err = document.getElementById('shortcut-error');
    if (!name || !command) { err.textContent = '名称和命令都不能为空'; return; }
    try {
      if (editingShortcutID) {
        await api.updateShortcut(editingShortcutID, { name, command });
      } else {
        await api.createShortcut({ name, command });
      }
      document.getElementById('shortcut-modal').classList.remove('open');
      loadShortcuts();
    } catch (e) { err.textContent = e.message || '保存失败'; }
  }

  async function deleteShortcut(id) {
    try {
      await api.deleteShortcut(id);
      loadShortcuts();
    } catch (e) { alert('删除失败: ' + (e.message || e)); }
  }
```

3d. 模板 `.cmd-editor` 内、`.editor-header` 之前插入快捷命令条:

```html
          <div class="shortcut-bar">
            <span class="shortcut-bar-title">快捷命令</span>
            <div class="shortcut-chips" id="shortcut-chips"></div>
            <button class="btn btn-ghost btn-sm" id="add-shortcut-btn" title="新增快捷命令">＋</button>
          </div>
```

3e. 模板末尾(`</div>` 前,`exec-log-downloads` 之后)插入弹窗:

```html
        <div class="modal-overlay" id="shortcut-modal">
          <div class="modal modal-sm">
            <h3 id="shortcut-modal-title">新增快捷命令</h3>
            <div class="modal-form">
              <div class="form-row"><label>名称</label><input id="shortcut-name" placeholder="如:磁盘占用"></div>
              <div class="form-row"><label>命令</label><textarea id="shortcut-command" placeholder="df -h" style="min-height:80px;width:100%"></textarea></div>
            </div>
            <p class="error-msg" id="shortcut-error"></p>
            <div class="modal-actions">
              <button class="btn btn-secondary" id="shortcut-cancel">取消</button>
              <button class="btn btn-primary" id="shortcut-save">保存</button>
            </div>
          </div>
        </div>
```

3f. 初始化回调里加事件绑定(在 `select-all-btn` 监听附近):

```js
    document.getElementById('add-shortcut-btn').addEventListener('click', () => openShortcutModal(null));
    document.getElementById('shortcut-save').addEventListener('click', saveShortcut);
    document.getElementById('shortcut-cancel').addEventListener('click', () => document.getElementById('shortcut-modal').classList.remove('open'));
    document.getElementById('shortcut-modal').addEventListener('click', e => {
      if (e.target === e.currentTarget) e.currentTarget.classList.remove('open');
    });
    document.getElementById('shortcut-name').addEventListener('keydown', e => { if (e.key === 'Enter') saveShortcut(); });
    document.getElementById('shortcut-command').addEventListener('keydown', e => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) saveShortcut(); });
```

- [ ] **Step 4: CSS**

`cmd/plugins/serve/web/css/app.css` 末尾追加:

```css
/* 快捷命令条 */
.shortcut-bar { display:flex; align-items:center; gap:8px; padding:8px 12px; border-bottom:1px solid var(--border); flex-wrap:wrap; }
.shortcut-bar-title { font-size:12px; color:var(--muted); white-space:nowrap; }
.shortcut-chips { display:flex; gap:6px; flex-wrap:wrap; align-items:center; flex:1; min-width:0; }
.shortcut-chip { display:inline-flex; align-items:center; gap:4px; padding:3px 10px; border:1px solid var(--border); border-radius:999px; cursor:pointer; font-size:12px; user-select:none; background:var(--bg); color:var(--fg); }
.shortcut-chip:hover { border-color:var(--accent-dim); }
.shortcut-chip.dragging { opacity:.5; border-style:dashed; }
.shortcut-chip-actions { display:inline-flex; gap:2px; visibility:hidden; }
.shortcut-chip:hover .shortcut-chip-actions { visibility:visible; }
.shortcut-chip-btn { background:none; border:none; cursor:pointer; color:var(--muted); font-size:12px; padding:0 2px; line-height:1; }
.shortcut-chip-btn:hover { color:var(--danger); }
```

- [ ] **Step 5: 语法校验 + 测试**

Run: `node --check cmd/plugins/serve/web/js/pages/exec.js`
Expected: 无输出。

Run: `cd cmd/plugins/serve && go test . -run TestExecJS_ShortcutBar -count=1`
Expected: PASS。

- [ ] **Step 6: 全量回归 + Commit**

Run: `cd cmd/plugins/serve && go test ./... -count=1`
Expected: 全 PASS。

```bash
git add cmd/plugins/serve/web/js/pages/exec.js cmd/plugins/serve/web/css/app.css cmd/plugins/serve/filesjs_test.go
git commit -m "feat(web): 命令执行页快捷命令条(点击填入/增删改弹窗/拖拽排序)"
```

---

## Self-Review

- **Spec 覆盖:** 数据表 ✔(Task1) CRUD+reorder API ✔(Task2) 归属隔离 ✔(Task2) 新用户默认指令+Hook 层 ✔(Task3) 已有用户不追加(restart 不重播)✔(Task3) 全部已登录用户可管理(路由放 auth group,无 RBAC 限制)✔(Task3) 前端横条/点击填入/增删改/拖拽 ✔(Task5)。`$LOGNAME` 原样存储 ✔(默认常量)。
- **占位符扫描:** 无 TBD/TODO;每个代码步骤含完整代码与命令。
- **类型一致性:** `CommandStore.Delete/Update` 在 Task1 单测与 Task2 handler 中签名同步改为返回 `(int64, error)`;`Reorder(ctx, userID, []int64)` 三处一致;`api.reorderShortcuts` 与后端 `ordered_ids` 一致;`defaultShortcutCommands` 三条命令与 spec 逐字一致。
- **风险提示:** Task3 的 `TestUserCreate_SeedsDefaults` 依赖 Task4 路由注册,计划已注明在 Task4 完成后才能通过;其余测试不受影响。
