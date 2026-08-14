package tui_test

import (
	"testing"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/testutil"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/tui"
)

func TestTuiCmdExists(t *testing.T) {
	cmd := tui.NewTuiCmd()

	if cmd.Use != "tui" {
		t.Errorf("expected Use 'tui', got '%s'", cmd.Use)
	}
	if cmd.Short != "启动交互式终端用户界面" {
		t.Errorf("expected Short '启动交互式终端用户界面', got '%s'", cmd.Short)
	}
}

func TestTuiHasNoSubCommands(t *testing.T) {
	cmd := tui.NewTuiCmd()

	if len(cmd.Commands()) != 0 {
		t.Errorf("expected tui to have no subcommands, got %d", len(cmd.Commands()))
	}
}

func TestTuiHelpContainsSections(t *testing.T) {
	cmd := tui.NewTuiCmd()

	sections := []string{
		"tui",
		"TUI",
		"节点",
		"命令",
		"文件",
		"剧本",
	}
	for _, section := range sections {
		testutil.AssertHelpContains(t, cmd, section)
	}
}

func TestTuiHasNoFlags(t *testing.T) {
	cmd := tui.NewTuiCmd()

	if cmd.Flags().HasFlags() {
		t.Error("expected tui to have no flags")
	}
}
