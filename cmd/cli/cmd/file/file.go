package file

import (
	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/internal/i18n"
)

// NewFileCmd 创建文件传输命令
func NewFileCmd() *cobra.Command {
	fileCmd := &cobra.Command{
		Use:   "file",
		Short: i18n.T("file.cmd.short"),
		Long:  i18n.T("file.cmd.long"),
	}

	fileCmd.AddCommand(NewUploadCmd())
	fileCmd.AddCommand(NewDownloadCmd())
	fileCmd.AddCommand(NewTransferCmd())

	return fileCmd
}
