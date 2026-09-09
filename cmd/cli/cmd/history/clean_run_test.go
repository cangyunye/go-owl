package history

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/internal/i18n"
)

// 评审 P1/B5:clean 迁移到 RunE 后,错误路径(原 os.Exit)才可测试;
// 错误经 RunE 返回由 cobra 打到 stderr,成功输出可经 cmd 流捕获。

func TestCleanRejectsNonPositiveRetention(t *testing.T) {
	t.Setenv("OWL_DB_PATH", filepath.Join(t.TempDir(), "hist.db"))

	var out, errBuf bytes.Buffer
	cmd := NewCleanCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"--days", "0"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --days 0, got nil")
	}
	want := i18n.T("history.clean.err_retention")
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error should contain %q, got: %v", want, err)
	}
}

func TestCleanWithForceRunsAndCleansUp(t *testing.T) {
	t.Setenv("OWL_DB_PATH", filepath.Join(t.TempDir(), "hist.db"))

	var out, errBuf bytes.Buffer
	cmd := NewCleanCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{"--days", "1", "--force"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("clean --force should succeed, got: %v", err)
	}
	if !strings.Contains(out.String(), i18n.T("history.clean.done")) {
		t.Errorf("output should contain done message %q, got: %q",
			i18n.T("history.clean.done"), out.String())
	}
	if errBuf.Len() != 0 {
		t.Errorf("stderr should be empty on success, got: %q", errBuf.String())
	}
}
