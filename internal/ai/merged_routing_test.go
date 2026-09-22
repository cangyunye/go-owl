package ai

import (
	"context"
	"strings"
	"testing"
)

// TestProcessMergedRouting_SingleCall：原生 FC 下路由与工具选择合并为一次
// LLM 调用——工具被调用（usedTools）即成功，不再消耗独立的路由往返。
func TestProcessMergedRouting_SingleCall(t *testing.T) {
	m := &mockToolCallingModel{
		toolResponses: []*ModelResponse{
			toolCallResponse(ToolCall{Name: "query_nodes", Arguments: map[string]interface{}{}}),
			textResponse("共 1 个节点。"),
		},
	}
	agent := newNativeTestAgent(&Config{}, m)
	resp, err := agent.Process(context.Background(), "列出所有节点", nil)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if m.routeIdx != 0 {
		t.Fatalf("route phase must be skipped in merged mode, route calls=%d", m.routeIdx)
	}
	if m.toolIdx < 1 {
		t.Fatalf("expected tool generation to happen, tool calls=%d", m.toolIdx)
	}
	if !strings.Contains(resp, "1 个节点") && !strings.Contains(resp, "query_nodes") {
		t.Fatalf("unexpected reply: %q", resp)
	}
}

// TestProcessMergedRouting_ChitChat：闲聊在合并模式下由首次调用直接回答，
// 无需路由+对话两次往返。
func TestProcessMergedRouting_ChitChat(t *testing.T) {
	m := &mockToolCallingModel{
		toolResponses: []*ModelResponse{
			textResponse("你好，我是 owl 运维助手。"),
		},
	}
	agent := newNativeTestAgent(&Config{}, m)
	resp, err := agent.Process(context.Background(), "你好", nil)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if m.routeIdx != 0 {
		t.Fatalf("chitchat must not spend a route call, got %d", m.routeIdx)
	}
	if resp != "你好，我是 owl 运维助手。" {
		t.Fatalf("unexpected reply %q", resp)
	}
}

// TestProcessMergedRouting_FallbackToTwoPhase：合并模式未能产出工具调用
// 且无直接回答时，回退既有两段式（此时路由阶段才发生）。
func TestProcessMergedRouting_FallbackToTwoPhase(t *testing.T) {
	m := &mockToolCallingModel{
		toolResponses: []*ModelResponse{
			textResponse("   "), // 合并模式：空文本 → 强制重试也空 → 指引文案 → 触发回退
			textResponse("   "),
		},
		routeResponses: []string{"sample"}, // 两段式路由：不受支持类别
	}
	agent := newNativeTestAgent(&Config{}, m)
	resp, err := agent.Process(context.Background(), "生成示例", nil)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if m.routeIdx == 0 {
		t.Fatal("expected fallback to legacy two-phase routing")
	}
	if resp != "该功能不支持 AI 操作" {
		t.Fatalf("expected legacy unsupported message, got %q", resp)
	}
}
