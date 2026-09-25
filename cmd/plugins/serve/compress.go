// 传输层减负：gzip 压缩与静态资源 ETag。
//
// 背景（设计文档 07 的「负载归属」一节）：前端资源与 JSON 列表反复传输同一份数据，
// 而此前既没有 gzip 也没有 ETag —— 每次刷新都要重传全部 JS/CSS，每个标签打开选择器
// 都要重传整份节点列表。这里做两件不改变架构、纯减负的事：
//
//   gzipResponses —— 对文本类响应做 gzip，跳过 WS/SSE 与其他流式端点
//   staticETag    —— 给 /static 资源按内容哈希发 ETag，命中直接 304
package serve

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// 不压缩的路径：WebSocket、终端、SSE 流式端点（压缩会破坏流式语义或增加延迟）
var gzipSkipPrefixes = []string{
	"/api/v1/ws",
	"/api/v1/session/terminal",
	"/api/v1/ai/chat/stream",
}

func compressibleType(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
	switch {
	case strings.HasPrefix(ct, "text/"):
		return true
	case ct == "application/json", ct == "application/javascript",
		ct == "application/xml", ct == "application/xhtml+xml",
		ct == "image/svg+xml", ct == "application/manifest+json":
		return true
	}
	return false
}

// gzipResponses 对响应体做 gzip。压缩与否在「写响应头」时判定：
// 那时 Content-Type 已由 handler 设定，且还没把头部发给客户端。
func gzipResponses() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !strings.Contains(c.GetHeader("Accept-Encoding"), "gzip") ||
			strings.Contains(strings.ToLower(c.GetHeader("Connection")), "upgrade") ||
			strings.Contains(c.GetHeader("Accept"), "text/event-stream") {
			c.Next()
			return
		}
		p := c.Request.URL.Path
		for _, skip := range gzipSkipPrefixes {
			if strings.HasPrefix(p, skip) {
				c.Next()
				return
			}
		}
		gw := &gzipWriter{ResponseWriter: c.Writer}
		defer gw.close()
		c.Writer = gw
		c.Next()
	}
}

type gzipWriter struct {
	gin.ResponseWriter
	gz       *gzip.Writer
	decided  bool
	compress bool
}

// decide 必须在「头部真正发出去之前」调用：压缩要改 Content-Encoding 与 Content-Length
func (w *gzipWriter) decide(code int) {
	if w.decided {
		return
	}
	w.decided = true
	if code == http.StatusNoContent || code == http.StatusNotModified || code == http.StatusSwitchingProtocols {
		return
	}
	if w.Header().Get("Content-Encoding") != "" {
		return
	}
	if !compressibleType(w.Header().Get("Content-Type")) {
		return
	}
	w.compress = true
	// 长度会变：删掉 Content-Length，交给底层按块/重算处理
	w.Header().Del("Content-Length")
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Add("Vary", "Accept-Encoding")
	w.gz = gzip.NewWriter(w.ResponseWriter)
}

// WriteHeader 只记录状态：gin 的 responseWriter 在这里不发头，真正发头在
// WriteHeaderNow/Write —— 而 Content-Type 通常在这之后才由 render 写入。
// 因此压缩判定推迟到那两处，否则会拿到空的 Content-Type 而误判为不可压缩。
func (w *gzipWriter) WriteHeader(code int) {
	w.ResponseWriter.WriteHeader(code)
}

func (w *gzipWriter) WriteHeaderNow() {
	w.decide(w.Status())
	w.ResponseWriter.WriteHeaderNow()
}

func (w *gzipWriter) Write(b []byte) (int, error) {
	w.decide(w.Status())
	if w.compress && w.gz != nil {
		return w.gz.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

func (w *gzipWriter) close() {
	if w.gz != nil {
		_ = w.gz.Close()
	}
}

// staticETag 给嵌入式静态资源发 ETag 与缓存头：内容随二进制固定，
// 用内容哈希当 ETag，命中 If-None-Match 直接 304，省掉每次刷新的 JS/CSS 重传。
// DevMode 下不启用（文件会变，哈希缓存会失真），继续走 no-cache。
func staticETag(root fs.FS, urlPrefix string) gin.HandlerFunc {
	var mu sync.Mutex
	etags := map[string]string{}

	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Next()
			return
		}
		name := strings.TrimPrefix(c.Request.URL.Path, urlPrefix)
		if name == "" || strings.HasSuffix(name, "/") {
			c.Next()
			return
		}
		data, err := fs.ReadFile(root, name)
		if err != nil {
			c.Next() // 交给 StaticFS 出 404
			return
		}

		mu.Lock()
		etag, ok := etags[name]
		if !ok {
			sum := sha256.Sum256(data)
			etag = `"` + hex.EncodeToString(sum[:8]) + `"`
			etags[name] = etag
		}
		mu.Unlock()

		c.Header("ETag", etag)
		c.Header("Cache-Control", "public, max-age=300")
		if inm := c.GetHeader("If-None-Match"); inm != "" && strings.Contains(inm, etag) {
			c.Status(http.StatusNotModified)
			c.Abort()
			return
		}
		ct := mime.TypeByExtension(path.Ext(name))
		if ct == "" {
			ct = "application/octet-stream"
		}
		c.Data(http.StatusOK, ct, data)
		c.Abort()
	}
}
