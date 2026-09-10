package serve

import (
	"log"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
)

// accessLog 记录每次 HTTP 访问：方法、脱敏后的 URI、状态码、耗时、来源 IP。
// /ws 与 /session/terminal 以 ?token= 传凭证，落日志前必须脱敏。
func accessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Printf("%s %s -> %d (%s) from %s",
			c.Request.Method,
			redactTokenQuery(c.Request.URL.RequestURI()),
			c.Writer.Status(),
			time.Since(start).Round(time.Microsecond),
			c.ClientIP(),
		)
	}
}

// redactTokenQuery 把 URI 中 token 参数的值替换为 ***，其余保持原样。
func redactTokenQuery(requestURI string) string {
	u, err := url.Parse(requestURI)
	if err != nil {
		return requestURI
	}
	q := u.Query()
	if q.Get("token") == "" {
		return requestURI
	}
	q.Set("token", "***")
	u.RawQuery = q.Encode()
	return u.String()
}
