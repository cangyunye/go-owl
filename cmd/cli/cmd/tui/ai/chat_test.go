package ai

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	owlavi "github.com/cangyunye/go-owl/internal/ai"
)

func runeKey(r rune) tea.Msg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func key(t tea.KeyType) tea.Msg { return tea.KeyMsg{Type: t} }

type fakeSender struct {
	fn func(ctx context.Context, input string) (string, error)
}

func (f fakeSender) Send(ctx context.Context, input string) (string, error) {
	if f.fn == nil {
		return "", nil
	}
	return f.fn(ctx, input)
}

func newChat(t *testing.T) Model {
	t.Helper()
	old := newSessionFn
	t.Cleanup(func() { newSessionFn = old })
	newSessionFn = func(store common.NodeStore) (*owlavi.Session, *owlavi.Config, error) {
		return nil, &owlavi.Config{AI: owlavi.AIConfig{Provider: "openai", Model: "gpt-4o"}}, nil
	}
	store := common.NewInMemoryNodeStoreAt(filepath.Join(t.TempDir(), "nodes.json"))
	return NewModel(store)
}

func TestChat_DefaultState(t *testing.T) {
	store := common.NewInMemoryNodeStoreAt("")
	m := NewModel(store)
	if m.InsertMode() {
		t.Fatal("should start in normal mode")
	}
	if p := m.Path(); len(p) != 1 || p[0] != "ai" {
		t.Fatalf("unexpected path: %v", p)
	}
	if m.IsDirty() {
		t.Fatal("AI panel never dirty")
	}
	if got := m.View(); got == "" {
		t.Fatal("view should not be empty")
	}
}

func TestChat_EnterToInsertAndSend(t *testing.T) {
	m := newChat(t)
	m.sender = fakeSender{fn: func(ctx context.Context, input string) (string, error) {
		return "回答: " + input, nil
	}}

	nm, _ := m.Update(runeKey('i'))
	m = nm.(Model)
	if !m.InsertMode() {
		t.Fatal("expected insert mode after 'i'")
	}

	nm, _ = m.Update(runeKey('a'))
	m = nm.(Model)
	nm, _ = m.Update(runeKey('b'))
	m = nm.(Model)

	nm, cmd := m.Update(key(tea.KeyEnter))
	m = nm.(Model)
	if !m.busy {
		t.Fatal("expected busy after send")
	}
	if len(m.messages) != 1 || m.messages[0].Role != "user" || m.messages[0].Content != "ab" {
		t.Fatalf("user message missing: %+v", m.messages)
	}

	msgs := execCmd(cmd)
	var done ChatDoneMsg
	for _, msg := range msgs {
		if d, ok := msg.(ChatDoneMsg); ok {
			done = d
		}
	}
	m = feed(m, msgs...)
	if done.Text != "回答: ab" {
		t.Fatalf("unexpected done text: %q", done.Text)
	}
	if m.busy {
		t.Fatal("expected not busy after done")
	}
	if len(m.messages) != 2 || m.messages[1].Role != "assistant" || m.messages[1].Content != "回答: ab" {
		t.Fatalf("assistant message missing: %+v", m.messages)
	}
}

func TestChat_EmptyInputIgnored(t *testing.T) {
	m := newChat(t)
	m.sender = fakeSender{}
	nm, _ := m.Update(runeKey('i'))
	m = nm.(Model)
	nm, cmd := m.Update(key(tea.KeyEnter))
	m = nm.(Model)
	if cmd != nil {
		t.Fatal("empty input should not send")
	}
	if len(m.messages) != 0 {
		t.Fatalf("no messages expected, got %+v", m.messages)
	}
}

func TestChat_BusyBlocksSecondSend(t *testing.T) {
	m := newChat(t)
	m.sender = fakeSender{}
	m.busy = true
	nm, _ := m.Update(runeKey('i'))
	m = nm.(Model)
	nm, _ = m.Update(runeKey('a'))
	m = nm.(Model)
	nm, cmd := m.Update(key(tea.KeyEnter))
	_ = nm
	if cmd != nil {
		t.Fatal("busy should block send")
	}
}

