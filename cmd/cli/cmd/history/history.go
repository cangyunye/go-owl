package history

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/cangyunye/go-owl/internal/history"
	"github.com/cangyunye/go-owl/internal/i18n"
	"github.com/cangyunye/go-owl/internal/logger"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	taskID       string
	nodeID       string
	opType       string
	status       string
	lastDuration string
	startTimeStr string
	endTimeStr   string
	limit        int
	offset       int
	format       string
	outputFile   string
	verbose      bool
)

func NewHistoryCmd() *cobra.Command {
	historyCmd := &cobra.Command{
		Use:   "history",
		Short: i18n.T("history.cmd.short"),
		Long:  i18n.T("history.cmd.long"),
		Run:   runHistory,
	}

	historyCmd.Flags().StringVar(&taskID, "task-id", "", i18n.T("history.flag_task_id"))
	historyCmd.Flags().StringVar(&nodeID, "node-id", "", i18n.T("history.flag_node_id"))
	historyCmd.Flags().StringVar(&opType, "op-type", "", i18n.T("history.flag_op_type"))
	historyCmd.Flags().StringVar(&status, "status", "", i18n.T("history.flag_status"))
	historyCmd.Flags().StringVar(&startTimeStr, "start-time", "", i18n.T("history.flag_start_time"))
	historyCmd.Flags().StringVar(&endTimeStr, "end-time", "", i18n.T("history.flag_end_time"))
	historyCmd.Flags().StringVar(&lastDuration, "last", "", i18n.T("history.flag_last"))
	historyCmd.Flags().IntVar(&limit, "limit", 50, i18n.T("history.flag_limit"))
	historyCmd.Flags().IntVar(&offset, "offset", 0, i18n.T("history.flag_offset"))

	historyCmd.Flags().StringVar(&format, "format", "table", i18n.T("history.flag_format"))
	historyCmd.Flags().StringVar(&outputFile, "output", "", i18n.T("history.flag_output"))
	historyCmd.Flags().BoolVar(&verbose, "verbose", false, i18n.T("history.flag_verbose"))

	historyCmd.AddCommand(NewCleanCmd())

	return historyCmd
}

func NewCleanCmd() *cobra.Command {
	var retentionDays int
	var force bool

	cleanCmd := &cobra.Command{
		Use:   "clean",
		Short: i18n.T("history.clean.short"),
		Long:  i18n.T("history.clean.long"),
		Run: func(cmd *cobra.Command, args []string) {
			logger.Init(nil)
			_, err := history.NewDB(history.DefaultConfig())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to initialize history DB: %v\n", err)
				os.Exit(1)
			}
			defer logger.Sync()

			if retentionDays <= 0 {
				fmt.Fprintf(os.Stderr, "Retention days must be greater than 0\n")
				os.Exit(1)
			}

			if !force {
				fmt.Printf("This will delete all history records older than %d days.\n", retentionDays)
				fmt.Print("Are you sure? (y/N): ")
				var confirm string
				fmt.Scanln(&confirm)
				if confirm != "y" && confirm != "Y" {
					fmt.Println("Operation cancelled.")
					return
				}
			}

			fmt.Printf("Cleaning up history older than %d days...\n", retentionDays)
			err = history.Cleanup(retentionDays)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Cleanup failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Cleanup completed successfully!")
		},
	}

	cleanCmd.Flags().IntVar(&retentionDays, "days", 30, i18n.T("history.clean.flag_days"))
	cleanCmd.Flags().BoolVar(&force, "force", false, i18n.T("history.clean.flag_force"))

	return cleanCmd
}

func runHistory(cmd *cobra.Command, args []string) {
	logger.Init(nil)
	_, err := history.NewDB(history.DefaultConfig())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize history DB: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	opts := &history.QueryOptions{
		TaskID: taskID,
		NodeID: nodeID,
		OpType: opType,
		Status: status,
		Limit:  limit,
		Offset: offset,
	}

	last, err := parseDuration(lastDuration)
	if err == nil && last > 0 {
		opts.StartTime = time.Now().Add(-last)
	}

	if startTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
			opts.StartTime = t
		}
	}
	if endTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
			opts.EndTime = t
		}
	}

	records, err := history.Query(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Query failed: %v\n", err)
		os.Exit(1)
	}

	w := cmd.OutOrStdout()
	if outputFile != "" {
		f, err := os.Create(outputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create output file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		w = f
	}

	switch format {
	case "json":
		json.NewEncoder(w).Encode(records)
	case "yaml":
		encoder := yaml.NewEncoder(w)
		encoder.SetIndent(2)
		encoder.Encode(records)
	case "table":
		printTable(w, records)
	default:
		printTable(w, records)
	}
}

