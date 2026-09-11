package node

import (
	"fmt"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/history"
	"github.com/cangyunye/go-owl/internal/i18n"
	"github.com/spf13/cobra"
)

// NewReencryptCmd 一次性迁移:把 nodes 表中的明文凭据就地加密。
// 需要设置 OWL_ENC_KEY;已加密与空值自动跳过,可重复执行。
func NewReencryptCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "reencrypt",
		Short:        i18n.T("node.reencrypt.short"),
		Long:         i18n.T("node.reencrypt.long"),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			gdb := history.GetGlobalDB()
			if gdb == nil {
				return fmt.Errorf("%s", i18n.T("node.reencrypt.no_db"))
			}
			n, err := common.ReencryptNodeCredentials(gdb.Connection())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", i18n.T("node.reencrypt.done", i18n.F(n)))
			return nil
		},
	}
}