func TestChat_SendErrorShownAsAssistant(t *testing.T) {
	m := newChat(t)
	m.sender = fakeSender{fn: func(ctx context.Context, input string) (string, error) {
		return "", fmt.Errorf("网络错误")
	}}
	nm, _ := m.Update(runeKey('i'))
	m = nm.(Model)
	nm, _ = m.Update(runeKey('x'))
	m = nm.(Model)
	nm, cmd := m.Update(key(tea.KeyEnter))
	m = nm.(Model)
	m = feed(m, execCmd(cmd)...)
	if len(m.messages) != 2 || !strings.Contains(m.messages[1].Content, "网络错误") {
		t.Fatalf("error not rendered: %+v", m.messages)
	}
}

func TestChat_ConfirmQuestionFlowsAsMessage(t *testing.T) {
	m := newChat(t)
	calls := 0
	m.sender = fakeSender{fn: func(ctx context.Context, input string) (string, error) {
		calls++
		if calls == 1 {
			return "即将执行: execute_command(...)\n是否继续？（是/否）", nil
		}
		return "已执行: 完成", nil
	}}
	send := func(input string) {
		nm, _ := m.Update(runeKey('i'))
		m = nm.(Model)
		for _, r := range input {
			nm, _ = m.Update(runeKey(r))
			m = nm.(Model)
		}
		nm, cmd := m.Update(key(tea.KeyEnter))
		m = nm.(Model)
		m = feed(m, execCmd(cmd)...)
	}
	send("删除 web-1")
	if len(m.messages) != 2 || !strings.Contains(m.messages[1].Content, "是否继续") {
		t.Fatalf("confirm question not rendered: %+v", m.messages)
	}
	send("是")
	if len(m.messages) != 4 || !strings.Contains(m.messages[3].Content, "已执行") {
		t.Fatalf("replay result not rendered: %+v", m.messages)
	}
}

func TestChat_NewSessionClearsAndReassembles(t *testing.T) {
	m := newChat(t)
	m.messages = []ChatMsg{{Role: "user", Content: "x"}}
	calls := 0
	newSessionFn = func(store common.NodeStore) (*owlavi.Session, *owlavi.Config, error) {
		calls++
		return nil, &owlavi.Config{AI: owlavi.AIConfig{Provider: "openai", Model: "gpt-4o"}}, nil
	}
	nm, _ := m.Update(runeKey('n'))
	m = nm.(Model)
	if calls != 1 {
		t.Fatalf("expected 1 reassembly, got %d", calls)
	}
	if len(m.messages) != 0 {
		t.Fatalf("messages not cleared: %+v", m.messages)
	}
}

func TestChat_EscExitsInsert(t *testing.T) {
	m := newChat(t)
	nm, _ := m.Update(runeKey('i'))
	m = nm.(Model)
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(Model)
	if m.InsertMode() {
		t.Fatal("expected normal mode after esc")
	}
}

func TestChat_EscLeavesPanelFromNormal(t *testing.T) {
	m := newChat(t)
	nm, cmd := m.Update(key(tea.KeyEsc))
	_ = nm
	if cmd == nil {
		t.Fatal("esc should return a cmd")
	}
	if _, ok := cmd().(LeavePanelMsg); !ok {
		t.Fatalf("expected LeavePanelMsg, got %T", cmd())
	}
}

// ---- 流式输出 ----

// chanSink msgSink 测试桩: 收集跨 goroutine 投递的 tea.Msg。
type chanSink struct {
	ch chan tea.Msg
}

func newChanSink() *chanSink { return &chanSink{ch: make(chan tea.Msg, 64)} }

func (c *chanSink) Send(msg tea.Msg) { c.ch <- msg }

func (c *chanSink) drain() []tea.Msg {
	var msgs []tea.Msg
	for {
		select {
		case m := <-c.ch:
			msgs = append(msgs, m)
		default:
			return msgs
		}
	}
}

