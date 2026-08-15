package file

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

func key(t tea.KeyType) tea.Msg { return tea.KeyMsg{Type: t} }
func runeKey(r rune) tea.Msg    { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func newTestModel(t *testing.T) FileModel {
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
	m := NewModel(newTestStore(t))
	if m.op != OpUpload {
		t.Fatalf("expected OpUpload, got %v", m.op)
	}
	if m.current() != LocFile {
		t.Fatalf("expected stack top LocFile, got %v", m.current())
	}
	if got := m.Path(); len(got) != 2 || got[0] != "file" || got[1] != "upload" {
		t.Fatalf("unexpected path: %v", got)
	}
	if m.Mode() != ModeNormal || m.InsertMode() {
		t.Fatal("expected ModeNormal")
	}
	if m.IsDirty() {
		t.Fatal("file panel never dirty")
	}
}

func newTestStore(t *testing.T) common.NodeStore {
	t.Helper()
	return common.NewInMemoryNodeStoreAt(filepath.Join(t.TempDir(), "nodes.json"))
}

func TestOpToggle_LeftRightCycles(t *testing.T) {
	m := newTestModel(t)
	// 标准按键: right 一次 → download, 再 right → transfer, 再 right → upload
	nm, _ := m.Update(key(tea.KeyRight))
	m = nm.(FileModel)
	if m.op != OpDownload {
		t.Fatalf("expected OpDownload after right, got %v", m.op)
	}
	if got := m.Path(); got[1] != "download" {
		t.Fatalf("expected path file/download, got %v", got)
	}
	nm, _ = m.Update(key(tea.KeyRight))
	m = nm.(FileModel)
	if m.op != OpTransfer {
		t.Fatalf("expected OpTransfer after second right, got %v", m.op)
	}
	nm, _ = m.Update(key(tea.KeyRight))
	m = nm.(FileModel)
	if m.op != OpUpload {
		t.Fatalf("expected wrap to OpUpload, got %v", m.op)
	}
	nm, _ = m.Update(key(tea.KeyLeft))
	m = nm.(FileModel)
	if m.op != OpTransfer {
		t.Fatalf("expected OpTransfer on left, got %v", m.op)
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

func TestRunView_ShowsFiveFieldsAndTargets(t *testing.T) {
	m := newTestModel(t)
	got := m.View()
	for _, want := range []string{"文件上传", "本地文件", "节点", "分组", "标签", "目标目录", "目标 3 台", "/tmp"} {
		if !contains(got, want) {
			t.Fatalf("view missing %q:\n%s", want, got)
		}
	}
}

func TestEscAtRootEmitsLeavePanel(t *testing.T) {
	m := newTestModel(t)
	nm, cmd := m.Update(key(tea.KeyEsc))
	m = nm.(FileModel)
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
	m = nm.(FileModel)
	if m.mode != ModeInsert {
		t.Fatal("expected ModeInsert after enter")
	}
	if !m.fileInput.Focused() {
		t.Fatal("expected file input focused")
	}
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(FileModel)
	if m.mode != ModeNormal {
		t.Fatal("expected ModeNormal after esc")
	}
	if m.fileInput.Focused() {
		t.Fatal("expected file input blurred")
	}
}

func TestInsertMode_LeftRightTogglesOpDisabled(t *testing.T) {
	m := newTestModel(t)
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(FileModel)
	nm, _ = m.Update(key(tea.KeyRight))
	m = nm.(FileModel)
	if m.op != OpUpload {
		t.Fatal("op must not toggle in Insert mode")
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
