package playbook

import (
	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/internal/i18n"
)

// NewPlaybookCmd 创建剧本管理命令
func NewPlaybookCmd() *cobra.Command {
	pbCmd := &cobra.Command{
		Use:   "playbook",
		Short: i18n.T("playbook.cmd.short"),
		Long:  i18n.T("playbook.cmd.long"),
	}

	pbCmd.AddCommand(NewPlaybookListCmd())
	pbCmd.AddCommand(NewPlaybookValidateCmd())
	pbCmd.AddCommand(NewPlaybookRunCmd())
	pbCmd.AddCommand(NewPlaybookTemplateCmd())
	pbCmd.AddCommand(NewPlaybookStateCmd())
	pbCmd.AddCommand(NewPlaybookNewCmd())
	pbCmd.AddCommand(NewPlaybookScaffoldCmd())

	return pbCmd
}
