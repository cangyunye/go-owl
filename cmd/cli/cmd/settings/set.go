package settings

import (
	"fmt"
	"os"
	"strings"

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
  default.labels     - 默认标签 (格式: key1=val1,key2=val2)
  target.groups      - 默认目标分组 (逗号分隔)
  target.label       - 默认目标标签
  target.nodes       - 默认目标节点 (逗号分隔)

示例：
  owl settings set output.format json
  owl settings set default.timeout 60s
  owl settings set default.group web
  owl settings set default.labels env=prod,region=us
  owl settings set target.groups web,db`,
		Args: cobra.ExactArgs(2),
		Run:  runSettingsSet,
	}

	return setCmd
}

func runSettingsSet(cmd *cobra.Command, args []string) {
	key := args[0]
	value := args[1]

	settings := loadSettings()

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
	case "default.labels":
		// 格式: key1=val1,key2=val2
		labels := make(map[string]string)
		if value != "" {
			pairs := strings.Split(value, ",")
			for _, pair := range pairs {
				kv := strings.SplitN(pair, "=", 2)
				if len(kv) == 2 {
					labels[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
				} else {
					fmt.Fprintf(os.Stderr, "Error: invalid label format '%s' (expected key=value)\n", pair)
					os.Exit(1)
				}
			}
		}
		settings.Default.Labels = labels
		fmt.Printf("✓ default.labels set to '%v'\n", labels)
	case "target.groups":
		settings.Target.Groups = value
		fmt.Printf("✓ target.groups set to '%s'\n", value)
	case "target.label":
		settings.Target.Label = value
		fmt.Printf("✓ target.label set to '%s'\n", value)
	case "target.nodes":
		settings.Target.Nodes = value
		fmt.Printf("✓ target.nodes set to '%s'\n", value)
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown setting '%s'\n", key)
		fmt.Println("Run 'owl settings template' to see all available settings.")
		os.Exit(1)
	}

	if err := saveSettings(settings); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to save settings: %v\n", err)
		os.Exit(1)
	}
	fmt.Println()
	fmt.Println("Settings saved to ~/.owl/config.yaml")
}
