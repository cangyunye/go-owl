package node

import (
	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/internal/i18n"
)

// NewNodeCmd 创建节点管理命令
func NewNodeCmd() *cobra.Command {
	nodeCmd := &cobra.Command{
		Use:   "node",
		Short: i18n.T("node.cmd.short"),
		Long:  i18n.T("node.cmd.long"),
	}

	// 添加子命令
	nodeCmd.AddCommand(NewListCmd())
	nodeCmd.AddCommand(NewAddCmd())
	nodeCmd.AddCommand(NewUpdateCmd())
	nodeCmd.AddCommand(NewRemoveCmd())
	nodeCmd.AddCommand(NewImportCmd())
	nodeCmd.AddCommand(NewExportCmd())
	nodeCmd.AddCommand(NewStatusCmd())
	nodeCmd.AddCommand(NewGroupsCmd())
	nodeCmd.AddCommand(NewLabelsCmd())
	nodeCmd.AddCommand(NewSampleCmd())
	nodeCmd.AddCommand(NewPingCmd())
	nodeCmd.AddCommand(NewCheckCmd())

	return nodeCmd
}
