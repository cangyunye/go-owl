package exec

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/settings"
	"github.com/cangyunye/go-owl/internal/control/async"
	"github.com/cangyunye/go-owl/internal/control/blacklist"
	"github.com/cangyunye/go-owl/internal/control/command"
	"github.com/cangyunye/go-owl/internal/history"
	"github.com/cangyunye/go-owl/internal/logfile"
	"github.com/cangyunye/go-owl/internal/logger"
	"github.com/cangyunye/go-owl/internal/node"
	nodeselect "github.com/cangyunye/go-owl/internal/node/select"
	"github.com/cangyunye/go-owl/internal/ssh"

	"github.com/cangyunye/go-owl/internal/i18n"
)

var (
	execNodes             string
	execGroup             []string
	execLabel             []string
	execStatus            string
	execTimeout           time.Duration
	execConnectTimeout    time.Duration
	execCommandTimeout    time.Duration
	execRetry             int
	execRetryInterval     time.Duration
	execRetryMaxInterval  time.Duration
	execNoRetry           bool
	execAsync             bool
	execAsyncTimeout      time.Duration
	execAsyncPollInterval time.Duration
	execAsyncMaxPollCount int
	execAsyncRemoteDir    string
	execFormat            string
	execNoColor           bool
	execParallel          bool
	execSerial            bool
	execDebug             bool
	execForce             bool
	execSyncNodes         bool
	execSilent            bool
)

func NewRunCmd() *cobra.Command {
	runCmd := &cobra.Command{
		Use:   "run <command>",
		Short: i18n.T("exec.run.short"),
		Long:  i18n.T("exec.run.long"),
		Args:  cobra.ExactArgs(1),
		Run:   runExecRun,
	}

	runCmd.Flags().StringVarP(&execNodes, "nodes", "N", "",
		i18n.T("exec.run.flag_nodes"))
	runCmd.Flags().StringSliceVarP(&execGroup, "groups", "g", nil, i18n.T("exec.run.flag_groups"))
	runCmd.Flags().StringSliceVar(&execGroup, "group", nil, i18n.T("exec.run.flag_group_deprecated"))
	runCmd.Flags().MarkHidden("group")
	runCmd.Flags().StringSliceVarP(&execLabel, "label", "l", nil,
		i18n.T("exec.run.flag_label"))
	runCmd.Flags().StringVarP(&execStatus, "status", "S", "",
		i18n.T("exec.run.flag_status"))
	runCmd.Flags().DurationVarP(&execTimeout, "timeout", "t", 60*time.Second,
		i18n.T("exec.run.flag_timeout"))
	runCmd.Flags().DurationVar(&execConnectTimeout, "connect-timeout", 10*time.Second,
		i18n.T("exec.run.flag_connect_timeout"))
	runCmd.Flags().DurationVar(&execCommandTimeout, "command-timeout", 30*time.Second,
		i18n.T("exec.run.flag_command_timeout"))
	runCmd.Flags().BoolVar(&execParallel, "parallel", true,
		i18n.T("exec.run.flag_parallel"))
	runCmd.Flags().BoolVar(&execSerial, "serial", false,
		i18n.T("exec.run.flag_serial"))
	runCmd.Flags().IntVar(&execRetry, "retry", 3,
		i18n.T("exec.run.flag_retry"))
	runCmd.Flags().DurationVar(&execRetryInterval, "retry-interval", 1*time.Second,
		i18n.T("exec.run.flag_retry_interval"))
	runCmd.Flags().DurationVar(&execRetryMaxInterval, "retry-max-interval", 30*time.Second,
		i18n.T("exec.run.flag_retry_max_interval"))
	runCmd.Flags().BoolVar(&execNoRetry, "no-retry", false,
		i18n.T("exec.run.flag_no_retry"))
	runCmd.Flags().BoolVar(&execAsync, "async", false,
		i18n.T("exec.run.flag_async"))
	runCmd.Flags().DurationVar(&execAsyncTimeout, "async-timeout", 1*time.Hour,
		i18n.T("exec.run.flag_async_timeout"))
	runCmd.Flags().DurationVar(&execAsyncPollInterval, "async-poll-interval", 10*time.Second,
		i18n.T("exec.run.flag_async_poll_interval"))
	runCmd.Flags().IntVar(&execAsyncMaxPollCount, "async-max-poll-count", 3600,
		i18n.T("exec.run.flag_async_max_poll_count"))
	runCmd.Flags().StringVar(&execAsyncRemoteDir, "async-remote-dir", "/tmp/owl",
		i18n.T("exec.run.flag_async_remote_dir"))
	runCmd.Flags().StringVarP(&execFormat, "format", "o", "simple",
		i18n.T("exec.run.flag_format"))
	runCmd.Flags().BoolVarP(&execNoColor, "no-color", "C", false,
		i18n.T("exec.run.flag_no_color"))
	runCmd.Flags().BoolVar(&execDebug, "debug", false,
		i18n.T("exec.run.flag_debug"))
	runCmd.Flags().BoolVarP(&execForce, "force", "f", false,
		i18n.T("exec.run.flag_force"))
	runCmd.Flags().BoolVar(&execSyncNodes, "sync-nodes", false,
		i18n.T("exec.run.flag_sync_nodes"))
	runCmd.Flags().BoolVarP(&execSilent, "silent", "s", false,
		i18n.T("exec.run.flag_silent"))

	return runCmd
}

