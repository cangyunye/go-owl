package ai

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/internal/ai"
	"github.com/cangyunye/go-owl/internal/i18n"
	"gopkg.in/yaml.v3"
)

// providerList 是 setup 向导支持的供应商列表。
var providerList = []string{"openai", "anthropic", "qwen", "dashscope", "deepseek"}

// providerDefaults 返回供应商对应的默认 model 与 base URL，
// 对齐前端 settings.js（owl-serve）的正式默认模型。
func providerDefaults(provider string) (model, baseURL string) {
	switch provider {
	case "anthropic":
		return "claude-sonnet-4-20250514", "https://api.anthropic.com/v1"
	case "qwen", "dashscope":
		return "qwen-max", "https://dashscope.aliyuncs.com/compatible-mode/v1"
	case "deepseek":
		return "deepseek-v4-flash", "https://api.deepseek.com"
	default:
		return "gpt-4o", "https://api.openai.com/v1"
	}
}

func NewConfigCmd() *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: i18n.T("ai.config.short"),
		Long:  i18n.T("ai.config.long"),
	}

	configCmd.AddCommand(NewConfigInitCmd())
	configCmd.AddCommand(NewConfigShowCmd())
	configCmd.AddCommand(NewConfigSetupCmd())

	return configCmd
}

func NewConfigInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: i18n.T("ai.config.init.short"),
		Long:  i18n.T("ai.config.init.long"),
		Run:   runConfigInit,
	}
}

func NewConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: i18n.T("ai.config.show.short"),
		Long:  i18n.T("ai.config.show.long"),
		Run:   runConfigShow,
	}
}

func NewConfigSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: i18n.T("ai.config.setup.short"),
		Long:  i18n.T("ai.config.setup.long"),
		Run:   runConfigSetup,
	}
}

func runConfigInit(cmd *cobra.Command, args []string) {
	configPath := getConfigPath()

	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		fmt.Printf("%s", i18n.T("ai.config.exists", configPath))
		fmt.Println(i18n.T("ai.config.exists_hint"))
		return
	}

	if err := createConfigDir(); err != nil {
		fmt.Printf("%s", i18n.T("ai.config.err_mkdir", err))
		os.Exit(1)
	}

	config := ai.DefaultConfig()

	data, err := yaml.Marshal(config)
	if err != nil {
		fmt.Printf("%s", i18n.T("ai.config.err_marshal", err))
		os.Exit(1)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		fmt.Printf("%s", i18n.T("ai.config.err_write", err))
		os.Exit(1)
	}

	fmt.Printf("%s", i18n.T("ai.config.created", configPath))
	fmt.Println()
	fmt.Println(i18n.T("ai.config.next_steps"))
	fmt.Println(i18n.T("ai.config.next_step1"))
	fmt.Println(i18n.T("ai.config.next_step2"))
}

func runConfigShow(cmd *cobra.Command, args []string) {
	configPath := getConfigPath()
	cfg, err := ai.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("%s", i18n.T("ai.config.err_load", err))
		os.Exit(1)
	}

	fmt.Print(renderConfigShow(cfg, configPath))
}

// renderConfigShow 渲染 show 输出（含配置路径），便于测试。
func renderConfigShow(cfg *ai.Config, configPath string) string {
	var sb strings.Builder
	sb.WriteString(i18n.T("ai.config.show.path", configPath))
	sb.WriteString("\n\n")
	sb.WriteString(i18n.T("ai.config.current"))
	sb.WriteString("\n")
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  Provider:    %s\n", cfg.AI.Provider))
	sb.WriteString(fmt.Sprintf("  Model:       %s\n", cfg.AI.Model))
	sb.WriteString(fmt.Sprintf("  API Key:     %s\n", maskAPIKey(cfg.AI.APIKey)))
	sb.WriteString(fmt.Sprintf("  Base URL:    %s\n", cfg.AI.BaseURL))
	sb.WriteString(fmt.Sprintf("  Timeout:     %ds\n", cfg.AI.Timeout))
	return sb.String()
}

func runConfigSetup(cmd *cobra.Command, args []string) {
	configPath := getConfigPath()
	reader := bufio.NewReader(os.Stdin)

	cfg, err := runConfigSetupInteractive(reader, configPath)
	if err != nil {
		fmt.Printf("%s", i18n.T("ai.config.setup.err_interactive", err))
		os.Exit(1)
	}

	if err := ai.SaveConfig(configPath, cfg); err != nil {
		fmt.Printf("%s", i18n.T("ai.config.err_write", err))
		os.Exit(1)
	}

	fmt.Println()
	fmt.Printf("%s", i18n.T("ai.config.setup.saved", configPath))
	fmt.Println()
	fmt.Println(i18n.T("ai.config.next_step2"))
}

