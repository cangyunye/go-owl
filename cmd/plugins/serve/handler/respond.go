package handler

import (
	"errors"
	"fmt"
	"log"
	"net/http"

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

// 成功响应约定（新增代码一律走这三个 helper，不要再手写 gin.H）：
//   - ok:       对象/载荷  → {"data": ...}
//   - 列表:     {"data": [...], "meta": {...}}（现网已大量存在，暂以手写为准，
//     未来如需强约束再抽 okList）
//   - okAction: 动作类    → {"ok": true, "message": "..."}
//
// 错误统一走 respondErr/internalErr（见上）。既有 GET 载荷、字段名与状态码
// 不在本约定的迁移范围内（前端按字段直读，改形需前端同批，见评审报告 P1-1）。

// ok 以标准包裹返回成功载荷：{"data": ...}。
func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// okAction 返回动作类端点的统一成功形态：{"ok": true, "message": msg}。
// 保留 message 以兼容读取它的外部脚本。
func okAction(c *gin.Context, message string) {
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": message})
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
