package settings

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewSettingsTemplateCmd 创建显示可配置项模板的命令
func NewSettingsTemplateCmd() *cobra.Command {
	templateCmd := &cobra.Command{
		Use:   "template",
		Short: "显示所有可配置项",
		Long: `显示所有可配置的设置项及其类型、默认值和说明。

示例：
  owl settings template`,
		Run: runSettingsTemplate,
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
	printRow("output.format", "string", `"table"`, "输出格式 (table, json, simple)")
	printRow("output.color", "bool", "true", "是否启用颜色输出")
	fmt.Println()

	// 默认设置
	fmt.Println("--- Default ---")
	fmt.Println()
	printRow("default.timeout", "duration", `"60s"`, "默认超时时间 (如 30s, 1m, 5m)")
	printRow("default.group", "string", `""`, "默认节点分组")
	printRow("default.parallel", "bool", "true", "默认是否并行执行")
	printRow("default.labels", "map", "{}", "默认节点标签 (格式: key1=val1,key2=val2)")
	fmt.Println()

	// 目标设置
	fmt.Println("--- Target ---")
	fmt.Println()
	printRow("target.groups", "string", `""`, "默认目标分组 (逗号分隔)")
	printRow("target.label", "string", `""`, "默认目标标签")
	printRow("target.nodes", "string", `""`, "默认目标节点 (逗号分隔)")
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
