package node

import (
	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

// NewNodeCmd 创建节点管理命令
func NewNodeCmd() *cobra.Command {
	nodeCmd := &cobra.Command{
		Use:   "node",
		Short: "节点管理",
		Long: `节点管理命令，支持以下操作：

- list: 列出节点
- add: 添加节点
- remove: 删除节点
- status: 查看节点状态
- groups: 管理节点分组
- labels: 管理节点标签`,
	}

	// 添加子命令
	nodeCmd.AddCommand(NewListCmd())
	nodeCmd.AddCommand(NewAddCmd())
	nodeCmd.AddCommand(NewRemoveCmd())
	nodeCmd.AddCommand(NewStatusCmd())
	nodeCmd.AddCommand(NewGroupsCmd())
	nodeCmd.AddCommand(NewLabelsCmd())

	return nodeCmd
}

// GetNodeStore 获取节点存储
func GetNodeStore() common.NodeStore {
	return common.GetNodeStore()
}
