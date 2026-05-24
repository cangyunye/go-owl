package exec_test

import (
	"testing"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/exec"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/testutil"
)

func TestExecCmdExists(t *testing.T) {
	parent := exec.NewExecCmd()

	if parent.Use != "exec" {
		t.Errorf("expected Use 'exec', got '%s'", parent.Use)
	}

	expected := []string{"run", "script"}
	testutil.AssertSubCommands(t, parent, expected)
}

func TestExecRunFlags(t *testing.T) {
	cmd := exec.NewRunCmd()

	testutil.AssertFlagExists(t, cmd, "nodes")
	testutil.AssertFlagDefault(t, cmd, "nodes", "")

	testutil.AssertFlagExists(t, cmd, "group")
	testutil.AssertFlagDefault(t, cmd, "group", "")

	testutil.AssertFlagExists(t, cmd, "label")
	testutil.AssertFlagShorthand(t, cmd, "label", "l")

	testutil.AssertFlagExists(t, cmd, "status")
	testutil.AssertFlagDefault(t, cmd, "status", "")

	testutil.AssertFlagExists(t, cmd, "timeout")
	testutil.AssertFlagDefault(t, cmd, "timeout", "1m0s")

	testutil.AssertFlagExists(t, cmd, "connect-timeout")
	testutil.AssertFlagDefault(t, cmd, "connect-timeout", "10s")

	testutil.AssertFlagExists(t, cmd, "command-timeout")
	testutil.AssertFlagDefault(t, cmd, "command-timeout", "30s")

	testutil.AssertFlagExists(t, cmd, "parallel")
	testutil.AssertFlagDefault(t, cmd, "parallel", "true")

	testutil.AssertFlagExists(t, cmd, "serial")
	testutil.AssertFlagDefault(t, cmd, "serial", "false")

	testutil.AssertFlagExists(t, cmd, "retry")
	testutil.AssertFlagDefault(t, cmd, "retry", "3")

	testutil.AssertFlagExists(t, cmd, "retry-interval")
	testutil.AssertFlagDefault(t, cmd, "retry-interval", "1s")

	testutil.AssertFlagExists(t, cmd, "retry-max-interval")
	testutil.AssertFlagDefault(t, cmd, "retry-max-interval", "30s")

	testutil.AssertFlagExists(t, cmd, "no-retry")
	testutil.AssertFlagDefault(t, cmd, "no-retry", "false")

	testutil.AssertFlagExists(t, cmd, "async")
	testutil.AssertFlagDefault(t, cmd, "async", "false")

	testutil.AssertFlagExists(t, cmd, "async-timeout")
	testutil.AssertFlagDefault(t, cmd, "async-timeout", "1h0m0s")

	testutil.AssertFlagExists(t, cmd, "async-poll-interval")
	testutil.AssertFlagDefault(t, cmd, "async-poll-interval", "10s")

	testutil.AssertFlagExists(t, cmd, "async-max-poll-count")
	testutil.AssertFlagDefault(t, cmd, "async-max-poll-count", "3600")

	testutil.AssertFlagExists(t, cmd, "async-remote-dir")
	testutil.AssertFlagDefault(t, cmd, "async-remote-dir", "/tmp/owl")

	testutil.AssertFlagExists(t, cmd, "output")
	testutil.AssertFlagShorthand(t, cmd, "output", "o")
	testutil.AssertFlagDefault(t, cmd, "output", "simple")

	testutil.AssertFlagExists(t, cmd, "no-color")
	testutil.AssertFlagDefault(t, cmd, "no-color", "false")

	testutil.AssertFlagExists(t, cmd, "debug")
	testutil.AssertFlagDefault(t, cmd, "debug", "false")

	testutil.AssertFlagExists(t, cmd, "force")
	testutil.AssertFlagShorthand(t, cmd, "force", "f")
	testutil.AssertFlagDefault(t, cmd, "force", "false")
}

func TestExecScriptFlags(t *testing.T) {
	cmd := exec.NewScriptCmd()

	testutil.AssertFlagExists(t, cmd, "nodes")
	testutil.AssertFlagDefault(t, cmd, "nodes", "")

	testutil.AssertFlagExists(t, cmd, "group")
	testutil.AssertFlagDefault(t, cmd, "group", "")

	testutil.AssertFlagExists(t, cmd, "label")
	testutil.AssertFlagShorthand(t, cmd, "label", "l")

	testutil.AssertFlagExists(t, cmd, "dest")
	testutil.AssertFlagDefault(t, cmd, "dest", "/tmp")

	testutil.AssertFlagExists(t, cmd, "args")
	testutil.AssertFlagDefault(t, cmd, "args", "")

	testutil.AssertFlagExists(t, cmd, "timeout")
	testutil.AssertFlagDefault(t, cmd, "timeout", "5m0s")

	testutil.AssertFlagExists(t, cmd, "inline")
	testutil.AssertFlagDefault(t, cmd, "inline", "false")

	testutil.AssertFlagExists(t, cmd, "keep")
	testutil.AssertFlagDefault(t, cmd, "keep", "false")

	testutil.AssertFlagExists(t, cmd, "force")
	testutil.AssertFlagShorthand(t, cmd, "force", "f")
	testutil.AssertFlagDefault(t, cmd, "force", "false")
}

func TestExecRunCmdUseArg(t *testing.T) {
	cmd := exec.NewRunCmd()

	if cmd.Use == "" {
		t.Error("expected non-empty Use for exec run")
	}
}

func TestExecScriptCmdUseArg(t *testing.T) {
	cmd := exec.NewScriptCmd()

	if cmd.Use == "" {
		t.Error("expected non-empty Use for exec script")
	}
}

func TestExecHelpContainsSubcommands(t *testing.T) {
	parent := exec.NewExecCmd()

	testutil.AssertHelpContains(t, parent, "run")
	testutil.AssertHelpContains(t, parent, "script")
}
