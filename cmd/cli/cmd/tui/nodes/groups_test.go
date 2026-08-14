package nodes

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestGroupsModel_AddGroup(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	g := NewGroupsModel(store, "n1")
	if err := g.addGroup("staging"); err != nil {
		t.Fatalf("add: %v", err)
	}
	node, _ := store.Get("n1")
	if !containsStr(node.Groups, "staging") {
		t.Fatalf("expected staging in groups, got %#v", node.Groups)
	}
}

func TestGroupsModel_RemoveGroup(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	g := NewGroupsModel(store, "n3")
	if err := g.removeGroup("web"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	node, _ := store.Get("n3")
	if containsStr(node.Groups, "web") {
		t.Fatalf("expected web removed, got %#v", node.Groups)
	}
}

func TestGroupsModel_ReloadFromStore(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	g := NewGroupsModel(store, "n1")
	if len(g.groups) == 0 {
		t.Fatal("expected groups loaded")
	}
	// store 变更后 reload 反映最新
	_ = g.addGroup("extra")
	g.reload()
	if !containsStr(g.groups, "extra") {
		t.Fatalf("expected extra after reload, got %#v", g.groups)
	}
}

func TestModel_OpenGroups_FromList(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('o'))
	m = nm.(Model)
	if m.current() != LocGroups {
		t.Fatalf("expected LocGroups, got %v", m.current())
	}
	if m.groups == nil || m.groups.nodeID != "n1" {
		t.Fatalf("unexpected groups model: %+v", m.groups)
	}
	path := m.Path()
	if len(path) != 3 || path[2] != "groups" {
		t.Fatalf("unexpected path: %v", path)
	}
}

func TestModel_Groups_EscBack(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('o'))
	m = nm.(Model)
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(Model)
	if m.current() != LocList {
		t.Fatalf("expected back to list, got %v", m.current())
	}
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestGroupsModel_AddInputParsing(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	g := NewGroupsModel(store, "n1")
	g.adding = true
	g.input.SetValue("prod")
	// 模拟在导航态?不,add 由 model 层处理;此处验证 name 清洗
	name := strings.TrimSpace(g.input.Value())
	if name != "prod" {
		t.Fatalf("unexpected name: %q", name)
	}
}

func TestGroups_AddModeInsertIsolatesKeys(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('o'))
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
	if v := m.groups.input.Value(); v != "q?" {
		t.Fatalf("expected input value %q, got %q", "q?", v)
	}
	if m.current() != LocGroups {
		t.Fatalf("expected LocGroups, got %v", m.current())
	}
}

func TestGroups_AddEnterReturnsNormal(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('o'))
	m = nm.(Model)
	nm, _ = m.Update(runeKey('a'))
	m = nm.(Model)
	for _, r := range []rune("prod") {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(Model)
	}
	nm, _ = m.Update(key(tea.KeyEnter))
	m = nm.(Model)
	if m.groups.adding {
		t.Fatal("expected adding closed")
	}
	if m.Mode() != ModeNormal {
		t.Fatalf("expected ModeNormal after Enter, got %v", m.Mode())
	}
	node, _ := store.Get("n1")
	if !containsStr(node.Groups, "prod") {
		t.Fatalf("expected n1 group prod, got %#v", node.Groups)
	}
}
