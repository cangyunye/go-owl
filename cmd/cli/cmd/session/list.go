package session

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/cangyunye/go-owl/internal/history"
	"github.com/spf13/cobra"
)

func NewListCmd() *cobra.Command {
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "列出历史会话记录",
		Long:  `列出所有已记录的历史会话，用于查看过去的批量操作审计记录`,
		RunE:  runList,
	}

	return listCmd
}

func runList(cmd *cobra.Command, args []string) error {
	if history.GetGlobalDB() == nil {
		fmt.Println("历史数据库未初始化")
		return nil
	}

	sessions, err := history.QuerySessions(100)
	if err != nil {
		return fmt.Errorf("查询会话记录失败: %w", err)
	}

	printSessionList(cmd.OutOrStdout(), sessions)
	return nil
}

func printSessionList(w io.Writer, sessions []*history.Session) {
	if len(sessions) == 0 {
		fmt.Fprintln(w, "暂无历史会话记录")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "使用 'owl session attach <node-id>' 创建新会话")
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.AlignRight|tabwriter.Debug)
	defer tw.Flush()

	fmt.Fprintln(tw, "会话 ID\t模式\t节点\t状态\t创建时间\t命令数\t成功率")
	fmt.Fprintln(tw, "────────\t────────\t────────\t────────\t────────\t────────\t────────")

	for _, s := range sessions {
		statusDisplay := s.Status
		switch s.Status {
		case "active":
			statusDisplay = "● active"
		case "closed":
			statusDisplay = "○ closed"
		case "timeout":
			statusDisplay = "◌ timeout"
		}

		successRate := "N/A"
		if s.CommandCount > 0 {
			successRate = fmt.Sprintf("%.0f%%", float64(s.SuccessCount)/float64(s.CommandCount)*100)
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\t%s\n",
			s.ID,
			s.Mode,
			strings.Join(s.NodeIDs, ","),
			statusDisplay,
			s.CreatedAt.Format("2006-01-02 15:04"),
			s.CommandCount,
			successRate,
		)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "使用 'owl session history --session-id <id>' 查看会话详情")
}
