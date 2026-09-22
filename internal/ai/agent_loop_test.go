package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/internal/common/model"
)

// mockToolCallingModel 同时实现 ChatModel 与 ToolCallingChatModel：
// 路由阶段走 Generate 文本队列，工具生成阶段走 GenerateTools 响应队列。
type mockToolCallingModel struct {
	routeResponses []string
	routeIdx       int
	toolResponses  []*ModelResponse
	toolIdx        int
	toolErr        error // 非空时 GenerateTools 首次返回该错误后清空（模拟 provider 不支持）
	toolMsgs       [][]Message
	toolsReceived  [][]ToolDef
}

func (m *mockToolCallingModel) Generate(ctx context.Context, messages []Message) (string, error) {
	if m.routeIdx >= len(m.routeResponses) {
		return "", fmt.Errorf("mock: no more route responses")
	}
	r := m.routeResponses[m.routeIdx]
	m.routeIdx++
	return r, nil
}

func (m *mockToolCallingModel) GenerateTools(ctx context.Context, messages []Message, tools []ToolDef) (*ModelResponse, error) {
	m.toolMsgs = append(m.toolMsgs, append([]Message{}, messages...))
	m.toolsReceived = append(m.toolsReceived, tools)
	if m.toolErr != nil {
		// 持续返回：真实 provider 的“不支持原生 tools”不会一次性消失
		return nil, m.toolErr
	}
	if m.toolIdx >= len(m.toolResponses) {
		return nil, fmt.Errorf("mock: no more tool responses")
	}
	r := m.toolResponses[m.toolIdx]
	m.toolIdx++
	return r, nil
}

func newNativeTestAgent(config *Config, chatModel ChatModel) *Agent {
	mgr := &mockNodeMgrForAI{
		nodes: []*model.Node{
			{Name: "node1", Address: "127.0.0.1", Port: 22, Status: "online"},
		},
	}
	agent, _ := NewAgent(nil, config, mgr, nil, nil)
	agent.SetChatModel(chatModel)
	return agent
}

func toolCallResponse(calls ...ToolCall) *ModelResponse {
	return &ModelResponse{FinishReason: "tool_calls", ToolCalls: calls}
}

func textResponse(content string) *ModelResponse {
	return &ModelResponse{FinishReason: "stop", Content: content}
}

