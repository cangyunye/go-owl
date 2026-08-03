package settings

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/internal/i18n"
)

// NewSettingsTemplateCmd 创建显示可配置项模板的命令
func NewSettingsTemplateCmd() *cobra.Command {
	templateCmd := &cobra.Command{
		Use:   "template",
		Short: i18n.T("settings.template.short"),
		Long:  i18n.T("settings.template.long"),
		Run:   runSettingsTemplate,
	}

	return templateCmd
}

func runSettingsTemplate(cmd *cobra.Command, args []string) {
	fmt.Println("Settings Configuration Template")
	fmt.Println("==============================")
	fmt.Println()
	fmt.Println("All configurable settings keys:")
	fmt.Println()

	// 输出设置
	fmt.Println("--- Output ---")
	fmt.Println()
	printRow("output.format", "string", `"table"`, i18n.T("settings.template.desc_output_format"))
	printRow("output.color", "bool", "true", i18n.T("settings.template.desc_output_color"))
	fmt.Println()

	// 默认设置
	fmt.Println("--- Default ---")
	fmt.Println()
	printRow("default.timeout", "duration", `"60s"`, i18n.T("settings.template.desc_default_timeout"))
	printRow("default.group", "string", `""`, i18n.T("settings.template.desc_default_group"))
	printRow("default.parallel", "bool", "true", i18n.T("settings.template.desc_default_parallel"))
	printRow("default.labels", "map", "{}", i18n.T("settings.template.desc_default_labels"))
	fmt.Println()

	// 目标设置
	fmt.Println("--- Target ---")
	fmt.Println()
	printRow("target.groups", "string", `""`, i18n.T("settings.template.desc_target_groups"))
	printRow("target.label", "string", `""`, i18n.T("settings.template.desc_target_label"))
	printRow("target.nodes", "string", `""`, i18n.T("settings.template.desc_target_nodes"))
	fmt.Println()

	fmt.Println("Usage:")
	fmt.Println("  owl settings set <key> <value>")
	fmt.Println("  owl settings target --groups web,db --label env=prod")
	fmt.Println()
	fmt.Println("Priority: CLI flag > command default > settings config")
}

func printRow(key, typ, def, desc string) {
	fmt.Printf("  %-22s  %-10s  %-12s  %s\n", key, typ, def, desc)
}
