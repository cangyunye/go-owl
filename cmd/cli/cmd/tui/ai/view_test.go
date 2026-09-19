package ai

import (
	"strings"
	"testing"
)

func TestWrapText(t *testing.T) {
	cases := []struct {
		in    string
		width int
		want  string
	}{
		{"abcdef", 3, "abcd\nef"},
		{"你好世界", 4, "你好\n世界"},
		{"a\nb", 5, "a\nb"},
		{"abc def", 4, "abc \ndef"},
		{"", 5, ""},
	}
	for _, c := range cases {
		if got := wrapText(c.in, c.width); got != c.want {
			t.Fatalf("wrapText(%q,%d) = %q, want %q", c.in, c.width, got, c.want)
		}
	}
}

func TestRenderMsg_Roles(t *testing.T) {
	got := renderMsg(ChatMsg{Role: "user", Content: "查询 web 组"}, 60)
	if !strings.Contains(got, "❯") || !strings.Contains(got, "查询 web 组") {
		t.Fatalf("user 应为 ❯ 前缀: %s", got)
	}
	if strings.Contains(got, "你:") {
		t.Fatalf("user 不应再有「你:」前缀: %s", got)
	}

	got = renderMsg(ChatMsg{Role: "assistant", Content: "db 组共 25 个节点"}, 60)
	if !strings.Contains(got, "db 组共 25 个节点") {
		t.Fatalf("assistant 应渲染正文: %s", got)
	}
	if strings.Contains(got, "AI:") {
		t.Fatalf("assistant 不应再有「AI:」前缀: %s", got)
	}

	got = renderMsg(ChatMsg{Role: "tool", Content: "query_nodes"}, 60)
	if !strings.Contains(got, "⏺") || !strings.Contains(got, "query_nodes") {
		t.Fatalf("tool 应渲染 ⏺ 工具名: %s", got)
	}
}

func TestView_ShowsMessagesInputAndKeys(t *testing.T) {
	m := newChat(t)
	m.messages = []ChatMsg{
		{Role: "user", Content: "查询 web"},
		{Role: "tool", Content: "query_nodes"},
		{Role: "assistant", Content: "完成"},
	}
	m.status = "完成"
	m.refreshViewport()
	got := m.View()
	// Normal 模式提示行
	for _, want := range []string{"查询 web", "⏺ query_nodes", "完成", "Enter/i 输入", "n 新会话", "Tab 切面板", "Esc 返回 Nodes"} {
		if !strings.Contains(got, want) {
			t.Fatalf("view missing %q: %s", want, got)
		}
	}
	// Insert 模式提示行
	m.mode = ModeInsert
	if got := m.View(); !strings.Contains(got, "Enter 发送") || !strings.Contains(got, "Ctrl+J 换行") {
		t.Fatalf("Insert 提示行缺失: %s", got)
	}
}

func TestView_NormalModeShowsPlaceholder(t *testing.T) {
	m := newChat(t)
	m.messages = []ChatMsg{}
	m.refreshViewport()
	got := m.View()
	if !strings.Contains(got, "输入指令") {
		t.Fatalf("placeholder missing: %s", got)
	}
}

func TestView_StatusLineShowsSpinnerWhenBusy(t *testing.T) {
	m := newChat(t)
	m.busy = true
	m.toolPhase = "🔧 正在执行 query_nodes…"
	got := m.View()
	if !strings.Contains(got, "正在执行 query_nodes") {
		t.Fatalf("busy 状态行缺工具阶段: %s", got)
	}

	m2 := newChat(t)
	m2.modelLabel = "deepseek/deepseek-chat"
	got = m2.View()
	if !strings.Contains(got, "deepseek/deepseek-chat") {
		t.Fatalf("idle 状态行应显示模型标签: %s", got)
	}
}
