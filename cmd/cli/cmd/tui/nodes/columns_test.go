package nodes

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewColumnsModel_Default(t *testing.T) {
	cm := NewColumnsModel(defaultColumnKeys)
	if len(cm.order) != len(columnDefs) {
		t.Fatalf("expected %d order, got %d", len(columnDefs), len(cm.order))
	}
	got := cm.selected()
	if len(got) != 4 || got[0] != "id" || got[1] != "name" || got[2] != "address" || got[3] != "status" {
		t.Fatalf("unexpected selected: %v", got)
	}
}

func TestColumns_ToggleAndApply(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('c'))
	m = nm.(Model)
	if m.current() != LocColumns {
		t.Fatal("expected in columns")
	}
	// cursor 在 id(第 0),按 Space 取消 id
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = nm.(Model)
	nm, _ = m.Update(key(tea.KeyEnter))
	m = nm.(Model)
	if m.current() != LocList {
		t.Fatal("expected back to list")
	}
	if len(m.columns) != 3 {
		t.Fatalf("expected 3 columns after unchecking id, got %v", m.columns)
	}
	for _, c := range m.columns {
		if c == "id" {
			t.Fatal("id should be removed")
		}
	}
}

func TestColumns_SelectAllAndReset(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('c'))
	m = nm.(Model)
	nm, _ = m.Update(runeKey('a'))
	m = nm.(Model)
	if len(m.columns) != len(columnDefs) {
		t.Fatalf("expected all columns, got %v", m.columns)
	}
	nm, _ = m.Update(runeKey('r'))
	m = nm.(Model)
	if len(m.columns) != 4 {
		t.Fatalf("expected default 4 columns after reset, got %v", m.columns)
	}
}

func TestColumns_EscRestoresSnapshot(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('c'))
	m = nm.(Model)
	nm, _ = m.Update(runeKey('a'))
	m = nm.(Model)
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(Model)
	if m.current() != LocList {
		t.Fatal("expected back to list")
	}
	if len(m.columns) != 4 {
		t.Fatalf("expected snapshot restored to 4, got %v", m.columns)
	}
}

func TestColumnsModel_Methods(t *testing.T) {
	cm := NewColumnsModel(defaultColumnKeys)
	cm.toggle(0)
	if cm.checked[0] {
		t.Fatal("expected id unchecked after toggle")
	}
	cm.selectAll()
	for i, c := range cm.checked {
		if !c {
			t.Fatalf("expected index %d checked", i)
		}
	}
	cm.reset()
	if len(cm.selected()) != 4 {
		t.Fatalf("expected 4 after reset, got %v", cm.selected())
	}
	cm.toggle(0)
	cm.restoreSnapshot()
	if len(cm.selected()) != 4 {
		t.Fatalf("expected snapshot restore, got %v", cm.selected())
	}
}
