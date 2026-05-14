package node

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
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
		Short: "查看节点状态",
		Long: `查看一个或所有节点的状态信息。

示例：
  owl node status node1
  owl node status --all
  owl node status --all -o json`,
		Args: cobra.RangeArgs(0, 1),
		Run:  runStatus,
	}

	statusCmd.Flags().BoolVar(&statusAll, "all", false,
		"显示所有节点状态")
	statusCmd.Flags().StringVarP(&statusFormat, "output", "o", "detail",
		"输出格式: detail, json, yaml")
	statusCmd.Flags().BoolVar(&statusNoColor, "no-color", false,
		"禁用颜色输出")

	return statusCmd
}

func runStatus(cmd *cobra.Command, args []string) {
	store := common.GetNodeStore()
	formatter := common.NewOutputFormatter(statusFormat, !statusNoColor)

	if statusAll || len(args) == 0 {
		// 显示所有节点状态
		nodes, err := store.List()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing nodes: %v\n", err)
			os.Exit(1)
		}

		modelNodes := toModelNodes(nodes)
		formatter.FormatNodes(modelNodes)
	} else {
		// 显示单个节点状态
		nodeID := args[0]
		node, err := store.Get(nodeID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		modelNode := toModelNode(node)
		formatter.FormatNode(modelNode)
	}
}
