package ai

import (
	"context"
	"strings"
	"testing"
)

// gateCounter 统计确认门被调用的次数
func gateCounter() (func(ToolCall) ConfirmationDecision, *int) {
	n := 0
	return func(call ToolCall) ConfirmationDecision {
		n++
		return ConfirmationDecision{Confirm: true, Question: "需要确认"}
	}, &n
}

// TestSafetyBlocked_Blacklist 黑名单命令在任何确认机制之前被内核拦截，
// 即使确认门放行也不得执行（CLI 端因此获得与 Web 端一致的黑名单）。
func TestSafetyBlocked_Blacklist(t *testing.T) {
	agent := newTestAgentForRoute(nil)
	agent.SetSafetyIdentity(func() string { return "tester" })
	agent.SetConfirmGate(func(call ToolCall) ConfirmationDecision {
		return ConfirmationDecision{Confirm: false} // 确认门全放行
	})

	result, err := agent.ExecuteToolCall(context.Background(), ToolCall{
		Name:      "execute_command",
		Arguments: map[string]interface{}{"command": "rm -rf /", "nodes": []interface{}{"node1"}},
	})
	if err != nil {
		t.Fatalf("ExecuteToolCall returned error: %v", err)
	}
	if !strings.Contains(result, "拦截") && !strings.Contains(result, "黑名单") {
		t.Fatalf("expected blacklist rejection text, got %q", result)
	}
}

// TestSafetyBlocked_OnlyGuardsCommandTools 只读工具与非命令类写工具不受拦截影响。
func TestSafetyBlocked_OnlyGuardsCommandTools(t *testing.T) {
	agent := newTestAgentForRoute(nil)
	result, err := agent.ExecuteToolCall(context.Background(), ToolCall{
		Name:      "query_nodes",
		Arguments: map[string]interface{}{},
	})
	if err != nil && result == "" {
		t.Fatalf("query_nodes should not be blocked: %v %q", err, result)
	}
}

// TestSafetyLowRiskSkipConfirm confirm_low_risk=false 时，命中低危白名单的
// 命令跳过确认门直接执行。
func TestSafetyLowRiskSkipConfirm(t *testing.T) {
	cfg := &Config{}
	disabled := false
	cfg.Safety.ConfirmLowRisk = &disabled
	agent := newTestAgentWithConfig(cfg)
	agent.SetSafetyIdentity(func() string { return "tester" })

	gate, count := gateCounter()
	agent.SetConfirmGate(gate)

	ok, question := agent.confirmToolCall(ToolCall{
		Name:      "execute_command",
		Arguments: map[string]interface{}{"command": "df -h", "nodes": []interface{}{"node1"}},
	})
	if *count != 0 {
		t.Fatalf("low-risk command must skip confirm gate, gate called %d times", *count)
	}
	if !ok || question != "" {
		t.Fatalf("expected allowed, got ok=%v question=%q", ok, question)
	}
}

// TestSafetyLowRiskConfirmByDefault confirm_low_risk 默认（true）时低危命令仍走确认。
func TestSafetyLowRiskConfirmByDefault(t *testing.T) {
	agent := newTestAgentWithConfig(&Config{})
	agent.SetSafetyIdentity(func() string { return "tester" })

	gate, count := gateCounter()
	agent.SetConfirmGate(gate)

	ok, question := agent.confirmToolCall(ToolCall{
		Name:      "execute_command",
		Arguments: map[string]interface{}{"command": "df -h", "nodes": []interface{}{"node1"}},
	})
	if *count != 1 {
		t.Fatalf("default policy must confirm low-risk command, gate called %d times", *count)
	}
	if ok || question != "需要确认" {
		t.Fatalf("expected intercepted, got ok=%v question=%q", ok, question)
	}
}

// TestSafetyWhitelistPrefixMatch 白名单按去空格后的前缀匹配，未命中走确认。
func TestSafetyWhitelistPrefixMatch(t *testing.T) {
	cfg := &Config{}
	disabled := false
	cfg.Safety.ConfirmLowRisk = &disabled
	agent := newTestAgentWithConfig(cfg)
	agent.SetSafetyIdentity(func() string { return "tester" })

	gate, count := gateCounter()
	agent.SetConfirmGate(gate)

	// 高危命令不在白名单 → 必须确认
	ok, question := agent.confirmToolCall(ToolCall{
		Name:      "execute_command",
		Arguments: map[string]interface{}{"command": "reboot", "nodes": []interface{}{"node1"}},
	})
	if *count != 1 {
		t.Fatalf("non-whitelisted command must go through gate, called %d times", *count)
	}
	if ok || question != "需要确认" {
		t.Fatalf("expected intercepted, got ok=%v question=%q", ok, question)
	}
}

func newTestAgentWithConfig(cfg *Config) *Agent {
	agent, _ := NewAgent(nil, cfg, &mockNodeMgrForAI{nodes: nodesForAI()}, nil, nil)
	return agent
}
