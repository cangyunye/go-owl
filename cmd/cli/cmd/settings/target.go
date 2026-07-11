package settings

import (
	"fmt"
	"os"
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
	settings := loadSettings()

	hasChange := false

	if cmd.Flags().Changed("groups") {
		settings.Target.Groups = strings.Join(targetGroup, ",")
		hasChange = true
	}
	if cmd.Flags().Changed("label") {
		settings.Target.Label = strings.Join(targetLabel, ",")
		hasChange = true
	}
	if cmd.Flags().Changed("nodes") {
		settings.Target.Nodes = targetNodes
		hasChange = true
	}

	if !hasChange {
		fmt.Println("Default Target Settings:")
		fmt.Println("=========================")
		if settings.Target.Groups != "" {
			fmt.Printf("  Groups: %s\n", settings.Target.Groups)
		}
		if settings.Target.Label != "" {
			fmt.Printf("  Label:  %s\n", settings.Target.Label)
		}
		if settings.Target.Nodes != "" {
			fmt.Printf("  Nodes:  %s\n", settings.Target.Nodes)
		}
		if settings.Target.Groups == "" && settings.Target.Label == "" && settings.Target.Nodes == "" {
			fmt.Println("  (no default target set)")
		}
		fmt.Println("\nTip: use --groups, --label, or --nodes to set default targets.")
		return
	}

	if err := saveSettings(settings); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to save settings: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Default Target Settings (saved):")
	fmt.Println("================================")
	if settings.Target.Groups != "" {
		fmt.Printf("  Groups: %s\n", settings.Target.Groups)
	}
	if settings.Target.Label != "" {
		fmt.Printf("  Label:  %s\n", settings.Target.Label)
	}
	if settings.Target.Nodes != "" {
		fmt.Printf("  Nodes:  %s\n", settings.Target.Nodes)
	}
}
