package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/internal/common/model"
)

// 场景 5 端到端语义验证（fake ChatModel 脚本化）：
// "帮我修复告警机器" → 先列节点+给方案+等待确认 → 用户确认 → 执行工具被
// 确认门拦截 → 再次确认 → 真正重放执行。全程不依赖真实 LLM。

type fixingAlertExecutor struct {
	stubAlertExecutor
	execCommands []string
	execNodes    []string
}

func (f *fixingAlertExecutor) ExecuteCommand(ctx context.Context, p ExecCommandParams) (*ExecResult, error) {
	f.execCommands = append(f.execCommands, p.Command)
	f.execNodes = append(f.execNodes, strings.Join(p.Nodes, ","))
	return &ExecResult{Text: "命令执行输出: ok"}, nil
}

func newFixingAgent(t *testing.T) (*Agent, *fixingAlertExecutor) {
	t.Helper()
	fx := &fixingAlertExecutor{stubAlertExecutor: stubAlertExecutor{
		listRows: []AlertRow{
			{ID: "AL-1", TypeID: "OWL-DSK-001", NodeID: "n1", NodeName: "web-01", Severity: "warn", Status: "open", Message: "磁盘使用率过高", FirstSeen: 1758247200, LastSeen: 1758252600},
		},
		listTotal: 1,
		remedyTypeRows: []AlertTypeRow{
			{ID: "OWL-DSK-001", Category: "disk", Name: "磁盘使用率过高", DefaultSeverity: "warn", Rule: "disk.usage > 90"},
		},
		remedyRows: []RemedyRow{
			{ID: "R-1", Name: "磁盘排查指引", Kind: "sop", Risk: "low", Content: "1. df -h\n2. du -h --max-depth=1 /", Rollback: "无需回滚"},
		},
	}}
	mgr := &mockNodeMgrForAI{nodes: []*model.Node{{Name: "web-01", Address: "127.0.0.1", Port: 22}}}
	agent, err := NewAgent(fx, &Config{}, mgr, nil, nil)
	if err != nil {
		t.Fatalf("NewAgent failed: %v", err)
	}
	agent.SetChatModel(&mockChatModel{responses: []string{
		"alert_remedy", // 路由标签
		"```json\n{\"tool_calls\":[{\"name\":\"alert_list\",\"arguments\":{\"alert_type_id\":\"OWL-DSK-001\"}}]}\n```",
		"```json\n{\"tool_calls\":[{\"name\":\"alert_remedy\",\"arguments\":{\"alert_type_id\":\"OWL-DSK-001\"}}]}\n```",
		"受影响节点：web-01（OWL-DSK-001，warn）。\n修复步骤：df -h 排查后清理。\n回滚方案：无需回滚。\n是否由我执行修复？回复\"是\"开始。",
		// 用户确认后的下一轮：发起修复命令（将被确认门拦截）
		"```json\n{\"tool_calls\":[{\"name\":\"execute_command\",\"arguments\":{\"command\":\"df -h\",\"nodes\":[\"web-01\"]}}]}\n```",
	}})
	return agent, fx
}