func runExecRun(cmd *cobra.Command, args []string) {
	execmd := args[0]

	// 从 owl settings 加载未显式指定的 flag 默认值
	applyExecSettingsFallback(cmd)

	logger.Init(nil)
	defer logger.Sync()
	_, err := history.NewDB(history.DefaultConfig())
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("exec.run.warn_history_db", err))
	}

	nodeLogWriter := logfile.NewNodeLogWriter("")

	handleExecNodeConflicts()

	nodeResolver := node.NewNodeResolver()

	var targetNodeIDs []string

	selector := nodeselect.NewSelector(nodeselect.NewResolverSource(nodeResolver))
	selectOpts := nodeselect.SelectOptions{}
	switch {
	case execNodes != "":
		selectOpts.NodeIDs = parseNodeList(execNodes)
	case len(execGroup) > 0:
		selectOpts.Groups = execGroup
	case len(execLabel) > 0:
		labels := make(map[string]string)
		if k, v, ok := strings.Cut(execLabel[0], "="); ok {
			labels[k] = v
		} else {
			labels[execLabel[0]] = ""
		}
		selectOpts.Labels = labels
	}
	selected, err := selector.Select(context.Background(), selectOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("exec.run.err_list", err))
		os.Exit(1)
	}
	for _, n := range selected {
		targetNodeIDs = append(targetNodeIDs, n.ID)
	}

	if len(targetNodeIDs) == 0 {
		fmt.Println(i18n.T("exec.run.no_target"))
		return
	}

	taskID := uuid.New().String()
	startTime := time.Now()

	// 处理并行模式：--serial 会覆盖 --parallel
	isParallel := execParallel && !execSerial

	silent := execSilent && execFormat == "simple"

	if !silent {
		fmt.Printf("%s", i18n.T("exec.run.header_cmd", execmd))
		fmt.Printf("%s", i18n.T("exec.run.header_nodes", i18n.F(len(targetNodeIDs))))
		if isParallel {
			fmt.Println(i18n.T("exec.run.mode_parallel"))
		} else {
			fmt.Println(i18n.T("exec.run.mode_serial"))
		}
		if execDebug {
			fmt.Println(i18n.T("exec.run.debug_on"))
		}
		fmt.Printf("%s", i18n.T("exec.run.header_task", taskID))
	}

	cfg, err := blacklist.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("exec.run.warn_blacklist", err))
	}
	checker := blacklist.NewChecker(cfg)

	if execForce {
		fmt.Println(i18n.T("exec.run.force_skip"))
	} else {
		type blockedNode struct {
			nodeID string
			user   string
			result *blacklist.CheckResult
		}
		var blockedNodes []blockedNode

		for _, nodeID := range targetNodeIDs {
			nodeInfo, err := nodeResolver.Resolve(nodeID)
			if err != nil {
				if execDebug {
					fmt.Printf("%s", i18n.T("exec.run.warn_resolve_skip", nodeID, err))
				}
				continue
			}
			result := checker.Check(nodeInfo.User, execmd)
			if result.Blocked {
				blockedNodes = append(blockedNodes, blockedNode{nodeID, nodeInfo.User, result})
			}
		}

		if len(blockedNodes) > 0 {
			fmt.Println(i18n.T("exec.run.danger_title"))
			fmt.Printf("%s", i18n.T("exec.run.danger_cmd", execmd))
			for _, bn := range blockedNodes {
				fmt.Printf("%s", i18n.T("exec.run.danger_node", bn.nodeID, bn.user))
				for _, match := range bn.result.Matches {
					fmt.Printf("%s", i18n.T("exec.run.danger_match", match.Line, match.Pattern))
				}
			}
			fmt.Print(i18n.T("exec.run.danger_prompt"))
			var input string
			fmt.Scanln(&input)
			if input != "y" && input != "Y" {
				fmt.Println(i18n.T("exec.run.cancelled"))
				return
			}
		}
	}

	history.RecordOperation(&history.Operation{
		TaskID:    taskID,
		OpType:    "command",
		Command:   execmd,
		Targets:   targetNodeIDs,
		Status:    "running",
		CreatedAt: startTime,
	})

	executor := command.NewExecutor(nodeResolver)
	if execDebug {
		executor.SetDebug(true)
	}
	defer executor.Close()

	ctx, cancel := context.WithTimeout(context.Background(), execTimeout)
	defer cancel()

	opts := &command.ExecuteOptions{
		Parallel: isParallel,
	}

	if execConnectTimeout > 0 || execCommandTimeout > 0 {
		opts.TimeoutConfig = &ssh.TimeoutConfig{
			ConnectTimeout: execConnectTimeout,
			CommandTimeout: execCommandTimeout,
		}
	} else if execTimeout > 0 {
		opts.Timeout = execTimeout
	}

	if !execNoRetry && execRetry > 0 {
		opts.RetryConfig = &command.RetryConfig{
			MaxRetries:      execRetry,
			InitialInterval: execRetryInterval,
			MaxInterval:     execRetryMaxInterval,
		}
	}

	if execAsync {
		asyncOpts := &async.AsyncOptions{
			Timeout:       execAsyncTimeout,
			PollInterval:  execAsyncPollInterval,
			MaxPollCount:  execAsyncMaxPollCount,
			RemoteBaseDir: execAsyncRemoteDir,
		}

		tasks, err := executor.RunAsync(ctx, targetNodeIDs, execmd, asyncOpts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s", i18n.T("exec.run.err_async_start", err))
			history.RecordOperation(&history.Operation{
				TaskID:    taskID,
				OpType:    "command",
				Command:   execmd,
				Targets:   targetNodeIDs,
				Status:    "failed",
				CreatedAt: startTime,
			})
			os.Exit(1)
		}

		for _, task := range tasks {
			fmt.Printf("%s", i18n.T("exec.run.async_started", task.NodeID, task.ID))
			if task.Error != nil {
				fmt.Printf("%s", i18n.T("exec.run.async_task_err", task.Error))
			}
		}
		history.RecordOperation(&history.Operation{
			TaskID:    taskID,
			OpType:    "command",
			Command:   execmd,
			Targets:   targetNodeIDs,
			Status:    "completed",
			CreatedAt: startTime,
		})
		return
	}

	success := 0
	failed := 0

	if silent {
		printSilentHeader()
	}

	processResult := func(result command.CommandResult) {
		if result.Success {
			success++
		} else {
			failed++
		}

		errorMsg := ""
		if result.Error != nil {
			errorMsg = result.Error.Error()
		}
		history.RecordCommandExecution(&history.CommandExecution{
			TaskID:     taskID,
			NodeID:     result.NodeID,
			Command:    execmd,
			ExitCode:   result.ExitCode,
			Stdout:     truncateOutput(result.Output, 4096),
			Stderr:     errorMsg,
			DurationMs: result.Duration.Milliseconds(),
			Success:    result.Success,
			CreatedAt:  time.Now(),
		})

		nodeLogWriter.AppendEntry(result.NodeID, taskID, execmd, result.ExitCode, result.Output, errorMsg, result.Duration)

		if silent {
			printSilentRow(result.NodeID, result.Success, result.ExitCode, result.Duration)
		} else {
			printResult(result)
		}
	}

	resultChan := executor.RunStreaming(ctx, targetNodeIDs, execmd, opts)
	for result := range resultChan {
		processResult(result)
	}

	if silent {
		printSilentSummary(success, failed)
	} else if execFormat != "json" {
		fmt.Printf("%s", i18n.T("exec.run.summary", i18n.F(success), i18n.F(failed)))
	}

	finalStatus := "completed"
	if failed > 0 {
		if success == 0 {
			finalStatus = "failed"
		} else {
			finalStatus = "partial_failure"
		}
	}

	history.RecordOperation(&history.Operation{
		TaskID:    taskID,
		OpType:    "command",
		Command:   execmd,
		Targets:   targetNodeIDs,
		Status:    finalStatus,
		CreatedAt: startTime,
	})

	if failed > 0 {
		os.Exit(1)
	}
}

