//go:build tui
// +build tui

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/tui"
)

// extraRootCommands 记录仅在带 tui 构建标签时才注册的子命令(测试断言用)。
var extraRootCommands = []string{"tui"}

// registerTUI 注册 owl tui 子命令(仅 -tags tui 构建时编译)。
// 纯净构建(无 tui 标签)时该文件不参与编译,registerTUI 由 tui_register_disabled.go 提供空实现,
// 且 tui 包因不被 import 而整体不链接,连带 bubbletea/bubbles/lipgloss 依赖一并裁掉。
func registerTUI(root *cobra.Command) {
	root.AddCommand(tui.NewTuiCmd())
}
