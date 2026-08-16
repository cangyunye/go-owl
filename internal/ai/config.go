package ai

import (
	"fmt"
	"os"
	"path/filepath"

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

func createConfigDir(path string) error {
	dir := filepath.Dir(path)
	return os.MkdirAll(dir, 0755)
}