func TestAlertFixFlowWithConfirmation(t *testing.T) {
	agent, fx := newFixingAgent(t)
	sess := NewSession(agent)
	sess.SetDefaultConfirmGate()
	ctx := context.Background()

	// 第一轮：列节点 + 给方案 + 请求确认（不执行）
	resp1, err := sess.Send(ctx, "帮我修复OWL-DSK-001告警的机器")
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	for _, want := range []string{"web-01", "df -h", "回滚", "是否由我执行修复"} {
		if !strings.Contains(resp1, want) {
			t.Errorf("plan reply missing %q:\n%s", want, resp1)
		}
	}
	if len(fx.execCommands) != 0 {
		t.Errorf("plan stage must not execute anything, got %v", fx.execCommands)
	}
	if sess.pendingContext != nil {
		t.Errorf("plan stage should not create pending confirmation, got %+v", sess.pendingContext)
	}

	// 第二轮：用户确认 → 发起 execute_command → 确认门拦截
	resp2, err := sess.Send(ctx, "是")
	if err != nil {
		t.Fatalf("Send confirm failed: %v", err)
	}
	if !strings.Contains(resp2, "是否继续") {
		t.Errorf("expected tool-level confirm question, got %q", resp2)
	}
	if sess.pendingContext == nil || sess.pendingContext.ToolCall.Name != "execute_command" {
		t.Fatalf("expected pending execute_command, got %+v", sess.pendingContext)
	}
	if len(fx.execCommands) != 0 {
		t.Errorf("command must not run before final confirm, got %v", fx.execCommands)
	}

	// 第三轮：最终确认 → 重放执行
	resp3, err := sess.Send(ctx, "是")
	if err != nil {
		t.Fatalf("Send final confirm failed: %v", err)
	}
	if !strings.Contains(resp3, "已执行") {
		t.Errorf("expected executed reply, got %q", resp3)
	}
	if len(fx.execCommands) != 1 || fx.execCommands[0] != "df -h" {
		t.Errorf("expected df -h executed once, got %v", fx.execCommands)
	}
	if len(fx.execNodes) != 1 || fx.execNodes[0] != "web-01" {
		t.Errorf("expected target web-01, got %v", fx.execNodes)
	}
	if sess.pendingContext != nil {
		t.Error("pending context should be cleared after execution")
	}
}

func TestAlertFixFlowReject(t *testing.T) {
	agent, fx := newFixingAgent(t)
	// "否"轮由 LLM 理解并取消：追加一条纯文本回复（不发工具调用）
	agent.SetChatModel(&mockChatModel{responses: []string{
		"alert_remedy",
		"```json\n{\"tool_calls\":[{\"name\":\"alert_list\",\"arguments\":{\"alert_type_id\":\"OWL-DSK-001\"}}]}\n```",
		"```json\n{\"tool_calls\":[{\"name\":\"alert_remedy\",\"arguments\":{\"alert_type_id\":\"OWL-DSK-001\"}}]}\n```",
		"受影响节点：web-01。\n是否由我执行修复？回复\"是\"开始。",
		"好的，已取消修复，未执行任何操作。",
		// 用户重新确认方案 → 发起执行 → 确认门拦截
		"```json\n{\"tool_calls\":[{\"name\":\"execute_command\",\"arguments\":{\"command\":\"df -h\",\"nodes\":[\"web-01\"]}}]}\n```",
	}})
	sess := NewSession(agent)
	sess.SetDefaultConfirmGate()
	ctx := context.Background()

	if _, err := sess.Send(ctx, "帮我修复OWL-DSK-001告警的机器"); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	// 用户拒绝方案：LLM 直接文本取消，不创建工具级待确认
	resp2, err := sess.Send(ctx, "否")
	if err != nil {
		t.Fatalf("Send reject failed: %v", err)
	}
	if !strings.Contains(resp2, "取消") {
		t.Errorf("expected cancel reply, got %q", resp2)
	}
	if len(fx.execCommands) != 0 {
		t.Errorf("nothing should execute after reject, got %v", fx.execCommands)
	}
	if sess.pendingContext != nil {
		t.Errorf("plan-level reject should not create pending confirmation, got %+v", sess.pendingContext)
	}

	// 重新确认方案 → 工具级确认待定
	if _, err := sess.Send(ctx, "是"); err != nil {
		t.Fatalf("Send re-confirm failed: %v", err)
	}
	if sess.pendingContext == nil {
		t.Fatal("expected pending confirmation after re-confirm")
	}
	// 工具级拒绝 → 取消执行
	if _, err := sess.Send(ctx, "否"); err != nil {
		t.Fatalf("Send tool-level reject failed: %v", err)
	}
	if len(fx.execCommands) != 0 {
		t.Errorf("reject at tool gate must cancel execution, got %v", fx.execCommands)
	}
	if sess.pendingContext != nil {
		t.Error("pending context should be cleared after reject")
	}
}
