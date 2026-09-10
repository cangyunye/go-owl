package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func userTestSetup(t *testing.T) (*store.UserStore, *gin.Engine) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	us := store.NewUserStore(db)
	require.NoError(t, us.Init(context.Background()))

	gin.SetMode(gin.TestMode)
	as := service.NewAuthService("test-secret-32byte-long-string!!")
	ah := NewAuthHandler(us, as)
	uh := NewUserHandler(us, as)

	r := gin.New()
	auth := r.Group("/api/v1")
	auth.Use(ah.AuthMiddleware())
	admin := auth.Group("", ah.RBACMiddleware(model.RoleAdmin))
	admin.GET("/users", uh.List)
	admin.PUT("/users/:id", uh.Update)
	admin.DELETE("/users/:id", uh.Delete)
	return us, r
}

// userMutation 以 admin 身份发起 PUT/DELETE。
func userMutation(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken())
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestUserDelete_NotFound(t *testing.T) {
	_, r := userTestSetup(t)
	w := userMutation(t, r, "DELETE", "/api/v1/users/9999", "")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUserDelete_LastAdmin_Refused(t *testing.T) {
	us, r := userTestSetup(t)
	ctx := context.Background()
	admin := &model.User{Username: "admin1", Role: model.RoleAdmin}
	require.NoError(t, us.Create(ctx, admin))
	require.NoError(t, us.Create(ctx, &model.User{Username: "viewer1", Role: model.RoleViewer}))

	w := userMutation(t, r, "DELETE", "/api/v1/users/"+strconv.FormatInt(admin.ID, 10), "")
	assert.Equal(t, http.StatusConflict, w.Code)

	_, err := us.FindByID(ctx, admin.ID)
	assert.NoError(t, err, "最后一个 admin 必须仍然存在")
}

func TestUserDelete_AdminAllowedWhenAnotherExists(t *testing.T) {
	us, r := userTestSetup(t)
	ctx := context.Background()
	a1 := &model.User{Username: "admin1", Role: model.RoleAdmin}
	a2 := &model.User{Username: "admin2", Role: model.RoleAdmin}
	require.NoError(t, us.Create(ctx, a1))
	require.NoError(t, us.Create(ctx, a2))

	w := userMutation(t, r, "DELETE", "/api/v1/users/"+strconv.FormatInt(a1.ID, 10), "")
	assert.Equal(t, http.StatusOK, w.Code)
	_, err := us.FindByID(ctx, a1.ID)
	assert.Error(t, err, "非最后一个 admin 应可删除")
}

func TestUserUpdate_DemoteLastAdmin_Refused(t *testing.T) {
	us, r := userTestSetup(t)
	ctx := context.Background()
	admin := &model.User{Username: "admin1", Role: model.RoleAdmin}
	require.NoError(t, us.Create(ctx, admin))
	require.NoError(t, us.Create(ctx, &model.User{Username: "viewer1", Role: model.RoleViewer}))

	w := userMutation(t, r, "PUT", "/api/v1/users/"+strconv.FormatInt(admin.ID, 10), `{"role":"viewer"}`)
	assert.Equal(t, http.StatusConflict, w.Code)

	got, err := us.FindByID(ctx, admin.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RoleAdmin, got.Role, "最后一个 admin 不得被降级")
}

func userGET(t *testing.T, r *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", path, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken())
	r.ServeHTTP(w, req)
	return w
}

type userListResponse struct {
	Data []struct {
		ID          int64  `json:"id"`
		Username    string `json:"username"`
		Role        string `json:"role"`
		DisplayName string `json:"display_name"`
	} `json:"data"`
	Meta struct {
		Total    int `json:"total"`
		Page     int `json:"page"`
		PageSize int `json:"page_size"`
	} `json:"meta"`
}

func TestUserList_PaginationAndSearch(t *testing.T) {
	us, r := userTestSetup(t)
	ctx := context.Background()
	for _, name := range []string{"alice", "bob", "charlie", "dave", "eve", "carol"} {
		require.NoError(t, us.Create(ctx, &model.User{Username: name, Role: model.RoleViewer}))
	}

	w := userGET(t, r, "/api/v1/users?page=1&page_size=2")
	require.Equal(t, http.StatusOK, w.Code)
	var resp userListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 6, resp.Meta.Total)
	assert.Equal(t, 1, resp.Meta.Page)
	assert.Equal(t, 2, resp.Meta.PageSize)
	assert.Len(t, resp.Data, 2)
	assert.Equal(t, "alice", resp.Data[0].Username)
	assert.Equal(t, "bob", resp.Data[1].Username)

	w = userGET(t, r, "/api/v1/users?page=2&page_size=2")
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.Meta.Page)
	assert.Equal(t, "charlie", resp.Data[0].Username)
	assert.Equal(t, "dave", resp.Data[1].Username)

	w = userGET(t, r, "/api/v1/users?q=ar&page=1&page_size=10")
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.Meta.Total)
	names := []string{resp.Data[0].Username, resp.Data[1].Username}
	assert.Contains(t, names, "carol")
	assert.Contains(t, names, "charlie")
}

func TestUserList_RoleFilter(t *testing.T) {
	us, r := userTestSetup(t)
	ctx := context.Background()
	require.NoError(t, us.Create(ctx, &model.User{Username: "admin1", Role: model.RoleAdmin}))
	require.NoError(t, us.Create(ctx, &model.User{Username: "viewer1", Role: model.RoleViewer}))
	require.NoError(t, us.Create(ctx, &model.User{Username: "viewer2", Role: model.RoleViewer}))

	w := userGET(t, r, "/api/v1/users?role=viewer")
	require.Equal(t, http.StatusOK, w.Code)
	var resp userListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.Meta.Total)
	assert.Len(t, resp.Data, 2)

	w = userGET(t, r, "/api/v1/users?role=admin")
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.Meta.Total)
	assert.Equal(t, "admin1", resp.Data[0].Username)

	// meta 需携带各角色用户数
	var meta struct {
		Total      int            `json:"total"`
		RoleCounts map[string]int `json:"role_counts"`
	}
	w = userGET(t, r, "/api/v1/users")
	var raw struct {
		Meta json.RawMessage `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	require.NoError(t, json.Unmarshal(raw.Meta, &meta))
	assert.Equal(t, 2, meta.RoleCounts["viewer"])
	assert.Equal(t, 1, meta.RoleCounts["admin"])
	assert.Equal(t, 0, meta.RoleCounts["operator"])
	assert.Equal(t, 0, meta.RoleCounts["editor"])

	// 非法 role 参数返回 400
	w = userGET(t, r, "/api/v1/users?role=superuser")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUserList_DefaultsAndRBAC(t *testing.T) {
	us, r := userTestSetup(t)
	ctx := context.Background()
	require.NoError(t, us.Create(ctx, &model.User{Username: "alice", Role: model.RoleViewer}))
	require.NoError(t, us.Create(ctx, &model.User{Username: "bob", Role: model.RoleAdmin}))

	w := userGET(t, r, "/api/v1/users")
	require.Equal(t, http.StatusOK, w.Code)
	var resp userListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.Meta.Total)
	assert.Equal(t, 1, resp.Meta.Page)
	assert.Equal(t, 20, resp.Meta.PageSize)

	// 非 admin 角色访问必须被拒绝
	w = httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/users", nil)
	req.Header.Set("Authorization", "Bearer "+viewerToken())
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}
