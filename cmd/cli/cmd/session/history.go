package session

import (
	"fmt"
	"strings"
	"time"

	"github.com/cangyunye/go-owl/internal/history"
	"github.com/cangyunye/go-owl/internal/i18n"
	"github.com/spf13/cobra"
)

var _ = time.Now()

var (
	historySessionID string
	historyNode      string
	historyLast      string
	historyVerbose   bool
	historyLimit     int
)

func NewHistoryCmd() *cobra.Command {
	historyCmd := &cobra.Command{
		Use:   "history [session-id]",
		Short: i18n.T("session.history.short"),
		Long:  i18n.T("session.history.long"),
		Args:  cobra.MaximumNArgs(1),
		RunE:  runHistory,
	}

	historyCmd.Flags().StringVar(&historySessionID, "session-id", "",
		i18n.T("session.history.flag_session_id"))
	historyCmd.Flags().StringVar(&historyNode, "node", "",
		i18n.T("session.history.flag_node"))
	historyCmd.Flags().StringVar(&historyLast, "last", "",
		i18n.T("session.history.flag_last"))
	historyCmd.Flags().BoolVarP(&historyVerbose, "verbose", "v", false,
		i18n.T("session.history.flag_verbose"))
	historyCmd.Flags().IntVarP(&historyLimit, "limit", "n", 20,
		i18n.T("session.history.flag_limit"))

	return historyCmd
}

func runHistory(cmd *cobra.Command, args []string) error {
	sessionID := historySessionID
	if sessionID == "" && len(args) > 0 {
		sessionID = args[0]
	}

	if history.GetGlobalDB() == nil {
		fmt.Println(i18n.T("session.db_not_initialized"))
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
		fmt.Printf("%s", i18n.T("session.history.not_found", sessionID))
		return
	}

	fmt.Println("─────────────────────────────────────")
	fmt.Printf("%s", i18n.T("session.history.label_id", session.ID))
	fmt.Printf("%s", i18n.T("session.history.label_mode", session.Mode))
	fmt.Printf("%s", i18n.T("session.history.label_status", session.Status))
	fmt.Printf("%s", i18n.T("session.history.label_created", session.CreatedAt.Format("2006-01-02 15:04:05")))
	if session.ClosedAt != nil {
		fmt.Printf("%s", i18n.T("session.history.label_closed", session.ClosedAt.Format("2006-01-02 15:04:05")))
	}
	fmt.Printf("%s", i18n.T("session.history.label_nodes", strings.Join(session.NodeIDs, ", ")))
	fmt.Println("─────────────────────────────────────")
	fmt.Printf("%s", i18n.T("session.history.label_commands", i18n.F(session.CommandCount)))
	fmt.Printf("%s", i18n.T("session.history.label_success", i18n.F(session.SuccessCount)))
	fmt.Printf("%s", i18n.T("session.history.label_failed", i18n.F(session.ErrorCount)))
	fmt.Println("─────────────────────────────────────")

	commands, err := history.QuerySessionCommands(sessionID, "", 0, 100)
	if err != nil || len(commands) == 0 {
		fmt.Println(i18n.T("session.history.no_commands"))
		return
	}

	fmt.Println(i18n.T("session.history.commands_title"))
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
	fmt.Println(i18n.T("session.history.recent_title"))
	fmt.Println(strings.Repeat("─", 80))

	sessions, err := history.QuerySessions(historyLimit)
	if err != nil || len(sessions) == 0 {
		fmt.Println(i18n.T("session.history.no_sessions"))
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
	fmt.Printf("%s", i18n.T("session.history.detail_hint"))
}
