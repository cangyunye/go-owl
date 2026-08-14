//go:build !tui
// +build !tui

package cmd

import (
	"github.com/spf13/cobra"
)

// extraRootCommands 在纯净构建(无 tui 标签)下为空。
var extraRootCommands []string

// registerTUI 纯净构建下为空操作:owl tui 子命令不注册、tui 包不链接。
func registerTUI(root *cobra.Command) {}
