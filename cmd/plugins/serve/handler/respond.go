package handler

import (
	"errors"
	"fmt"
	"log"

	"github.com/gin-gonic/gin"
)

// businessError 表示可安全透传给客户端的业务/校验错误（消息面向用户，
// 不含实现细节）。只有经 bizErr 构造的错误才会在 4xx 响应中原样返回。
type businessError struct {
	msg string
}

func (e *businessError) Error() string { return e.msg }

// bizErr 构造业务/校验错误。
func bizErr(format string, a ...any) error {
	return &businessError{msg: fmt.Sprintf(format, a...)}
}

// respondErr 统一错误响应：业务错误透传消息；其余按内部错误处理，
// 对外只给 public 短描述，细节记服务端日志。
func respondErr(c *gin.Context, status int, public string, err error) {
	var be *businessError
	if errors.As(err, &be) {
		c.JSON(status, gin.H{"code": status, "message": be.msg})
		return
	}
	c.JSON(status, gin.H{"code": status, "message": internalErr(public, err)})
}

// internalErr 返回对外的短错误描述，内部错误细节仅记服务端日志。
// 5xx 响应一律经此构造 message，避免把 SQL/文件系统/上游 provider 的
// 实现细节回传客户端（此前多个 handler 直接拼接 err.Error()）。
func internalErr(public string, err error) string {
	if err != nil {
		log.Printf("serve api: %s: %v", public, err)
	}
	return public
}
