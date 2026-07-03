package handler

import (
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

func rbacTestRouter(t *testing.T, roles ...model.Role) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	us := store.NewUserStore(db)
	as := service.NewAuthService("test-secret-32byte-long-string!!")
	ah := NewAuthHandler(us, as)

	r := gin.New()
	r.GET("/api/v1/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	protected := r.Group("/api/v1")
	protected.Use(ah.AuthMiddleware())
	if len(roles) > 0 {
		protected.Use(ah.RBACMiddleware(roles...))
	}
	protected.GET("/nodes", func(c *gin.Context) {
		c.JSON(200, gin.H{"data": []string{}})
	})

	return r
}

func injectAuth(router *gin.Engine, token string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/nodes", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)
	return w
}

func TestRBAC_ViewerCanAccessReadonly(t *testing.T) {
	router := rbacTestRouter(t, model.RoleViewer)
	token := testToken(t, "viewer", "viewer")
	w := injectAuth(router, token)
	assert.Equal(t, 200, w.Code)
}

func TestRBAC_EditorCanAccessReadonly(t *testing.T) {
	router := rbacTestRouter(t, model.RoleEditor)
	token := testToken(t, "editor", "editor")
	w := injectAuth(router, token)
	assert.Equal(t, 200, w.Code)
}

func TestRBAC_OperatorCanAccessReadonly(t *testing.T) {
	router := rbacTestRouter(t, model.RoleOperator)
	token := testToken(t, "operator", "operator")
	w := injectAuth(router, token)
	assert.Equal(t, 200, w.Code)
}

func TestRBAC_AdminCanAccessReadonly(t *testing.T) {
	router := rbacTestRouter(t, model.RoleAdmin)
	token := testToken(t, "admin", "admin")
	w := injectAuth(router, token)
	assert.Equal(t, 200, w.Code)
}

func TestRBAC_ForbiddenWithWrongRole(t *testing.T) {
	router := rbacTestRouter(t, model.RoleAdmin)
	token := testToken(t, "viewer", "viewer")
	w := injectAuth(router, token)
	assert.Equal(t, 403, w.Code)

	var resp struct {
		Message string `json:"message"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "insufficient permissions", resp.Message)
}

func TestRBAC_Unauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	us := store.NewUserStore(db)
	as := service.NewAuthService("test-secret-32byte-long-string!!")
	ah := NewAuthHandler(us, as)

	r := gin.New()
	r.GET("/api/v1/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	protected := r.Group("/api/v1")
	protected.Use(ah.AuthMiddleware())
	protected.Use(ah.RBACMiddleware(model.RoleAdmin))
	protected.GET("/nodes", func(c *gin.Context) {
		c.JSON(200, gin.H{"data": []string{}})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/nodes", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, 401, w.Code)
}

func testToken(t *testing.T, username, role string) string {
	t.Helper()
	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, err := as.GenerateToken(username, role)
	require.NoError(t, err)
	return token
}

func TestRBAC_OwnerEdit(t *testing.T) {
	// Intentionally skipped - placeholder for ownership-based access
}

var _ = rbacTestRouter
