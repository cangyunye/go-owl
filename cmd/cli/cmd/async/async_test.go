package async_test

import (
	"testing"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/async"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/testutil"
)

func TestAsyncCmdExists(t *testing.T) {
	parent := async.NewAsyncCmd()

	if parent.Use != "async" {
		t.Errorf("expected Use 'async', got '%s'", parent.Use)
	}

	expected := []string{"list", "status", "wait", "cancel", "cleanup"}
	testutil.AssertSubCommands(t, parent, expected)
}

func TestAsyncListCmd(t *testing.T) {
	cmd := async.NewListCmd()

	if cmd.Use != "list" {
		t.Errorf("expected Use 'list', got '%s'", cmd.Use)
	}
}

func TestAsyncStatusCmd(t *testing.T) {
	cmd := async.NewStatusCmd()

	if cmd.Use != "status <task-id>" {
		t.Errorf("expected Use 'status <task-id>', got '%s'", cmd.Use)
	}
}

func TestAsyncWaitCmd(t *testing.T) {
	cmd := async.NewWaitCmd()

	if cmd.Use != "wait <task-id>" {
		t.Errorf("expected Use 'wait <task-id>', got '%s'", cmd.Use)
	}

	testutil.AssertFlagExists(t, cmd, "poll-interval")
	testutil.AssertFlagDefault(t, cmd, "poll-interval", "10s")
}

func TestAsyncCancelCmd(t *testing.T) {
	cmd := async.NewCancelCmd()

	if cmd.Use != "cancel <task-id>" {
		t.Errorf("expected Use 'cancel <task-id>', got '%s'", cmd.Use)
	}
}

func TestAsyncCleanupCmd(t *testing.T) {
	cmd := async.NewCleanupCmd()

	if cmd.Use != "cleanup" {
		t.Errorf("expected Use 'cleanup', got '%s'", cmd.Use)
	}
}

func TestAsyncHelpContainsSubcommands(t *testing.T) {
	parent := async.NewAsyncCmd()

	testutil.AssertHelpContains(t, parent, "list")
	testutil.AssertHelpContains(t, parent, "status")
	testutil.AssertHelpContains(t, parent, "wait")
	testutil.AssertHelpContains(t, parent, "cancel")
	testutil.AssertHelpContains(t, parent, "cleanup")
}
