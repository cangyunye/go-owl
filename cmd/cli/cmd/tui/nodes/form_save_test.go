package nodes

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFormAdd_SavePersists(t *testing.T) {
	store := newTestStore(t)
	m := openAddForm(t, store)
	m = enterAndType(t, m, "new-node")
	m = moveToAndEdit(t, m, 1)
	m = typeField(m, "web-9")
	m = moveToAndEdit(t, m, 2)
	m = typeField(m, "10.9.9.9")
	m = save(t, m)
	if m.current() != LocList {
		t.Fatalf("expected back to list, got loc %v", m.current())
	}
	got, err := store.Get("new-node")
	if err != nil {
		t.Fatalf("expected node persisted: %v", err)
	}
	if got.Name != "web-9" || got.Address != "10.9.9.9" {
		t.Fatalf("unexpected node: %+v", got)
	}
	if got.Status != "offline" {
		t.Fatalf("expected offline status, got %q", got.Status)
	}
	if m.status == "" {
		t.Fatal("expected status message")
	}
}

func TestFormAdd_Validation_Required(t *testing.T) {
	store := newTestStore(t)
	m := openAddForm(t, store)
	// ID 必填未填,直接保存 → 错误行回显,不弹栈
	nm, _ := m.Update(runeKey('s'))
	m = nm.(Model)
	if m.current() != LocNew {
		t.Fatal("expected stay in form")
	}
	if m.form.error == "" {
		t.Fatal("expected validation error")
	}
}

func TestFormAdd_Validation_Port(t *testing.T) {
	store := newTestStore(t)
	m := openAddForm(t, store)
	m = enterAndType(t, m, "n1")
	m = moveToAndEdit(t, m, 1)
	m = typeField(m, "web")
	m = moveToAndEdit(t, m, 2)
	m = typeField(m, "1.2.3.4")
	m = moveToAndEdit(t, m, 3)
	m = typeField(m, "99999")
	m = save(t, m)
	if m.current() != LocNew {
		t.Fatal("expected stay in form on invalid port")
	}
	if m.form.error == "" || m.form.cursor != 3 {
		t.Fatalf("expected error and focus port, got error=%q cursor=%d", m.form.error, m.form.cursor)
	}
}

func TestFormAdd_DuplicateID(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := openAddForm(t, store)
	m = enterAndType(t, m, "n1")
	m = moveToAndEdit(t, m, 1)
	m = typeField(m, "web")
	m = moveToAndEdit(t, m, 2)
	m = typeField(m, "1.2.3.4")
	m = save(t, m)
	if m.current() != LocNew {
		t.Fatal("expected stay in form on duplicate")
	}
	if m.form.error == "" {
		t.Fatal("expected duplicate error")
	}
}

func TestFormAdd_EscDirty_ConfirmDiscard(t *testing.T) {
	store := newTestStore(t)
	m := openAddForm(t, store)
	m = enterAndType(t, m, "n1")
	// 第一次 Esc:退出输入态回到导航态(此时仍是表单,未触发丢弃确认)
	nm, _ := m.Update(key(tea.KeyEsc))
	m = nm.(Model)
	// 第二次 Esc:导航态 + 有改动 → 进入丢弃确认
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(Model)
	if !m.form.confirmDiscard {
		t.Fatal("expected confirmDiscard when dirty")
	}
	nm, _ = m.Update(runeKey('n'))
	m = nm.(Model)
	if m.current() != LocNew || m.form.confirmDiscard {
		t.Fatal("expected stay in form after n")
	}
	// 再次两次 Esc:先退输入?此时导航态,直接一次 Esc 即确认
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(Model)
	if !m.form.confirmDiscard {
		t.Fatal("expected confirmDiscard again")
	}
	nm, _ = m.Update(runeKey('y'))
	m = nm.(Model)
	if m.current() != LocList {
		t.Fatal("expected back to list after y")
	}
}

func enterAndType(t *testing.T, m Model, s string) Model {
	t.Helper()
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(Model)
	return typeField(m, s)
}

func moveToAndEdit(t *testing.T, m Model, target int) Model {
	t.Helper()
	if m.form == nil {
		t.Fatal("form nil")
	}
	// 若仍处输入态,先 Esc 退回导航态,否则 Down 移动的是文本光标而非字段
	if m.Mode() == ModeInsert {
		nm, _ := m.Update(key(tea.KeyEsc))
		m = nm.(Model)
	}
	for m.form.cursor != target {
		nm, _ := m.Update(key(tea.KeyDown))
		m = nm.(Model)
		if m.form.cursor == target {
			break
		}
	}
	nm, _ := m.Update(key(tea.KeyEnter))
	return nm.(Model)
}

// save 退出输入态后按 s 保存(输入态按 s 只会输入字符)
func save(t *testing.T, m Model) Model {
	t.Helper()
	if m.Mode() == ModeInsert {
		nm, _ := m.Update(key(tea.KeyEsc))
		m = nm.(Model)
	}
	nm, _ := m.Update(runeKey('s'))
	return nm.(Model)
}

func TestFormAdd_Validation_PortOutOfRange_FocusJumps(t *testing.T) {
	store := newTestStore(t)
	m := openAddForm(t, store)
	m = enterAndType(t, m, "n1")
	m = moveToAndEdit(t, m, 1)
	m = typeField(m, "web")
	m = moveToAndEdit(t, m, 2)
	m = typeField(m, "1.2.3.4")
	// 直接设置超范围端口(有效 int 但 >65535),并把光标移到 User(4)
	m.form.fields[3].input.SetValue("70000")
	m.form.cursor = 4
	m = save(t, m)
	if m.current() != LocNew {
		t.Fatal("expected stay in form on out-of-range port")
	}
	if m.form.cursor != 3 {
		t.Fatalf("expected focus jump to Port(3), got %d", m.form.cursor)
	}
}

func TestFormEdit_StatusField_Prefilled(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('e'))
	m = nm.(Model)
	idx := len(m.form.fields) - 1 // status 是最后一个字段
	if m.form.fields[idx].key != "status" {
		t.Fatalf("expected last field status, got %q", m.form.fields[idx].key)
	}
	if m.form.fields[idx].input.Value() != "online" {
		t.Fatalf("expected prefilled online, got %q", m.form.fields[idx].input.Value())
	}
}

func TestFormEdit_SaveStatusOffline(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('e'))
	m = nm.(Model)
	idx := len(m.form.fields) - 1
	m.form.fields[idx].input.SetValue("offline")
	nm, _ = m.Update(runeKey('s'))
	m = nm.(Model)
	if m.current() != LocList {
		t.Fatalf("expected back to list, got %v", m.current())
	}
	got, _ := store.Get("n1")
	if got.Status != "offline" {
		t.Fatalf("expected status offline, got %q", got.Status)
	}
}

func TestFormEdit_SaveStatusInvalid(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('e'))
	m = nm.(Model)
	idx := len(m.form.fields) - 1
	m.form.fields[idx].input.SetValue("bogus")
	nm, _ = m.Update(runeKey('s'))
	m = nm.(Model)
	if m.current() != LocEdit {
		t.Fatalf("expected stay in form on invalid status")
	}
	if m.form.error == "" {
		t.Fatal("expected validation error")
	}
}
