package ai

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	internalai "github.com/cangyunye/go-owl/internal/ai"
)

// setupInput 用给定输入串驱动 runConfigSetupInteractive，返回更新后的配置。
func setupInput(t *testing.T, configPath, input string) *internalai.Config {
	t.Helper()
	cfg, err := runConfigSetupInteractive(bufio.NewReader(strings.NewReader(input)), configPath)
	if err != nil {
		t.Fatalf("runConfigSetupInteractive failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	return cfg
}

func TestConfigSetup_SelectByNumber(t *testing.T) {
	// 选择 qwen(3)，model 回车用默认，api key 输入，base url 回车用默认，timeout 回车保持
	cfg := setupInput(t, filepath.Join(t.TempDir(), "config.yaml"),
		"3\n\nsk-test-123\n\n\n")

	if cfg.AI.Provider != "qwen" {
		t.Errorf("expected provider 'qwen', got '%s'", cfg.AI.Provider)
	}
	if cfg.AI.Model != "qwen-turbo" {
		t.Errorf("expected default model 'qwen-turbo', got '%s'", cfg.AI.Model)
	}
	if cfg.AI.APIKey != "sk-test-123" {
		t.Errorf("expected api key 'sk-test-123', got '%s'", cfg.AI.APIKey)
	}
	if cfg.AI.BaseURL != "https://dashscope.aliyuncs.com/compatible-mode/v1" {
		t.Errorf("expected default dashscope base url, got '%s'", cfg.AI.BaseURL)
	}
	if cfg.AI.Timeout != 120 {
		t.Errorf("expected timeout 120, got %d", cfg.AI.Timeout)
	}
}

func TestConfigSetup_SelectByName(t *testing.T) {
	cfg := setupInput(t, filepath.Join(t.TempDir(), "config.yaml"),
		"deepseek\n\nsk-ds-9\n\n60\n")

	if cfg.AI.Provider != "deepseek" {
		t.Errorf("expected provider 'deepseek', got '%s'", cfg.AI.Provider)
	}
	if cfg.AI.Model != "deepseek-chat" {
		t.Errorf("expected default model 'deepseek-chat', got '%s'", cfg.AI.Model)
	}
	if cfg.AI.BaseURL != "https://api.deepseek.com" {
		t.Errorf("expected default deepseek base url, got '%s'", cfg.AI.BaseURL)
	}
	if cfg.AI.Timeout != 60 {
		t.Errorf("expected timeout 60, got %d", cfg.AI.Timeout)
	}
}

func TestConfigSetup_InvalidProviderRetries(t *testing.T) {
	// 先输入非法选项 99，再输入 1(openai)
	cfg := setupInput(t, filepath.Join(t.TempDir(), "config.yaml"),
		"99\n1\n\nsk-openai-1\n\n\n")

	if cfg.AI.Provider != "openai" {
		t.Errorf("expected provider 'openai', got '%s'", cfg.AI.Provider)
	}
	if cfg.AI.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("expected default openai base url, got '%s'", cfg.AI.BaseURL)
	}
}

func TestConfigSetup_AppendsToExisting(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	// 先写入一份带 prompts/safety 的现有配置
	existing := &internalai.Config{
		AI: internalai.AIConfig{
			Provider: "openai",
			Model:    "gpt-4o",
			APIKey:   "old-key",
			BaseURL:  "https://api.openai.com/v1",
			Timeout:  120,
		},
		Prompts: internalai.PromptsConfig{
			System:   "my-system.md",
			Playbook: "my-playbook.md",
			Command:  "my-command.md",
			Transfer: "my-transfer.md",
		},
		Safety: internalai.SafetyConfig{
			ConfirmDangerous: false,
			BlockedCommands:  []string{"custom-block"},
		},
	}
	if err := internalai.SaveConfig(configPath, existing); err != nil {
		t.Fatalf("failed to save existing config: %v", err)
	}

	// 只追加设置 provider=anthropic，其余字段回车保留现有值
	cfg := setupInput(t, configPath, "anthropic\n\n\n\n\n")

	if cfg.AI.Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got '%s'", cfg.AI.Provider)
	}
	if cfg.AI.APIKey != "old-key" {
		t.Errorf("expected existing api key 'old-key' preserved, got '%s'", cfg.AI.APIKey)
	}
	if cfg.Prompts.System != "my-system.md" {
		t.Errorf("expected prompts.system preserved, got '%s'", cfg.Prompts.System)
	}
	if cfg.Safety.ConfirmDangerous {
		t.Error("expected safety.confirm_dangerous preserved as false")
	}
	if len(cfg.Safety.BlockedCommands) != 1 || cfg.Safety.BlockedCommands[0] != "custom-block" {
		t.Errorf("expected safety.blocked_commands preserved, got %v", cfg.Safety.BlockedCommands)
	}
}

func TestConfigSetup_KeepsDefaultPromptsWhenNoFile(t *testing.T) {
	cfg := setupInput(t, filepath.Join(t.TempDir(), "config.yaml"),
		"1\n\nsk-1\n\n\n")

	if cfg.Prompts.System != "system.md" {
		t.Errorf("expected default prompts.system 'system.md', got '%s'", cfg.Prompts.System)
	}
}

func TestConfigShow_RenderIncludesPath(t *testing.T) {
	cfg := internalai.DefaultConfig()
	path := filepath.Join(string(os.PathSeparator)+"home", "user", ".owl", "config.yaml")

	out := renderConfigShow(cfg, path)

	if !strings.Contains(out, path) {
		t.Errorf("expected render output to contain config path '%s', got:\n%s", path, out)
	}
	if !strings.Contains(out, "openai") {
		t.Errorf("expected render output to contain provider 'openai', got:\n%s", out)
	}
}

func TestConfigShow_RenderMasksAPIKey(t *testing.T) {
	cfg := &internalai.Config{
		AI: internalai.AIConfig{
			Provider: "openai",
			Model:    "gpt-4o",
			APIKey:   "sk-secret-key-123456",
			BaseURL:  "https://api.openai.com/v1",
			Timeout:  120,
		},
	}
	out := renderConfigShow(cfg, "/tmp/config.yaml")

	if strings.Contains(out, "sk-secret-key-123456") {
		t.Error("expected render output NOT to contain full api key")
	}
	if !strings.Contains(out, "sk-s") {
		t.Errorf("expected render output to contain masked api key prefix, got:\n%s", out)
	}
}

func TestConfigSetup_SaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	cfg := setupInput(t, configPath, "5\n\nsk-save-1\n\n\n")
	if err := internalai.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	loaded, err := internalai.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}
	if loaded.AI.Provider != "deepseek" {
		t.Errorf("expected round-trip provider 'deepseek', got '%s'", loaded.AI.Provider)
	}
	if loaded.AI.APIKey != "sk-save-1" {
		t.Errorf("expected round-trip api key 'sk-save-1', got '%s'", loaded.AI.APIKey)
	}
}
