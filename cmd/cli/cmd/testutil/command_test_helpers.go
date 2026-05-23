package testutil

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func AssertCommandExists(t *testing.T, parent *cobra.Command, name string) {
	t.Helper()

	cmd, _, err := parent.Find([]string{name})
	if err != nil {
		t.Errorf("expected subcommand '%s' to exist under '%s', but it was not found: %v", name, parent.Name(), err)
		return
	}
	if cmd == nil {
		t.Errorf("expected subcommand '%s' to exist under '%s', but it was nil", name, parent.Name())
	}
}

func AssertCommandNotExists(t *testing.T, parent *cobra.Command, name string) {
	t.Helper()

	cmd, _, err := parent.Find([]string{name})
	if err == nil && cmd != nil && cmd != parent {
		t.Errorf("expected subcommand '%s' NOT to exist under '%s', but it was found", name, parent.Name())
	}
}

func AssertSubCommands(t *testing.T, parent *cobra.Command, expected []string) {
	t.Helper()

	existing := make(map[string]bool)
	for _, c := range parent.Commands() {
		existing[c.Name()] = true
	}

	for _, name := range expected {
		if !existing[name] {
			t.Errorf("expected subcommand '%s' under '%s', but it was not found", name, parent.Name())
		}
	}
}

func AssertFlagExists(t *testing.T, cmd *cobra.Command, flagName string) {
	t.Helper()

	flag := cmd.Flags().Lookup(flagName)
	if flag == nil {
		t.Errorf("expected flag '--%s' to exist on command '%s'", flagName, cmd.Name())
	}
}

func AssertFlagDefault(t *testing.T, cmd *cobra.Command, flagName string, expected string) {
	t.Helper()

	flag := cmd.Flags().Lookup(flagName)
	if flag == nil {
		t.Errorf("expected flag '--%s' to exist on command '%s'", flagName, cmd.Name())
		return
	}
	if flag.DefValue != expected {
		t.Errorf("flag '--%s': expected default '%s', got '%s'", flagName, expected, flag.DefValue)
	}
}

func AssertFlagShorthand(t *testing.T, cmd *cobra.Command, flagName string, expectedShorthand string) {
	t.Helper()

	flag := cmd.Flags().Lookup(flagName)
	if flag == nil {
		t.Errorf("expected flag '--%s' to exist on command '%s'", flagName, cmd.Name())
		return
	}
	if flag.Shorthand != expectedShorthand {
		t.Errorf("flag '--%s': expected shorthand '-%s', got '-%s'", flagName, expectedShorthand, flag.Shorthand)
	}
}

func AssertFlagUsageContains(t *testing.T, cmd *cobra.Command, flagName string, expected string) {
	t.Helper()

	flag := cmd.Flags().Lookup(flagName)
	if flag == nil {
		t.Errorf("expected flag '--%s' to exist on command '%s'", flagName, cmd.Name())
		return
	}
	if !strings.Contains(flag.Usage, expected) {
		t.Errorf("flag '--%s' usage: expected to contain '%s', got '%s'", flagName, expected, flag.Usage)
	}
}

func AssertHelpContains(t *testing.T, cmd *cobra.Command, text string) {
	t.Helper()

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.Help()

	output := buf.String()
	if !strings.Contains(output, text) {
		t.Errorf("expected help output of '%s' to contain '%s', but it did not.\nOutput:\n%s", cmd.Name(), text, output)
	}
}

func ExecuteCommand(t *testing.T, cmd *cobra.Command, args ...string) string {
	t.Helper()

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)

	err := cmd.Execute()
	if err != nil {
		t.Logf("command '%s' with args %v returned error: %v", cmd.Name(), args, err)
	}

	return buf.String()
}

func ExecuteCommandMustFail(t *testing.T, cmd *cobra.Command, args ...string) string {
	t.Helper()

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)

	err := cmd.Execute()
	if err == nil {
		t.Errorf("command '%s' with args %v was expected to fail, but succeeded", cmd.Name(), args)
	}

	return buf.String()
}
