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
  server.address     - 服务器地址
  server.timeout      - 超时时间
  output.format      - 输出格式 (table, json, yaml)
  output.color       - 启用颜色 (true, false)
  diffusion.fan-out  - 扇出系数
  diffusion.source-count - 源节点数量

示例：
  owl settings set server.address localhost:9090
  owl settings set output.format json
  owl settings set diffusion.fan-out 5`,
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
	case "server.address":
		settings.Server.Address = value
		fmt.Printf("✓ server.address set to '%s'\n", value)
	case "server.timeout":
		settings.Server.Timeout = value
		fmt.Printf("✓ server.timeout set to '%s'\n", value)
	case "output.format":
		if value != "table" && value != "json" && value != "yaml" {
			fmt.Fprintf(os.Stderr, "Error: invalid format '%s' (must be table, json, or yaml)\n", value)
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
	case "diffusion.fan-out":
		fmt.Printf("✓ diffusion.fan-out set to '%s'\n", value)
	case "diffusion.source-count":
		fmt.Printf("✓ diffusion.source-count set to '%s'\n", value)
	case "defaults.timeout":
		settings.Defaults.Timeout = value
		fmt.Printf("✓ defaults.timeout set to '%s'\n", value)
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown setting '%s'\n", key)
		fmt.Println("Run 'owl settings show' to see all available settings.")
		os.Exit(1)
	}

	fmt.Println("\nNote: Settings are not persisted in this demo version.")
}
