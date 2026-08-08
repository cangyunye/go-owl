package ai

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	common "github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	internalhistory "github.com/cangyunye/go-owl/internal/history"
	"github.com/cangyunye/go-owl/internal/i18n"
)

var (
	aiHistoryLimit   int
	aiHistorySession string
	aiHistoryDays    int
)

func NewHistoryCmd() *cobra.Command {
	historyCmd := &cobra.Command{
		Use:   "history",
		Short: i18n.T("ai.history.short"),
		Long:  i18n.T("ai.history.long"),
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: i18n.T("ai.history.list.short"),
		Run:   runAIHistoryList,
	}
	listCmd.Flags().IntVar(&aiHistoryLimit, "limit", 20, i18n.T("ai.history.list.flag_limit"))
	listCmd.Flags().StringVar(&aiHistorySession, "session", "", i18n.T("ai.history.list.flag_session"))

	showCmd := &cobra.Command{
		Use:   "show <session-id>",
		Short: i18n.T("ai.history.show.short"),
		Long:  i18n.T("ai.history.show.long"),
		Args:  cobra.ExactArgs(1),
		Run:   runAIHistoryShow,
	}

	cleanCmd := &cobra.Command{
		Use:   "clean",
		Short: i18n.T("ai.history.clean.short"),
		Run:   runAIHistoryClean,
	}
	cleanCmd.Flags().IntVar(&aiHistoryDays, "days", 30, i18n.T("ai.history.clean.flag_days"))

	historyCmd.AddCommand(listCmd, showCmd, cleanCmd)
	return historyCmd
}

func runAIHistoryList(cmd *cobra.Command, args []string) {
	sessions, err := internalhistory.QueryAiChatSessionsGlobal(aiHistorySession, aiHistoryLimit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("ai.history.err_query", err))
		return
	}

	if len(sessions) == 0 {
		fmt.Println(i18n.T("ai.history.empty"))
		return
	}

	fmt.Printf("%s %s %s %s %s %s\n",
		common.PadRight(i18n.T("ai.history.col_session"), 10), common.PadRight(i18n.T("ai.history.col_time"), 22), common.PadRight(i18n.T("ai.history.col_input"), 30),
		common.PadRight(i18n.T("ai.history.col_tool"), 18), common.PadRight(i18n.T("ai.history.col_steps"), 8), common.PadRight(i18n.T("ai.history.col_duration"), 8))
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
		fmt.Fprintf(os.Stderr, "%s", i18n.T("ai.history.err_query", err))
		return
	}

	if len(steps) == 0 {
		fmt.Printf("%s", i18n.T("ai.history.show.not_found", sessionID))
		return
	}

	fmt.Printf("%s", i18n.T("ai.history.show.session", sessionID))
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
			fmt.Printf("%s", i18n.T("ai.history.show.output", truncateStr(s.Output, 200)))
		}
		if s.ToolCalls != "" {
			fmt.Printf("%s", i18n.T("ai.history.show.tool_calls", truncateStr(s.ToolCalls, 200)))
		}
		if s.ToolResults != "" {
			fmt.Printf("%s", i18n.T("ai.history.show.result", truncateStr(s.ToolResults, 200)))
		}
		if s.Error != "" {
			fmt.Printf("%s", i18n.T("ai.history.show.error", s.Error))
		}
		fmt.Printf("%s", i18n.T("ai.history.show.duration", i18n.F(s.DurationMs)))
		fmt.Println("──────────────────────────────────────────")
	}
}

func runAIHistoryClean(cmd *cobra.Command, args []string) {
	count, err := internalhistory.CleanAiChatGlobal(aiHistoryDays)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("ai.history.err_clean", err))
		return
	}
	fmt.Printf("%s", i18n.T("ai.history.clean.done", i18n.F(count), i18n.F(aiHistoryDays)))
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
