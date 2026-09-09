package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

// 评审 P0-2(docs/review/2026-09-09-cli-review.md):
// --help/--version 等纯展示路径不应打开历史库,节点存储应保持
// common 包 init() 提供的内存实现;真实子命令执行时才升级为 DB-backed。

func setOWLDBPath(t *testing.T) {
	t.Helper()
	t.Setenv("OWL_DB_PATH", filepath.Join(t.TempDir(), "lifecycle.db"))
}

func withArgs(t *testing.T, args ...string) {
	t.Helper()
	old := os.Args
	os.Args = append([]string{"owl"}, args...)
	t.Cleanup(func() { os.Args = old })
}

func withStoreRestore(t *testing.T) {
	t.Helper()
	storeBefore := common.GetNodeStore()
	t.Cleanup(func() { common.SetNodeStore(storeBefore) })
}

func TestExecuteHelpDoesNotOpenDB(t *testing.T) {
	setOWLDBPath(t)
	withStoreRestore(t)
	withArgs(t, "--help")

	if err := Execute(); err != nil {
		t.Fatalf("Execute(--help) returned error: %v", err)
	}

	dbPath := os.Getenv("OWL_DB_PATH")
	if _, err := os.Stat(dbPath); err == nil {
		t.Errorf("history DB file should not be created for --help, but exists: %s", dbPath)
	} else if !os.IsNotExist(err) {
		t.Fatalf("unexpected stat error: %v", err)
	}

	if _, ok := common.GetNodeStore().(*common.NodeStoreDB); ok {
		t.Error("node store should stay in-memory for --help, but got DB-backed store")
	}
}

func TestExecuteSubcommandUpgradesNodeStoreToDB(t *testing.T) {
	setOWLDBPath(t)
	withStoreRestore(t)
	withArgs(t, "settings", "show")

	if err := Execute(); err != nil {
		t.Fatalf("Execute(settings show) returned error: %v", err)
	}

	if _, ok := common.GetNodeStore().(*common.NodeStoreDB); !ok {
		t.Error("node store should be DB-backed after a real subcommand runs")
	}
}
