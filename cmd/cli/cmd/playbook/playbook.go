package playbook

import (
	"github.com/spf13/cobra"
)

// NewPlaybookCmd 创建剧本管理命令
func NewPlaybookCmd() *cobra.Command {
	pbCmd := &cobra.Command{
		Use:   "playbook",
		Short: "剧本管理",
		Long: `剧本管理命令，支持以下操作：

- list: 列出剧本
- validate: 验证剧本语法
- info: 显示剧本信息
- run: 执行剧本`,
	}

	pbCmd.AddCommand(NewPlaybookListCmd())
	pbCmd.AddCommand(NewPlaybookValidateCmd())
	pbCmd.AddCommand(NewPlaybookInfoCmd())
	pbCmd.AddCommand(NewPlaybookRunCmd())

	return pbCmd
}
