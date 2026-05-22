package settings

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// NewSettingsSetCmd 创建设置值命令
func NewSettingsSetCmd() *cobra.Command {
	setCmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "设置配置值",
		Long: `设置配置项的值。

支持的配置项：
  output.format      - 输出格式 (table, json, simple)
  output.color       - 启用颜色 (true, false)
  default.timeout    - 默认超时时间 (例如 30s, 1m)
  default.group      - 默认分组
  default.parallel   - 默认并行执行 (true, false)

示例：
  owl settings set output.format json
  owl settings set default.timeout 60s
  owl settings set default.group web`,
		Args: cobra.ExactArgs(2),
		Run:  runSettingsSet,
	}

	return setCmd
}

func runSettingsSet(cmd *cobra.Command, args []string) {
	key := args[0]
	value := args[1]

	settings := getCurrentSettings()

	switch key {
	case "output.format":
		if value != "table" && value != "json" && value != "simple" {
			fmt.Fprintf(os.Stderr, "Error: invalid format '%s' (must be table, json, or simple)\n", value)
			os.Exit(1)
		}
		settings.Output.Format = value
		fmt.Printf("✓ output.format set to '%s'\n", value)
	case "output.color":
		if value != "true" && value != "false" {
			fmt.Fprintf(os.Stderr, "Error: invalid value '%s' (must be true or false)\n", value)
			os.Exit(1)
		}
		settings.Output.Color = value == "true"
		fmt.Printf("✓ output.color set to '%s'\n", value)
	case "default.timeout":
		settings.Default.Timeout = value
		fmt.Printf("✓ default.timeout set to '%s'\n", value)
	case "default.group":
		settings.Default.Group = value
		fmt.Printf("✓ default.group set to '%s'\n", value)
	case "default.parallel":
		if value != "true" && value != "false" {
			fmt.Fprintf(os.Stderr, "Error: invalid value '%s' (must be true or false)\n", value)
			os.Exit(1)
		}
		settings.Default.Parallel = value == "true"
		fmt.Printf("✓ default.parallel set to '%s'\n", value)
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown setting '%s'\n", key)
		fmt.Println("Run 'owl settings show' to see all available settings.")
		os.Exit(1)
	}

	fmt.Println("\nNote: Settings are not persisted in this demo version.")
}
