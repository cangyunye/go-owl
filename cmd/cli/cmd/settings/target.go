package settings

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/internal/i18n"
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
		Short: i18n.T("settings.target.short"),
		Long:  i18n.T("settings.target.long"),
		Run:   runSettingsTarget,
	}

	targetCmd.Flags().StringSliceVarP(&targetGroup, "groups", "g", nil, i18n.T("settings.target.flag_groups"))
	targetCmd.Flags().StringSliceVar(&targetGroup, "group", nil, i18n.T("settings.target.flag_group_deprecated"))
	targetCmd.Flags().MarkHidden("group")
	targetCmd.Flags().StringSliceVarP(&targetLabel, "label", "l", nil,
		i18n.T("settings.target.flag_label"))
	targetCmd.Flags().StringVarP(&targetNodes, "nodes", "N", "",
		i18n.T("settings.target.flag_nodes"))

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
