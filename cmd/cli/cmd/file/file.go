package file

import (
	"github.com/spf13/cobra"
)

// NewFileCmd 创建文件传输命令
func NewFileCmd() *cobra.Command {
	fileCmd := &cobra.Command{
		Use:   "file",
		Short: "文件传输",
		Long: `文件传输命令，支持以下操作：

- upload: 上传文件到节点
- download: 从节点下载文件
- transfer: 节点间扩散传输 (P2P 模式)`,
	}

	fileCmd.AddCommand(NewUploadCmd())
	fileCmd.AddCommand(NewDownloadCmd())
	fileCmd.AddCommand(NewTransferCmd())

	return fileCmd
}
