package serve

import (
	"os"
	"path/filepath"
	"testing"
)

// owl.db 存有节点凭据（未启用加密时明文）与 jwt_secret，任何本地用户
// 可读即可伪造 JWT / 拖走全部节点凭据。目录与库文件权限必须收紧：
// 目录 0700、库文件（含 WAL/SHM）0600。

func TestEnsureDBDir_Permissions(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "sub", "dir", "owl.db")
	if err := ensureDBDir(dbPath); err != nil {
		t.Fatalf("ensureDBDir: %v", err)
	}
	info, err := os.Stat(filepath.Dir(dbPath))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0700 {
		t.Fatalf("数据库目录权限应为 0700，实际 %v", got)
	}
}

func TestHardenDBFile_Permissions(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "owl.db")
	if err := os.WriteFile(dbPath, []byte("sqlite"), 0644); err != nil {
		t.Fatal(err)
	}
	walPath := dbPath + "-wal"
	if err := os.WriteFile(walPath, []byte("wal"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := hardenDBFile(dbPath); err != nil {
		t.Fatalf("hardenDBFile: %v", err)
	}

	for _, p := range []string{dbPath, walPath} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0600 {
			t.Fatalf("%s 权限应为 0600，实际 %v", p, got)
		}
	}
}

func TestHardenDBFile_MissingFileNoError(t *testing.T) {
	// 文件尚不存在（首次启动 Init 之前调用）不应报错
	if err := hardenDBFile(filepath.Join(t.TempDir(), "not-exist.db")); err != nil {
		t.Fatalf("不存在的库文件不应报错: %v", err)
	}
}
