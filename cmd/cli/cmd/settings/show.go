package settings

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/internal/i18n"
)

// NewSettingsShowCmd 创建显示设置命令
func NewSettingsShowCmd() *cobra.Command {
	showCmd := &cobra.Command{
		Use:   "show",
		Short: i18n.T("settings.show.short"),
		Run:   runSettingsShow,
	}

	return showCmd
}

func runSettingsShow(cmd *cobra.Command, args []string) {
	out := cmd.OutOrStdout()
	settings := getCurrentSettings()

	fmt.Fprintln(out, i18n.T("settings.show.header"))
	fmt.Fprintln(out, "============================================")
	fmt.Fprintln(out)
	fmt.Fprintln(out, i18n.T("settings.show.output_label"))
	fmt.Fprintln(out, i18n.T("settings.show.field_format", settings.Output.Format))
	fmt.Fprintln(out, i18n.T("settings.show.field_color", settings.Output.Color))
	fmt.Fprintln(out)
	fmt.Fprintln(out, i18n.T("settings.show.default_label"))
	fmt.Fprintln(out, i18n.T("settings.show.field_timeout", settings.Default.Timeout))
	fmt.Fprintln(out, i18n.T("settings.show.field_parallel", settings.Default.Parallel))
	if settings.Default.Group != "" {
		fmt.Fprintln(out, i18n.T("settings.show.field_group", settings.Default.Group))
	}
	if len(settings.Default.Labels) > 0 {
		fmt.Fprintln(out, i18n.T("settings.show.field_labels", FormatLabels(settings.Default.Labels)))
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, i18n.T("settings.show.target_label"))
	if settings.Target.Groups != "" || settings.Target.Label != "" || settings.Target.Nodes != "" {
		if settings.Target.Groups != "" {
			fmt.Fprintln(out, i18n.T("settings.target.field_groups", settings.Target.Groups))
		}
		if settings.Target.Label != "" {
			fmt.Fprintln(out, i18n.T("settings.target.field_label", settings.Target.Label))
		}
		if settings.Target.Nodes != "" {
			fmt.Fprintln(out, i18n.T("settings.target.field_nodes", settings.Target.Nodes))
		}
	} else {
		fmt.Fprintln(out, i18n.T("settings.target.show_none"))
	}
}

// FormatLabels 将标签 map 渲染为可读的 key=value 列表（按键名排序，确定性输出）。
func FormatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, k+"="+labels[k])
	}
	return strings.Join(pairs, ", ")
}
