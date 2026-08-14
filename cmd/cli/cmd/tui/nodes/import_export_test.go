package nodes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestModel_DoExport_YAML(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	path := filepath.Join(t.TempDir(), "nodes.yaml")
	if err := m.doExport(path, "yaml"); err != nil {
		t.Fatalf("export: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), "n1") || !strings.Contains(string(data), "web-1") {
		t.Fatalf("export missing nodes: %s", data)
	}
}

func TestModel_DoExport_JSON(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	path := filepath.Join(t.TempDir(), "nodes.json")
	if err := m.doExport(path, "json"); err != nil {
		t.Fatalf("export: %v", err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "n1") {
		t.Fatalf("export missing node: %s", data)
	}
}

func TestModel_DoImport_NewNodes(t *testing.T) {
	store := newTestStore(t)
	m := NewModel(store)
	path := filepath.Join(t.TempDir(), "in.yaml")
	content := "version: \"1.0\"\nnodes:\n  - id: imp1\n    name: imp-1\n    address: 10.9.9.9\n    port: 22\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := m.doImport(path, false); err != nil {
		t.Fatalf("import: %v", err)
	}
	got, err := store.Get("imp1")
	if err != nil || got.Name != "imp-1" {
		t.Fatalf("imported node wrong: %+v err=%v", got, err)
	}
}

func TestModel_DoImport_SkipExisting(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	path := filepath.Join(t.TempDir(), "in.yaml")
	content := "version: \"1.0\"\nnodes:\n  - id: n1\n    name: replaced\n    address: 10.9.9.9\n    port: 22\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// 不 overwrite → 跳过既有节点,名称不变
	if err := m.doImport(path, false); err != nil {
		t.Fatalf("import: %v", err)
	}
	got, _ := store.Get("n1")
	if got.Name != "web-1" {
		t.Fatalf("expected skip existing, got name %q", got.Name)
	}
}

func TestModel_DoImport_OverwriteExisting(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	path := filepath.Join(t.TempDir(), "in.yaml")
	content := "version: \"1.0\"\nnodes:\n  - id: n1\n    name: replaced\n    address: 10.9.9.9\n    port: 22\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := m.doImport(path, true); err != nil {
		t.Fatalf("import: %v", err)
	}
	got, _ := store.Get("n1")
	if got.Name != "replaced" {
		t.Fatalf("expected overwrite, got name %q", got.Name)
	}
}

func TestModel_OpenImportExport_FromList(t *testing.T) {
	store := newTestStore(t)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('i'))
	m = nm.(Model)
	if m.current() != LocImportExport {
		t.Fatalf("expected LocImportExport, got %v", m.current())
	}
	if m.importExport == nil {
		t.Fatal("expected importExport model")
	}
	path := m.Path()
	if len(path) != 2 || path[1] != "import" {
		t.Fatalf("unexpected path: %v", path)
	}
}

func TestImportExport_InsertModeTypesPath(t *testing.T) {
	store := newTestStore(t)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('i'))
	m = nm.(Model)
	if m.Mode() != ModeInsert {
		t.Fatalf("expected ModeInsert on open, got %v", m.Mode())
	}
	if !m.importExport.path.Focused() {
		t.Fatal("expected path input focused on open")
	}
	// f/o 应被输入进路径,而不是切换格式/覆盖
	for _, r := range []rune("f/o") {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(Model)
	}
	if v := m.importExport.path.Value(); v != "f/o" {
		t.Fatalf("expected path value %q, got %q", "f/o", v)
	}
	if m.importExport.format != "yaml" {
		t.Fatalf("expected format still yaml, got %q", m.importExport.format)
	}
	if m.importExport.overwrite {
		t.Fatal("expected overwrite still false")
	}
	if m.current() != LocImportExport {
		t.Fatalf("expected LocImportExport, got %v", m.current())
	}
}

func TestImportExport_EscExitsInsertThenToggles(t *testing.T) {
	store := newTestStore(t)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('i'))
	m = nm.(Model)
	// Esc:退出 Insert,路径失焦,留在视图
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(Model)
	if m.Mode() != ModeNormal {
		t.Fatalf("expected ModeNormal after Esc, got %v", m.Mode())
	}
	if m.importExport.path.Focused() {
		t.Fatal("expected path blurred after Esc")
	}
	if m.current() != LocImportExport {
		t.Fatalf("expected still LocImportExport after Esc, got %v", m.current())
	}
	// Normal 态:命令键生效
	nm, _ = m.Update(runeKey('f'))
	m = nm.(Model)
	if m.importExport.format != "json" {
		t.Fatalf("expected format json after f, got %q", m.importExport.format)
	}
	nm, _ = m.Update(runeKey('o'))
	m = nm.(Model)
	if !m.importExport.overwrite {
		t.Fatal("expected overwrite true after o")
	}
	nm, _ = m.Update(key(tea.KeyRight))
	m = nm.(Model)
	if m.importExport.op != "import" {
		t.Fatalf("expected op import after right, got %q", m.importExport.op)
	}
	nm, _ = m.Update(runeKey('e'))
	m = nm.(Model)
	if m.Mode() != ModeInsert {
		t.Fatalf("expected ModeInsert after e, got %v", m.Mode())
	}
	if !m.importExport.path.Focused() {
		t.Fatal("expected path focused after e")
	}
}

func TestImportExport_EnterExecuteReturnsToNormalMode(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('i'))
	m = nm.(Model)
	if m.Mode() != ModeInsert {
		t.Fatalf("expected ModeInsert on open, got %v", m.Mode())
	}
	m.importExport.path.SetValue(filepath.Join(t.TempDir(), "out.yaml"))
	nm, _ = m.Update(key(tea.KeyEnter))
	m = nm.(Model)
	if m.current() != LocList {
		t.Fatalf("expected LocList after enter execute, got %v", m.current())
	}
	if m.Mode() != ModeNormal {
		t.Fatalf("expected ModeNormal after enter execute, got %v", m.Mode())
	}
}
