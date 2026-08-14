package nodes

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

func openAddForm(t *testing.T, store common.NodeStore) Model {
	t.Helper()
	m := NewModel(store)
	nm, _ := m.Update(runeKey('a'))
	return nm.(Model)
}

func typeField(m Model, runes string) Model {
	for _, r := range runes {
		nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(Model)
	}
	return m
}

func TestOpenAddForm_PathAndCursor(t *testing.T) {
	store := newTestStore(t)
	m := openAddForm(t, store)
	if m.current() != LocNew {
		t.Fatal("expected LocNew")
	}
	if m.form == nil {
		t.Fatal("expected form non-nil")
	}
	path := m.Path()
	if len(path) != 2 || path[1] != "new" {
		t.Fatalf("unexpected path: %v", path)
	}
	if m.form.cursor != 0 {
		t.Fatalf("expected cursor 0 (ID), got %d", m.form.cursor)
	}
	if m.Mode() != ModeNormal {
		t.Fatal("expected Normal navigate mode on open")
	}
}

func TestFormEnter_EntersInsertMode(t *testing.T) {
	m := openAddForm(t, newTestStore(t))
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(Model)
	if m.Mode() != ModeInsert {
		t.Fatal("expected Insert after Enter on field")
	}
}

func TestFormInsert_Isolation_NoGlobalKeys(t *testing.T) {
	m := openAddForm(t, newTestStore(t))
	nm, _ := m.Update(key(tea.KeyEnter)) // 进入 ID 输入
	m = nm.(Model)
	before := m.form.cursor
	// 在输入态下发 q / s / 方向键 / Esc 之外的所有键,都不应触发保存/导航/回列表
	for _, r := range []rune("qsa./") {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(Model)
		if m.Mode() != ModeInsert {
			t.Fatalf("mode broke on %q", r)
		}
		if m.current() != LocNew {
			t.Fatalf("location popped on %q", r)
		}
		if m.form.cursor != before {
			t.Fatalf("form cursor moved on %q", r)
		}
	}
	if got := m.form.fields[0].input.Value(); got != "qsa./" {
		t.Fatalf("expected value qsa./, got %q", got)
	}
}

func TestFormEsc_ExitsInsertMode(t *testing.T) {
	m := openAddForm(t, newTestStore(t))
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(Model)
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(Model)
	if m.Mode() != ModeNormal {
		t.Fatal("expected Normal after Esc in insert")
	}
	if m.current() != LocNew {
		t.Fatal("expected still in form")
	}
}

func TestFormWrapAround_UpFromTop(t *testing.T) {
	m := openAddForm(t, newTestStore(t))
	// 首个可编辑字段是 ID(0);按 ↑ 应回卷到最后一个可编辑字段(Labels, 9)
	nm, _ := m.Update(key(tea.KeyUp))
	m = nm.(Model)
	if m.form.cursor != len(m.form.fields)-1 {
		t.Fatalf("expected wrap to last field %d, got %d", len(m.form.fields)-1, m.form.cursor)
	}
}

func TestFormWrapAround_DownFromBottom(t *testing.T) {
	m := openAddForm(t, newTestStore(t))
	// 跳到最后一个可编辑字段,再按 ↓ 回卷到 ID
	m.form.cursor = len(m.form.fields) - 1
	nm, _ := m.Update(key(tea.KeyDown))
	m = nm.(Model)
	if m.form.cursor != 0 {
		t.Fatalf("expected wrap to first field 0, got %d", m.form.cursor)
	}
}

func TestFormMove_SkipsReadonlyInEdit(t *testing.T) {
	node := &common.NodeInfo{ID: "n1", Name: "web", Address: "1.2.3.4", Port: 22}
	f := NewFormModel(FormEdit, node)
	if f.cursor != 1 {
		t.Fatalf("edit form should start at first editable field (Name), got %d", f.cursor)
	}
	// 从 Name(1) 按 ↑ 回卷到 Labels(9),不会落到只读 ID(0)
	f.move(-1)
	if f.cursor != len(f.fields)-1 {
		t.Fatalf("expected wrap to last field, got %d", f.cursor)
	}
}
