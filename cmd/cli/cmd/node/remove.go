package node

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

// NewRemoveCmd 创建删除节点命令
func NewRemoveCmd() *cobra.Command {
	removeCmd := &cobra.Command{
		Use:   "remove <node-id> [node-id...]",
		Short: "删除节点",
		Long: `从管理列表中删除一个或多个节点。

示例：
  owl node remove node1
  owl node remove node1 node2 node3  # 批量删除`,
		Args: cobra.MinimumNArgs(1),
		Run:  runRemove,
	}

	return removeCmd
}

func runRemove(cmd *cobra.Command, args []string) {
	store := common.GetNodeStore()
	success := 0
	failed := 0

	for _, nodeID := range args {
		if err := store.Remove(nodeID); err != nil {
			fmt.Printf("Failed to remove node '%s': %v\n", nodeID, err)
			failed++
		} else {
			fmt.Printf("Node '%s' removed successfully\n", nodeID)
			success++
		}
	}

	// 持久化到文件
	if success > 0 {
		store.Save()
	}

	fmt.Printf("\nRemoved: %d nodes, Failed: %d\n", success, failed)
	if failed > 0 {
		os.Exit(1)
	}
}