// TestAgentNativeToolCalling_MultiTurn 原生 function calling 下，工具执行后
// 必须继续循环由 LLM 总结，最终回复是总结文本而非工具原始输出。
func TestAgentNativeToolCalling_MultiTurn(t *testing.T) {
	m := &mockToolCallingModel{
		routeResponses: []string{"node_list"},
		toolResponses: []*ModelResponse{
			toolCallResponse(ToolCall{Name: "query_nodes", Arguments: map[string]interface{}{}}),
			textResponse("查询完成：共 1 个节点 node1，状态 online。"),
		},
	}
	agent := newNativeTestAgent(&Config{}, m)

	reply, err := agent.Process(context.Background(), "列出所有节点", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if reply != "查询完成：共 1 个节点 node1，状态 online。" {
		t.Fatalf("expected summary reply, got %q", reply)
	}

	// 第二轮 GenerateTools 收到的消息必须包含 role=tool 的结果回注
	if len(m.toolMsgs) < 2 {
		t.Fatalf("expected 2 GenerateTools calls, got %d", len(m.toolMsgs))
	}
	var sawToolResult bool
	for _, msg := range m.toolMsgs[1] {
		if msg.Role == "tool" && strings.Contains(msg.Content, "node1") {
			sawToolResult = true
		}
	}
	if !sawToolResult {
		t.Error("expected tool result message fed back in second round")
	}
}

// TestAgentTextProtocol_MultiTurnSummary 文本协议下同样多轮：首轮工具调用、
// 次轮 LLM 总结——turn==0 早退已被移除（summarize 默认开启）。
func TestAgentTextProtocol_MultiTurnSummary(t *testing.T) {
	toolCallJSON := "```json\n" + `{"tool_calls":[{"name":"query_nodes","arguments":{}}]}` + "\n```"
	m := &mockChatModel{responses: []string{"node_list", toolCallJSON, "文本协议总结：节点正常。"}}
	agent := newNativeTestAgent(&Config{}, m)

	reply, err := agent.Process(context.Background(), "列出所有节点", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if reply != "文本协议总结：节点正常。" {
		t.Fatalf("expected summary reply, got %q", reply)
	}
}

// TestAgentSummarizeDisabled_KeepsEarlyReturn summarize_after_tool=false 时
// 保留旧行为：首轮工具结果直接返回，不额外调用 LLM。
func TestAgentSummarizeDisabled_KeepsEarlyReturn(t *testing.T) {
	disabled := false
	toolCallJSON := "```json\n" + `{"tool_calls":[{"name":"query_nodes","arguments":{}}]}` + "\n```"
	m := &mockChatModel{responses: []string{"node_list", toolCallJSON}}
	cfg := &Config{}
	cfg.AI.SummarizeAfterTool = &disabled
	agent := newNativeTestAgent(cfg, m)

	reply, err := agent.Process(context.Background(), "列出所有节点", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if !strings.Contains(reply, "node1") {
		t.Fatalf("expected raw tool result, got %q", reply)
	}
	if m.callCount != 2 {
		t.Errorf("expected exactly 2 LLM calls (route+tools), got %d", m.callCount)
	}
}

// TestAgentNativeUnsupported_DowngradesToText provider 不支持原生 tools 时
// 自动降级文本协议，同一请求内完成，不报错。
func TestAgentNativeUnsupported_DowngradesToText(t *testing.T) {
	toolCallJSON := "```json\n" + `{"tool_calls":[{"name":"query_nodes","arguments":{}}]}` + "\n```"
	m := &mockToolCallingModel{
		routeResponses: []string{"node_list", toolCallJSON, "降级总结：完成。"},
		toolErr:        ErrToolsUnsupported,
	}
	agent := newNativeTestAgent(&Config{}, m)

	reply, err := agent.Process(context.Background(), "列出所有节点", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if reply != "降级总结：完成。" {
		t.Fatalf("expected downgraded summary reply, got %q", reply)
	}
}

// TestAgentMaxTurnsConfigured 循环轮数可配置，耗尽后返回最后一个工具结果
// （修复原 fullResponse 死代码返回空串的问题）。
func TestAgentMaxTurnsConfigured(t *testing.T) {
	m := &mockToolCallingModel{
		routeResponses: []string{"node_list"},
		toolResponses: []*ModelResponse{
			toolCallResponse(ToolCall{Name: "query_nodes", Arguments: map[string]interface{}{}}),
			toolCallResponse(ToolCall{Name: "query_nodes", Arguments: map[string]interface{}{}}),
		},
	}
	cfg := &Config{}
	cfg.AI.MaxTurns = 2
	agent := newNativeTestAgent(cfg, m)

	reply, err := agent.Process(context.Background(), "列出所有节点", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if !strings.Contains(reply, "node1") {
		t.Fatalf("expected last tool result after exhausting maxTurns, got %q", reply)
	}
	if len(m.toolMsgs) != 2 {
		t.Errorf("expected exactly 2 tool rounds, got %d", len(m.toolMsgs))
	}
}

// TestAgentNativeToolsOff disables native protocol entirely via config.
func TestAgentNativeToolsOff(t *testing.T) {
	toolCallJSON := "```json\n" + `{"tool_calls":[{"name":"query_nodes","arguments":{}}]}` + "\n```"
	m := &mockToolCallingModel{
		routeResponses: []string{"node_list", toolCallJSON, "关闭原生后的总结。"},
	}
	cfg := &Config{}
	cfg.AI.NativeTools = "off"
	agent := newNativeTestAgent(cfg, m)

	reply, err := agent.Process(context.Background(), "列出所有节点", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if reply != "关闭原生后的总结。" {
		t.Fatalf("unexpected reply %q", reply)
	}
	if len(m.toolMsgs) != 0 {
		t.Error("GenerateTools must not be called when native_tools=off")
	}
}

// TestAgentNativeToolsUnsupportedErrorSurfaces 非 4xx 的工具调用错误必须上抛。
func TestAgentNativeToolsUnsupportedErrorSurfaces(t *testing.T) {
	m := &mockToolCallingModel{
		routeResponses: []string{"node_list"},
		toolErr:        errors.New("connection refused"),
	}
	agent := newNativeTestAgent(&Config{}, m)

	_, err := agent.Process(context.Background(), "列出所有节点", nil)
	if err == nil {
		t.Fatal("expected error to surface")
	}
	if errors.Is(err, ErrToolsUnsupported) {
		t.Error("plain error must not be classified as tools-unsupported")
	}
}

// deltaRecorder 包装 mockToolCallingModel：实现流式 FC 接口，
// 仅在第二轮（总结轮）回调文本增量。
type deltaRecorder struct {
	inner   *mockToolCallingModel
	callIdx int
}

func (d *deltaRecorder) Generate(ctx context.Context, messages []Message) (string, error) {
	return d.inner.Generate(ctx, messages)
}
func (d *deltaRecorder) GenerateTools(ctx context.Context, messages []Message, tools []ToolDef) (*ModelResponse, error) {
	return d.inner.GenerateTools(ctx, messages, tools)
}
func (d *deltaRecorder) GenerateToolsStream(ctx context.Context, messages []Message, tools []ToolDef, onDelta func(string)) (*ModelResponse, error) {
	d.callIdx++
	if d.callIdx >= 2 && onDelta != nil {
		onDelta("总结增量A")
		onDelta("总结增量B")
	}
	return d.inner.GenerateTools(ctx, messages, tools)
}

// TestAgentNativeStreamDeltaForwarding 原生模式下总结轮的文本增量必须经
// OnProgress("delta", ...) 转发给宿主（Web SSE / CLI 终端）。
func TestAgentNativeStreamDeltaForwarding(t *testing.T) {
	inner := &mockToolCallingModel{
		routeResponses: []string{"node_list"},
		toolResponses: []*ModelResponse{
			toolCallResponse(ToolCall{Name: "query_nodes", Arguments: map[string]interface{}{}}),
			textResponse("总结增量A总结增量B"),
		},
	}
	recorder := &deltaRecorder{inner: inner}
	agent := newNativeTestAgent(&Config{}, recorder)

	var deltas []string
	reply, err := agent.Process(context.Background(), "列出所有节点", func(step, detail string) {
		if step == "delta" {
			deltas = append(deltas, detail)
		}
	})
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if len(deltas) != 2 || deltas[0] != "总结增量A" || deltas[1] != "总结增量B" {
		t.Fatalf("expected deltas forwarded, got %v", deltas)
	}
	if reply != "总结增量A总结增量B" {
		t.Fatalf("expected full summary reply, got %q", reply)
	}
}

// streamableTextMock 为文本协议客户端补充流式接口（拆整段为单次增量）
type streamableTextMock struct {
	inner *mockChatModel
}

func (s *streamableTextMock) Generate(ctx context.Context, messages []Message) (string, error) {
	return s.inner.Generate(ctx, messages)
}

func (s *streamableTextMock) GenerateStream(ctx context.Context, messages []Message, onDelta func(string)) (string, error) {
	resp, err := s.inner.Generate(ctx, messages)
	if err != nil {
		return "", err
	}
	if onDelta != nil {
		onDelta(resp)
	}
	return resp, nil
}

// TestAgentTextProtocolStreamDeltaForwarding 文本协议下 GenerateStream 的增量同样转发，
// 且最终回复仍是完整总结。
func TestAgentTextProtocolStreamDeltaForwarding(t *testing.T) {
	toolCallJSON := "```json\n" + `{"tool_calls":[{"name":"query_nodes","arguments":{}}]}` + "\n```"
	m := &mockChatModel{responses: []string{"node_list", toolCallJSON, "文本总结"}}
	agent := newNativeTestAgent(&Config{}, &streamableTextMock{inner: m})

	var deltas []string
	reply, err := agent.Process(context.Background(), "列出所有节点", func(step, detail string) {
		if step == "delta" {
			deltas = append(deltas, detail)
		}
	})
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if reply != "文本总结" {
		t.Fatalf("unexpected reply %q", reply)
	}
	if len(deltas) < 1 || deltas[len(deltas)-1] != "文本总结" {
		t.Fatalf("expected summary delta forwarded, got %v", deltas)
	}
}

// TestAgentRouteUncertainFallsBackToConversation 路由 uncertain 时降级为直接对话：
// LLM 文本直接回答（不再收口为"我不确定您要做什么"），仍保留调工具的能力。
func TestAgentRouteUncertainFallsBackToConversation(t *testing.T) {
	m := &mockChatModel{responses: []string{
		"uncertain",
		"你好！我是 owl 智能运维助手，可以帮你查询节点、执行命令、管理剧本等。",
	}}
	agent := newNativeTestAgent(&Config{}, m)

	reply, err := agent.Process(context.Background(), "你是谁", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if reply != "你好！我是 owl 智能运维助手，可以帮你查询节点、执行命令、管理剧本等。" {
		t.Fatalf("expected conversational fallback reply, got %q", reply)
	}
}

// TestAgentRouteInvalidLabelFallsBack 无效标签（重试仍失败）同样降级对话而非硬报错。
func TestAgentRouteInvalidLabelFallsBack(t *testing.T) {
	m := &mockChatModel{responses: []string{
		"这不是一个有效标签",
		"这也不是有效标签",
		"当前共有 107 个节点。",
	}}
	agent := newNativeTestAgent(&Config{}, m)

	reply, err := agent.Process(context.Background(), "随便说点什么", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if reply != "当前共有 107 个节点。" {
		t.Fatalf("expected fallback reply, got %q", reply)
	}
}

// TestAgentRouteFallbackCanStillCallTools 降级对话模式保留工具调用能力。
func TestAgentRouteFallbackCanStillCallTools(t *testing.T) {
	toolCallJSON := "```json\n" + `{"tool_calls":[{"name":"query_nodes","arguments":{}}]}` + "\n```"
	m := &mockChatModel{responses: []string{
		"uncertain",
		toolCallJSON,
		"查询完成。",
	}}
	agent := newNativeTestAgent(&Config{}, m)

	reply, err := agent.Process(context.Background(), "列出所有节点", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	// 三条 mock 全部消费 = 路由降级 → 工具调用 → 总结，链路完整
	if reply != "查询完成。" {
		t.Fatalf("expected summary reply from full loop, got %q", reply)
	}
	if m.callCount != 3 {
		t.Fatalf("expected 3 LLM calls (route+tool+summary), got %d", m.callCount)
	}
}
