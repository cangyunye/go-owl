package node

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/i18n"
)

// NewLabelsCmd 创建标签管理命令
func NewLabelsCmd() *cobra.Command {
	labelsCmd := &cobra.Command{
		Use:   "labels",
		Short: i18n.T("node.labels.short"),
		Long:  i18n.T("node.labels.long"),
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
		Short: i18n.T("node.labels.set.short"),
		Args:  cobra.MinimumNArgs(2),
		Run:   runLabelsSet,
	}
}

func runLabelsSet(cmd *cobra.Command, args []string) {
	nodeID := args[0]
	store := common.GetNodeStore()

	node, err := store.Get(nodeID)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("node.labels.err_get", err))
		os.Exit(1)
	}

	// 解析标签
	for _, label := range args[1:] {
		parts := splitLabel(label)
		if len(parts) != 2 {
			fmt.Fprintln(os.Stderr, i18n.T("node.labels.set.err_format", label))
			os.Exit(1)
		}
		if node.Labels == nil {
			node.Labels = make(map[string]string)
		}
		node.Labels[parts[0]] = parts[1]
	}

	if err := store.Update(node); err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("node.labels.err_update", err))
		os.Exit(1)
	}

	store.Save()
	fmt.Printf("%s\n", i18n.T("node.labels.set.ok", nodeID))
	common.PrintLabels(node.Labels)
}

// NewLabelsRemoveCmd 移除标签
func NewLabelsRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <node-id> <key>",
		Short: i18n.T("node.labels.remove.short"),
		Args:  cobra.ExactArgs(2),
		Run:   runLabelsRemove,
	}
}

func runLabelsRemove(cmd *cobra.Command, args []string) {
	nodeID, key := args[0], args[1]
	store := common.GetNodeStore()

	node, err := store.Get(nodeID)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("node.labels.err_get", err))
		os.Exit(1)
	}

	if _, ok := node.Labels[key]; !ok {
		fmt.Printf("%s\n", i18n.T("node.labels.remove.not_found", key, nodeID))
		return
	}

	delete(node.Labels, key)
	if err := store.Update(node); err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("node.labels.err_update", err))
		os.Exit(1)
	}

	store.Save()
	fmt.Printf("%s\n", i18n.T("node.labels.remove.ok", key, nodeID))
}

// NewLabelsShowCmd 显示标签
func NewLabelsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <node-id> [key]",
		Short: i18n.T("node.labels.show.short"),
		Args:  cobra.RangeArgs(1, 2),
		Run:   runLabelsShow,
	}
}

func runLabelsShow(cmd *cobra.Command, args []string) {
	nodeID := args[0]
	store := common.GetNodeStore()

	node, err := store.Get(nodeID)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("node.labels.err_get", err))
		os.Exit(1)
	}

	if len(args) == 2 {
		key := args[1]
		if value, ok := node.Labels[key]; ok {
			fmt.Printf("%s=%s\n", key, value)
		} else {
			fmt.Printf("%s\n", i18n.T("node.labels.show.not_found", key, nodeID))
		}
		return
	}

	fmt.Printf("%s\n", i18n.T("node.labels.show.title", nodeID))
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
