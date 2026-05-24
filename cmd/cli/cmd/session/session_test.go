package session_test

import (
	"testing"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/session"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/testutil"
)

func TestSessionCmdExists(t *testing.T) {
	parent := session.NewCmd()

	if parent.Use != "session" {
		t.Errorf("expected Use 'session', got '%s'", parent.Use)
	}

	expected := []string{"attach", "list", "history"}
	testutil.AssertSubCommands(t, parent, expected)
}

func TestSessionAttachFlags(t *testing.T) {
	cmd := session.NewAttachCmd()

	testutil.AssertFlagExists(t, cmd, "nodes")
	testutil.AssertFlagDefault(t, cmd, "nodes", "")

	testutil.AssertFlagExists(t, cmd, "ssh-config")
	testutil.AssertFlagDefault(t, cmd, "ssh-config", "")

	testutil.AssertFlagExists(t, cmd, "key")
	testutil.AssertFlagDefault(t, cmd, "key", "")

	testutil.AssertFlagExists(t, cmd, "timeout")
	testutil.AssertFlagDefault(t, cmd, "timeout", "30m")
}

func TestSessionListCmd(t *testing.T) {
	cmd := session.NewListCmd()

	if cmd.Use != "list" {
		t.Errorf("expected Use 'list', got '%s'", cmd.Use)
	}
}

func TestSessionHistoryFlags(t *testing.T) {
	cmd := session.NewHistoryCmd()

	testutil.AssertFlagExists(t, cmd, "session-id")
	testutil.AssertFlagDefault(t, cmd, "session-id", "")

	testutil.AssertFlagExists(t, cmd, "node")
	testutil.AssertFlagDefault(t, cmd, "node", "")

	testutil.AssertFlagExists(t, cmd, "last")
	testutil.AssertFlagDefault(t, cmd, "last", "")

	testutil.AssertFlagExists(t, cmd, "verbose")
	testutil.AssertFlagShorthand(t, cmd, "verbose", "v")
	testutil.AssertFlagDefault(t, cmd, "verbose", "false")

	testutil.AssertFlagExists(t, cmd, "limit")
	testutil.AssertFlagShorthand(t, cmd, "limit", "n")
	testutil.AssertFlagDefault(t, cmd, "limit", "20")
}

func TestSessionAttachCmdUse(t *testing.T) {
	cmd := session.NewAttachCmd()

	if cmd.Use == "" {
		t.Error("expected non-empty Use for session attach")
	}
}

func TestSessionHistoryCmdUse(t *testing.T) {
	cmd := session.NewHistoryCmd()

	if cmd.Use == "" {
		t.Error("expected non-empty Use for session history")
	}
}

func TestSessionHelpContainsSubcommands(t *testing.T) {
	parent := session.NewCmd()

	testutil.AssertHelpContains(t, parent, "attach")
	testutil.AssertHelpContains(t, parent, "list")
	testutil.AssertHelpContains(t, parent, "history")
}
