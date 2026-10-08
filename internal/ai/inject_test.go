package ai

import (
	"context"
	"fmt"
	"strings"
	"testing"

	model "github.com/cangyunye/go-owl/internal/common/model"
)

// ---- 测试替身：可控输出的 Executor ----

type stubExecutor struct {
	Executor
	queryResult   string
	commandResult string
}

func (e *stubExecutor) QueryNodes(ctx context.Context, p QueryNodesParams) (*QueryNodesResult, error) {
	return &QueryNodesResult{Text: e.queryResult}, nil
}

func (e *stubExecutor) ExecuteCommand(ctx context.Context, p ExecCommandParams) (*ExecResult, error) {
	return &ExecResult{Text: e.commandResult}, nil
}

func newStubAgent(config *Config, chatModel ChatModel, exec Executor) *Agent {
	mgr := &mockNodeMgrForAI{
		nodes: []*model.Node{
			{Name: "node1", Address: "127.0.0.1", Port: 22, Status: "online"},
		},
	}
	agent, _ := NewAgent(exec, config, mgr, nil, nil)
	agent.SetChatModel(chatModel)
	// 测试放行确认门（零值 = 不拦截）
	agent.SetConfirmGate(func(ToolCall) ConfirmationDecision { return ConfirmationDecision{} })
	return agent
}

func boolPtr(b bool) *bool { return &b }

func makeLargeTable(rows int) string {
	var b strings.Builder
	b.WriteString("ID    Name    Status    Groups\n")
	b.WriteString("----  ----    ------    ------\n")
	for i := 0; i < rows; i++ {
		fmt.Fprintf(&b, "node-%03d  server-%03d  online  web,prod,env=staging,tier=backend-xxxxx\n", i, i)
	}
	return b.String()
}

