package file

import (
	"os"
	"path/filepath"
	"testing"
)

func seedDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// 文件
	for _, n := range []string{"a.txt", "c.log", ".hidden"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// 目录
	for _, n := range []string{"b_dir", "d_sub"} {
		if err := os.Mkdir(filepath.Join(dir, n), 0755); err != nil {
			t.Fatal(err)
		}
	}
	// 隐藏目录
	if err := os.Mkdir(filepath.Join(dir, ".secret"), 0755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestBrowser_NewListsSortedDirsFirst(t *testing.T) {
	b := NewFileBrowser(seedDir(t))
	if b.err != "" {
		t.Fatalf("unexpected err: %s", b.err)
	}
	want := []string{"b_dir", "d_sub", "a.txt", "c.log"}
	got := make([]string, len(b.entries))
	for i, e := range b.entries {
		got[i] = e.Name
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("entry %d: want %s got %s (all: %v)", i, want[i], got[i], got)
		}
	}
	if !b.entries[0].IsDir {
		t.Fatal("expected first entry to be a dir")
	}
}

func TestBrowser_HiddenFilteredAndToggle(t *testing.T) {
	b := NewFileBrowser(seedDir(t))
	for _, e := range b.entries {
		if e.Name == ".hidden" || e.Name == ".secret" {
			t.Fatalf("hidden entries must be filtered: %s", e.Name)
		}
	}
	b.ToggleHidden()
	found := false
	for _, e := range b.entries {
		if e.Name == ".hidden" {
			found = true
		}
	}
	if !found {
		t.Fatal("hidden entries must appear after toggle")
	}
}

func TestBrowser_EnterDirMovesCursorAndReloads(t *testing.T) {
	root := seedDir(t)
	b := NewFileBrowser(root)
	// cursor 0 = b_dir
	path, isFile, err := b.Enter()
	if err != nil {
		t.Fatal(err)
	}
	if isFile {
		t.Fatal("expected dir")
	}
	if path != filepath.Join(root, "b_dir") {
		t.Fatalf("expected %s, got %s", filepath.Join(root, "b_dir"), path)
	}
	if b.dir != filepath.Join(root, "b_dir") {
		t.Fatalf("expected dir switch, got %s", b.dir)
	}
}

func TestBrowser_ParentReturns(t *testing.T) {
	root := seedDir(t)
	b := NewFileBrowser(root)
	b.Jump(filepath.Join(root, "b_dir"))
	if !b.Parent() {
		t.Fatal("expected parent success")
	}
	if b.dir != root {
		t.Fatalf("expected back to %s, got %s", root, b.dir)
	}
	// t.TempDir() 不在文件系统根, 先跳到真正的根再断言
	volRoot := filepath.VolumeName(root) + string(os.PathSeparator)
	b.dir = volRoot
	if b.Parent() {
		t.Fatal("expected parent failure at root")
	}
}

func TestBrowser_EnterFileReturnsAbsPath(t *testing.T) {
	root := seedDir(t)
	b := NewFileBrowser(root)
	// 移到 a.txt (index 2)
	b.cursor = 2
	path, isFile, err := b.Enter()
	if err != nil {
		t.Fatal(err)
	}
	if !isFile {
		t.Fatal("expected file")
	}
	if path != filepath.Join(root, "a.txt") {
		t.Fatalf("expected %s, got %s", filepath.Join(root, "a.txt"), path)
	}
}

func TestBrowser_JumpToDirAndFile(t *testing.T) {
	root := seedDir(t)
	b := NewFileBrowser(root)
	sub := filepath.Join(root, "b_dir")
	if err := b.Jump(sub); err != nil {
		t.Fatal(err)
	}
	if b.dir != sub {
		t.Fatalf("expected dir %s, got %s", sub, b.dir)
	}
	f := filepath.Join(root, "a.txt")
	if err := b.Jump(f); err != nil {
		t.Fatal(err)
	}
	if got := b.currentPath(); got != f {
		t.Fatalf("expected file path %s, got %s", f, got)
	}
}

func TestBrowser_JumpInvalidKeepsDir(t *testing.T) {
	root := seedDir(t)
	b := NewFileBrowser(root)
	if err := b.Jump(filepath.Join(root, "no-such")); err == nil {
		t.Fatal("expected error for missing path")
	}
	if b.dir != root {
		t.Fatalf("dir must stay %s, got %s", root, b.dir)
	}
}

func TestBrowser_CursorClamped(t *testing.T) {
	b := NewFileBrowser(seedDir(t))
	for i := 0; i < 10; i++ {
		b.Down()
	}
	if b.cursor >= len(b.entries) {
		t.Fatalf("cursor out of range: %d >= %d", b.cursor, len(b.entries))
	}
	for i := 0; i < 10; i++ {
		b.Up()
	}
	if b.cursor < 0 {
		t.Fatalf("cursor negative: %d", b.cursor)
	}
}
