package ai

import (
	"github.com/cangyunye/go-owl/cmd/cli/cmd/ai/input"
	"github.com/cangyunye/go-owl/internal/i18n"
)

// TaskSlashCommands 跨端共享的 task 类斜杠命令目录(选中后展开提示词模板)。
// CLI REPL 与 TUI AI 面板共用,保证模板文案单一来源;action 类命令
// (/help /new /clear /quit)由各宿主按自身语义自行注入。
func TaskSlashCommands() []input.SlashCommand {
	return []input.SlashCommand{
		{Name: "exec", Category: "task", Icon: "▶️", Label: i18n.T("ai.slash.exec_label"), Desc: i18n.T("ai.slash.exec_desc"), Template: i18n.T("ai.slash.exec_template"), Args: []string{"nodes", "command"}},
		{Name: "check", Category: "task", Icon: "🩺", Label: i18n.T("ai.slash.check_label"), Desc: i18n.T("ai.slash.check_desc"), Template: i18n.T("ai.slash.check_template"), Args: []string{"nodes"}},
		{Name: "diagnose", Category: "task", Icon: "🔍", Label: i18n.T("ai.slash.diagnose_label"), Desc: i18n.T("ai.slash.diagnose_desc"), Template: i18n.T("ai.slash.diagnose_template"), Args: []string{"target"}},
		{Name: "query", Category: "task", Icon: "📊", Label: i18n.T("ai.slash.query_label"), Desc: i18n.T("ai.slash.query_desc"), Template: i18n.T("ai.slash.query_template"), Args: []string{"condition"}},
		{Name: "playbook", Category: "task", Icon: "🛠️", Label: i18n.T("ai.slash.playbook_label"), Desc: i18n.T("ai.slash.playbook_desc"), Template: i18n.T("ai.slash.playbook_template"), Args: []string{"requirement"}},
		{Name: "transfer", Category: "task", Icon: "📤", Label: i18n.T("ai.slash.transfer_label"), Desc: i18n.T("ai.slash.transfer_desc"), Template: i18n.T("ai.slash.transfer_template"), Args: []string{"source_file", "nodes", "dest_dir"}},
		{Name: "script", Category: "task", Icon: "🧩", Label: i18n.T("ai.slash.script_label"), Desc: i18n.T("ai.slash.script_desc"), Template: i18n.T("ai.slash.script_template"), Args: []string{"nodes", "script"}},
	}
}
