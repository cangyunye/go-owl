package ai

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 评审 P1(docs/review/2026-09-09-cli-review.md):
// 命令错误必须走 cmd.ErrOrStderr(可捕获、可分离重定向),
// 成功输出走 cmd.OutOrStdout;不得直写进程 stdout,也不得用 os.Exit 跳过 defer。

func writeBrokenAIConfig(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".owl")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir .owl: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("a: b\n- c\n"), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func TestConfigShowErrorGoesToStderr(t *testing.T) {
	writeBrokenAIConfig(t)

	var out, errBuf bytes.Buffer
	cmd := NewConfigShowCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when config file is broken")
	}
	if out.Len() != 0 {
		t.Errorf("stdout should be empty on error, got: %q", out.String())
	}
	if !strings.Contains(errBuf.String(), "config") {
		t.Errorf("stderr should carry the load error, got: %q", errBuf.String())
	}
}

func TestConfigInitOutputCapturable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	var out, errBuf bytes.Buffer
	cmd := NewConfigInitCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("config init should succeed, got: %v", err)
	}
	if !strings.Contains(out.String(), "config.yaml") {
		t.Errorf("init output should mention created config path via cmd stream, got: %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".owl", "config.yaml")); err != nil {
		t.Errorf("config file should be created: %v", err)
	}
}
