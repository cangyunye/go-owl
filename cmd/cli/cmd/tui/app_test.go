package tui_test

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/tui"
)

func newStore(t *testing.T) common.NodeStore {
	t.Helper()
	s := common.NewInMemoryNodeStoreAt(filepath.Join(t.TempDir(), "nodes.json"))
	for _, n := range []*common.NodeInfo{
		{ID: "n1", Name: "web-1", Address: "10.0.0.1", Port: 22, User: "root", Status: "online", Groups: []string{"web"}, Labels: map[string]string{"env": "prod"}},
		{ID: "n2", Name: "db-1", Address: "10.0.0.2", Port: 22, User: "admin", Status: "offline", Groups: []string{"db"}},
	} {
		if err := s.Add(n); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return s
}

func key(t tea.KeyType) tea.Msg { return tea.KeyMsg{Type: t} }

func runeKey(r rune) tea.Msg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func TestApp_BreadcrumbView(t *testing.T) {
	a := tui.NewApp(newStore(t))
	v := a.View()
	if !strings.Contains(v, "/nodes") {
		t.Fatalf("expected /nodes breadcrumb in view: %q", v)
	}
}

func TestApp_QuitInNormalMode(t *testing.T) {
	a := tui.NewApp(newStore(t))
	m, cmd := a.Update(runeKey('q'))
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
	// tea.Quit 是返回 Msg 的 Cmd,需调用后断言
	if msg := cmd(); msg != nil {
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Fatalf("expected tea.QuitMsg, got %T", msg)
		}
	} else {
		t.Fatal("expected non-nil quit msg")
	}
	_ = m
}

func TestApp_QuitBlockedInInsertMode(t *testing.T) {
	a := tui.NewApp(newStore(t))
	// 打开过滤输入(进入 Insert)
	m, _ := a.Update(runeKey('/'))
	a = m.(*tui.App)
	// Insert 态按 q 不得退出
	m, cmd := a.Update(runeKey('q'))
	if cmd != nil {
		t.Fatalf("expected no quit in insert mode, got %T", cmd)
	}
	_ = m
}

func TestApp_HelpOverlay(t *testing.T) {
	a := tui.NewApp(newStore(t))
	m, _ := a.Update(runeKey('?'))
	a = m.(*tui.App)
	v := a.View()
	if !strings.Contains(v, "Normal=命令") {
		t.Fatalf("expected help overlay: %q", v)
	}
	m, _ = a.Update(key(tea.KeyEsc))
	a = m.(*tui.App)
	if strings.Contains(a.View(), "Normal=命令") {
		t.Fatal("expected help closed after Esc")
	}
}

func TestApp_QuitConfirmWhenDirty(t *testing.T) {
	a := tui.NewApp(newStore(t))
	// 进入新增表单(深层位置 = dirty)
	m, _ := a.Update(runeKey('a'))
	a = m.(*tui.App)
	m, cmd := a.Update(runeKey('q'))
	if cmd != nil {
		t.Fatal("expected no immediate quit when dirty")
	}
	a = m.(*tui.App)
	// 确认 y → 退出
	_, cmd = a.Update(runeKey('y'))
	if cmd == nil {
		t.Fatal("expected quit cmd after confirm")
	}
	if msg := cmd(); msg != nil {
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Fatalf("expected quit after confirm, got %T", msg)
		}
	}
}

func TestApp_InsertModeBypassesAppKeys(t *testing.T) {
	a := tui.NewApp(newStore(t))
	m, _ := a.Update(runeKey('/'))
	a = m.(*tui.App)
	// Insert 态 '?' 不应开帮助
	_, cmd := a.Update(runeKey('?'))
	if cmd != nil {
		t.Fatal("expected no help toggle in insert mode")
	}
}

func TestApp_NodeCRUDFlow(t *testing.T) {
	a := tui.NewApp(newStore(t))
	// add: 打开表单,填 ID/Name/Address,保存
	m, _ := a.Update(runeKey('a'))
	a = m.(*tui.App)
	for _, r := range []rune("new-1") {
		m, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		a = m.(*tui.App)
	}
	_ = m
}
