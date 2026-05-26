package node_test

import (
	"testing"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/node"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/testutil"
)

func TestNodeCmdExists(t *testing.T) {
	parent := node.NewNodeCmd()

	if parent.Use != "node" {
		t.Errorf("expected Use 'node', got '%s'", parent.Use)
	}

	expected := []string{
		"list", "add", "update", "remove", "import", "export",
		"status", "groups", "labels", "sample", "ping", "check",
	}
	testutil.AssertSubCommands(t, parent, expected)
}

func TestNodeAddFlags(t *testing.T) {
	cmd := node.NewAddCmd()

	testutil.AssertFlagExists(t, cmd, "name")
	testutil.AssertFlagShorthand(t, cmd, "name", "n")
	testutil.AssertFlagDefault(t, cmd, "name", "")
	testutil.AssertFlagUsageContains(t, cmd, "name", "必需")

	testutil.AssertFlagExists(t, cmd, "address")
	testutil.AssertFlagShorthand(t, cmd, "address", "a")
	testutil.AssertFlagDefault(t, cmd, "address", "")
	testutil.AssertFlagUsageContains(t, cmd, "address", "必需")

	testutil.AssertFlagExists(t, cmd, "port")
	testutil.AssertFlagShorthand(t, cmd, "port", "p")
	testutil.AssertFlagDefault(t, cmd, "port", "22")

	testutil.AssertFlagExists(t, cmd, "user")
	testutil.AssertFlagShorthand(t, cmd, "user", "u")
	testutil.AssertFlagDefault(t, cmd, "user", "")

	testutil.AssertFlagExists(t, cmd, "password")
	testutil.AssertFlagExists(t, cmd, "ssh-key")
	testutil.AssertFlagExists(t, cmd, "proxy-jump")
	testutil.AssertFlagExists(t, cmd, "groups")
	testutil.AssertFlagExists(t, cmd, "labels")
	testutil.AssertFlagShorthand(t, cmd, "labels", "l")
}

func TestNodeListFlags(t *testing.T) {
	cmd := node.NewListCmd()

	testutil.AssertFlagExists(t, cmd, "format")
	testutil.AssertFlagShorthand(t, cmd, "format", "o")
	testutil.AssertFlagDefault(t, cmd, "format", "table")

	testutil.AssertFlagExists(t, cmd, "group")
	testutil.AssertFlagExists(t, cmd, "label")
	testutil.AssertFlagExists(t, cmd, "status")
	testutil.AssertFlagExists(t, cmd, "no-color")
}

func TestNodeUpdateFlags(t *testing.T) {
	cmd := node.NewUpdateCmd()

	testutil.AssertFlagExists(t, cmd, "name")
	testutil.AssertFlagShorthand(t, cmd, "name", "n")

	testutil.AssertFlagExists(t, cmd, "address")
	testutil.AssertFlagShorthand(t, cmd, "address", "a")

	testutil.AssertFlagExists(t, cmd, "port")
	testutil.AssertFlagShorthand(t, cmd, "port", "p")
	testutil.AssertFlagDefault(t, cmd, "port", "0")

	testutil.AssertFlagExists(t, cmd, "user")
	testutil.AssertFlagShorthand(t, cmd, "user", "u")

	testutil.AssertFlagExists(t, cmd, "password")
	testutil.AssertFlagExists(t, cmd, "ssh-key")
	testutil.AssertFlagExists(t, cmd, "proxy-jump")
	testutil.AssertFlagExists(t, cmd, "groups")
	testutil.AssertFlagExists(t, cmd, "labels")
	testutil.AssertFlagShorthand(t, cmd, "labels", "l")
	testutil.AssertFlagExists(t, cmd, "status")
}

func TestNodeRemoveCmd(t *testing.T) {
	cmd := node.NewRemoveCmd()
	testutil.AssertCommandExists(t, node.NewNodeCmd(), "remove")

	if cmd.Use == "" {
		t.Error("expected non-empty Use")
	}
}

func TestNodeStatusFlags(t *testing.T) {
	cmd := node.NewStatusCmd()

	testutil.AssertFlagExists(t, cmd, "all")
	testutil.AssertFlagExists(t, cmd, "output")
	testutil.AssertFlagShorthand(t, cmd, "output", "o")
	testutil.AssertFlagDefault(t, cmd, "output", "detail")
	testutil.AssertFlagExists(t, cmd, "no-color")
}

