package exec

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

func key(t tea.KeyType) tea.Msg { return tea.KeyMsg{Type: t} }
func runeKey(r rune) tea.Msg    { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func newTestModel(t *testing.T) ExecModel {
	t.Helper()
	store := common.NewInMemoryNodeStoreAt(filepath.Join(t.TempDir(), "nodes.json"))
	for _, n := range []*common.NodeInfo{
		{ID: "n1", Name: "web-1", Groups: []string{"web"}, Labels: map[string]string{"env": "prod"}},
		{ID: "n2", Name: "db-1", Groups: []string{"db"}, Labels: map[string]string{"env": "dev"}},
		{ID: "n3", Name: "cache-1", Groups: []string{"web", "cache"}, Labels: map[string]string{"env": "prod", "role": "cache"}},
	} {
		if err := store.Add(n); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	m := NewModel(store)
	nodes, _ := store.List()
	m.CaptureTargets(nodes)
	return m
}

func TestNewModel_DefaultState(t *testing.T) {
	m := newTestModel(t)
	if m.format != "simple" {
		t.Fatalf("expected format simple, got %s", m.format)
	}
	if m.current() != LocRun {
		t.Fatalf("expected stack top LocRun, got %v", m.current())
	}
	if got := m.Path(); len(got) != 2 || got[0] != "exec" || got[1] != "run" {
		t.Fatalf("unexpected path: %v", got)
	}
	if m.Mode() != ModeNormal || m.InsertMode() {
		t.Fatal("expected ModeNormal")
	}
	if m.IsDirty() {
		t.Fatal("exec panel never dirty")
	}
}

func TestFormatCycle_FToJsonToSimple(t *testing.T) {
	m := newTestModel(t)
	nm, _ := m.Update(runeKey('f'))
	m = nm.(ExecModel)
	if m.format != "detail" {
		t.Fatalf("expected detail after first f, got %s", m.format)
	}
	nm, _ = m.Update(runeKey('f'))
	m = nm.(ExecModel)
	if m.format != "json" {
		t.Fatalf("expected json after second f, got %s", m.format)
	}
	nm, _ = m.Update(runeKey('f'))
	m = nm.(ExecModel)
	if m.format != "simple" {
		t.Fatalf("expected simple after third f, got %s", m.format)
	}
}

func TestResolve_ExplicitNodes(t *testing.T) {
	m := newTestModel(t)
	m.nodesInput.SetValue("n1,n3")
	nodes, err := m.resolveTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || nodes[0].ID != "n1" || nodes[1].ID != "n3" {
		t.Fatalf("expected [n1 n3], got %v", nodeIDs(nodes))
	}
}

func TestResolve_Groups(t *testing.T) {
	m := newTestModel(t)
	m.groupsInput.SetValue("web")
	nodes, err := m.resolveTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || nodes[0].ID != "n1" || nodes[1].ID != "n3" {
		t.Fatalf("expected [n1 n3], got %v", nodeIDs(nodes))
	}
}

func TestResolve_Labels(t *testing.T) {
	m := newTestModel(t)
	m.labelsInput.SetValue("env=prod")
	nodes, err := m.resolveTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || nodes[0].ID != "n1" || nodes[1].ID != "n3" {
		t.Fatalf("expected [n1 n3], got %v", nodeIDs(nodes))
	}
}

func TestResolve_EmptyFallsBackToSnapshot(t *testing.T) {
	m := newTestModel(t)
	nodes, err := m.resolveTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 {
		t.Fatalf("expected 3 from snapshot, got %d", len(nodes))
	}
}

func TestResolve_PriorityNodesOverGroups(t *testing.T) {
	m := newTestModel(t)
	m.nodesInput.SetValue("n2")
	m.groupsInput.SetValue("web")
	nodes, err := m.resolveTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].ID != "n2" {
		t.Fatalf("expected [n2] (nodes wins), got %v", nodeIDs(nodes))
	}
}

func TestResolve_DedupeAndSort(t *testing.T) {
	m := newTestModel(t)
	m.nodesInput.SetValue("n3,n1,n3")
	nodes, err := m.resolveTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || nodes[0].ID != "n1" || nodes[1].ID != "n3" {
		t.Fatalf("expected deduped sorted [n1 n3], got %v", nodeIDs(nodes))
	}
}

func TestRunView_ShowsFourFieldsAndFormat(t *testing.T) {
	m := newTestModel(t)
	got := m.View()
	for _, want := range []string{"命令", "节点", "分组", "标签", "simple", "目标", "3 台"} {
		if !contains(got, want) {
			t.Fatalf("view missing %q:\n%s", want, got)
		}
	}
}

func TestEscAtRootEmitsLeavePanel(t *testing.T) {
	m := newTestModel(t)
	nm, cmd := m.Update(key(tea.KeyEsc))
	m = nm.(ExecModel)
	if cmd == nil {
		t.Fatal("expected cmd")
	}
	msg := cmd()
	if _, ok := msg.(LeavePanelMsg); !ok {
		t.Fatalf("expected LeavePanelMsg, got %T", msg)
	}
}

func TestEnterEditsFieldAndEscRestores(t *testing.T) {
	m := newTestModel(t)
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(ExecModel)
	if m.mode != ModeInsert {
		t.Fatal("expected ModeInsert after enter")
	}
	if !m.cmdInput.Focused() {
		t.Fatal("expected cmd input focused")
	}
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(ExecModel)
	if m.mode != ModeNormal {
		t.Fatal("expected ModeNormal after esc")
	}
	if m.cmdInput.Focused() {
		t.Fatal("expected cmd input blurred")
	}
}

func nodeIDs(nodes []*common.NodeInfo) []string {
	ids := make([]string, 0, len(nodes))
	for _, n := range nodes {
		ids = append(ids, n.ID)
	}
	return ids
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
