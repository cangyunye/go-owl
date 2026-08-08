package serve_test

import (
	"testing"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/serve"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/testutil"
)

func TestServeCmdExists(t *testing.T) {
	cmd := serve.NewServeCmd()

	if cmd.Use != "serve" {
		t.Errorf("expected Use 'serve', got '%s'", cmd.Use)
	}
	if cmd.Short != "启动 OWL Web 管理控制台" {
		t.Errorf("expected Short '启动 OWL Web 管理控制台', got '%s'", cmd.Short)
	}
}

func TestServeFlags(t *testing.T) {
	cmd := serve.NewServeCmd()

	testutil.AssertFlagExists(t, cmd, "port")
	testutil.AssertFlagShorthand(t, cmd, "port", "p")
	testutil.AssertFlagDefault(t, cmd, "port", "8080")

	testutil.AssertFlagExists(t, cmd, "host")
	testutil.AssertFlagDefault(t, cmd, "host", "127.0.0.1")

	testutil.AssertFlagExists(t, cmd, "dev")
	testutil.AssertFlagDefault(t, cmd, "dev", "false")

	testutil.AssertFlagExists(t, cmd, "reset-admin")
	testutil.AssertFlagDefault(t, cmd, "reset-admin", "false")

	testutil.AssertFlagExists(t, cmd, "ai-debug")
	testutil.AssertFlagDefault(t, cmd, "ai-debug", "false")
}

func TestServeHelpContainsSections(t *testing.T) {
	cmd := serve.NewServeCmd()

	sections := []string{
		"serve",
		"Web",
		"port",
		"host",
	}
	for _, section := range sections {
		testutil.AssertHelpContains(t, cmd, section)
	}
}

func TestServeHasNoSubCommands(t *testing.T) {
	cmd := serve.NewServeCmd()

	if len(cmd.Commands()) != 0 {
		t.Errorf("expected serve to have no subcommands, got %d", len(cmd.Commands()))
	}
}
