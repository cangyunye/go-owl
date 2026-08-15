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

func TestRenderMsg_RoleLabels(t *testing.T) {
	got := renderMsg(ChatMsg{Role: "user", Content: "查询 web 组"}, 60)
	if !strings.Contains(got, "你: ") {
		t.Fatalf("user label missing: %s", got)
	}
	got = renderMsg(ChatMsg{Role: "assistant", Content: "完成"}, 60)
	if !strings.Contains(got, "AI: ") {
		t.Fatalf("assistant label missing: %s", got)
	}
}

func TestView_ShowsMessagesInputAndKeys(t *testing.T) {
	m := newChat(t)
	m.messages = []ChatMsg{
		{Role: "user", Content: "查询 web"},
		{Role: "assistant", Content: "完成"},
	}
	m.status = "完成"
	m.refreshViewport()
	got := m.View()
	for _, want := range []string{"查询 web", "完成", "Enter 输入/发送", "n 新会话", "Esc 返回 Nodes"} {
		if !strings.Contains(got, want) {
			t.Fatalf("view missing %q: %s", want, got)
		}
	}
}

func TestView_NormalModeShowsPlaceholder(t *testing.T) {
	m := newChat(t)
	m.messages = []ChatMsg{}
	m.refreshViewport()
	got := m.View()
	if !strings.Contains(got, "输入指令…") {
		t.Fatalf("placeholder missing: %s", got)
	}
}