func handleExecNodeConflicts() {
	if execSyncNodes {
		db, err := history.NewDB(history.DefaultConfig())
		if err != nil || db == nil {
			return
		}
		sqlDB := db.Connection()
		if sqlDB == nil {
			return
		}
		if err := common.SyncNodesJSONToDB(sqlDB); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to sync nodes: %v\n", err)
			os.Exit(1)
		}
		return
	}

	common.CheckNodeConflictsBeforeExec()
}

func parseNodeList(s string) []string {
	var result []string
	current := ""
	for _, c := range s {
		if c == ',' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func truncateOutput(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func escapeJSON(s string) string {
	var result strings.Builder
	for _, c := range s {
		switch c {
		case '"':
			result.WriteString(`\"`)
		case '\\':
			result.WriteString(`\\`)
		case '\n':
			result.WriteString(`\n`)
		case '\r':
			result.WriteString(`\r`)
		case '\t':
			result.WriteString(`\t`)
		default:
			result.WriteRune(c)
		}
	}
	return result.String()
}

func printResult(result command.CommandResult) {
	if execFormat == "json" {
		fmt.Printf(`{"node":"%s","success":%v,"output":"%s","exit_code":%d}`+"\n",
			result.NodeID, result.Success, escapeJSON(result.Output), result.ExitCode)
	} else if execFormat == "detail" {
		fmt.Printf("%s", i18n.T("exec.run.detail_sep"))
		fmt.Printf("%s", i18n.T("exec.run.detail_node", result.NodeID))
		if result.Success {
			fmt.Printf("%s", i18n.T("exec.run.detail_ok", i18n.F(result.ExitCode)))
		} else {
			fmt.Printf("%s", i18n.T("exec.run.detail_fail"))
			if result.ErrorType.String() != "" {
				fmt.Printf("%s", i18n.T("exec.run.detail_err_type", result.ErrorType))
			}
			if result.ErrorDetail != "" {
				fmt.Printf("%s", i18n.T("exec.run.detail_err_detail", result.ErrorDetail))
			} else if result.Error != nil {
				fmt.Printf("%s", i18n.T("exec.run.detail_err", result.Error))
			}
			if result.ErrorType.Suggestion() != "" {
				fmt.Printf("%s", i18n.T("exec.run.detail_suggestion", result.ErrorType.Suggestion()))
			}
		}
		fmt.Printf("%s", i18n.T("exec.run.detail_output", result.Output))

		if execDebug && len(result.DebugInfo) > 0 {
			fmt.Println(i18n.T("exec.run.detail_debug_title"))
			for _, line := range result.DebugInfo {
				fmt.Printf("   - %s\n", line)
			}
		}
	} else {
		if result.Success {
			fmt.Printf("%s", i18n.T("exec.run.ok", result.NodeID))
			if result.Output != "" {
				for _, line := range strings.Split(result.Output, "\n") {
					fmt.Printf("   %s\n", line)
				}
			}
		} else {
			fmt.Printf("%s", i18n.T("exec.run.fail", result.NodeID))

			if result.ErrorType.String() != "" {
				fmt.Printf("%s", i18n.T("exec.run.type", result.ErrorType))
			}

			if result.ErrorDetail != "" {
				fmt.Printf("%s", i18n.T("exec.run.detail", result.ErrorDetail))
			} else if result.Error != nil {
				fmt.Printf("%s", i18n.T("exec.run.err", result.Error))
			}

			if result.ErrorType.Suggestion() != "" {
				fmt.Printf("%s", i18n.T("exec.run.suggestion", result.ErrorType.Suggestion()))
			}

			if result.Output != "" {
				fmt.Println(i18n.T("exec.run.output_title"))
				for _, line := range strings.Split(result.Output, "\n") {
					fmt.Printf("      %s\n", line)
				}
			}

			if execDebug && len(result.DebugInfo) > 0 {
				fmt.Println(i18n.T("exec.run.debug_title"))
				for _, line := range result.DebugInfo {
					fmt.Printf("      - %s\n", line)
				}
			}
		}
	}
}

func printSilentHeader() {
	fmt.Printf("%-24s %-8s %-9s %s\n", "NODE", "STATUS", "EXIT CODE", "DURATION")
	fmt.Println(strings.Repeat("─", 60))
}

func printSilentRow(nodeID string, success bool, exitCode int, duration time.Duration) {
	status := "FAILED"
	if success {
		status = "SUCCESS"
	}
	durationStr := formatDuration(duration)
	fmt.Printf("%-24s %-8s %-9d %s\n", nodeID, status, exitCode, durationStr)
}

func printSilentSummary(success, failed int) {
	fmt.Println(strings.Repeat("─", 60))
	fmt.Printf("Total: %d success, %d failed\n", success, failed)
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}

// applyExecSettingsFallback 从 owl settings 加载未显式指定的 flag 默认值
// 优先级: CLI flag > 命令内置默认值 > owl settings 配置
func applyExecSettingsFallback(cmd *cobra.Command) {
	s := settings.GetCurrentSettings()

	// --groups: 如果用户未指定，使用 settings 中的 default.group 或 target.groups
	if !cmd.Flags().Changed("groups") {
		group := s.Default.Group
		if group == "" {
			group = s.Target.Groups
		}
		if group != "" {
			execGroup = strings.Split(group, ",")
		}
	}

	// --label: 如果用户未指定，使用 settings 中的 default.labels
	if !cmd.Flags().Changed("label") {
		for k, v := range s.Default.Labels {
			execLabel = append(execLabel, k+"="+v)
		}
	}

	// --format: 如果用户未指定，使用 settings 中的 output.format
	if !cmd.Flags().Changed("format") && s.Output.Format != "" {
		execFormat = s.Output.Format
	}

	// --no-color: 如果用户未指定，从 settings output.color 取反
	if !cmd.Flags().Changed("no-color") {
		execNoColor = !s.Output.Color
	}

	// --parallel / --serial: 如果用户未指定，使用 settings 中的 default.parallel
	if !cmd.Flags().Changed("parallel") && !cmd.Flags().Changed("serial") {
		execParallel = s.Default.Parallel
	}
}
