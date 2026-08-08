package session

import (
	"github.com/cangyunye/go-owl/internal/i18n"
	"github.com/spf13/cobra"
)

var sessionTimeout string

// NewCmd 创建 session 命令
func NewCmd() *cobra.Command {
	sessionCmd := &cobra.Command{
		Use:   "session",
		Short: i18n.T("session.cmd.short"),
		Long:  i18n.T("session.cmd.long"),
	}

	sessionCmd.AddCommand(NewAttachCmd())
	sessionCmd.AddCommand(NewListCmd())
	sessionCmd.AddCommand(NewHistoryCmd())

	return sessionCmd
}
