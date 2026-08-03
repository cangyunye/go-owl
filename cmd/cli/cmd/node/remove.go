package node

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/i18n"
)

// NewRemoveCmd 创建删除节点命令
func NewRemoveCmd() *cobra.Command {
	removeCmd := &cobra.Command{
		Use:   "remove <node-id> [node-id...]",
		Short: i18n.T("node.remove.short"),
		Long:  i18n.T("node.remove.long"),
		Args:  cobra.MinimumNArgs(1),
		Run:   runRemove,
	}

	return removeCmd
}

func runRemove(cmd *cobra.Command, args []string) {
	store := common.GetNodeStore()
	success := 0
	failed := 0

	for _, nodeID := range args {
		if err := store.Remove(nodeID); err != nil {
			fmt.Printf("%s\n", i18n.T("node.remove.failed", nodeID, err))
			failed++
		} else {
			fmt.Printf("%s\n", i18n.T("node.remove.ok", nodeID))
			success++
		}
	}

	// 持久化到文件
	if success > 0 {
		store.Save()
	}

	fmt.Printf("%s\n", i18n.T("node.remove.summary", i18n.F(success), i18n.F(failed)))
	if failed > 0 {
		os.Exit(1)
	}
}
