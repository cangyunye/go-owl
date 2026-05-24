package ai

import (
	"os"
	"testing"
)

func LoadConfigForTest(t *testing.T) *Config {
	t.Helper()

	if os.Getenv("OWL_TEST_AI_ENABLED") != "true" {
		return &Config{
			AI: AIConfig{
				Provider: "openai",
				APIKey:   "test-key",
				Model:    "gpt-4o",
				BaseURL:  "https://api.openai.com/v1",
				Timeout:  120,
			},
			Safety: SafetyConfig{
				ConfirmDangerous: true,
			},
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot determine home directory: %v", err)
	}

	path := home + "/.owl/config.yaml"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("AI config file not found at %s, set OWL_TEST_AI_ENABLED=false or create the file", path)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("failed to load config from %s: %v", path, err)
	}

	if cfg.AI.APIKey == "" {
		t.Skipf("no API key found in %s or environment, skipping test", path)
	}

	return cfg
}

func TestAIConfig(t *testing.T) {
	cfg, err := LoadConfig(t.TempDir() + "/nonexistent.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.AI.Provider != "openai" {
		t.Errorf("expected default provider 'openai', got '%s'", cfg.AI.Provider)
	}
	if cfg.AI.Model != "gpt-4o" {
		t.Errorf("expected default model 'gpt-4o', got '%s'", cfg.AI.Model)
	}
	if cfg.AI.Timeout != 120 {
		t.Errorf("expected default timeout 120, got %d", cfg.AI.Timeout)
	}
	if !cfg.Safety.ConfirmDangerous {
		t.Error("expected ConfirmDangerous to be true")
	}
}

func TestLoadConfig_FromFile(t *testing.T) {
	if os.Getenv("OWL_TEST_AI_ENABLED") != "true" {
		t.Skip("skipping config file test; set OWL_TEST_AI_ENABLED=true to run")
	}

	home, _ := os.UserHomeDir()
	path := home + "/.owl/config.yaml"

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("config file not found at %s", path)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.AI.Provider == "" {
		t.Error("expected non-empty provider")
	}
	if cfg.AI.APIKey == "" {
		t.Error("expected non-empty API key")
	}
}

func TestLoadConfig_EnvFallback(t *testing.T) {
	os.Setenv("OWL_API_KEY", "env-test-key")
	defer os.Unsetenv("OWL_API_KEY")

	cfg, err := LoadConfig(t.TempDir() + "/nonexistent.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AI.APIKey != "env-test-key" {
		t.Errorf("expected API key 'env-test-key' from env, got '%s'", cfg.AI.APIKey)
	}
}

func TestDefaultConfig_Structure(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.AI.Provider != "openai" {
		t.Errorf("expected provider 'openai', got '%s'", cfg.AI.Provider)
	}
	if cfg.AI.Model != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got '%s'", cfg.AI.Model)
	}
	if cfg.AI.Timeout != 120 {
		t.Errorf("expected timeout 120, got %d", cfg.AI.Timeout)
	}
	if !cfg.Safety.ConfirmDangerous {
		t.Error("expected ConfirmDangerous true")
	}
	if len(cfg.Safety.BlockedCommands) == 0 {
		t.Error("expected blocked commands to be non-empty")
	}
	blocked := []string{"rm -rf /", ":(){:|:&};:", "dd if=/dev/zero of=/dev/sda"}
	for _, cmd := range blocked {
		found := false
		for _, b := range cfg.Safety.BlockedCommands {
			if b == cmd {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected blocked command '%s' to be present", cmd)
		}
	}
}

func TestConfig_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test-config.yaml"

	cfg := &Config{
		AI: AIConfig{
			Provider: "deepseek",
			Model:    "deepseek-chat",
			APIKey:   "saved-test-key",
			BaseURL:  "https://api.deepseek.com",
			Timeout:  60,
		},
	}

	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if loaded.AI.Provider != "deepseek" {
		t.Errorf("expected provider 'deepseek', got '%s'", loaded.AI.Provider)
	}
	if loaded.AI.Model != "deepseek-chat" {
		t.Errorf("expected model 'deepseek-chat', got '%s'", loaded.AI.Model)
	}
	if loaded.AI.APIKey != "saved-test-key" {
		t.Errorf("expected API key 'saved-test-key', got '%s'", loaded.AI.APIKey)
	}
	if loaded.AI.BaseURL != "https://api.deepseek.com" {
		t.Errorf("expected base URL 'https://api.deepseek.com', got '%s'", loaded.AI.BaseURL)
	}
	if loaded.AI.Timeout != 60 {
		t.Errorf("expected timeout 60, got %d", loaded.AI.Timeout)
	}
}

func TestConfig_SavePermission(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/perm-test.yaml"

	cfg := DefaultConfig()
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected permission 0600, got %04o", info.Mode().Perm())
	}
}
