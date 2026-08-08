package playbook

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/internal/history"
	"github.com/cangyunye/go-owl/internal/i18n"
	"github.com/cangyunye/go-owl/internal/logger"
)

var (
	pbStatePlaybook string
	pbStateStatus   string
	pbStateNode     string
	pbStateLimit    int
)

func NewPlaybookStateCmd() *cobra.Command {
	stateCmd := &cobra.Command{
		Use:   "state",
		Short: i18n.T("playbook.state.cmd.short"),
		Long:  i18n.T("playbook.state.cmd.long"),
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: i18n.T("playbook.state.list.short"),
		Run:   runStateList,
	}
	listCmd.Flags().StringVar(&pbStatePlaybook, "playbook", "", i18n.T("playbook.state.list.flag_playbook"))
	listCmd.Flags().StringVar(&pbStateStatus, "status", "", i18n.T("playbook.state.list.flag_status"))
	listCmd.Flags().IntVar(&pbStateLimit, "limit", 20, i18n.T("playbook.state.list.flag_limit"))

	showCmd := &cobra.Command{
		Use:   "show <run-id>",
		Short: i18n.T("playbook.state.show.short"),
		Args:  cobra.ExactArgs(1),
		Run:   runStateShow,
	}
	showCmd.Flags().StringVar(&pbStateNode, "node", "", i18n.T("playbook.state.show.flag_node"))
	showCmd.Flags().StringVar(&pbStateStatus, "status", "", i18n.T("playbook.state.show.flag_status"))

	stateCmd.AddCommand(listCmd)
	stateCmd.AddCommand(showCmd)

	return stateCmd
}

func runStateList(cmd *cobra.Command, args []string) {
	logger.Init(nil)
	defer logger.Sync()
	if _, err := history.NewDB(history.DefaultConfig()); err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.state.err_db", err))
		os.Exit(1)
	}

	runs, err := history.ListPlaybookRuns(pbStatePlaybook, pbStateStatus, pbStateLimit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.state.err_list", err))
		os.Exit(1)
	}

	if len(runs) == 0 {
		fmt.Println(i18n.T("playbook.state.no_records"))
		return
	}

	fmt.Println(i18n.T("playbook.state.list_title"))
	fmt.Printf("%-14s  %-20s  %-5s  %-12s  %-10s  %-16s\n",
		i18n.T("playbook.state.h_run_id"), i18n.T("playbook.state.h_playbook"),
		i18n.T("playbook.state.h_node"), i18n.T("playbook.state.h_progress"),
		i18n.T("playbook.state.h_status"), i18n.T("playbook.state.h_started"))
	fmt.Println(strings.Repeat("─", 85))

	for _, run := range runs {
		shortID := run.ID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}
		progress := fmt.Sprintf("%d/%d", run.CompletedSteps, run.TotalSteps)
		if run.TotalSteps > 0 {
			pct := run.CompletedSteps * 100 / run.TotalSteps
			progress += fmt.Sprintf(" (%d%%)", pct)
		}
		fmt.Printf("%-14s  %-20s  %-5d  %-12s  %-10s  %-16s\n",
			shortID,
			truncateStr(run.PlaybookName, 20),
			len(run.Nodes),
			progress,
			run.Status,
			run.StartedAt.Format("01-02 15:04"))
	}
}

func runStateShow(cmd *cobra.Command, args []string) {
	logger.Init(nil)
	defer logger.Sync()
	if _, err := history.NewDB(history.DefaultConfig()); err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.state.err_db", err))
		os.Exit(1)
	}

	runID := args[0]

	run, err := history.GetPlaybookRun(runID)
	if err != nil || run == nil {
		if len(runID) < 36 {
			runs, listErr := history.ListPlaybookRuns("", "", 100)
			if listErr == nil {
				for _, r := range runs {
					if strings.HasPrefix(r.ID, runID) {
						run = r
						runID = r.ID
						break
					}
				}
			}
		}
		if run == nil {
			fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.state.err_not_found", runID))
			os.Exit(1)
		}
	}

	fmt.Printf("%s", i18n.T("playbook.state.run_line", shortRunID(run.ID), run.PlaybookName, run.Status))
	fmt.Println(strings.Repeat("─", 60))

	steps, err := history.GetStepStates(runID, pbStateNode, pbStateStatus)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.state.err_steps", err))
		os.Exit(1)
	}

	if len(steps) == 0 {
		fmt.Println(i18n.T("playbook.state.no_steps"))
		return
	}

	currentNode := ""
	for _, step := range steps {
		if step.NodeID != currentNode {
			if currentNode != "" {
				fmt.Println()
			}
			fmt.Printf("%s", i18n.T("playbook.state.node_header", step.NodeID))
			currentNode = step.NodeID
		}

		icon := stepIcon(step.Status)
		line := i18n.T("playbook.state.step_line", icon, i18n.F(step.StepIndex+1), step.StepName, step.Status)
		if step.ExitCode != 0 {
			line += fmt.Sprintf("  exit=%d", step.ExitCode)
		}
		if step.Error != "" {
			line += fmt.Sprintf("  error=%q", truncateStr(step.Error, 60))
		}
		if step.DurationMs > 0 {
			line += fmt.Sprintf("  (%.1fs)", float64(step.DurationMs)/1000.0)
		}
		fmt.Println(line)
	}

	if run.Status == "failed" {
		fmt.Println(strings.Repeat("─", 60))
		fmt.Printf("%s", i18n.T("playbook.state.resume_hint"))
	}
}

func stepIcon(status string) string {
	switch status {
	case "completed":
		return "✓"
	case "failed":
		return "✗"
	case "running":
		return "▶"
	case "skipped":
		return "⏭"
	default:
		return "○"
	}
}

func shortRunID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
