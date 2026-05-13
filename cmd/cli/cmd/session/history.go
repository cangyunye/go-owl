package session

import (
	"fmt"
	"strings"
	"time"

	"github.com/cangyunye/go-owl/internal/history"
	"github.com/spf13/cobra"
)

var _ = time.Now()

var (
	historySessionID string
	historyNode     string
	historyLast     string
	historyVerbose  bool
	historyLimit    int
)

func NewHistoryCmd() *cobra.Command {
	historyCmd := &cobra.Command{
		Use:   "history [session-id]",
		Short: "查看会话历史",
		Long:  `查看会话命令历史记录，支持按节点和时间筛选`,
		Args:  cobra.MaximumNArgs(1),
		RunE:  runHistory,
	}

	historyCmd.Flags().StringVar(&historySessionID, "session-id", "",
		"指定会话 ID")
	historyCmd.Flags().StringVar(&historyNode, "node", "",
		"按节点筛选")
	historyCmd.Flags().StringVar(&historyLast, "last", "",
		"查看最近时间（如: 1h, 30m, 1d）")
	historyCmd.Flags().BoolVarP(&historyVerbose, "verbose", "v", false,
		"显示详细输出")
	historyCmd.Flags().IntVarP(&historyLimit, "limit", "n", 20,
		"显示最近 N 条记录")

	return historyCmd
}

func runHistory(cmd *cobra.Command, args []string) error {
	sessionID := historySessionID
	if sessionID == "" && len(args) > 0 {
		sessionID = args[0]
	}

	if history.GetGlobalDB() == nil {
		fmt.Println("历史数据库未初始化")
		return nil
	}

	if sessionID != "" {
		displaySessionHistory(sessionID)
	} else {
		displayRecentHistory()
	}

	return nil
}

func displaySessionHistory(sessionID string) {
	session, err := history.GetSession(sessionID)
	if err != nil || session == nil {
		fmt.Printf("会话 %s 未找到\n", sessionID)
		return
	}

	fmt.Println("─────────────────────────────────────")
	fmt.Printf("会话 ID:    %s\n", session.ID)
	fmt.Printf("模式:      %s\n", session.Mode)
	fmt.Printf("状态:      %s\n", session.Status)
	fmt.Printf("创建时间:  %s\n", session.CreatedAt.Format("2006-01-02 15:04:05"))
	if session.ClosedAt != nil {
		fmt.Printf("关闭时间:  %s\n", session.ClosedAt.Format("2006-01-02 15:04:05"))
	}
	fmt.Printf("节点:      %s\n", strings.Join(session.NodeIDs, ", "))
	fmt.Println("─────────────────────────────────────")
	fmt.Printf("命令数:    %d\n", session.CommandCount)
	fmt.Printf("成功:      %d\n", session.SuccessCount)
	fmt.Printf("失败:      %d\n", session.ErrorCount)
	fmt.Println("─────────────────────────────────────")

	commands, err := history.QuerySessionCommands(sessionID, "", 0, 100)
	if err != nil || len(commands) == 0 {
		fmt.Println("\n暂无命令历史")
		return
	}

	fmt.Println("\n命令历史:")
	fmt.Println(strings.Repeat("─", 80))
	for i, c := range commands {
		status := "✓"
		if c.ExitCode != 0 {
			status = "✗"
		}
		fmt.Printf("[%d] %s %s %s\n", i+1, status, c.ExecutedAt.Format("15:04:05"), c.Command)
	}
	fmt.Println()
}

func displayRecentHistory() {
	fmt.Println("最近的会话:")
	fmt.Println(strings.Repeat("─", 80))

	sessions, err := history.QuerySessions(historyLimit)
	if err != nil || len(sessions) == 0 {
		fmt.Println("暂无会话历史")
		return
	}

	for _, s := range sessions {
		statusIcon := "●"
		switch s.Status {
		case "active":
			statusIcon = "●"
		case "closed":
			statusIcon = "○"
		case "timeout":
			statusIcon = "◌"
		}

		successRate := "100%"
		if s.CommandCount > 0 {
			successRate = fmt.Sprintf("%.0f%%", float64(s.SuccessCount)/float64(s.CommandCount)*100)
		}

		fmt.Printf("%s %s | %s | %s | %s | %d cmd\n",
			statusIcon,
			s.ID,
			s.CreatedAt.Format("01-02 15:04"),
			strings.Join(s.NodeIDs, ","),
			successRate,
			s.CommandCount,
		)
	}

	fmt.Println()
	fmt.Printf("查看详情: owl session history --session-id <id>\n")
}
