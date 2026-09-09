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
	out := cmd.OutOrStdout()
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
		fmt.Fprintln(out, i18n.T("settings.target.show_header"))
		fmt.Fprintln(out, "=========================")
		if settings.Target.Groups != "" {
			fmt.Fprintln(out, i18n.T("settings.target.field_groups", settings.Target.Groups))
		}
		if settings.Target.Label != "" {
			fmt.Fprintln(out, i18n.T("settings.target.field_label", settings.Target.Label))
		}
		if settings.Target.Nodes != "" {
			fmt.Fprintln(out, i18n.T("settings.target.field_nodes", settings.Target.Nodes))
		}
		if settings.Target.Groups == "" && settings.Target.Label == "" && settings.Target.Nodes == "" {
			fmt.Fprintln(out, i18n.T("settings.target.show_none"))
		}
		fmt.Fprintln(out, i18n.T("settings.target.show_tip"))
		return
	}

	if err := saveSettings(settings); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "%s\n", i18n.T("settings.target.err_save", err))
		os.Exit(1)
	}

	fmt.Fprintln(out, i18n.T("settings.target.show_saved_header"))
	fmt.Fprintln(out, "================================")
	if settings.Target.Groups != "" {
		fmt.Fprintln(out, i18n.T("settings.target.field_groups", settings.Target.Groups))
	}
	if settings.Target.Label != "" {
		fmt.Fprintln(out, i18n.T("settings.target.field_label", settings.Target.Label))
	}
	if settings.Target.Nodes != "" {
		fmt.Fprintln(out, i18n.T("settings.target.field_nodes", settings.Target.Nodes))
	}
}
