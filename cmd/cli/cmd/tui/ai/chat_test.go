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

	msg := cmd()
	done, ok := msg.(ChatDoneMsg)
	if !ok {
		t.Fatalf("expected ChatDoneMsg, got %T", msg)
	}
	if done.Text != "回答: ab" {
		t.Fatalf("unexpected done text: %q", done.Text)
	}

	nm, _ = m.Update(done)
	m = nm.(Model)
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
	nm, _ = m.Update(cmd())
	m = nm.(Model)
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
		nm, _ = m.Update(cmd())
		m = nm.(Model)
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
