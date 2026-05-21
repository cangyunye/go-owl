package ai

import (
	"fmt"
	"os"
	"path/filepath"

	aiPrompts "github.com/cangyunye/go-owl/internal/ai/prompts"
	"gopkg.in/yaml.v3"
)

type Config struct {
	AI      AIConfig      `yaml:"ai"`
	Prompts PromptsConfig `yaml:"prompts"`
	Safety  SafetyConfig  `yaml:"safety"`
}

type AIConfig struct {
	Provider string `yaml:"provider"` // openai, anthropic, dashscope
	Model    string `yaml:"model"`    // gpt-4o, claude-3, qwen-turbo
	APIKey   string `yaml:"api_key"`
	BaseURL  string `yaml:"base_url"`
	Timeout  int    `yaml:"timeout"` // seconds
}

type PromptsConfig struct {
	System   string `yaml:"system"`
	Playbook string `yaml:"playbook"`
	Command  string `yaml:"command"`
	Transfer string `yaml:"transfer"`
}

type SafetyConfig struct {
	ConfirmDangerous bool     `yaml:"confirm_dangerous"`
	AllowedCommands  []string `yaml:"allowed_commands"`
	BlockedCommands  []string `yaml:"blocked_commands"`
}

func DefaultConfig() *Config {
	apiKey := os.Getenv("OWL_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	baseURL := os.Getenv("OWL_BASE_URL")

	return &Config{
		AI: AIConfig{
			Provider: "openai",
			Model:    "gpt-4o",
			APIKey:   apiKey,
			BaseURL:  baseURL,
			Timeout:  120,
		},
		Prompts: PromptsConfig{
			System:   "system.md",
			Playbook: "playbook.md",
			Command:  "command.md",
			Transfer: "transfer.md",
		},
		Safety: SafetyConfig{
			ConfirmDangerous: true,
			AllowedCommands:  []string{},
			BlockedCommands: []string{
				"rm -rf /",
				"rm -rf /*",
				":(){:|:&};:",
				">/dev/sda",
				"dd if=/dev/zero of=/dev/sda",
			},
		},
	}
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultConfig(), nil
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if cfg.AI.APIKey == "" {
		cfg.AI.APIKey = os.Getenv("OWL_API_KEY")
	}
	if cfg.AI.APIKey == "" {
		cfg.AI.APIKey = os.Getenv("OPENAI_API_KEY")
	}
	if cfg.AI.APIKey == "" {
		cfg.AI.APIKey = os.Getenv("DASHSCOPE_API_KEY")
	}
	if cfg.AI.BaseURL == "" {
		cfg.AI.BaseURL = os.Getenv("OWL_BASE_URL")
	}

	return &cfg, nil
}

func SaveConfig(path string, cfg *Config) error {
	if err := createConfigDir(path); err != nil {
		return err
	}
	
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	
	return os.WriteFile(path, data, 0600)
}

func getConfigPath() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "/tmp"
	}
	return filepath.Join(home, ".owl", "config.yaml")
}

func createConfigDir(path string) error {
	dir := filepath.Dir(path)
	return os.MkdirAll(dir, 0755)
}

func GetPromptPath(name string) string {
	home, _ := os.UserHomeDir()
	owlDir := filepath.Join(home, ".owl", "prompts")
	os.MkdirAll(owlDir, 0755)
	return filepath.Join(owlDir, name)
}

func SaveDefaultPrompts() error {
	promptsDir := filepath.Join(getHomeDir(), ".owl", "prompts")
	os.MkdirAll(promptsDir, 0755)

	promptsMap := map[string]string{
		"system.md":   aiPrompts.SystemPrompt,
		"playbook.md": aiPrompts.PlaybookPrompt,
		"command.md":  aiPrompts.CommandPrompt,
		"transfer.md": aiPrompts.TransferPrompt,
	}

	for name, content := range promptsMap {
		path := filepath.Join(promptsDir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				return err
			}
		}
	}

	return nil
}

func getHomeDir() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return "/tmp"
	}
	return home
}
