package ai_test

import (
	"testing"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/ai"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/testutil"
)

func TestAICmdExists(t *testing.T) {
	parent := ai.NewAICmd()

	if parent.Use != "ai" {
		t.Errorf("expected Use 'ai', got '%s'", parent.Use)
	}

	testutil.AssertCommandExists(t, parent, "models")

	configCmd, _, _ := parent.Find([]string{"config"})
	if configCmd == nil {
		t.Fatal("expected 'config' subcommand to exist")
	}

	testutil.AssertCommandExists(t, configCmd, "init")
	testutil.AssertCommandExists(t, configCmd, "show")
}

func TestAIFlags(t *testing.T) {
	cmd := ai.NewAICmd()

	testutil.AssertFlagExists(t, cmd, "model")
	testutil.AssertFlagDefault(t, cmd, "model", "gpt-4o")

	testutil.AssertFlagExists(t, cmd, "provider")
	testutil.AssertFlagDefault(t, cmd, "provider", "openai")

	testutil.AssertFlagExists(t, cmd, "api-key")
	testutil.AssertFlagDefault(t, cmd, "api-key", "")

	testutil.AssertFlagExists(t, cmd, "base-url")
	testutil.AssertFlagDefault(t, cmd, "base-url", "")

	testutil.AssertFlagExists(t, cmd, "timeout")
	testutil.AssertFlagDefault(t, cmd, "timeout", "120")

	testutil.AssertFlagExists(t, cmd, "session")
	testutil.AssertFlagDefault(t, cmd, "session", "")
}

func TestAIModelsFlags(t *testing.T) {
	cmd := ai.NewModelsCmd()

	if cmd.Use != "models" {
		t.Errorf("expected Use 'models', got '%s'", cmd.Use)
	}

	testutil.AssertFlagExists(t, cmd, "provider")
	testutil.AssertFlagDefault(t, cmd, "provider", "openai")

	testutil.AssertFlagExists(t, cmd, "api-key")
	testutil.AssertFlagDefault(t, cmd, "api-key", "")

	testutil.AssertFlagExists(t, cmd, "base-url")
	testutil.AssertFlagDefault(t, cmd, "base-url", "")

	testutil.AssertFlagExists(t, cmd, "timeout")
	testutil.AssertFlagDefault(t, cmd, "timeout", "30")
}

func TestAIConfigSubcommands(t *testing.T) {
	configCmd := ai.NewConfigCmd()

	if configCmd.Use != "config" {
		t.Errorf("expected Use 'config', got '%s'", configCmd.Use)
	}

	testutil.AssertCommandExists(t, configCmd, "init")
	testutil.AssertCommandExists(t, configCmd, "show")
}

func TestAIConfigInitCmd(t *testing.T) {
	cmd := ai.NewConfigInitCmd()

	if cmd.Use != "init" {
		t.Errorf("expected Use 'init', got '%s'", cmd.Use)
	}
}

func TestAIConfigShowCmd(t *testing.T) {
	cmd := ai.NewConfigShowCmd()

	if cmd.Use != "show" {
		t.Errorf("expected Use 'show', got '%s'", cmd.Use)
	}
}

func TestAIHelpContainsSubcommands(t *testing.T) {
	parent := ai.NewAICmd()

	testutil.AssertHelpContains(t, parent, "models")
	testutil.AssertHelpContains(t, parent, "config")
}

func TestAIModelsProviderFlagUsage(t *testing.T) {
	cmd := ai.NewModelsCmd()

	flag := cmd.Flags().Lookup("provider")
	if flag == nil {
		t.Fatal("expected --provider flag to exist")
	}

	if flag.DefValue != "openai" {
		t.Errorf("expected provider default 'openai', got '%s'", flag.DefValue)
	}

	if flag.Usage == "" {
		t.Error("expected provider flag to have usage description")
	}
}
