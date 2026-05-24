package playbook_test

import (
	"testing"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/playbook"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/testutil"
)

func TestPlaybookCmdExists(t *testing.T) {
	parent := playbook.NewPlaybookCmd()

	if parent.Use != "playbook" {
		t.Errorf("expected Use 'playbook', got '%s'", parent.Use)
	}

	expected := []string{"list", "run", "info", "validate"}
	testutil.AssertSubCommands(t, parent, expected)
}

func TestPlaybookListFlags(t *testing.T) {
	cmd := playbook.NewPlaybookListCmd()

	testutil.AssertFlagExists(t, cmd, "library")
	testutil.AssertFlagDefault(t, cmd, "library", "./playbooks")

	testutil.AssertFlagExists(t, cmd, "output")
	testutil.AssertFlagShorthand(t, cmd, "output", "o")
	testutil.AssertFlagDefault(t, cmd, "output", "table")
}

func TestPlaybookRunFlags(t *testing.T) {
	cmd := playbook.NewPlaybookRunCmd()

	testutil.AssertFlagExists(t, cmd, "nodes")
	testutil.AssertFlagDefault(t, cmd, "nodes", "")

	testutil.AssertFlagExists(t, cmd, "group")
	testutil.AssertFlagDefault(t, cmd, "group", "")

	testutil.AssertFlagExists(t, cmd, "label")
	testutil.AssertFlagShorthand(t, cmd, "label", "l")

	testutil.AssertFlagExists(t, cmd, "tags")
	testutil.AssertFlagDefault(t, cmd, "tags", "")

	testutil.AssertFlagExists(t, cmd, "skip-tags")
	testutil.AssertFlagDefault(t, cmd, "skip-tags", "")

	testutil.AssertFlagExists(t, cmd, "extra-vars")
	testutil.AssertFlagExists(t, cmd, "check")
	testutil.AssertFlagDefault(t, cmd, "check", "false")

	testutil.AssertFlagExists(t, cmd, "diff")
	testutil.AssertFlagDefault(t, cmd, "diff", "false")

	testutil.AssertFlagExists(t, cmd, "default-connect-timeout")
	testutil.AssertFlagDefault(t, cmd, "default-connect-timeout", "10s")

	testutil.AssertFlagExists(t, cmd, "default-command-timeout")
	testutil.AssertFlagDefault(t, cmd, "default-command-timeout", "5m0s")

	testutil.AssertFlagExists(t, cmd, "default-retry")
	testutil.AssertFlagDefault(t, cmd, "default-retry", "0")

	testutil.AssertFlagExists(t, cmd, "default-retry-interval")
	testutil.AssertFlagDefault(t, cmd, "default-retry-interval", "1s")

	testutil.AssertFlagExists(t, cmd, "default-retry-max-interval")
	testutil.AssertFlagDefault(t, cmd, "default-retry-max-interval", "30s")
}

func TestPlaybookInfoCmd(t *testing.T) {
	cmd := playbook.NewPlaybookInfoCmd()

	if cmd.Use == "" {
		t.Error("expected non-empty Use for playbook info")
	}
}

func TestPlaybookValidateCmd(t *testing.T) {
	cmd := playbook.NewPlaybookValidateCmd()

	if cmd.Use == "" {
		t.Error("expected non-empty Use for playbook validate")
	}
}

func TestPlaybookHelpContainsSubcommands(t *testing.T) {
	parent := playbook.NewPlaybookCmd()

	testutil.AssertHelpContains(t, parent, "list")
	testutil.AssertHelpContains(t, parent, "run")
	testutil.AssertHelpContains(t, parent, "info")
	testutil.AssertHelpContains(t, parent, "validate")
}
