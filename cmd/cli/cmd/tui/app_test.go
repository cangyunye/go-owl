package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/tui/exec"
)

func key(t tea.KeyType) tea.Msg { return tea.KeyMsg{Type: t} }
func runeKey(r rune) tea.Msg    { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func newApp(t *testing.T) *App {
	t.Helper()
	store := common.NewInMemoryNodeStoreAt(filepath.Join(t.TempDir(), "nodes.json"))
	for _, n := range []*common.NodeInfo{
		{ID: "n1", Name: "web-1", Address: "10.0.0.1", Port: 22, User: "root", Groups: []string{"web"}, Labels: map[string]string{"env": "prod"}},
		{ID: "n2", Name: "db-1", Address: "10.0.0.2", Port: 22, User: "admin", Groups: []string{"db"}, Labels: map[string]string{"env": "dev"}},
	} {
		if err := store.Add(n); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return NewApp(store)
}

func TestApp_DefaultPanelIsNodes(t *testing.T) {
	m := newApp(t)
	if m.panel != 0 {
		t.Fatalf("expected panel 0, got %d", m.panel)
	}
	if got := m.View(); !strings.Contains(got, "[Nodes]") {
		t.Fatalf("menu bar missing [Nodes]: %s", got)
	}
}

func TestApp_TabSwitchesToExecAndBack(t *testing.T) {
	m := newApp(t)
	nm, _ := m.Update(key(tea.KeyTab))
	m = nm.(*App)
	if m.panel != 1 {
		t.Fatalf("expected panel 1 after tab, got %d", m.panel)
	}
	if got := m.View(); !strings.Contains(got, "[Exec]") {
		t.Fatalf("menu bar missing [Exec]: %s", got)
	}
	nm, _ = m.Update(key(tea.KeyTab))
	m = nm.(*App)
	if m.panel != 0 {
		t.Fatalf("expected panel 0 after second tab, got %d", m.panel)
	}
}

func TestApp_DigitKeysJumpToPanel(t *testing.T) {
	m := newApp(t)
	nm, _ := m.Update(runeKey('2'))
	m = nm.(*App)
	if m.panel != 1 {
		t.Fatalf("expected panel 1 on '2', got %d", m.panel)
	}
	nm, _ = m.Update(runeKey('1'))
	m = nm.(*App)
	if m.panel != 0 {
		t.Fatalf("expected panel 0 on '1', got %d", m.panel)
	}
}

func TestApp_XJumpsToExec(t *testing.T) {
	m := newApp(t)
	nm, _ := m.Update(runeKey('x'))
	m = nm.(*App)
	if m.panel != 1 {
		t.Fatalf("expected panel 1 on 'x', got %d", m.panel)
	}
}

func TestApp_ExecCapturesVisibleSnapshot(t *testing.T) {
	m := newApp(t)
	// 过滤到 web 组后切到 exec, 快照应只有 n1
	nm, _ := m.Update(runeKey('/'))
	nm, _ = nm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g:web")})
	nm, _ = nm.Update(key(tea.KeyEnter))
	m = nm.(*App)
	nm, _ = m.Update(runeKey('2'))
	m = nm.(*App)
	if len(m.exec.Targets()) != 1 || m.exec.Targets()[0].ID != "n1" {
		t.Fatalf("expected snapshot [n1], got %v", m.exec.Targets())
	}
}

func TestApp_LeavePanelMsgReturnsToNodes(t *testing.T) {
	m := newApp(t)
	nm, _ := m.Update(runeKey('2'))
	m = nm.(*App)
	nm, _ = m.Update(exec.LeavePanelMsg{})
	m = nm.(*App)
	if m.panel != 0 {
		t.Fatalf("expected panel 0 after LeavePanelMsg, got %d", m.panel)
	}
}

func TestApp_QuitOnNodesStillWorks(t *testing.T) {
	m := newApp(t)
	nm, cmd := m.Update(runeKey('q'))
	m = nm.(*App)
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected QuitMsg, got %T", cmd())
	}
}

func TestApp_HelpContainsFilterSyntaxExamples(t *testing.T) {
	m := newApp(t)
	nm, _ := m.Update(runeKey('?'))
	m = nm.(*App)
	v := m.View()
	for _, want := range []string{
		"g:web && l:env=prod",
		"s:online",
	} {
		if !strings.Contains(v, want) {
			t.Fatalf("expected help overlay to contain filter example %q: %q", want, v)
		}
	}
}
