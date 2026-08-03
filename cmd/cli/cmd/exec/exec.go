package exec

import (
	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/internal/i18n"
)

// NewExecCmd 创建执行命令
func NewExecCmd() *cobra.Command {
	execCmd := &cobra.Command{
		Use:   "exec",
		Short: i18n.T("exec.cmd.short"),
		Long:  i18n.T("exec.cmd.long"),
	}

	execCmd.AddCommand(NewRunCmd())
	execCmd.AddCommand(NewScriptCmd())

	return execCmd
}
