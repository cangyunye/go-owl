// Package cmd CLI 命令行工具入口
package cmd

import (
	"fmt"
	"os"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/ai"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/async"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/exec"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/file"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/history"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/metrics"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/node"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/playbook"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/serve"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/session"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/settings"
	internalhistory "github.com/cangyunye/go-owl/internal/history"
	"github.com/cangyunye/go-owl/internal/i18n"
	"github.com/cangyunye/go-owl/internal/logger"

	"github.com/spf13/cobra"
)

var (
	version   = "1.1.0"
	commitID  = "dev"
	buildTime = "unknown"
)

// Execute 执行根命令
func Execute() error {
	db, err := internalhistory.NewDB(internalhistory.DefaultConfig())
	if db == nil || err != nil {
		logger.Warn("failed to initialize database, falling back to in-memory node store", logger.WithError(err))
	} else {
		common.MigrateNodesJSONToDB(db.Connection())
		common.InitNodeStoreFromDB(db.Connection())
	}

	rootCmd := NewRootCmd()
	return rootCmd.Execute()
}

// NewRootCmd 创建根命令
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "owl",
		Short: i18n.T("root.short"),
		Long:  i18n.T("root.long_help"),

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
	rootCmd.AddCommand(serve.NewServeCmd())
	rootCmd.AddCommand(metrics.NewMetricsCmd())
	registerTUI(rootCmd)

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
		fmt.Fprintf(os.Stderr, "%s\n", i18n.T("root.error_prefix", fmt.Sprintf("%s: %v", msg, err)))
	} else {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.T("root.error_prefix", msg))
	}
	os.Exit(1)
}
