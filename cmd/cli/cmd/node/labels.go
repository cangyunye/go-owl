package node

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

// NewLabelsCmd 创建标签管理命令
func NewLabelsCmd() *cobra.Command {
	labelsCmd := &cobra.Command{
		Use:   "labels",
		Short: "管理节点标签",
		Long: `管理节点的标签，支持设置、删除、查看标签。

示例：
  owl node labels set node1 env=prod
  owl node labels set node1 env=prod region=us-east  # 多个标签
  owl node labels remove node1 env
  owl node labels show node1`,
	}

	labelsCmd.AddCommand(NewLabelsSetCmd())
	labelsCmd.AddCommand(NewLabelsRemoveCmd())
	labelsCmd.AddCommand(NewLabelsShowCmd())

	return labelsCmd
}

// NewLabelsSetCmd 设置标签
func NewLabelsSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <node-id> <key=value> [key=value...]",
		Short: "设置节点标签",
		Args:  cobra.MinimumNArgs(2),
		Run:   runLabelsSet,
	}
}

func runLabelsSet(cmd *cobra.Command, args []string) {
	nodeID := args[0]
	store := common.GetNodeStore()

	node, err := store.Get(nodeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// 解析标签
	for _, label := range args[1:] {
		parts := splitLabel(label)
		if len(parts) != 2 {
			fmt.Fprintf(os.Stderr, "Invalid label format: %s (expected key=value)\n", label)
			os.Exit(1)
		}
		if node.Labels == nil {
			node.Labels = make(map[string]string)
		}
		node.Labels[parts[0]] = parts[1]
	}

	if err := store.Update(node); err != nil {
		fmt.Fprintf(os.Stderr, "Error updating node: %v\n", err)
		os.Exit(1)
	}

	store.Save()
	fmt.Printf("Labels updated for node '%s'\n", nodeID)
	common.PrintLabels(node.Labels)
}

// NewLabelsRemoveCmd 移除标签
func NewLabelsRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <node-id> <key>",
		Short: "移除节点标签",
		Args:  cobra.ExactArgs(2),
		Run:   runLabelsRemove,
	}
}

func runLabelsRemove(cmd *cobra.Command, args []string) {
	nodeID, key := args[0], args[1]
	store := common.GetNodeStore()

	node, err := store.Get(nodeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if _, ok := node.Labels[key]; !ok {
		fmt.Printf("Label '%s' not found on node '%s'\n", key, nodeID)
		return
	}

	delete(node.Labels, key)
	if err := store.Update(node); err != nil {
		fmt.Fprintf(os.Stderr, "Error updating node: %v\n", err)
		os.Exit(1)
	}

	store.Save()
	fmt.Printf("Label '%s' removed from node '%s'\n", key, nodeID)
}

// NewLabelsShowCmd 显示标签
func NewLabelsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <node-id> [key]",
		Short: "显示节点的所有标签，或指定标签",
		Args:  cobra.RangeArgs(1, 2),
		Run:   runLabelsShow,
	}
}

func runLabelsShow(cmd *cobra.Command, args []string) {
	nodeID := args[0]
	store := common.GetNodeStore()

	node, err := store.Get(nodeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(args) == 2 {
		key := args[1]
		if value, ok := node.Labels[key]; ok {
			fmt.Printf("%s=%s\n", key, value)
		} else {
			fmt.Printf("Label '%s' not found on node '%s'\n", key, nodeID)
		}
		return
	}

	fmt.Printf("Labels for node '%s':\n", nodeID)
	common.PrintLabels(node.Labels)
}

func splitLabel(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}