// typeAndSend 进 Insert 模式输入 text 并回车发送,返回发送后的 Model 与 sendCmd(未执行)。
func typeAndSend(t *testing.T, m Model, text string) (Model, tea.Cmd) {
	t.Helper()
	nm, _ := m.Update(runeKey('i'))
	m = nm.(Model)
	for _, r := range text {
		nm, _ = m.Update(runeKey(r))
		m = nm.(Model)
	}
	nm, cmd := m.Update(key(tea.KeyEnter))
	return nm.(Model), cmd
}

// execCmd 执行 tea.Cmd(递归展开 tea.Batch),返回全部消息。
func execCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var out []tea.Msg
	for _, c := range batch {
		out = append(out, execCmd(c)...)
	}
	return out
}

// feed 依次把消息喂给 Model.Update,返回最终 Model。
func feed(m Model, msgs ...tea.Msg) Model {
	for _, msg := range msgs {
		nm, _ := m.Update(msg)
		m = nm.(Model)
	}
	return m
}

func TestChat_ProgressMsgMapping(t *testing.T) {
	d, ok := progressMsg("delta", "你").(ChatDeltaMsg)
	if !ok || d.Text != "你" {
		t.Fatalf("delta 应映射 ChatDeltaMsg, got %#v", progressMsg("delta", "你"))
	}
	tp, ok := progressMsg("generate", "query_nodes").(ToolProgressMsg)
	if !ok || tp.Phase != "generate" || tp.Name != "query_nodes" {
		t.Fatalf("generate 应映射 ToolProgressMsg, got %#v", progressMsg("generate", "query_nodes"))
	}
	tp, ok = progressMsg("execute", "execute_command").(ToolProgressMsg)
	if !ok || tp.Phase != "execute" || tp.Name != "execute_command" {
		t.Fatalf("execute 应映射 ToolProgressMsg, got %#v", progressMsg("execute", "execute_command"))
	}
	for _, step := range []string{"route", "analyze", "result"} {
		if msg := progressMsg(step, "x"); msg != nil {
			t.Fatalf("step %q 应忽略, got %#v", step, msg)
		}
	}
}

func TestChat_BridgeProgressSendsToSink(t *testing.T) {
	m := newChat(t)
	sink := newChanSink()
	m.prog = sink
	m.bridgeProgress("delta", "你")
	m.bridgeProgress("route", "node_list") // 状态类事件忽略
	m.bridgeProgress("execute", "query_nodes")
	msgs := sink.drain()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 msgs, got %d: %T", len(msgs), msgs)
	}
	if d, ok := msgs[0].(ChatDeltaMsg); !ok || d.Text != "你" {
		t.Fatalf("msgs[0] = %#v, want ChatDeltaMsg{你}", msgs[0])
	}
	if tp, ok := msgs[1].(ToolProgressMsg); !ok || tp.Phase != "execute" || tp.Name != "query_nodes" {
		t.Fatalf("msgs[1] = %#v, want ToolProgressMsg{execute,query_nodes}", msgs[1])
	}
}

func TestChat_BridgeProgressNoProgNoPanic(t *testing.T) {
	m := newChat(t) // 测试桩无真实 program,prog 为 nil
	m.bridgeProgress("delta", "x")
}

