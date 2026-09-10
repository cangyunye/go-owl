package handler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// TestNodeSeed_500DoesNotLeakInternalError 校验 5xx 只返回通用文案：
// 底层报错（如 sql: database is closed）不得出现在响应体里。
func TestNodeSeed_500DoesNotLeakInternalError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.Close()) // 关库，令写入失败

	r := gin.New()
	r.POST("/seed", NewNodeHandler(db).Seed)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/seed", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "seed failed")
	assert.NotContains(t, body, "database is closed")
	assert.NotContains(t, body, "sql:")
}