// runConfigSetupInteractive 执行交互式供应商设置向导，返回合并后的配置。
// 现有配置（如果存在）会被加载，仅更新 AI 字段，prompts/safety 等保留。
func runConfigSetupInteractive(reader *bufio.Reader, configPath string) (*ai.Config, error) {
	cfg, err := ai.LoadConfig(configPath)
	if err != nil {
		return nil, err
	}

	fmt.Println(i18n.T("ai.config.setup.wizard_title"))
	fmt.Println(i18n.T("ai.config.setup.wizard_sep"))
	fmt.Println()

	provider, err := promptProvider(reader, cfg.AI.Provider)
	if err != nil {
		return nil, err
	}
	switched := provider != cfg.AI.Provider
	cfg.AI.Provider = provider

	defaultModel, defaultBaseURL := providerDefaults(provider)

	model, err := promptModel(reader, cfg.AI.Model, defaultModel, switched)
	if err != nil {
		return nil, err
	}
	cfg.AI.Model = model

	apiKey, err := promptAPIKey(reader, cfg.AI.APIKey)
	if err != nil {
		return nil, err
	}
	cfg.AI.APIKey = apiKey

	baseURL, err := promptBaseURL(reader, cfg.AI.BaseURL, defaultBaseURL, switched)
	if err != nil {
		return nil, err
	}
	cfg.AI.BaseURL = baseURL

	timeout, err := promptTimeout(reader, cfg.AI.Timeout)
	if err != nil {
		return nil, err
	}
	cfg.AI.Timeout = timeout

	return cfg, nil
}

func promptProvider(reader *bufio.Reader, current string) (string, error) {
	for {
		fmt.Println(i18n.T("ai.config.setup.choose_provider"))
		for i, p := range providerList {
			marker := "  "
			if p == current {
				marker = "> "
			}
			fmt.Printf("  %s%d. %s\n", marker, i+1, p)
		}
		fmt.Printf("%s", i18n.T("ai.config.setup.provider_prompt", current))
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		choice := strings.ToLower(strings.TrimSpace(input))
		if choice == "" {
			if current != "" {
				return current, nil
			}
			continue
		}
		if idx, err := strconv.Atoi(choice); err == nil && idx >= 1 && idx <= len(providerList) {
			return providerList[idx-1], nil
		}
		for _, p := range providerList {
			if choice == p {
				return p, nil
			}
		}
		fmt.Println(i18n.T("ai.config.setup.err_invalid_provider"))
	}
}

// effectiveModel 决定提示中展示的默认 model：
// 切换供应商时用新供应商默认值，否则保留现有配置值。
func effectiveModel(current, defaultModel string, switched bool) string {
	if switched {
		return defaultModel
	}
	if current != "" {
		return current
	}
	return defaultModel
}

func promptModel(reader *bufio.Reader, current, defaultModel string, switched bool) (string, error) {
	display := effectiveModel(current, defaultModel, switched)
	fmt.Printf("%s", i18n.T("ai.config.setup.model_prompt", display))
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	model := strings.TrimSpace(input)
	if model == "" {
		return display, nil
	}
	return model, nil
}

func promptAPIKey(reader *bufio.Reader, current string) (string, error) {
	fmt.Printf("%s", i18n.T("ai.config.setup.api_key_prompt"))
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	key := strings.TrimSpace(input)
	if key == "" {
		return current, nil
	}
	return key, nil
}

// effectiveBaseURL 决定提示中展示的默认 Base URL：
// 切换供应商时用新供应商默认值，否则保留现有配置值。
func effectiveBaseURL(current, defaultBaseURL string, switched bool) string {
	if switched {
		return defaultBaseURL
	}
	if current != "" {
		return current
	}
	return defaultBaseURL
}

func promptBaseURL(reader *bufio.Reader, current, defaultBaseURL string, switched bool) (string, error) {
	display := effectiveBaseURL(current, defaultBaseURL, switched)
	fmt.Printf("%s", i18n.T("ai.config.setup.base_url_prompt", display))
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	baseURL := strings.TrimSpace(input)
	if baseURL == "" {
		return display, nil
	}
	return baseURL, nil
}

func promptTimeout(reader *bufio.Reader, current int) (int, error) {
	display := current
	if display <= 0 {
		display = 120
	}
	fmt.Printf("%s", i18n.T("ai.config.setup.timeout_prompt", i18n.F(display)))
	input, err := reader.ReadString('\n')
	if err != nil {
		return 0, err
	}
	choice := strings.TrimSpace(input)
	if choice == "" {
		return display, nil
	}
	timeout, err := strconv.Atoi(choice)
	if err != nil || timeout <= 0 {
		return display, nil
	}
	return timeout, nil
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
		return i18n.T("ai.config.not_set")
	}
	if len(key) <= 8 {
		return strings.Repeat("*", len(key))
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}
