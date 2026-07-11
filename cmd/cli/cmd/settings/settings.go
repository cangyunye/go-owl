package settings

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// NewSettingsCmd 创建设置命令
func NewSettingsCmd() *cobra.Command {
	settingsCmd := &cobra.Command{
		Use:   "settings",
		Short: "设置管理",
		Long: `管理 owl 的配置设置。

示例：
  owl settings show
  owl settings set output.format json
  owl settings set default.timeout 60s
  owl settings target --groups web,db`,
	}

	settingsCmd.AddCommand(NewSettingsShowCmd())
	settingsCmd.AddCommand(NewSettingsSetCmd())
	settingsCmd.AddCommand(NewSettingsTargetCmd())
	settingsCmd.AddCommand(NewSettingsTemplateCmd())

	return settingsCmd
}

// Settings 配置结构
type Settings struct {
	Output  OutputSettings  `yaml:"output"`
	Default DefaultSettings `yaml:"default"`
	Target  TargetSettings  `yaml:"target"`
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

// TargetSettings 默认目标设置
type TargetSettings struct {
	Groups string `yaml:"groups,omitempty"` // 默认目标分组（逗号分隔）
	Label  string `yaml:"label,omitempty"`  // 默认目标标签
	Nodes  string `yaml:"nodes,omitempty"`  // 默认目标节点（逗号分隔）
}

// getConfigPath 返回配置文件路径 (~/.owl/config.yaml)
func getConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "/tmp"
	}
	return filepath.Join(home, ".owl", "config.yaml")
}

// defaultSettings 返回硬编码默认设置
func defaultSettings() *Settings {
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
		Target: TargetSettings{},
	}
}

// loadSettings 从 ~/.owl/config.yaml 的 settings 节加载配置
func loadSettings() *Settings {
	path := getConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return defaultSettings()
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return defaultSettings()
	}

	settingsRaw, ok := raw["settings"]
	if !ok {
		return defaultSettings()
	}

	settingsData, err := yaml.Marshal(settingsRaw)
	if err != nil {
		return defaultSettings()
	}

	var s Settings
	if err := yaml.Unmarshal(settingsData, &s); err != nil {
		return defaultSettings()
	}

	return &s
}

// saveSettings 将设置持久化到 ~/.owl/config.yaml 的 settings 节
func saveSettings(s *Settings) error {
	path := getConfigPath()

	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// 读取已有配置或新建
	var raw map[string]interface{}
	data, err := os.ReadFile(path)
	if err == nil {
		yaml.Unmarshal(data, &raw)
	}
	if raw == nil {
		raw = make(map[string]interface{})
	}

	raw["settings"] = s

	out, err := yaml.Marshal(raw)
	if err != nil {
		return err
	}

	return os.WriteFile(path, out, 0600)
}

// getCurrentSettings 获取当前设置（优先从配置加载，失败则回退到默认值）
func getCurrentSettings() *Settings {
	return loadSettings()
}

// GetCurrentSettings 导出给其他包使用的获取当前设置方法
func GetCurrentSettings() *Settings {
	return loadSettings()
}
