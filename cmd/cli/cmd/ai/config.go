package ai

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/internal/ai"
	"github.com/cangyunye/go-owl/internal/i18n"
	"gopkg.in/yaml.v3"
)

func NewConfigCmd() *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: i18n.T("ai.config.short"),
		Long:  i18n.T("ai.config.long"),
	}

	configCmd.AddCommand(NewConfigInitCmd())
	configCmd.AddCommand(NewConfigShowCmd())

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

	fmt.Println(i18n.T("ai.config.current"))
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
		return i18n.T("ai.config.not_set")
	}
	if len(key) <= 8 {
		return strings.Repeat("*", len(key))
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}