func TestChat_DeltaStreamsIntoPendingLine(t *testing.T) {
	m := newChat(t)
	m.sender = fakeSender{}
	m, _ = typeAndSend(t, m, "列一下 db 组")
	if !m.busy || !m.streaming {
		t.Fatalf("send 后应 busy+streaming, busy=%v streaming=%v", m.busy, m.streaming)
	}
	if !m.tickArmed {
		t.Fatal("send 应挂起节流节拍(spinner+flush 共用)")
	}
	if len(m.messages) != 1 {
		t.Fatalf("send 后应只有 user 消息, got %+v", m.messages)
	}
	if v := m.View(); !strings.Contains(v, "▍") {
		t.Fatalf("send 后应有待位行+光标: %s", v)
	}

	nm, cmd := m.Update(ChatDeltaMsg{Text: "db 组共 "})
	m = nm.(Model)
	if cmd != nil {
		t.Fatal("节拍已挂起时 delta 不应重复挂")
	}
	nm, _ = m.Update(ChatDeltaMsg{Text: "25 个节点"})
	m = nm.(Model)
	if m.streamBuf != "db 组共 25 个节点" {
		t.Fatalf("streamBuf = %q", m.streamBuf)
	}
	if len(m.messages) != 1 {
		t.Fatalf("delta 不应进入 messages, got %+v", m.messages)
	}
	if v := m.View(); strings.Contains(v, "db 组共 25 个节点") {
		t.Fatal("未 flush 前视口不应显示增量")
	}

	nm, rearm := m.Update(FlushTickMsg{})
	m = nm.(Model)
	if v := m.View(); !strings.Contains(v, "db 组共 25 个节点") || !strings.Contains(v, "▍") {
		t.Fatalf("flush 后视口应含流式文本+光标: %s", v)
	}
	if rearm == nil {
		t.Fatal("busy 期间节拍应续订")
	}
}

func TestChat_FlushTickRefreshesViewportAndRearms(t *testing.T) {
	m := newChat(t)
	m.sender = fakeSender{}
	m, _ = typeAndSend(t, m, "hello")
	nm, _ := m.Update(ChatDeltaMsg{Text: "流式回复中"})
	m = nm.(Model)
	if v := m.View(); strings.Contains(v, "流式回复中") {
		t.Fatal("未 flush 前不应出现增量")
	}
	nm, rearm := m.Update(FlushTickMsg{})
	m = nm.(Model)
	if v := m.View(); !strings.Contains(v, "流式回复中") {
		t.Fatalf("flush 后视口应含增量: %s", v)
	}
	if rearm == nil {
		t.Fatal("busy 期间应续订 tick")
	}
	nm, _ = m.Update(ChatDoneMsg{Text: "流式回复中"})
	m = nm.(Model)
	nm, rearm2 := m.Update(rearm())
	m = nm.(Model)
	if rearm2 != nil {
		t.Fatal("busy 结束后不应续订")
	}
}

func TestChat_DoneSettlesStream(t *testing.T) {
	m := newChat(t)
	m.sender = fakeSender{fn: func(ctx context.Context, input string) (string, error) {
		return "db 组共 25 个节点", nil
	}}
	m, cmd := typeAndSend(t, m, "列一下 db 组")
	m = feed(m,
		ChatDeltaMsg{Text: "db 组共 "},
		ChatDeltaMsg{Text: "25 个"},
		ToolProgressMsg{Phase: "execute", Name: "query_nodes"},
	)
	if len(m.messages) != 2 || m.messages[1].Role != "tool" || m.messages[1].Content != "query_nodes" {
		t.Fatalf("execute 应追加工具活动行: %+v", m.messages)
	}
	m = feed(m, execCmd(cmd)...)
	if m.busy || m.streaming || m.streamBuf != "" || m.toolPhase != "" {
		t.Fatalf("流式状态未结算: busy=%v streaming=%v buf=%q phase=%q", m.busy, m.streaming, m.streamBuf, m.toolPhase)
	}
	// 工具活动行保留在对话流里,最终文本追加其后
	if len(m.messages) != 3 || m.messages[2].Content != "db 组共 25 个节点" {
		t.Fatalf("结算应用服务端最终文本: %+v", m.messages)
	}
	if v := m.View(); strings.Contains(v, "▍") {
		t.Fatalf("结算后光标应消失: %s", v)
	}
}

func TestChat_DoneErrorClearsStream(t *testing.T) {
	m := newChat(t)
	m.sender = fakeSender{fn: func(ctx context.Context, input string) (string, error) {
		return "", fmt.Errorf("网络错误")
	}}
	m, cmd := typeAndSend(t, m, "x")
	m = feed(m, ChatDeltaMsg{Text: "半截"})
	m = feed(m, execCmd(cmd)...)
	if m.streaming || m.streamBuf != "" {
		t.Fatalf("出错也应结算流式状态: streaming=%v buf=%q", m.streaming, m.streamBuf)
	}
	if len(m.messages) != 2 || !strings.Contains(m.messages[1].Content, "网络错误") {
		t.Fatalf("error not rendered: %+v", m.messages)
	}
}

