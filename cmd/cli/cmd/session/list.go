package session

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/cangyunye/go-owl/internal/history"
	"github.com/cangyunye/go-owl/internal/i18n"
	"github.com/spf13/cobra"
)

func NewListCmd() *cobra.Command {
	listCmd := &cobra.Command{
		Use:   "list",
		Short: i18n.T("session.list.short"),
		Long:  i18n.T("session.list.long"),
		RunE:  runList,
	}

	return listCmd
}

func runList(cmd *cobra.Command, args []string) error {
	if history.GetGlobalDB() == nil {
		fmt.Println(i18n.T("session.db_not_initialized"))
		return nil
	}

	sessions, err := history.QuerySessions(100)
	if err != nil {
		return fmt.Errorf(i18n.Raw("session.list.err_query"), err)
	}

	printSessionList(cmd.OutOrStdout(), sessions)
	return nil
}

func printSessionList(w io.Writer, sessions []*history.Session) {
	if len(sessions) == 0 {
		fmt.Fprintln(w, i18n.T("session.list.empty"))
		fmt.Fprintln(w)
		fmt.Fprintln(w, i18n.T("session.list.empty_hint"))
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.AlignRight|tabwriter.Debug)
	defer tw.Flush()

	fmt.Fprintln(tw, i18n.T("session.list.header"))
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
	fmt.Fprintln(w, i18n.T("session.list.detail_hint"))
}
