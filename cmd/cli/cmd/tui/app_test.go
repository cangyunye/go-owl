package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	tuiai "github.com/cangyunye/go-owl/cmd/cli/cmd/tui/ai"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/tui/exec"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/tui/file"
	"github.com/cangyunye/go-owl/internal/control/transfer"
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

func TestApp_Digit3JumpsToFile(t *testing.T) {
	m := newApp(t)
	nm, _ := m.Update(runeKey('3'))
	m = nm.(*App)
	if m.panel != 2 {
		t.Fatalf("expected panel 2 on '3', got %d", m.panel)
	}
	if got := m.View(); !strings.Contains(got, "[File]") {
		t.Fatalf("menu bar missing [File]: %s", got)
	}
}

func TestApp_FJumpsToFileFromNodes(t *testing.T) {
	m := newApp(t)
	nm, _ := m.Update(runeKey('f'))
	m = nm.(*App)
	if m.panel != 2 {
		t.Fatalf("expected panel 2 on 'f', got %d", m.panel)
	}
}

func TestApp_FNotInterceptedInsideImportExportDialog(t *testing.T) {
	m := newApp(t)
	// 打开导入/导出对话框(i),Esc 回 Normal 后按 f 应切换格式,而不是被 App 劫持跳到 File 面板
	nm, _ := m.Update(runeKey('i'))
	m = nm.(*App)
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(*App)
	nm, _ = m.Update(runeKey('f'))
	m = nm.(*App)
	if m.panel != 0 {
		t.Fatalf("'f' inside import/export dialog must not switch panel, got %d", m.panel)
	}
	if got := m.View(); !strings.Contains(got, "导入/导出") {
		t.Fatalf("expected import/export dialog still open: %s", got)
	}
	if got := m.View(); !strings.Contains(got, "json") {
		t.Fatalf("expected format toggled to json after 'f': %s", got)
	}
}

func TestApp_FIgnoredInExecPanel(t *testing.T) {
	m := newApp(t)
	nm, _ := m.Update(runeKey('2'))
	m = nm.(*App)
	nm, _ = m.Update(runeKey('f'))
	m = nm.(*App)
	if m.panel != 1 {
		t.Fatalf("'f' must not switch panel from exec, got %d", m.panel)
	}
}

func TestApp_FileCapturesVisibleSnapshot(t *testing.T) {
	m := newApp(t)
	// 过滤到 web 组后切到 file, 快照应只有 n1
	nm, _ := m.Update(runeKey('/'))
	nm, _ = nm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g:web")})
	nm, _ = nm.Update(key(tea.KeyEnter))
	m = nm.(*App)
	nm, _ = m.Update(runeKey('3'))
	m = nm.(*App)
	if len(m.file.Targets()) != 1 || m.file.Targets()[0].ID != "n1" {
		t.Fatalf("expected snapshot [n1], got %v", m.file.Targets())
	}
}

func TestApp_FileLeavePanelMsgReturnsToNodes(t *testing.T) {
	m := newApp(t)
	nm, _ := m.Update(runeKey('3'))
	m = nm.(*App)
	nm, _ = m.Update(file.LeavePanelMsg{})
	m = nm.(*App)
	if m.panel != 0 {
		t.Fatalf("expected panel 0 after LeavePanelMsg, got %d", m.panel)
	}
}

func TestApp_DoneMsgRoutedToFilePanelWhileInactive(t *testing.T) {
	m := newApp(t)
	nm, _ := m.Update(runeKey('3')) // 进入 File
	m = nm.(*App)
	nm, _ = m.Update(runeKey('2')) // 切到 Exec(File 不活跃)
	m = nm.(*App)
	// 模拟传输完成消息在 File 面板不活跃时到达
	nm, _ = m.Update(file.UploadDoneMsg{Results: []transfer.TransferResult{{NodeID: "n1", Path: "/tmp/a.tar", Method: "scp"}}})
	m = nm.(*App)
	if m.panel != 1 {
		t.Fatalf("panel must stay 1, got %d", m.panel)
	}
	if m.file.Loading() {
		t.Fatal("expected loading cleared by routed msg")
	}
	if len(m.file.Results()) != 1 || m.file.Results()[0].NodeID != "n1" {
		t.Fatalf("expected routed results [n1], got %v", m.file.Results())
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

func TestApp_Digit4JumpsToAI(t *testing.T) {
	m := newApp(t)
	nm, _ := m.Update(runeKey('4'))
	m = nm.(*App)
	if m.panel != 3 {
		t.Fatalf("expected panel 3 on '4', got %d", m.panel)
	}
	if got := m.View(); !strings.Contains(got, "[AI]") {
		t.Fatalf("menu bar missing [AI]: %s", got)
	}
}

func TestApp_TabCyclesFourPanels(t *testing.T) {
	m := newApp(t)
	for _, want := range []int{1, 2, 3, 0} {
		nm, _ := m.Update(key(tea.KeyTab))
		m = nm.(*App)
		if m.panel != want {
			t.Fatalf("expected panel %d after tab, got %d", want, m.panel)
		}
	}
}

func TestApp_AIEscReturnsToNodes(t *testing.T) {
	m := newApp(t)
	nm, _ := m.Update(runeKey('4'))
	m = nm.(*App)
	nm, _ = m.Update(tuiai.LeavePanelMsg{})
	m = nm.(*App)
	if m.panel != 0 {
		t.Fatalf("expected panel 0 after AI esc, got %d", m.panel)
	}
}
