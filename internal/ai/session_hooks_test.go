package ai

import (
	"context"
	"testing"
)

// TestSessionHooks_SessionScoped：共享同一 Agent 的多个会话必须各自持有
// 确认门/节点回调，不再覆写 Agent 全局单槽（Web 多用户并发互串的根因）。
func TestSessionHooks_SessionScoped(t *testing.T) {
	agent := offlineTestAgent(t)
	s1 := NewSession(agent)
	s2 := NewSession(agent)

	if agent.confirmGate != nil || agent.nodeContextHook != nil {
		t.Fatal("session setup must not touch shared agent slots")
	}
	if s1.confirmGateFn == nil || s2.confirmGateFn == nil {
		t.Fatal("each session must own its confirm gate")
	}
	if s1.nodeHookFn == nil || s2.nodeHookFn == nil {
		t.Fatal("each session must own its node hook")
	}

	// ctx 携带：挂在 s1 上就解析出 s1 的回调
	h := hooksFromCtx(withSessionHooks(context.Background(), s1))
	if h == nil || h.gate == nil || h.hook == nil {
		t.Fatal("expected s1 hooks from ctx")
	}
	if h2 := hooksFromCtx(context.Background()); h2 != nil {
		t.Fatal("bare ctx must not carry hooks")
	}
}

// TestSessionHooks_GateResolution：ctx 会话门优先于 Agent 兜底门。
func TestSessionHooks_GateResolution(t *testing.T) {
	agent := offlineTestAgent(t)
	agentCalled := false
	agent.SetConfirmGate(func(call ToolCall) ConfirmationDecision {
		agentCalled = true
		return ConfirmationDecision{Confirm: false}
	})
	s := NewSession(agent)
	sessionCalled := false
	s.confirmGateFn = func(call ToolCall) ConfirmationDecision {
		sessionCalled = true
		return ConfirmationDecision{Confirm: false}
	}

	gate := resolveGate(withSessionHooks(context.Background(), s), agent)
	_ = gate(ToolCall{Name: "execute_command"})
	if agentCalled || !sessionCalled {
		t.Fatalf("expected session gate to win (agent=%v session=%v)", agentCalled, sessionCalled)
	}

	// 无会话 ctx 时回退 Agent 槽（CLI 直连 Process 的既有路径）
	gate2 := resolveGate(context.Background(), agent)
	_ = gate2(ToolCall{Name: "execute_command"})
	if !agentCalled {
		t.Fatal("fallback to agent gate expected without session ctx")
	}
}
