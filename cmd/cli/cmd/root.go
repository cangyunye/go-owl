// Package cmd CLI 命令行工具入口
package cmd

import (
	"fmt"
	"os"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/ai"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/async"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/exec"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/file"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/history"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/node"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/playbook"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/session"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/settings"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/tui"
	internalhistory "github.com/cangyunye/go-owl/internal/history"

	"github.com/spf13/cobra"
)

var (
	version   = "1.0.0"
	commitID  = "dev"
	buildTime = "unknown"
)

// Execute 执行根命令
func Execute() error {
	// 初始化历史记录数据库
	internalhistory.NewDB(internalhistory.DefaultConfig())

	rootCmd := NewRootCmd()
	return rootCmd.Execute()
}

// NewRootCmd 创建根命令
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "owl",
		Short: "owl - 智能分布式运维工具",
		Long: `owl 是一个智能 Linux 分布式运维工具，支持：

- 节点管理：节点注册、分组、标签管理
- 批量命令执行：支持按节点、分组、标签选择目标
- 脚本传输执行：批量传输并执行 Shell 脚本
- 剧本执行：Ansible-like YAML 剧本流程执行
- 文件传输：支持自扩散传输（P2P 模式）
- AI 助手：通过自然语言执行运维操作

完整文档：https://github.com/cangyunye/go-owl`,
		Version: version,
	}

	// 添加子命令
	rootCmd.AddCommand(node.NewNodeCmd())
	rootCmd.AddCommand(exec.NewExecCmd())
	rootCmd.AddCommand(file.NewFileCmd())
	rootCmd.AddCommand(playbook.NewPlaybookCmd())
	rootCmd.AddCommand(settings.NewSettingsCmd())
	rootCmd.AddCommand(ai.NewAICmd())
	rootCmd.AddCommand(history.NewHistoryCmd())
	rootCmd.AddCommand(session.NewCmd())
	rootCmd.AddCommand(async.NewAsyncCmd())
	rootCmd.AddCommand(tui.NewTuiCmd())

	// 添加版本信息
	rootCmd.SetVersionTemplate(fmt.Sprintf(`owl version: %s
build: %s
commit: %s
`, version, buildTime, commitID))

	return rootCmd
}

// exitWithError 退出并显示错误
func exitWithError(msg string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s: %v\n", msg, err)
	} else {
		fmt.Fprintf(os.Stderr, "Error: %s\n", msg)
	}
	os.Exit(1)
}
