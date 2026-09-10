package handler

import "log"

// internalErr 返回对外的短错误描述，内部错误细节仅记服务端日志。
// 5xx 响应一律经此构造 message，避免把 SQL/文件系统/上游 provider 的
// 实现细节回传客户端（此前多个 handler 直接拼接 err.Error()）。
func internalErr(public string, err error) string {
	if err != nil {
		log.Printf("serve api: %s: %v", public, err)
	}
	return public
}
