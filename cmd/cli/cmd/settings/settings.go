package settings

import (
	"github.com/spf13/cobra"
)

// NewSettingsCmd 创建设置命令
func NewSettingsCmd() *cobra.Command {
	settingsCmd := &cobra.Command{
		Use:   "settings",
		Short: "设置管理",
		Long: `管理 owl 的配置设置。

示例：
  owl settings show
  owl settings set server.address localhost:8080
  owl settings set output.format json`,
	}

	settingsCmd.AddCommand(NewSettingsShowCmd())
	settingsCmd.AddCommand(NewSettingsSetCmd())
	settingsCmd.AddCommand(NewSettingsTargetCmd())

	return settingsCmd
}

// Settings 配置结构
type Settings struct {
	Output  OutputSettings  `yaml:"output"`
	Default DefaultSettings `yaml:"default"`
}

// OutputSettings 输出设置
type OutputSettings struct {
	Format string `yaml:"format"` // "table" | "json" | "simple"
	Color  bool   `yaml:"color"`  // 是否启用颜色
}

// DefaultSettings 默认设置
type DefaultSettings struct {
	Timeout  string            `yaml:"timeout"`  // 默认超时时间
	Group    string            `yaml:"group"`    // 默认分组
	Parallel bool              `yaml:"parallel"` // 默认并行执行
	Labels   map[string]string `yaml:"labels"`   // 默认标签
}

// getCurrentSettings 获取当前设置
func getCurrentSettings() *Settings {
	return &Settings{
		Output: OutputSettings{
			Format: "table",
			Color:  true,
		},
		Default: DefaultSettings{
			Timeout:  "60s",
			Group:    "",
			Parallel: true,
			Labels:   map[string]string{},
		},
	}
}
