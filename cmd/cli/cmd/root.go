// Package cmd CLI 命令行工具入口
package cmd

import (
	"fmt"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/ai"
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
	version   = "1.4.0"
	commitID  = "dev"
	buildTime = "unknown"

	// historyDB 由 PersistentPreRun 打开,Execute 退出前关闭;
	// --help/--version 等纯展示路径不触发生命周期,不再无谓开库。
	historyDB internalhistory.DBInterface
)

// Execute 执行根命令
func Execute() error {
	rootCmd := NewRootCmd()
	err := rootCmd.Execute()
	if historyDB != nil {
		if cerr := historyDB.Close(); cerr != nil {
			logger.Warn("failed to close history database", logger.WithError(cerr))
		}
		historyDB = nil
	}
	return err
}

// openHistoryDB 打开历史库并把节点存储升级为 DB-backed。
// 失败时保留 common 包 init() 提供的内存存储并告警。
func openHistoryDB() {
	db, err := internalhistory.NewDB(internalhistory.DefaultConfig())
	if db == nil || err != nil {
		logger.Warn("failed to initialize database, falling back to in-memory node store", logger.WithError(err))
		return
	}
	historyDB = db
	common.MigrateNodesJSONToDB(db.Connection())
	common.InitNodeStoreFromDB(db.Connection())
}

// NewRootCmd 创建根命令
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "owl",
		Short: i18n.T("root.short"),
		Long:  i18n.T("root.long_help"),

		Version: version,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			openHistoryDB()
			return nil
		},
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
