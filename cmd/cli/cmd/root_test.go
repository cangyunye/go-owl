package cmd_test

import (
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/cmd/cli/cmd"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/testutil"
)

func TestRootCmdExists(t *testing.T) {
	root := cmd.NewRootCmd()
	if root == nil {
		t.Fatal("expected NewRootCmd() to return non-nil command")
	}
	if root.Use != "owl" {
		t.Errorf("expected Use 'owl', got '%s'", root.Use)
	}
}

func TestRootCmdVersion(t *testing.T) {
	root := cmd.NewRootCmd()

	verFlag := root.Version
	if verFlag == "" {
		t.Error("expected version to be set")
	}

	output := testutil.ExecuteCommand(t, root, "--version")
	if !strings.Contains(output, "owl version") {
		t.Errorf("expected version output to contain 'owl version', got: %s", output)
	}
}

func TestAllSubCommands(t *testing.T) {
	root := cmd.NewRootCmd()

	expected := []string{
		"node",
		"exec",
		"file",
		"playbook",
		"session",
		"ai",
		"history",
		"settings",
		"async",
		"tui",
	}

	testutil.AssertSubCommands(t, root, expected)
}

func TestRootCmdHelp(t *testing.T) {
	root := cmd.NewRootCmd()

	testutil.AssertHelpContains(t, root, "智能 Linux 分布式运维工具")

	helpSections := []string{
		"节点管理",
		"批量命令执行",
		"脚本传输执行",
		"剧本执行",
		"文件传输",
		"AI 助手",
	}

	for _, section := range helpSections {
		testutil.AssertHelpContains(t, root, section)
	}
}

func TestRootCmdShort(t *testing.T) {
	root := cmd.NewRootCmd()

	if root.Short != "owl - 智能分布式运维工具" {
		t.Errorf("expected Short 'owl - 智能分布式运维工具', got '%s'", root.Short)
	}
}

func TestRootCmdHasSubCommands(t *testing.T) {
	root := cmd.NewRootCmd()
	subs := root.Commands()

	if len(subs) < 9 {
		t.Errorf("expected at least 9 subcommands, got %d", len(subs))
	}
}
