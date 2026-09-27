package serve

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 传输层减负：文本响应要压缩，流式端点不能被压缩（gzip 会破坏 SSE/WS 语义）。
func TestGzipResponses_CompressesTextSkipsStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gzipResponses())
	r.GET("/json", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": strings.Repeat("owl", 800)})
	})
	r.POST("/api/v1/ai/chat/stream", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.String(http.StatusOK, "data: "+strings.Repeat("x", 2000)+"\n\n")
	})
	r.GET("/api/v1/ws", func(c *gin.Context) {
		c.Header("Content-Type", "text/plain")
		c.String(http.StatusOK, strings.Repeat("y", 2000))
	})

	call := func(method, path string, gzipAccept bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		if gzipAccept {
			req.Header.Set("Accept-Encoding", "gzip")
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	t.Run("JSON 压缩且可解回原文", func(t *testing.T) {
		w := call("GET", "/json", true)
		require.Equal(t, "gzip", w.Header().Get("Content-Encoding"))
		assert.Contains(t, w.Header().Get("Vary"), "Accept-Encoding")
		assert.Empty(t, w.Header().Get("Content-Length"), "压缩后长度必须重算，不能沿用原值")

		gr, err := gzip.NewReader(bytes.NewReader(w.Body.Bytes()))
		require.NoError(t, err)
		plain, err := io.ReadAll(gr)
		require.NoError(t, err)
		assert.Contains(t, string(plain), `"data"`)
		assert.Less(t, w.Body.Len(), len(plain), "压缩应确实变小")
	})

	t.Run("SSE 不压缩", func(t *testing.T) {
		w := call("POST", "/api/v1/ai/chat/stream", true)
		assert.Empty(t, w.Header().Get("Content-Encoding"))
	})

	t.Run("WebSocket 端点不压缩", func(t *testing.T) {
		w := call("GET", "/api/v1/ws", true)
		assert.Empty(t, w.Header().Get("Content-Encoding"))
	})

	t.Run("客户端未声明 gzip 时不压缩", func(t *testing.T) {
		w := call("GET", "/json", false)
		assert.Empty(t, w.Header().Get("Content-Encoding"))
	})
}

// 静态资源：内容哈希当 ETag，命中 If-None-Match 走 304，避免每次刷新重传 JS/CSS。
func TestStaticETag_ServesHashAndNotModified(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := fstest.MapFS{
		"js/app.js": {Data: []byte("console.log('owl')")},
	}
	r := gin.New()
	grp := r.Group("/static")
	grp.Use(staticETag(root, "/static/"))
	grp.StaticFS("/", http.FS(root))

	req := httptest.NewRequest("GET", "/static/js/app.js", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	etag := w.Header().Get("ETag")
	require.NotEmpty(t, etag, "静态资源必须带 ETag")
	assert.Contains(t, w.Header().Get("Cache-Control"), "max-age")
	assert.Contains(t, w.Body.String(), "owl")

	req2 := httptest.NewRequest("GET", "/static/js/app.js", nil)
	req2.Header.Set("If-None-Match", etag)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusNotModified, w2.Code)
	assert.Empty(t, w2.Body.String(), "304 不应带响应体")

	req3 := httptest.NewRequest("GET", "/static/js/missing.js", nil)
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusNotFound, w3.Code, "不存在的资源仍应交给 StaticFS 出 404")
}
