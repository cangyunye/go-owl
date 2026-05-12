package settings

import (
	"fmt"

	"github.com/spf13/cobra"
)

// targetFlags
var (
	targetGroup string
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
  owl settings target --group web
  owl settings target --label env=prod
  owl settings target --nodes node1,node2`,
		Run: runSettingsTarget,
	}

	targetCmd.Flags().StringVar(&targetGroup, "group", "",
		"默认分组")
	targetCmd.Flags().StringSliceVarP(&targetLabel, "label", "l", nil,
		"默认标签")
	targetCmd.Flags().StringVar(&targetNodes, "nodes", "",
		"默认节点")

	return targetCmd
}

func runSettingsTarget(cmd *cobra.Command, args []string) {
	hasTarget := false

	fmt.Println("Default Target Settings:")
	fmt.Println("=========================")

	if targetGroup != "" {
		fmt.Printf("  Group: %s\n", targetGroup)
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
