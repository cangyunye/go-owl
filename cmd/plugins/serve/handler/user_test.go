package handler

import (
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
	return us, r
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
