package session

import (
	"github.com/spf13/cobra"
)

var sessionTimeout string

// NewCmd 创建 session 命令
func NewCmd() *cobra.Command {
	sessionCmd := &cobra.Command{
		Use:   "session",
		Short: "交互式会话管理",
		Long:  `管理持久 SSH 会话，支持单节点实时交互和多节点批量管理`,
	}

	sessionCmd.AddCommand(NewAttachCmd())
	sessionCmd.AddCommand(NewListCmd())
	sessionCmd.AddCommand(NewHistoryCmd())

	return sessionCmd
}
