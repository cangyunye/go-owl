package nodes

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestLabelsModel_SetAndRemove(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	lm := NewLabelsModel(store, "n1")
	// n1 初始 labels: env=prod
	if err := lm.setLabel("tier=backend"); err != nil {
		t.Fatalf("set: %v", err)
	}
	node, _ := store.Get("n1")
	if node.Labels["tier"] != "backend" {
		t.Fatalf("expected tier=backend, got %#v", node.Labels)
	}
	if err := lm.setLabel("env="); err != nil {
		t.Fatalf("clear env: %v", err)
	}
	node, _ = store.Get("n1")
	if _, ok := node.Labels["env"]; ok {
		t.Fatalf("expected env removed, got %#v", node.Labels)
	}
}

func TestLabelsModel_ReloadSortsKeys(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	lm := NewLabelsModel(store, "n3") // n3 labels: env=prod, role=cache
	if len(lm.keys) != 2 || lm.keys[0] != "env" || lm.keys[1] != "role" {
		t.Fatalf("unexpected keys: %#v", lm.keys)
	}
}

func TestModel_OpenLabels_FromList(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('l'))
	m = nm.(Model)
	if m.current() != LocLabels {
		t.Fatalf("expected LocLabels, got %v", m.current())
	}
	if m.labels == nil || m.labels.nodeID != "n1" {
		t.Fatalf("unexpected labels model: %+v", m.labels)
	}
	path := m.Path()
	if len(path) != 3 || path[2] != "labels" {
		t.Fatalf("unexpected path: %v", path)
	}
}

func TestModel_Labels_AddFlow(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('l'))
	m = nm.(Model)
	nm, _ = m.Update(runeKey('a'))
	m = nm.(Model)
	if !m.labels.adding {
		t.Fatal("expected adding mode")
	}
	// 输入 tier=worker 并回车
	for _, r := range []rune("tier=worker") {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(Model)
	}
	nm, _ = m.Update(key(tea.KeyEnter))
	m = nm.(Model)
	node, _ := store.Get("n1")
	if node.Labels["tier"] != "worker" {
		t.Fatalf("expected tier=worker, got %#v", node.Labels)
	}
	if m.labels.adding {
		t.Fatal("expected adding closed")
	}
}

func TestModel_Labels_RemoveFlow(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('l'))
	m = nm.(Model)
	// 光标在 env(第一个 key),按 d 删除
	nm, _ = m.Update(runeKey('d'))
	m = nm.(Model)
	node, _ := store.Get("n1")
	if _, ok := node.Labels["env"]; ok {
		t.Fatalf("expected env removed, got %#v", node.Labels)
	}
}

func TestLabels_AddModeInsertIsolatesKeys(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('l'))
	m = nm.(Model)
	nm, _ = m.Update(runeKey('a'))
	m = nm.(Model)
	if m.Mode() != ModeInsert {
		t.Fatalf("expected ModeInsert after a, got %v", m.Mode())
	}
	// q/? 应被输入,而不是被 App 劫持为退出/帮助
	for _, r := range []rune("q?") {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(Model)
	}
	if m.Mode() != ModeInsert {
		t.Fatalf("expected still ModeInsert, got %v", m.Mode())
	}
	if v := m.labels.input.Value(); v != "q?" {
		t.Fatalf("expected input value %q, got %q", "q?", v)
	}
	if m.current() != LocLabels {
		t.Fatalf("expected LocLabels, got %v", m.current())
	}
}

func TestLabels_AddEnterReturnsNormal(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('l'))
	m = nm.(Model)
	nm, _ = m.Update(runeKey('a'))
	m = nm.(Model)
	for _, r := range []rune("tier=worker") {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(Model)
	}
	nm, _ = m.Update(key(tea.KeyEnter))
	m = nm.(Model)
	if m.labels.adding {
		t.Fatal("expected adding closed")
	}
	if m.Mode() != ModeNormal {
		t.Fatalf("expected ModeNormal after Enter, got %v", m.Mode())
	}
	node, _ := store.Get("n1")
	if node.Labels["tier"] != "worker" {
		t.Fatalf("expected n1 tier=worker, got %#v", node.Labels)
	}
}
