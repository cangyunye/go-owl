package settings

import (
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/testutil"
	"github.com/cangyunye/go-owl/internal/i18n"
)

// 评审 P1(docs/review/2026-09-09-cli-review.md):
// settings target/show 的用户可见文案必须走 i18n 目录(CONTEXT.md 规则),
// 且写入 cmd 输出流以便捕获与测试,不得内嵌硬编码文案或直写 stdout。
func TestSettingsTargetShowUsesI18nAndCmdStream(t *testing.T) {
	cmd := NewSettingsTargetCmd()
	out := testutil.ExecuteCommand(t, cmd)

	want := i18n.T("settings.target.show_header")
	if !strings.Contains(out, want) {
		t.Errorf("target show output should contain %q, got:\n%s", want, out)
	}
}

func TestSettingsShowUsesI18nAndCmdStream(t *testing.T) {
	cmd := NewSettingsShowCmd()
	out := testutil.ExecuteCommand(t, cmd)

	want := i18n.T("settings.show.header")
	if !strings.Contains(out, want) {
		t.Errorf("settings show output should contain %q, got:\n%s", want, out)
	}
}
