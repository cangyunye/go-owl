package handler

import "context"

// ExecIdentity 是一次执行链路的操作者身份（AI 或页面），经 context 传递，
// 替代共享 Executor 上的单例身份字段（并发请求互不踩踏）。
type ExecIdentity struct {
	Username string
	Role     string
}

type execIdentityKey struct{}

// WithIdentity 把操作者身份注入 context。
func WithIdentity(ctx context.Context, id ExecIdentity) context.Context {
	return context.WithValue(ctx, execIdentityKey{}, id)
}

// IdentityFromContext 读取身份；未注入时返回零值（视为匿名）。
func IdentityFromContext(ctx context.Context) ExecIdentity {
	if v, ok := ctx.Value(execIdentityKey{}).(ExecIdentity); ok {
		return v
	}
	return ExecIdentity{}
}