func TestNodeGroupsSubcommands(t *testing.T) {
	parent := node.NewGroupsCmd()

	expected := []string{"add", "remove", "list", "show"}
	testutil.AssertSubCommands(t, parent, expected)

	addCmd := node.NewGroupsAddCmd()
	if addCmd.Use == "" {
		t.Error("expected non-empty Use for groups add")
	}

	removeCmd := node.NewGroupsRemoveCmd()
	if removeCmd.Use == "" {
		t.Error("expected non-empty Use for groups remove")
	}

	listCmd := node.NewGroupsListCmd()
	if listCmd.Use == "" {
		t.Error("expected non-empty Use for groups list")
	}

	showCmd := node.NewGroupsShowCmd()
	if showCmd.Use == "" {
		t.Error("expected non-empty Use for groups show")
	}
}

func TestNodeLabelsSubcommands(t *testing.T) {
	parent := node.NewLabelsCmd()

	expected := []string{"set", "remove", "show"}
	testutil.AssertSubCommands(t, parent, expected)

	setCmd := node.NewLabelsSetCmd()
	if setCmd.Use == "" {
		t.Error("expected non-empty Use for labels set")
	}

	removeCmd := node.NewLabelsRemoveCmd()
	if removeCmd.Use == "" {
		t.Error("expected non-empty Use for labels remove")
	}

	showCmd := node.NewLabelsShowCmd()
	if showCmd.Use == "" {
		t.Error("expected non-empty Use for labels show")
	}
}

func TestNodeImportFlags(t *testing.T) {
	cmd := node.NewImportCmd()

	testutil.AssertFlagExists(t, cmd, "file")
	testutil.AssertFlagShorthand(t, cmd, "file", "f")

	testutil.AssertFlagExists(t, cmd, "overwrite")
	testutil.AssertFlagDefault(t, cmd, "overwrite", "false")

	testutil.AssertFlagExists(t, cmd, "skip-existing")
	testutil.AssertFlagDefault(t, cmd, "skip-existing", "false")

	testutil.AssertFlagExists(t, cmd, "dry-run")
	testutil.AssertFlagDefault(t, cmd, "dry-run", "false")

	testutil.AssertFlagExists(t, cmd, "template")
	testutil.AssertFlagDefault(t, cmd, "template", "false")

	testutil.AssertFlagExists(t, cmd, "format")
	testutil.AssertFlagShorthand(t, cmd, "format", "o")
	testutil.AssertFlagDefault(t, cmd, "format", "yaml")
}

func TestNodeExportFlags(t *testing.T) {
	cmd := node.NewExportCmd()

	testutil.AssertFlagExists(t, cmd, "file")
	testutil.AssertFlagShorthand(t, cmd, "file", "f")

	testutil.AssertFlagExists(t, cmd, "format")
	testutil.AssertFlagShorthand(t, cmd, "format", "o")
	testutil.AssertFlagDefault(t, cmd, "format", "yaml")

	testutil.AssertFlagExists(t, cmd, "nodes")
	testutil.AssertFlagExists(t, cmd, "groups")
	testutil.AssertFlagExists(t, cmd, "labels")
}

func TestNodePingFlags(t *testing.T) {
	cmd := node.NewPingCmd()

	testutil.AssertFlagExists(t, cmd, "all")
	testutil.AssertFlagDefault(t, cmd, "all", "false")

	testutil.AssertFlagExists(t, cmd, "timeout")
	testutil.AssertFlagShorthand(t, cmd, "timeout", "t")
	testutil.AssertFlagDefault(t, cmd, "timeout", "3s")
}

func TestNodeCheckFlags(t *testing.T) {
	cmd := node.NewCheckCmd()

	testutil.AssertFlagExists(t, cmd, "all")
	testutil.AssertFlagDefault(t, cmd, "all", "false")

	testutil.AssertFlagExists(t, cmd, "timeout")
	testutil.AssertFlagShorthand(t, cmd, "timeout", "t")
	testutil.AssertFlagDefault(t, cmd, "timeout", "10s")

	testutil.AssertFlagExists(t, cmd, "workers")
	testutil.AssertFlagShorthand(t, cmd, "workers", "w")
	testutil.AssertFlagDefault(t, cmd, "workers", "5")
}

func TestNodeSampleCmd(t *testing.T) {
	testutil.AssertCommandExists(t, node.NewNodeCmd(), "sample")

	cmd := node.NewSampleCmd()
	if cmd.Use != "sample" {
		t.Errorf("expected Use 'sample', got '%s'", cmd.Use)
	}
}

func TestNodeHelpContainsModules(t *testing.T) {
	parent := node.NewNodeCmd()

	sections := []string{
		"list", "add", "update", "remove",
		"import", "export", "status", "groups",
		"labels", "sample", "ping", "check",
	}
	for _, section := range sections {
		testutil.AssertHelpContains(t, parent, section)
	}
}