// flattenMsgs 拼接全部注入消息内容,便于断言
func flattenMsgs(msgs [][]Message) string {
	var b strings.Builder
	for _, batch := range msgs {
		for _, m := range batch {
			b.WriteString(m.Content)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// ---- 1. 默认零限制:工具结果全量回注,无截断 ----

func TestAgentFullInjectByDefault(t *testing.T) {
	big := makeLargeTable(200) // > 8KB(旧预算)
	m := &mockToolCallingModel{
		routeResponses: []string{"exec_run"},
		toolResponses: []*ModelResponse{
			toolCallResponse(ToolCall{Name: "execute_command", Arguments: map[string]interface{}{"command": "cat big.log"}}),
			textResponse("日志分析完成。"),
		},
	}
	agent := newStubAgent(&Config{}, m, &stubExecutor{commandResult: big})

	reply, err := agent.Process(context.Background(), "分析这个大文件", nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if reply != "日志分析完成。" {
		t.Fatalf("unexpected reply: %q", reply)
	}
	injected := flattenMsgs(m.toolMsgs)
	if !strings.Contains(injected, big) {
		t.Fatalf("默认必须全量回注工具结果(零截断): 注入 %d 字节, 原文 %d 字节", len(injected), len(big))
	}
	if strings.Contains(injected, "中间省略") {
		t.Fatal("默认模式不允许出现截断标记")
	}
}

// ---- 2. 分页仅在显式 context_size 时启用 ----

func TestAgentPagingOnlyWhenContextSizeSet(t *testing.T) {
	big := makeLargeTable(200)

	// context_size 未设置: 不分页, 单次全量回注
	m0 := &mockToolCallingModel{
		routeResponses: []string{"exec_run"},
		toolResponses: []*ModelResponse{
			toolCallResponse(ToolCall{Name: "execute_command", Arguments: map[string]interface{}{"command": "cat big.log"}}),
			textResponse("完成"),
		},
	}
	agent0 := newStubAgent(&Config{}, m0, &stubExecutor{commandResult: big})
	if _, err := agent0.Process(context.Background(), "分析", nil); err != nil {
		t.Fatal(err)
	}
	if got := flattenMsgs(m0.toolMsgs); strings.Contains(got, "第 1/") {
		t.Fatal("未设置 context_size 不得分页")
	}

	// context_size 设置后: 超限结果逐页注入(≈7.3KB / 4096B → 恰好 2 页)。
	// 注入序列:首页随工具轮 → 模型确认 → 末页+结论指令同轮 → 模型出终稿。
	pageSize := 4096
	m1 := &mockToolCallingModel{
		routeResponses: []string{"exec_run"},
		toolResponses: []*ModelResponse{
			toolCallResponse(ToolCall{Name: "execute_command", Arguments: map[string]interface{}{"command": "cat big.log"}}),
			textResponse("第1页收到"),
			textResponse("分析结论:一切正常。"),
		},
	}
	big = makeLargeTable(100)
	agent1 := newStubAgent(&Config{AI: AIConfig{ContextSize: pageSize}}, m1, &stubExecutor{commandResult: big})
	reply, err := agent1.Process(context.Background(), "分析", nil)
	if err != nil {
		t.Fatal(err)
	}
	if reply != "分析结论:一切正常。" {
		t.Fatalf("末页后的响应才是终稿, got %q", reply)
	}
	injected := flattenMsgs(m1.toolMsgs)
	if !strings.Contains(injected, "第 1/") || !strings.Contains(injected, "第 2/") {
		t.Fatalf("缺少分页标记:\n%s", injected[:400])
	}
	// 分页模式注入的是紧凑编码且被页标记分隔,按行校验数据完整送达
	for _, line := range strings.Split(compactForContext(big), "\n") {
		if line != "" && !strings.Contains(injected, line) {
			t.Fatalf("分页模式丢失数据行: %q", line)
		}
	}
}

// ---- 3. 分页上限兜底:超 maxResultPages 退回截断 + 告知 ----

func TestAgentPagingCapFallback(t *testing.T) {
	big := makeLargeTable(300) // 多页
	m := &mockToolCallingModel{
		routeResponses: []string{"exec_run"},
		toolResponses: []*ModelResponse{
			toolCallResponse(ToolCall{Name: "execute_command", Arguments: map[string]interface{}{"command": "cat big.log"}}),
			textResponse("汇总完成。"),
		},
	}
	agent := newStubAgent(&Config{AI: AIConfig{ContextSize: 1024, MaxResultPages: 2}}, m, &stubExecutor{commandResult: big})

	reply, err := agent.Process(context.Background(), "分析", nil)
	if err != nil {
		t.Fatal(err)
	}
	injected := flattenMsgs(m.toolMsgs)
	if !strings.Contains(injected, "中间省略") {
		t.Fatalf("超页数上限必须退回首尾截断; injected tail: %q", injected[max(0, len(injected)-300):])
	}
	if !strings.Contains(reply, "超出上下文预算") {
		t.Fatalf("兜底截断必须在回复中显式告知, got %q", reply)
	}
}

// ---- 4. 查询类首轮直出 ----

func TestAgentReadToolDirectReturn(t *testing.T) {
	table := makeLargeTable(5)
	m := &mockToolCallingModel{
		routeResponses: []string{"node_list"},
		toolResponses: []*ModelResponse{
			toolCallResponse(ToolCall{Name: "query_nodes", Arguments: map[string]interface{}{}}),
		},
	}
	agent := newStubAgent(&Config{}, m, &stubExecutor{queryResult: table})

	reply, err := agent.Process(context.Background(), "列出所有节点", nil)
	if err != nil {
		t.Fatalf("直出路径不应需要第二次 LLM 调用: %v", err)
	}
	if reply != table {
		t.Fatalf("查询类工具首轮必须直出原始结果, got %q", reply)
	}
	if len(m.toolMsgs) != 1 {
		t.Fatalf("直出只允许一次 LLM 调用, got %d", len(m.toolMsgs))
	}
}

func TestAgentReadToolDirectReturn_Off(t *testing.T) {
	m := &mockToolCallingModel{
		routeResponses: []string{"node_list"},
		toolResponses: []*ModelResponse{
			toolCallResponse(ToolCall{Name: "query_nodes"}),
			textResponse("查询完成汇总。"),
		},
	}
	cfg := &Config{AI: AIConfig{QueryDirectReturn: boolPtr(false)}}
	agent := newStubAgent(cfg, m, &stubExecutor{queryResult: makeLargeTable(3)})

	reply, err := agent.Process(context.Background(), "列出所有节点", nil)
	if err != nil {
		t.Fatal(err)
	}
	if reply != "查询完成汇总。" {
		t.Fatalf("关闭直出后应走总结循环, got %q", reply)
	}
	if len(m.toolMsgs) != 2 {
		t.Fatalf("关闭直出应有两次 LLM 调用, got %d", len(m.toolMsgs))
	}
}

// ---- 5. 辅助函数单元测试 ----

func TestSplitResultPages(t *testing.T) {
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = strings.Repeat("x", 100)
	}
	s := strings.Join(lines, "\n")
	pages := splitResultPages(s, 250)
	// 不变量 1:各页拼接 == 规范化原文(每行带换行)
	if got := strings.Join(pages, ""); got != s+"\n" {
		t.Fatalf("切页重组必须无损还原: got %d bytes, want %d", len(got), len(s)+1)
	}
	// 不变量 2:常规页不超过页宽
	for i, p := range pages {
		if len(p) > 250 {
			t.Fatalf("第 %d 页超宽: %d bytes", i, len(p))
		}
	}
	// 不变量 3:单行超页宽硬切,拼接无损且不产生超页
	huge := strings.Repeat("y", 1000)
	hp := splitResultPages(huge, 300)
	if strings.Join(hp, "") != huge {
		t.Fatal("硬切重组必须还原原文")
	}
	for i, p := range hp {
		if len(p) > 300 {
			t.Fatalf("硬切第 %d 页超宽: %d bytes", i, len(p))
		}
	}
}

func TestCompactForContext(t *testing.T) {
	in := "ID         Name       Address\n" +
		"node-01    web-01     10.0.0.1:22\n"
	out := compactForContext(in)
	if strings.Contains(out, "  ") {
		t.Fatalf("连续空格必须压缩为单空格, got %q", out)
	}
	if !strings.Contains(out, "node-01 web-01 10.0.0.1:22") {
		t.Fatalf("字段内容必须保留, got %q", out)
	}
	cjk := compactForContext("节点    状态\nweb-01  在线")
	if !strings.Contains(cjk, "节点 状态") || !strings.Contains(cjk, "web-01 在线") {
		t.Fatalf("CJK 字段必须保留, got %q", cjk)
	}
}

func TestTableTrimsTrailingPadding(t *testing.T) {
	tool := NewQueryNodesTool(nil, nil, nil)
	table := tool.formatAsTable([]*model.Node{{
		ID: "node-1", Name: "web-01", Address: "10.0.0.1", Port: 22,
		Status: model.NodeStatusOnline, Groups: []string{"web"},
	}})
	for i, line := range strings.Split(table, "\n") {
		if line != strings.TrimRight(line, " \t") {
			t.Fatalf("第 %d 行存在行尾填充空白: %q", i, line)
		}
	}
	if !strings.Contains(table, "node-1") {
		t.Fatal("表格内容丢失")
	}
}
