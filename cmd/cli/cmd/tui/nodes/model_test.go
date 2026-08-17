package nodes

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
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

func seedMany(t *testing.T, store common.NodeStore, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		id := fmt.Sprintf("node-%02d", i)
		node := &common.NodeInfo{
			ID: id, Name: "srv-" + id, Address: "10.0.0." + strconv.Itoa(i),
			Port: 22, User: "root", Status: "online",
		}
		if err := store.Add(node); err != nil {
			t.Fatalf("seed add: %v", err)
		}
	}
}

func TestWindowSizeMsg_UpdatesWidthAndHeight(t *testing.T) {
	m := NewModel(newTestStore(t))
	if m.width != 120 {
		t.Fatalf("expected default width 120, got %d", m.width)
	}
	if m.height != 24 {
		t.Fatalf("expected default height 24, got %d", m.height)
	}
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(Model)
	if m.width != 100 {
		t.Fatalf("expected width 100, got %d", m.width)
	}
	if m.height != 30 {
		t.Fatalf("expected height 30, got %d", m.height)
	}
}

func TestListView_ScrollsToKeepCursorVisible(t *testing.T) {
	store := newTestStore(t)
	seedMany(t, store, 30)
	m := NewModel(store)
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m = nm.(Model)
	for i := 0; i < 20; i++ {
		nm, _ = m.Update(key(tea.KeyDown))
		m = nm.(Model)
	}
	if m.cursor != 20 {
		t.Fatalf("expected cursor 20, got %d", m.cursor)
	}
	if m.offset <= 0 {
		t.Fatalf("expected window to scroll forward, got offset %d", m.offset)
	}
	v := m.View()
	if !strings.Contains(v, "node-21") {
		t.Fatalf("cursor row should be visible, offset=%d:\n%s", m.offset, v)
	}
	if strings.Contains(v, "node-01") {
		t.Fatalf("scrolled window should not render node-01, offset=%d:\n%s", m.offset, v)
	}
}

func TestListView_PageDownPageUp(t *testing.T) {
	store := newTestStore(t)
	seedMany(t, store, 30)
	m := NewModel(store)
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m = nm.(Model)
	nm, _ = m.Update(key(tea.KeyPgDown))
	m = nm.(Model)
	if m.cursor != 15 {
		t.Fatalf("expected cursor 15 after PgDown, got %d", m.cursor)
	}
	if m.offset != 1 {
		t.Fatalf("expected offset 1 after PgDown, got %d", m.offset)
	}
	nm, _ = m.Update(key(tea.KeyPgUp))
	m = nm.(Model)
	if m.cursor != 0 || m.offset != 0 {
		t.Fatalf("expected cursor/offset 0 after PgUp, got %d/%d", m.cursor, m.offset)
	}
}

func TestListView_CtrlDUCtrlU(t *testing.T) {
	store := newTestStore(t)
	seedMany(t, store, 30)
	m := NewModel(store)
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m = nm.(Model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	m = nm.(Model)
	if m.cursor != 15 {
		t.Fatalf("expected cursor 15 after Ctrl+D, got %d", m.cursor)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	m = nm.(Model)
	if m.cursor != 0 || m.offset != 0 {
		t.Fatalf("expected cursor/offset 0 after Ctrl+U, got %d/%d", m.cursor, m.offset)
	}
}

func TestListView_GJumpsToBottomAndBack(t *testing.T) {
	store := newTestStore(t)
	seedMany(t, store, 30)
	m := NewModel(store)
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	m = nm.(Model)
	nm, _ = m.Update(runeKey('G'))
	m = nm.(Model)
	if m.cursor != 29 {
		t.Fatalf("expected cursor 29 after G, got %d", m.cursor)
	}
	if m.offset != 15 {
		t.Fatalf("expected offset 15 after G, got %d", m.offset)
	}
	v := m.View()
	if !strings.Contains(v, "node-30") {
		t.Fatalf("last node should be visible after G:\n%s", v)
	}
	if strings.Contains(v, "node-01") {
		t.Fatalf("first node should not be visible after G, offset=%d:\n%s", m.offset, v)
	}
	nm, _ = m.Update(runeKey('g'))
	m = nm.(Model)
	if m.cursor != 0 || m.offset != 0 {
		t.Fatalf("expected cursor/offset 0 after g, got %d/%d", m.cursor, m.offset)
	}
}

func idsOf(nodes []*common.NodeInfo) []string {
	ids := make([]string, 0, len(nodes))
	for _, n := range nodes {
		ids = append(ids, n.ID)
	}
	return ids
}

func TestMark_ToggleWithSpace(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	// 光标在 n1, Space 勾选
	nm, _ := m.Update(runeKey(' '))
	m = nm.(Model)
	if !m.IsMarked("n1") || m.MarkedCount() != 1 {
		t.Fatalf("expected n1 marked, got %d", m.MarkedCount())
	}
	// 再 Space 取消
	nm, _ = m.Update(runeKey(' '))
	m = nm.(Model)
	if m.IsMarked("n1") || m.MarkedCount() != 0 {
		t.Fatal("expected n1 unmarked after second space")
	}
}

func TestMarkedIDs_Sorted(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	m.marked["n3"] = true
	m.marked["n1"] = true
	if got := m.MarkedIDs(); len(got) != 2 || got[0] != "n1" || got[1] != "n3" {
		t.Fatalf("expected sorted [n1 n3], got %v", got)
	}
}

func TestMarked_MovesWithCursorAndSurvivesFilter(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	// 勾选 n1, 下移到 n2 勾选
	nm, _ := m.Update(runeKey(' '))
	m = nm.(Model)
	nm, _ = m.Update(key(tea.KeyDown))
	m = nm.(Model)
	nm, _ = m.Update(runeKey(' '))
	m = nm.(Model)
	if m.MarkedCount() != 2 {
		t.Fatalf("expected 2 marked, got %d", m.MarkedCount())
	}
	// 过滤到 db 组(n2), 勾选保留
	m.filter = ParseFilterQuery("g:db")
	m.reload()
	if !m.IsMarked("n1") || !m.IsMarked("n2") {
		t.Fatal("marks must survive filter change")
	}
	if len(m.visible()) != 1 || m.visible()[0].ID != "n2" {
		t.Fatalf("filter should show only n2, got %v", idsOf(m.visible()))
	}
}

func TestMark_DoesNotAffectIsDirty(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey(' '))
	m = nm.(Model)
	if m.IsDirty() {
		t.Fatal("marks are session state, must not dirty the model")
	}
}

func TestFilter_Exported(t *testing.T) {
	m := NewModel(newTestStore(t))
	m.filter = ParseFilterQuery("g:web l:env=prod")
	fq := m.Filter()
	if len(fq.Groups) != 1 || fq.Groups[0] != "web" || fq.Labels["env"] != "prod" {
		t.Fatalf("unexpected filter export: %#v", fq)
	}
}
