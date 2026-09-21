package history

import (
	"path/filepath"
	"testing"
)

// TestNewDB_SamePathReusesInstance：同一路径重复调用 NewDB 必须复用已打开
// 的连接池，不再重复 Open+InitSchema（此前每条 owl 命令会泄漏 2-3 个连接）。
func TestNewDB_SamePathReusesInstance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reuse.db")
	db1, err := NewDB(&Config{Enabled: true, DBPath: path})
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	defer db1.Close()

	db2, err := NewDB(&Config{Enabled: true, DBPath: path})
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	if db1 != db2 {
		t.Fatal("expected same instance for same path")
	}
	if GetGlobalDB() != db1 {
		t.Fatal("global DB should stay bound to the instance")
	}
}

// TestNewDB_ReopenAfterCloseCreatesFresh：Close 后同路径重开必须得到新实例。
func TestNewDB_ReopenAfterCloseCreatesFresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reopen.db")
	db1, err := NewDB(&Config{Enabled: true, DBPath: path})
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if GetGlobalDB() != nil {
		t.Fatal("Close should unbind the global DB")
	}

	db2, err := NewDB(&Config{Enabled: true, DBPath: path})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()
	if db1 == db2 {
		t.Fatal("expected a fresh instance after Close")
	}
}
