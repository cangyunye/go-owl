package node

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/i18n"
)

// statusFlags
var (
	statusAll     bool
	statusFormat  string
	statusNoColor bool
)

// NewStatusCmd 创建节点状态命令
func NewStatusCmd() *cobra.Command {
	statusCmd := &cobra.Command{
		Use:   "status [node-id]",
		Short: i18n.T("node.status.short"),
		Long:  i18n.T("node.status.long"),
		Args:  cobra.RangeArgs(0, 1),
		Run:   runStatus,
	}

	statusCmd.Flags().BoolVarP(&statusAll, "all", "a", false,
		i18n.T("node.status.flag_all"))
	statusCmd.Flags().StringVarP(&statusFormat, "output", "o", "detail",
		i18n.T("node.status.flag_format"))
	statusCmd.Flags().BoolVarP(&statusNoColor, "no-color", "C", false,
		i18n.T("node.status.flag_no_color"))

	return statusCmd
}

func runStatus(cmd *cobra.Command, args []string) {
	store := common.GetNodeStore()
	formatter := common.NewOutputFormatter(statusFormat, !statusNoColor)

	if statusAll || len(args) == 0 {
		// 显示所有节点状态
		nodes, err := store.List()
		if err != nil {
			fmt.Fprintln(os.Stderr, i18n.T("node.status.err_list", err))
			os.Exit(1)
		}

		modelNodes := toModelNodes(nodes)
		formatter.FormatNodes(modelNodes)
	} else {
		// 显示单个节点状态
		nodeID := args[0]
		node, err := store.Get(nodeID)
		if err != nil {
			fmt.Fprintln(os.Stderr, i18n.T("node.status.err_get", err))
			os.Exit(1)
		}

		modelNode := toModelNode(node)
		formatter.FormatNode(modelNode)
	}
}
