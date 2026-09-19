package ai

import (
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/internal/i18n"
)

// TestTaskSlashCommandsCatalog 钉住跨端共享的 task 类斜杠命令目录:
// CLI REPL 与 TUI 都从这里取模板,文案走 i18n。
func TestTaskSlashCommandsCatalog(t *testing.T) {
	cmds := TaskSlashCommands()
	want := []string{"exec", "check", "diagnose", "query", "playbook", "transfer", "script"}
	if len(cmds) != len(want) {
		t.Fatalf("expected %d commands, got %d", len(want), len(cmds))
	}
	names := map[string]bool{}
	for _, c := range cmds {
		if c.Category != "task" {
			t.Fatalf("%s: category = %q, want task", c.Name, c.Category)
		}
		if c.Icon == "" || c.Label == "" || c.Desc == "" {
			t.Fatalf("%s: icon/label/desc 不能为空", c.Name)
		}
		if c.Template == "" || !strings.Contains(c.Template, "{") {
			t.Fatalf("%s: 模板应含 {arg} 占位符, got %q", c.Name, c.Template)
		}
		if len(c.Args) == 0 {
			t.Fatalf("%s: Args 不能为空", c.Name)
		}
		if c.Action != nil {
			t.Fatalf("%s: task 类不应有 Action", c.Name)
		}
		names[c.Name] = true
	}
	for _, n := range want {
		if !names[n] {
			t.Fatalf("缺命令 /%s", n)
		}
	}
	// 目录与 i18n 文案同源: 逐条抽查一条
	if got, want := cmds[0].Template, i18n.T("ai.slash.exec_template"); got != want {
		t.Fatalf("exec 模板应取自 i18n: got %q want %q", got, want)
	}
}
