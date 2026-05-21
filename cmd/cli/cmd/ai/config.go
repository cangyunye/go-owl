package ai

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/internal/ai"
	"gopkg.in/yaml.v3"
)

func NewConfigCmd() *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "AI 配置管理",
		Long: `管理 AI 配置文件。

示例：
  owl ai config       # 交互式配置
  owl ai config init  # 快速初始化
  owl ai config show  # 显示当前配置`,
	}

	configCmd.AddCommand(NewConfigInitCmd())
	configCmd.AddCommand(NewConfigShowCmd())

	return configCmd
}

func NewConfigInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "初始化配置文件",
		Long:  `创建默认配置文件到 ~/.owl/config.yaml`,
		Run:   runConfigInit,
	}
}

func NewConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "显示当前配置",
		Long:  `显示当前的 AI 配置信息 (隐藏 API Key)`,
		Run:   runConfigShow,
	}
}

func runConfigInit(cmd *cobra.Command, args []string) {
	configPath := getConfigPath()

	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		fmt.Printf("配置文件已存在: %s\n", configPath)
		fmt.Println("如需重新生成，请先删除该文件")
		return
	}

	if err := createConfigDir(); err != nil {
		fmt.Printf("创建配置目录失败: %v\n", err)
		os.Exit(1)
	}

	config := ai.DefaultConfig()

	data, err := yaml.Marshal(config)
	if err != nil {
		fmt.Printf("序列化配置失败: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		fmt.Printf("写入配置文件失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ 配置文件已创建: %s\n", configPath)
	fmt.Println()
	fmt.Println("下一步：")
	fmt.Println("  1. 编辑配置文件设置 API Key")
	fmt.Println("  2. 或使用 'owl ai models' 检查连接")
}

func runConfigShow(cmd *cobra.Command, args []string) {
	configPath := getConfigPath()
	cfg, err := ai.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("加载配置失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("当前配置:")
	fmt.Println()
	fmt.Printf("  Provider:    %s\n", cfg.AI.Provider)
	fmt.Printf("  Model:       %s\n", cfg.AI.Model)
	fmt.Printf("  API Key:     %s\n", maskAPIKey(cfg.AI.APIKey))
	fmt.Printf("  Base URL:    %s\n", cfg.AI.BaseURL)
	fmt.Printf("  Timeout:     %ds\n", cfg.AI.Timeout)
}

func getConfigPath() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "/tmp"
	}
	return filepath.Join(home, ".owl", "config.yaml")
}

func createConfigDir() error {
	configPath := getConfigPath()
	dir := filepath.Dir(configPath)
	return os.MkdirAll(dir, 0755)
}

func maskAPIKey(key string) string {
	if key == "" {
		return "(未设置)"
	}
	if len(key) <= 8 {
		return strings.Repeat("*", len(key))
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}
