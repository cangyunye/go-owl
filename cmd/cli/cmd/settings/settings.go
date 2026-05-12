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
	Server    ServerSettings    `yaml:"server"`
	Output    OutputSettings    `yaml:"output"`
	Diffusion DiffusionSettings `yaml:"diffusion"`
	Defaults  DefaultSettings   `yaml:"defaults"`
}

// ServerSettings 服务器设置
type ServerSettings struct {
	Address string `yaml:"address"`
	Timeout string `yaml:"timeout"`
}

// OutputSettings 输出设置
type OutputSettings struct {
	Format string `yaml:"format"`
	Color  bool   `yaml:"color"`
}

// DiffusionSettings 扩散传输设置
type DiffusionSettings struct {
	FanOut      int `yaml:"fan_out"`
	MaxDepth    int `yaml:"max_depth"`
	SourceCount int `yaml:"source_count"`
}

// DefaultSettings 默认设置
type DefaultSettings struct {
	Groups  []string          `yaml:"groups"`
	Labels  map[string]string `yaml:"labels"`
	Timeout string            `yaml:"timeout"`
}

// getCurrentSettings 获取当前设置
func getCurrentSettings() *Settings {
	return &Settings{
		Server: ServerSettings{
			Address: "localhost:8080",
			Timeout: "30s",
		},
		Output: OutputSettings{
			Format: "table",
			Color:  true,
		},
		Diffusion: DiffusionSettings{
			FanOut:      3,
			MaxDepth:    10,
			SourceCount: 2,
		},
		Defaults: DefaultSettings{
			Groups:  []string{},
			Labels:  map[string]string{},
			Timeout: "60s",
		},
	}
}
