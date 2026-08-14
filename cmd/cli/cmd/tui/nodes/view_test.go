package nodes

import (
	"strings"
	"testing"
)

func TestView_ListRendersNodesAndDetail(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	v := m.View()
	if !strings.Contains(v, "n1") || !strings.Contains(v, "web-1") {
		t.Fatalf("list missing node: %q", v)
	}
	if !strings.Contains(v, "db-1") {
		t.Fatalf("detail missing selected node name: %q", v)
	}
	if !strings.Contains(v, "env=prod") {
		t.Fatalf("detail missing labels: %q", v)
	}
	if !strings.Contains(v, "Groups") {
		t.Fatalf("detail missing Groups label: %q", v)
	}
}

func TestView_EmptyList(t *testing.T) {
	m := NewModel(newTestStore(t))
	if m.View() == "" {
		t.Fatal("expected non-empty empty-state view")
	}
}

func TestView_FormRendersFields(t *testing.T) {
	store := newTestStore(t)
	m := openAddForm(t, store)
	v := m.View()
	for _, label := range []string{"ID", "Name", "Address", "Port", "Groups", "Labels"} {
		if !strings.Contains(v, label) {
			t.Fatalf("form missing %s: %q", label, v)
		}
	}
}

func TestView_ConfirmRendersNode(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('d'))
	m = nm.(Model)
	v := m.View()
	if !strings.Contains(v, "n1") {
		t.Fatalf("confirm missing node: %q", v)
	}
}

func TestView_ColumnsRendersFields(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('c'))
	m = nm.(Model)
	v := m.View()
	for _, label := range []string{"id", "name", "status", "labels"} {
		if !strings.Contains(v, label) {
			t.Fatalf("columns missing %s: %q", label, v)
		}
	}
}