func TestChat_ToolProgressShowsInStatus(t *testing.T) {
	m := newChat(t)
	m.sender = fakeSender{}
	m, _ = typeAndSend(t, m, "x")
	nm, _ := m.Update(ToolProgressMsg{Phase: "execute", Name: "query_nodes"})
	m = nm.(Model)
	if v := m.View(); !strings.Contains(v, "正在执行 query_nodes") {
		t.Fatalf("状态栏缺工具阶段: %s", v)
	}
	nm, _ = m.Update(ChatDoneMsg{Text: "ok"})
	m = nm.(Model)
	if v := m.View(); strings.Contains(v, "正在执行 query_nodes") {
		t.Fatalf("done 后 toolPhase 应清除: %s", v)
	}
}

func TestChat_NewSessionClearsStreamState(t *testing.T) {
	// 内核保证 delta 仅在 Send 进行中产生(先于 done 入队),无需 busy 门卫;
	// 这里钉住 n 键对残留流式状态的自愈。
	m := newChat(t)
	m.streaming = true
	m.streamBuf = "半截"
	m.toolPhase = "🔧 正在执行 x…"
	m.pendingFlush = true
	nm, _ := m.Update(runeKey('n'))
	m = nm.(Model)
	if m.streaming || m.streamBuf != "" || m.toolPhase != "" || m.pendingFlush {
		t.Fatalf("n 键应清流式状态: streaming=%v buf=%q phase=%q pending=%v", m.streaming, m.streamBuf, m.toolPhase, m.pendingFlush)
	}
	if v := m.View(); strings.Contains(v, "▍") {
		t.Fatalf("n 键后不应有待位行: %s", v)
	}
}

func TestChat_ScrollFollowDuringStream(t *testing.T) {
	m := newChat(t)
	m.sender = fakeSender{}
	var sb strings.Builder
	for i := 0; i < 30; i++ {
		fmt.Fprintf(&sb, "历史行 %02d\n", i)
	}
	m.messages = []ChatMsg{{Role: "assistant", Content: sb.String()}}
	m.refreshViewport()
	if !m.view.AtBottom() {
		t.Fatalf("precond: 初始应在底部, yOffset=%d", m.view.YOffset)
	}
	// 用户上翻
	m.view.SetYOffset(0)
	if m.view.AtBottom() {
		t.Fatal("precond: 应处于上翻状态")
	}
	m.busy = true
	m.streaming = true
	nm, tick := m.Update(ChatDeltaMsg{Text: "新增增量"})
	m = nm.(Model)
	nm, rearm := m.Update(tick())
	m = nm.(Model)
	if m.view.AtBottom() {
		t.Fatal("用户上翻时流式增量不应拽回底部")
	}
	// 回到底部 → 恢复跟随(流式期间节拍自续订,下一拍刷新)
	// 注: bubbles viewport 默认键位不含 end,这里直接模拟"用户已回到底部"这一状态。
	m.view.GotoBottom()
	nm, _ = m.Update(rearm())
	m = nm.(Model)
	if !m.view.AtBottom() {
		t.Fatal("回底后应恢复跟随")
	}
}