func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	var dur time.Duration
	var err error
	suffix := s[len(s)-1]
	switch suffix {
	case 'h', 'H':
		hours, err := strconv.ParseFloat(s[:len(s)-1], 64)
		if err == nil {
			dur = time.Duration(hours) * time.Hour
		}
	case 'd', 'D':
		days, err := strconv.ParseFloat(s[:len(s)-1], 64)
		if err == nil {
			dur = time.Duration(days) * 24 * time.Hour
		}
	default:
		dur, err = time.ParseDuration(s)
	}
	return dur, err
}

func printTable(w io.Writer, records []*history.Record) {
	writer := tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.FilterHTML)
	defer writer.Flush()

	fmt.Fprintln(writer, "TIME\tTASK ID\tOP TYPE\tCOMMAND\tTARGETS\tSTATUS")
	fmt.Fprintln(writer, "-----\t-------\t--------\t-------\t---\t------")
	for _, r := range records {
		if r.Operation != nil {
			op := r.Operation
			targets := "[" + strings.Join(op.Targets, ",") + "]"
			cmdDisplay := op.Command
			if len(cmdDisplay) > 60 {
				cmdDisplay = cmdDisplay[:57] + "..."
			}
			fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n",
				op.CreatedAt.Format("2006-01-02 15:04:05"),
				op.TaskID,
				op.OpType,
				cmdDisplay,
				targets,
				op.Status,
			)

			if verbose {
				printVerboseDetails(writer, r)
			}
		}
	}
}

func printVerboseDetails(w io.Writer, record *history.Record) {
	op := record.Operation
	if op == nil {
		return
	}

	writer := tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.FilterHTML)

	if len(record.CommandExecutions) > 0 {
		fmt.Fprintln(writer, "  ── Command Executions ──")
		fmt.Fprintln(writer, "  NODE\tEXIT CODE\tDURATION\tSTATUS\tCOMMAND")
		for _, exec := range record.CommandExecutions {
			status := "✅"
			if !exec.Success {
				status = "❌"
			}
			cmdDisplay := exec.Command
			if len(cmdDisplay) > 40 {
				cmdDisplay = cmdDisplay[:37] + "..."
			}
			fmt.Fprintf(writer, "  %s\t%d\t%dms\t%s\t%s\n",
				exec.NodeID, exec.ExitCode, exec.DurationMs, status, cmdDisplay)
		}
		writer.Flush()
	}

	if len(record.Transfers) > 0 {
		fmt.Fprintln(writer, "  ── File Transfers ──")
		fmt.Fprintln(writer, "  NODE\tFILE\tSIZE\tMETHOD\tSTATUS")
		for _, tf := range record.Transfers {
			status := "✅"
			if tf.Status == "failed" {
				status = "❌"
			} else if tf.Status == "partial_failure" {
				status = "⚠️"
			}
			sizeDisplay := formatFileSize(tf.FileSize)
			fmt.Fprintf(writer, "  %s\t%s\t%s\t%s\t%s\n",
				tf.NodeID, tf.FileName, sizeDisplay, tf.TransferType, status)
		}
		writer.Flush()
	}

	if len(record.Communications) > 0 {
		fmt.Fprintln(writer, "  ── Node Communications ──")
		fmt.Fprintln(writer, "  NODE\tDIRECTION\tTYPE\tSTATUS")
		for _, comm := range record.Communications {
			status := "✅"
			if !comm.Success {
				status = "❌"
			}
			fmt.Fprintf(writer, "  %s\t%s\t%s\t%s\n",
				comm.NodeID, comm.Direction, comm.MessageType, status)
		}
		writer.Flush()
	}
}

func formatFileSize(size int64) string {
	if size <= 0 {
		return "N/A"
	}
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}
