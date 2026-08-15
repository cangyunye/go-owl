package tui

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/tui/theme"
	"github.com/cangyunye/go-owl/internal/i18n"
)

func NewTuiCmd() *cobra.Command {
	tuiCmd := &cobra.Command{
		Use:   "tui",
		Short: i18n.T("tui.cmd.short"),
		Long:  i18n.T("tui.cmd.long"),
		Run:   runTui,
	}

	return tuiCmd
}

func runTui(cmd *cobra.Command, args []string) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintln(os.Stderr, "错误: owl tui 需要在交互式终端中运行")
		os.Exit(1)
	}

	// 冲突交互提示会阻塞读路径(TTY 下逐个 Scanln),TUI 需要独占终端。
	// 关闭后冲突检测仅记录警告,数据仍以数据库主源为准,不阻塞 TUI 启动。
	store := common.GetNodeStore()
	if dbs, ok := store.(*common.NodeStoreDB); ok {
		dbs.SetConflictPrompt(false)
	}

	// 请求 truecolor 但终端不支持(如 conhost)时回退 ANSI 并提示
	if theme.DowngradedToANSI() {
		fmt.Fprintln(os.Stderr, "提示: OWL_TUI_THEME=truecolor 但当前终端不支持真彩色(conhost 无法渲染 24bit 色),已回退 ANSI 主题。")
		fmt.Fprintln(os.Stderr, "      请在 Windows Terminal / ConEmu 等支持真彩的终端中运行,或取消该环境变量。")
	}

	app := NewApp(store)
	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "错误: owl tui 异常:", err)
		os.Exit(1)
	}
}
