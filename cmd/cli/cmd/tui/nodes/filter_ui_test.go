package nodes

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFilterOpen_EntersInsertMode(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('/'))
	m = nm.(Model)
	if m.Mode() != ModeInsert {
		t.Fatal("expected Insert mode after /")
	}
	if !m.filterOpen {
		t.Fatal("expected filterOpen")
	}
}

func TestFilterType_LiveFilters(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('/'))
	m = nm.(Model)
	for _, r := range []rune("g:web") {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(Model)
	}
	if len(m.visible()) != 2 {
		t.Fatalf("expected 2 visible (n1,n3 in group web), got %d", len(m.visible()))
	}
	if m.visible()[0].ID != "n1" || m.visible()[1].ID != "n3" {
		t.Fatalf("unexpected visible: %s, %s", m.visible()[0].ID, m.visible()[1].ID)
	}
}

func TestFilterEnter_AppliesAndReturnsNormal(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('/'))
	m = nm.(Model)
	for _, r := range []rune("l:env=prod") {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(Model)
	}
	nm, _ = m.Update(key(tea.KeyEnter))
	m = nm.(Model)
	if m.Mode() != ModeNormal {
		t.Fatal("expected Normal after Enter")
	}
	if m.filterOpen {
		t.Fatal("expected filter closed")
	}
	if len(m.visible()) != 2 {
		t.Fatalf("expected 2 visible, got %d", len(m.visible()))
	}
	if m.filterText != "l:env=prod" {
		t.Fatalf("unexpected filterText: %q", m.filterText)
	}
}

func TestFilterEsc_CancelsAndRestores(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('/'))
	m = nm.(Model)
	for _, r := range []rune("g:web") {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(Model)
	}
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(Model)
	if m.Mode() != ModeNormal || m.filterOpen {
		t.Fatal("expected filter closed and Normal after Esc")
	}
	if len(m.visible()) != 3 {
		t.Fatalf("expected all 3 visible after cancel, got %d", len(m.visible()))
	}
}
