package nodes

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestEditForm_PrefilledAndReadonlyID(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('e'))
	m = nm.(Model)
	if m.current() != LocEdit {
		t.Fatal("expected LocEdit")
	}
	if m.form == nil {
		t.Fatal("expected form")
	}
	// ID 只读且预填
	if m.form.fields[0].editable {
		t.Fatal("expected ID readonly in edit")
	}
	if m.form.fields[0].input.Value() != "n1" {
		t.Fatalf("expected prefilled id n1, got %q", m.form.fields[0].input.Value())
	}
	if m.form.cursor != 1 {
		t.Fatalf("expected cursor at Name(1), got %d", m.form.cursor)
	}
	path := m.Path()
	if len(path) != 3 || path[1] != "n1" || path[2] != "edit" {
		t.Fatalf("unexpected path: %v", path)
	}
}

func TestEditForm_SaveUpdates(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('e'))
	m = nm.(Model)
	// 光标在 Name(1)。Name 已预填 "web-1",直接改值(打字会拼接,故用 SetValue)
	m.form.fields[1].input.SetValue("web-9")
	// 编辑表单此刻为导航态,直接 s 保存
	nm, _ = m.Update(runeKey('s'))
	m = nm.(Model)
	if m.current() != LocList {
		t.Fatal("expected back to list")
	}
	got, _ := store.Get("n1")
	if got.Name != "web-9" {
		t.Fatalf("expected updated name, got %q", got.Name)
	}
	// 编辑不应清空原有字段(Address/Port 保留)
	if got.Address != "10.0.0.1" {
		t.Fatalf("expected address preserved, got %q", got.Address)
	}
}

func TestConfirm_OpenAndPath(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('d'))
	m = nm.(Model)
	if m.current() != LocDelete {
		t.Fatal("expected LocDelete")
	}
	if m.confirm == nil || m.confirm.node.ID != "n1" {
		t.Fatalf("unexpected confirm: %+v", m.confirm)
	}
	path := m.Path()
	if len(path) != 3 || path[1] != "n1" || path[2] != "delete" {
		t.Fatalf("unexpected path: %v", path)
	}
}

func TestConfirm_DeleteExecutes(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('d'))
	m = nm.(Model)
	// 默认光标在 Delete(0),Enter 执行
	nm, _ = m.Update(key(tea.KeyEnter))
	m = nm.(Model)
	if m.current() != LocList {
		t.Fatal("expected back to list")
	}
	if _, err := store.Get("n1"); err == nil {
		t.Fatal("expected node removed")
	}
	if m.status == "" {
		t.Fatal("expected status message")
	}
}

func TestConfirm_Cancel(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('d'))
	m = nm.(Model)
	nm, _ = m.Update(key(tea.KeyRight)) // 切到 Cancel
	m = nm.(Model)
	nm, _ = m.Update(key(tea.KeyEnter))
	m = nm.(Model)
	if m.current() != LocList {
		t.Fatal("expected back to list without delete")
	}
	if _, err := store.Get("n1"); err != nil {
		t.Fatal("expected node still present")
	}
}

func TestConfirm_EscCancels(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('d'))
	m = nm.(Model)
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(Model)
	if m.current() != LocList {
		t.Fatal("expected back to list")
	}
	if _, err := store.Get("n1"); err != nil {
		t.Fatal("expected node still present")
	}
}
