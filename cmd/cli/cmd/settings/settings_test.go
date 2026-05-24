package settings_test

import (
	"testing"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/settings"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/testutil"
)

func TestSettingsCmdExists(t *testing.T) {
	parent := settings.NewSettingsCmd()

	if parent.Use != "settings" {
		t.Errorf("expected Use 'settings', got '%s'", parent.Use)
	}

	expected := []string{"show", "set", "target"}
	testutil.AssertSubCommands(t, parent, expected)
}

func TestSettingsShowCmd(t *testing.T) {
	cmd := settings.NewSettingsShowCmd()

	if cmd.Use != "show" {
		t.Errorf("expected Use 'show', got '%s'", cmd.Use)
	}
}

func TestSettingsSetCmd(t *testing.T) {
	cmd := settings.NewSettingsSetCmd()

	if cmd.Use != "set <key> <value>" {
		t.Errorf("expected Use 'set <key> <value>', got '%s'", cmd.Use)
	}
}

func TestSettingsTargetFlags(t *testing.T) {
	cmd := settings.NewSettingsTargetCmd()

	testutil.AssertFlagExists(t, cmd, "group")
	testutil.AssertFlagDefault(t, cmd, "group", "")

	testutil.AssertFlagExists(t, cmd, "label")
	testutil.AssertFlagShorthand(t, cmd, "label", "l")

	testutil.AssertFlagExists(t, cmd, "nodes")
	testutil.AssertFlagDefault(t, cmd, "nodes", "")
}

func TestSettingsTargetCmdUse(t *testing.T) {
	cmd := settings.NewSettingsTargetCmd()

	if cmd.Use != "target" {
		t.Errorf("expected Use 'target', got '%s'", cmd.Use)
	}
}

func TestSettingsHelpContainsSubcommands(t *testing.T) {
	parent := settings.NewSettingsCmd()

	testutil.AssertHelpContains(t, parent, "show")
	testutil.AssertHelpContains(t, parent, "set")
	testutil.AssertHelpContains(t, parent, "target")
}
