package ai

import (
	"context"
	"strings"
	"testing"
)

func TestConfirmGateInterceptsWriteOp(t *testing.T) {
	agent := newTestAgentForRoute([]string{
		"exec",
		"```json\n{\"tool_calls\":[{\"name\":\"execute_command\",\"arguments\":{\"command\":\"uptime\",\"nodes\":[\"node1\"]}}]}\n```",
	})
	gateCalls := 0
	agent.SetConfirmGate(func(call ToolCall) ConfirmationDecision {
		gateCalls++
		return ConfirmationDecision{
			Confirm:  true,
			Summary:  "执行 uptime",
			Question: "即将执行：执行 uptime，是否继续？（是/否）",
		}
	})

	resp, err := agent.Process(context.Background(), "在 node1 上执行 uptime", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if resp != "即将执行：执行 uptime，是否继续？（是/否）" {
		t.Errorf("expected confirm question, got %q", resp)
	}
	if gateCalls != 1 {
		t.Errorf("expected gate to be called once, got %d", gateCalls)
	}
}

func TestConfirmGateInterceptsInProcessWithContext(t *testing.T) {
	agent := newTestAgentForRoute([]string{
		"```json\n{\"tool_calls\":[{\"name\":\"execute_command\",\"arguments\":{\"command\":\"uptime\",\"nodes\":[\"node1\"]}}]}\n```",
	})
	agent.SetConfirmGate(func(call ToolCall) ConfirmationDecision {
		return ConfirmationDecision{
			Confirm:  true,
			Summary:  "执行 uptime",
			Question: "即将执行：执行 uptime，是否继续？（是/否）",
		}
	})

	msgs := []Message{{Role: "system", Content: "system"}, {Role: "user", Content: "执行 uptime"}}
	updated, resp, err := agent.ProcessWithContext(context.Background(), msgs, nil)
	if err != nil {
		t.Fatalf("ProcessWithContext failed: %v", err)
	}
	if resp != "即将执行：执行 uptime，是否继续？（是/否）" {
		t.Errorf("expected confirm question, got %q", resp)
	}
	if len(updated) != len(msgs)+1 {
		t.Errorf("expected one appended assistant message, got %d messages", len(updated))
	}
}

func TestSessionConfirmFlow(t *testing.T) {
	agent := newTestAgentForRoute([]string{
		"exec",
		"```json\n{\"tool_calls\":[{\"name\":\"execute_command\",\"arguments\":{\"command\":\"uptime\",\"nodes\":[\"node1\"]}}]}\n```",
	})
	sess := NewSession(agent)
	ctx := context.Background()

	resp, err := sess.Send(ctx, "在 node1 上执行 uptime")
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if !strings.Contains(resp, "是否继续") {
		t.Errorf("expected confirm question, got %q", resp)
	}
	if sess.pendingContext == nil || sess.pendingContext.State != "awaiting_confirmation" {
		t.Fatal("expected pending confirmation context")
	}
	if sess.pendingContext.ToolCall.Name != "execute_command" {
		t.Errorf("expected pending tool to be execute_command, got %q", sess.pendingContext.ToolCall.Name)
	}

	resp2, err := sess.Send(ctx, "是")
	if err != nil {
		t.Fatalf("Send confirm failed: %v", err)
	}
	if sess.pendingContext != nil {
		t.Error("expected pending context to be cleared after confirm")
	}
	if !strings.Contains(resp2, "已执行") {
		t.Errorf("expected execution result, got %q", resp2)
	}
	if len(sess.operations) != 1 {
		t.Errorf("expected 1 operation record, got %d", len(sess.operations))
	}
}

func TestSessionConfirmNo(t *testing.T) {
	agent := newTestAgentForRoute([]string{
		"exec",
		"```json\n{\"tool_calls\":[{\"name\":\"execute_command\",\"arguments\":{\"command\":\"uptime\",\"nodes\":[\"node1\"]}}]}\n```",
	})
	sess := NewSession(agent)
	ctx := context.Background()

	if _, err := sess.Send(ctx, "执行 uptime"); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	resp, err := sess.Send(ctx, "否")
	if err != nil {
		t.Fatalf("Send cancel failed: %v", err)
	}
	if resp != "已取消该操作" {
		t.Errorf("expected cancel message, got %q", resp)
	}
	if sess.pendingContext != nil {
		t.Error("expected pending context to be cleared after cancel")
	}
	if len(sess.operations) != 0 {
		t.Errorf("expected no operation recorded after cancel, got %d", len(sess.operations))
	}
}

func TestSessionPendingBlocksNewRequest(t *testing.T) {
	agent := newTestAgentForRoute([]string{
		"exec",
		"```json\n{\"tool_calls\":[{\"name\":\"execute_command\",\"arguments\":{\"command\":\"uptime\",\"nodes\":[\"node1\"]}}]}\n```",
	})
	sess := NewSession(agent)
	ctx := context.Background()

	if _, err := sess.Send(ctx, "执行 uptime"); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	resp, err := sess.Send(ctx, "再列出所有节点")
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if !strings.Contains(resp, "未确认") {
		t.Errorf("expected pending prompt, got %q", resp)
	}
	if sess.pendingContext == nil {
		t.Error("expected pending context to remain")
	}
}

func TestSessionReadOpNoConfirm(t *testing.T) {
	agent := newTestAgentForRoute([]string{
		"node",
		"```json\n{\"tool_calls\":[{\"name\":\"query_nodes\",\"arguments\":{}}]}\n```",
	})
	sess := NewSession(agent)
	ctx := context.Background()

	resp, err := sess.Send(ctx, "列出所有节点")
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if sess.pendingContext != nil {
		t.Error("read-only operation should not require confirmation")
	}
	if resp == "我不确定您要做什么" {
		t.Errorf("expected node query result, got %q", resp)
	}
}

func TestSessionOperationMemory(t *testing.T) {
	agent := newTestAgentForRoute([]string{
		"exec",
		"```json\n{\"tool_calls\":[{\"name\":\"execute_command\",\"arguments\":{\"command\":\"uptime\",\"nodes\":[\"node1\"]}}]}\n```",
	})
	sess := NewSession(agent)
	ctx := context.Background()

	if _, err := sess.Send(ctx, "执行 uptime"); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if _, err := sess.Send(ctx, "是"); err != nil {
		t.Fatalf("Send confirm failed: %v", err)
	}

	mem := sess.operationMemory()
	if !strings.Contains(mem, "execute_command") {
		t.Errorf("expected operation memory to contain execute_command, got %q", mem)
	}
	if !strings.Contains(mem, "uptime") {
		t.Errorf("expected operation memory to contain command detail, got %q", mem)
	}
}

func TestSummarizeToolCall(t *testing.T) {
	call := ToolCall{
		Name: "node_remove",
		Arguments: map[string]interface{}{
			"nodes": []interface{}{"node1", "node2"},
		},
	}
	s := SummarizeToolCall(call)
	if !strings.Contains(s, "node_remove") || !strings.Contains(s, "node1") {
		t.Errorf("expected summary to contain tool and params, got %q", s)
	}
}

func TestSingleShotModeRejectsWriteOp(t *testing.T) {
	agent := newTestAgentForRoute([]string{
		"exec",
		"```json\n{\"tool_calls\":[{\"name\":\"execute_command\",\"arguments\":{\"command\":\"uptime\",\"nodes\":[\"node1\"]}}]}\n```",
	})
	agent.SetConfirmGate(RejectWriteOpsGate())

	resp, err := agent.Process(context.Background(), "执行 uptime", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if !strings.Contains(resp, "交互模式") {
		t.Errorf("expected rejection hint mentioning interactive mode, got %q", resp)
	}
}
