package history_test

import (
	"testing"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/history"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/testutil"
)

func TestHistoryCmdExists(t *testing.T) {
	parent := history.NewHistoryCmd()

	if parent.Use != "history" {
		t.Errorf("expected Use 'history', got '%s'", parent.Use)
	}

	testutil.AssertCommandExists(t, parent, "clean")
}

func TestHistoryFlags(t *testing.T) {
	cmd := history.NewHistoryCmd()

	testutil.AssertFlagExists(t, cmd, "task-id")
	testutil.AssertFlagDefault(t, cmd, "task-id", "")

	testutil.AssertFlagExists(t, cmd, "node-id")
	testutil.AssertFlagDefault(t, cmd, "node-id", "")

	testutil.AssertFlagExists(t, cmd, "op-type")
	testutil.AssertFlagDefault(t, cmd, "op-type", "")

	testutil.AssertFlagExists(t, cmd, "status")
	testutil.AssertFlagDefault(t, cmd, "status", "")

	testutil.AssertFlagExists(t, cmd, "start-time")
	testutil.AssertFlagDefault(t, cmd, "start-time", "")

	testutil.AssertFlagExists(t, cmd, "end-time")
	testutil.AssertFlagDefault(t, cmd, "end-time", "")

	testutil.AssertFlagExists(t, cmd, "last")
	testutil.AssertFlagDefault(t, cmd, "last", "")

	testutil.AssertFlagExists(t, cmd, "limit")
	testutil.AssertFlagDefault(t, cmd, "limit", "50")

	testutil.AssertFlagExists(t, cmd, "offset")
	testutil.AssertFlagDefault(t, cmd, "offset", "0")

	testutil.AssertFlagExists(t, cmd, "format")
	testutil.AssertFlagDefault(t, cmd, "format", "table")

	testutil.AssertFlagExists(t, cmd, "output")
	testutil.AssertFlagDefault(t, cmd, "output", "")

	testutil.AssertFlagExists(t, cmd, "verbose")
	testutil.AssertFlagDefault(t, cmd, "verbose", "false")
}

func TestHistoryCleanFlags(t *testing.T) {
	cmd := history.NewCleanCmd()

	if cmd.Use != "clean" {
		t.Errorf("expected Use 'clean', got '%s'", cmd.Use)
	}

	testutil.AssertFlagExists(t, cmd, "days")
	testutil.AssertFlagDefault(t, cmd, "days", "30")

	testutil.AssertFlagExists(t, cmd, "force")
	testutil.AssertFlagDefault(t, cmd, "force", "false")
}

func TestHistoryHelpContainsClean(t *testing.T) {
	parent := history.NewHistoryCmd()

	testutil.AssertHelpContains(t, parent, "clean")
}