func TestChat_SendStreamsThroughBridge(t *testing.T) {
	m := newChat(t)
	sink := newChanSink()
	m.prog = sink
	m.sender = fakeSender{fn: func(ctx context.Context, input string) (string, error) {
		m.bridgeProgress("generate", "query_nodes")
		m.bridgeProgress("execute", "query_nodes")
		m.bridgeProgress("delta", "db 组共 ")
		m.bridgeProgress("delta", "25 个节点")
		return "db 组共 25 个节点", nil
	}}
	m, cmd := typeAndSend(t, m, "列一下 db 组")
	var doneMsg tea.Msg
	for _, msg := range execCmd(cmd) {
		if _, ok := msg.(ChatDoneMsg); ok {
			doneMsg = msg
		}
	}
	if doneMsg == nil {
		t.Fatal("send 消息中缺 ChatDoneMsg")
	}
	msgs := sink.drain()
	if len(msgs) != 4 {
		t.Fatalf("expected 4 progress msgs, got %d: %T", len(msgs), msgs)
	}
	if tp, ok := msgs[0].(ToolProgressMsg); !ok || tp.Phase != "generate" || tp.Name != "query_nodes" {
		t.Fatalf("msgs[0] = %#v", msgs[0])
	}
	if tp, ok := msgs[1].(ToolProgressMsg); !ok || tp.Phase != "execute" {
		t.Fatalf("msgs[1] = %#v", msgs[1])
	}
	if d, ok := msgs[2].(ChatDeltaMsg); !ok || d.Text != "db 组共 " {
		t.Fatalf("msgs[2] = %#v", msgs[2])
	}
	m = feed(m, msgs...)
	nm, _ := m.Update(FlushTickMsg{})
	m = nm.(Model)
	if v := m.View(); !strings.Contains(v, "db 组共 25 个节点") || !strings.Contains(v, "▍") {
		t.Fatalf("flush 后应见流式文本+光标: %s", v)
	}
	m = feed(m, doneMsg)
	if m.busy || m.streaming {
		t.Fatalf("done 后应复位: busy=%v streaming=%v", m.busy, m.streaming)
	}
	if len(m.messages) != 3 || m.messages[2].Content != "db 组共 25 个节点" {
		t.Fatalf("最终文本不一致: %+v", m.messages)
	}
}

func TestChat_ToolProgressGenerateDoesNotAppend(t *testing.T) {
	m := newChat(t)
	m.sender = fakeSender{}
	m, _ = typeAndSend(t, m, "x")
	m = feed(m, ToolProgressMsg{Phase: "generate", Name: "query_nodes"})
	if len(m.messages) != 1 {
		t.Fatalf("generate 只是准备调用,不应追加工具行: %+v", m.messages)
	}
	if m.toolPhase == "" {
		t.Fatal("generate 应更新状态栏阶段")
	}
	m = feed(m, ToolProgressMsg{Phase: "execute", Name: "query_nodes"})
	if len(m.messages) != 2 || m.messages[1].Role != "tool" {
		t.Fatalf("execute 应追加工具活动行: %+v", m.messages)
	}
	if v := m.View(); !strings.Contains(v, "⏺ query_nodes") {
		t.Fatalf("对话流应可见工具活动行: %s", v)
	}
}

func TestChat_SpinnerAdvancesOnTick(t *testing.T) {
	m := newChat(t)
	m.sender = fakeSender{}
	m, _ = typeAndSend(t, m, "x")
	before := m.spinnerIdx
	nm, rearm := m.Update(FlushTickMsg{})
	m = nm.(Model)
	if m.spinnerIdx == before {
		t.Fatal("busy 期间节拍应推进 spinner")
	}
	if rearm == nil {
		t.Fatal("busy 期间应续订")
	}
	m = feed(m, ChatDoneMsg{Text: "ok"})
	nm, rearm2 := m.Update(FlushTickMsg{})
	m = nm.(Model)
	if rearm2 != nil {
		t.Fatal("busy 结束后不应续订")
	}
}

func TestChat_WindowSizeBudget(t *testing.T) {
	m := newChat(t)
	m = feed(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if m.width != 98 {
		t.Fatalf("width = %d, want 98", m.width)
	}
	if m.height != 21 || m.view.Height != 21 {
		t.Fatalf("高度预算应精确(30-9): height=%d view.Height=%d", m.height, m.view.Height)
	}
	if m.ta.Width() <= 0 || m.ta.Width() >= 98 {
		t.Fatalf("textarea 宽度应适配边框: %d", m.ta.Width())
	}
	// 面板 View 行数 = 状态 1 + 视口 21 + 输入框 4 + 提示 1 = 27,加 App chrome 3 行恰为 30,不截断
	if lines := strings.Count(m.View(), "\n") + 1; lines != 27 {
		t.Fatalf("面板 View 行数 = %d, want 27", lines)
	}
}
