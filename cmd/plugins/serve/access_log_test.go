package serve

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAccessLog_RedactsToken 访问日志不得记录 query 中的 token 原文。
func TestAccessLog_RedactsToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(accessLog())
	r.GET("/api/v1/ws", func(c *gin.Context) { c.Status(http.StatusSwitchingProtocols) })

	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(orig)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/ws?token=super-secret-value", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusSwitchingProtocols, w.Code)

	out := buf.String()
	assert.Contains(t, out, "GET /api/v1/ws?token=")
	assert.NotContains(t, out, "super-secret-value", "token 原文不得落日志")
	assert.Contains(t, out, "101") // 状态码被记录(本测试路由直接返回 101)
}

// TestAccessLog_RecordsPlainPaths 无敏感参数的请求按原样记录。
func TestAccessLog_RecordsPlainPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(accessLog())
	r.GET("/api/v1/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(orig)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/health", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	assert.Contains(t, buf.String(), "GET /api/v1/health -> 200")
}

// TestRedactTokenQuery 无 token 的 URI 原样返回。
func TestRedactTokenQuery(t *testing.T) {
	assert.Equal(t, "/api/v1/nodes?page=2", redactTokenQuery("/api/v1/nodes?page=2"))
	assert.Equal(t, "/api/v1/ws?token=%2A%2A%2A", redactTokenQuery("/api/v1/ws?token=abc"))
	assert.Equal(t, "/plain", redactTokenQuery("/plain"))
}
