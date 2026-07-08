package settings

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// targetFlags
var (
	targetGroup []string
	targetLabel []string
	targetNodes string
)

// NewSettingsTargetCmd 创建默认目标命令
func NewSettingsTargetCmd() *cobra.Command {
	targetCmd := &cobra.Command{
		Use:   "target",
		Short: "设置默认目标节点",
		Long: `设置默认的目标节点选择条件。

示例：
  owl settings target --groups web
  owl settings target --label env=prod
  owl settings target --nodes node1,node2`,
		Run: runSettingsTarget,
	}

	targetCmd.Flags().StringSliceVarP(&targetGroup, "groups", "g", nil, "默认分组 (多个分组用逗号分隔或多次使用 -g)")
	targetCmd.Flags().StringSliceVar(&targetGroup, "group", nil, "(已废弃，请使用 --groups)")
	targetCmd.Flags().MarkHidden("group")
	targetCmd.Flags().StringSliceVarP(&targetLabel, "label", "l", nil,
		"默认标签")
	targetCmd.Flags().StringVarP(&targetNodes, "nodes", "N", "",
		"默认节点")

	return targetCmd
}

func runSettingsTarget(cmd *cobra.Command, args []string) {
	hasTarget := false

	fmt.Println("Default Target Settings:")
	fmt.Println("=========================")

	if len(targetGroup) > 0 {
		fmt.Printf("  Group: %s\n", strings.Join(targetGroup, ", "))
		hasTarget = true
	}

	if len(targetLabel) > 0 {
		fmt.Printf("  Labels: %v\n", targetLabel)
		hasTarget = true
	}

	if targetNodes != "" {
		fmt.Printf("  Nodes: %s\n", targetNodes)
		hasTarget = true
	}

	if !hasTarget {
		fmt.Println("  (no default target set)")
	}

	fmt.Println("\nNote: Settings are not persisted in this demo version.")
}
