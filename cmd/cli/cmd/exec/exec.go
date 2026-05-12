package exec

import (
	"github.com/spf13/cobra"
)

// NewExecCmd 创建执行命令
func NewExecCmd() *cobra.Command {
	execCmd := &cobra.Command{
		Use:   "exec",
		Short: "命令和脚本执行",
		Long: `命令和脚本执行，支持以下操作：

- run: 执行 Shell 命令
- script: 执行脚本文件
- playbook: 执行 Ansible 剧本`,
	}

	execCmd.AddCommand(NewRunCmd())
	execCmd.AddCommand(NewScriptCmd())
	execCmd.AddCommand(NewPlaybookCmd())

	return execCmd
}
