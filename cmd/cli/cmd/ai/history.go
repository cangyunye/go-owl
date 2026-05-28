package ai

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	common "github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	internalhistory "github.com/cangyunye/go-owl/internal/history"
)

var (
	aiHistoryLimit   int
	aiHistorySession string
	aiHistoryDays    int
)

func NewHistoryCmd() *cobra.Command {
	historyCmd := &cobra.Command{
		Use:   "history",
		Short: "AI 对话历史记录管理",
		Long:  `查询和管理 owl ai 的对话历史记录`,
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "列出最近的 AI 对话会话",
		Run:   runAIHistoryList,
	}
	listCmd.Flags().IntVar(&aiHistoryLimit, "limit", 20, "显示的最大记录数")
	listCmd.Flags().StringVar(&aiHistorySession, "session", "", "按会话 ID 过滤")

	showCmd := &cobra.Command{
		Use:   "show <session-id>",
		Short: "显示指定会话的完整对话链",
		Long:  `显示指定会话的完整对话链。使用 "owl ai history list" 查看可用的会话 ID。`,
		Args:  cobra.ExactArgs(1),
		Run:   runAIHistoryShow,
	}

	cleanCmd := &cobra.Command{
		Use:   "clean",
		Short: "清理过期的 AI 聊天记录",
		Run:   runAIHistoryClean,
	}
	cleanCmd.Flags().IntVar(&aiHistoryDays, "days", 30, "保留最近 N 天的记录")

	historyCmd.AddCommand(listCmd, showCmd, cleanCmd)
	return historyCmd
}

func runAIHistoryList(cmd *cobra.Command, args []string) {
	sessions, err := internalhistory.QueryAiChatSessionsGlobal(aiHistorySession, aiHistoryLimit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "查询失败: %v\n", err)
		return
	}

	if len(sessions) == 0 {
		fmt.Println("暂无 AI 对话历史记录")
		return
	}

	fmt.Printf("%s %s %s %s %s %s\n",
		common.PadRight("会话ID", 10), common.PadRight("时间", 22), common.PadRight("用户输入", 30),
		common.PadRight("工具", 18), common.PadRight("步骤数", 8), common.PadRight("耗时", 8))
	fmt.Println(strings.Repeat("-", 101))

	for _, s := range sessions {
		sid := s.SessionID
		if len(sid) > 8 {
			sid = sid[:8]
		}
		input := s.FirstInput
		if common.DisplayWidth(input) > 30 {
			input = common.TruncateByWidth(input, 27) + "..."
		}
		toolName := s.ToolName
		if toolName == "" {
			toolName = "-"
		}
		duration := fmt.Sprintf("%dms", s.DurationMs)
		if s.DurationMs > 1000 {
			duration = fmt.Sprintf("%.1fs", float64(s.DurationMs)/1000.0)
		}
		fmt.Printf("%s %s %s %s %s %s\n",
			common.PadRight(sid, 10), common.PadRight(s.StartTime, 22), common.PadRight(input, 30),
			common.PadRight(toolName, 18), common.PadRight(fmt.Sprintf("%d", s.StepCount), 8),
			common.PadRight(duration, 8))
	}
}

func runAIHistoryShow(cmd *cobra.Command, args []string) {
	sessionID := args[0]
	steps, err := internalhistory.QueryAiChatStepsGlobal(sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "查询失败: %v\n", err)
		return
	}

	if len(steps) == 0 {
		fmt.Printf("未找到会话 %s 的记录\n", sessionID)
		return
	}

	fmt.Printf("会话: %s\n", sessionID)
	fmt.Println("──────────────────────────────────────────")
	for _, s := range steps {
		roleIcon := map[string]string{
			"user":      "👤",
			"assistant": "🤖",
			"system":    "⚙️",
			"tool":      "🔧",
		}[s.Role]
		if roleIcon == "" {
			roleIcon = "  "
		}

		fmt.Printf("[%s] %s [%s] %s\n", s.CreatedAt, roleIcon, s.Step, s.Role)
		if s.Output != "" {
			fmt.Printf("  输出: %s\n", truncateStr(s.Output, 200))
		}
		if s.ToolCalls != "" {
			fmt.Printf("  工具调用: %s\n", truncateStr(s.ToolCalls, 200))
		}
		if s.ToolResults != "" {
			fmt.Printf("  结果: %s\n", truncateStr(s.ToolResults, 200))
		}
		if s.Error != "" {
			fmt.Printf("  ❌ 错误: %s\n", s.Error)
		}
		fmt.Printf("  耗时: %dms\n", s.DurationMs)
		fmt.Println("──────────────────────────────────────────")
	}
}

func runAIHistoryClean(cmd *cobra.Command, args []string) {
	count, err := internalhistory.CleanAiChatGlobal(aiHistoryDays)
	if err != nil {
		fmt.Fprintf(os.Stderr, "清理失败: %v\n", err)
		return
	}
	fmt.Printf("已清理 %d 条超过 %d 天的 AI 聊天记录\n", count, aiHistoryDays)
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
