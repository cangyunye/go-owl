package nodes

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

func key(t tea.KeyType) tea.Msg { return tea.KeyMsg{Type: t} }

func runeKey(r rune) tea.Msg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func seedNodes(t *testing.T, store common.NodeStore) {
	t.Helper()
	for _, n := range []*common.NodeInfo{
		{ID: "n2", Name: "db-1", Address: "10.0.0.2", Port: 22, User: "admin", Status: "offline", Groups: []string{"db"}, Labels: map[string]string{"env": "dev"}},
		{ID: "n3", Name: "cache-1", Address: "10.0.0.3", Port: 22, User: "root", Status: "online", Groups: []string{"cache", "web"}, Labels: map[string]string{"env": "prod", "role": "cache"}},
		{ID: "n1", Name: "web-1", Address: "10.0.0.1", Port: 22, User: "root", Status: "online", Groups: []string{"web"}, Labels: map[string]string{"env": "prod"}},
	} {
		if err := store.Add(n); err != nil {
			t.Fatalf("seed add: %v", err)
		}
	}
}

func newTestStore(t *testing.T) common.NodeStore {
	t.Helper()
	return common.NewInMemoryNodeStoreAt(filepath.Join(t.TempDir(), "nodes.json"))
}

func TestNewModel_LoadsAndSortsByID(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	if len(m.nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(m.nodes))
	}
	if m.nodes[0].ID != "n1" || m.nodes[1].ID != "n2" || m.nodes[2].ID != "n3" {
		t.Fatalf("expected sorted by id, got %s,%s,%s", m.nodes[0].ID, m.nodes[1].ID, m.nodes[2].ID)
	}
	if m.cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", m.cursor)
	}
}

func TestNewModel_ModeAndPath(t *testing.T) {
	m := NewModel(newTestStore(t))
	if m.Mode() != ModeNormal {
		t.Fatal("expected ModeNormal")
	}
	path := m.Path()
	if len(path) != 1 || path[0] != "nodes" {
		t.Fatalf("unexpected path: %v", path)
	}
}

func TestMoveCursor_DownAndUp(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(key(tea.KeyDown))
	m = nm.(Model)
	if m.cursor != 1 {
		t.Fatalf("expected cursor 1, got %d", m.cursor)
	}
	nm, _ = m.Update(key(tea.KeyUp))
	m = nm.(Model)
	if m.cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", m.cursor)
	}
}

func TestMoveCursor_Clamps(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	for i := 0; i < 5; i++ {
		nm, _ := m.Update(key(tea.KeyDown))
		m = nm.(Model)
	}
	if m.cursor != 2 {
		t.Fatalf("expected clamp at 2, got %d", m.cursor)
	}
	nm, _ := m.Update(key(tea.KeyUp))
	nm, _ = nm.(Model).Update(key(tea.KeyUp))
	nm, _ = nm.(Model).Update(key(tea.KeyUp))
	nm, _ = nm.(Model).Update(key(tea.KeyUp))
	m = nm.(Model)
	if m.cursor != 0 {
		t.Fatalf("expected clamp at 0, got %d", m.cursor)
	}
}

func TestFocusPane_LeftRight(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(key(tea.KeyRight))
	m = nm.(Model)
	if m.focus != paneDetail {
		t.Fatal("expected focus detail")
	}
	nm, _ = m.Update(key(tea.KeyLeft))
	m = nm.(Model)
	if m.focus != paneList {
		t.Fatal("expected focus list")
	}
}

func TestJumpTopBottom_G(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('G'))
	m = nm.(Model)
	if m.cursor != 2 {
		t.Fatalf("expected cursor 2, got %d", m.cursor)
	}
	nm, _ = m.Update(runeKey('g'))
	m = nm.(Model)
	if m.cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", m.cursor)
	}
}

func TestSelectedNode(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	n := m.selectedNode()
	if n == nil || n.ID != "n1" {
		t.Fatalf("unexpected selected node: %+v", n)
	}
}

func TestComputeColumnWidths_Fits(t *testing.T) {
	cols := []Column{{Key: "id", Pref: 10}, {Key: "name", Pref: 10}}
	got := computeColumnWidths(cols, 40)
	if got[0] != 10 || got[1] != 10 {
		t.Fatalf("unexpected widths: %v", got)
	}
}

func TestComputeColumnWidths_Scales(t *testing.T) {
	cols := []Column{{Key: "id", Pref: 20}, {Key: "name", Pref: 20}, {Key: "status", Pref: 10}}
	got := computeColumnWidths(cols, 30)
	total := 0
	for _, w := range got {
		if w < 6 {
			t.Fatalf("width below floor: %v", got)
		}
		total += w
	}
	if total > 30 {
		t.Fatalf("total %d exceeds avail 30: %v", total, got)
	}
}

func TestTruncateCell(t *testing.T) {
	if s := truncateCell("hello", 3); s != "he…" {
		t.Fatalf("unexpected: %q", s)
	}
	if s := truncateCell("hello", 10); s != "hello     " {
		t.Fatalf("unexpected pad: %q", s)
	}
}

func TestFilterEsc_RestoresAppliedFilter(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	// 先提交 g:web(Enter)
	nm, _ := m.Update(runeKey('/'))
	m = nm.(Model)
	for _, r := range []rune("g:web") {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(Model)
	}
	nm, _ = m.Update(key(tea.KeyEnter))
	m = nm.(Model)
	if len(m.visible()) != 2 {
		t.Fatalf("expected 2 after commit g:web, got %d", len(m.visible()))
	}
	// 再打开改成 g:db(live 1 个),然后 Esc → 恢复 g:web(2 个)
	nm, _ = m.Update(runeKey('/'))
	m = nm.(Model)
	for _, r := range []rune("g:db") {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(Model)
	}
	if len(m.visible()) != 1 {
		t.Fatalf("expected 1 live after typing g:db, got %d", len(m.visible()))
	}
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(Model)
	if len(m.visible()) != 2 {
		t.Fatalf("expected 2 after Esc restore, got %d", len(m.visible()))
	}
	if m.filterText != "g:web" {
		t.Fatalf("expected filterText restored to g:web, got %q", m.filterText)
	}
}

func TestCellValue_VariousKeys(t *testing.T) {
	n := &common.NodeInfo{ID: "n1", Name: "web", Address: "1.2.3.4", Port: 22, User: "root", Status: "online", Groups: []string{"web"}, Labels: map[string]string{"b": "2", "a": "1"}, LastCheckAt: "x"}
	cases := map[string]string{
		"id": "n1", "name": "web", "address": "1.2.3.4", "port": "22", "user": "root",
		"status": "online", "groups": "web", "labels": "a=1,b=2", "last_check": "x",
	}
	for k, want := range cases {
		if got := cellValue(n, k); got != want {
			t.Fatalf("cellValue(%s) = %q, want %q", k, got, want)
		}
	}
}

func TestWindowSizeMsg_UpdatesWidth(t *testing.T) {
	m := NewModel(newTestStore(t))
	if m.width != 120 {
		t.Fatalf("expected default width 120, got %d", m.width)
	}
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(Model)
	if m.width != 100 {
		t.Fatalf("expected width 100, got %d", m.width)
	}
}
