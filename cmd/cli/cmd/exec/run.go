package exec

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/google/uuid"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/control/async"
	"github.com/cangyunye/go-owl/internal/control/blacklist"
	"github.com/cangyunye/go-owl/internal/control/command"
	"github.com/cangyunye/go-owl/internal/history"
	"github.com/cangyunye/go-owl/internal/logfile"
	"github.com/cangyunye/go-owl/internal/logger"
	"github.com/cangyunye/go-owl/internal/node"
	"github.com/cangyunye/go-owl/internal/ssh"
)

var (
	execNodes              string
	execGroup              string
	execLabel              []string
	execStatus             string
	execTimeout            time.Duration
	execConnectTimeout     time.Duration
	execCommandTimeout     time.Duration
	execRetry              int
	execRetryInterval      time.Duration
	execRetryMaxInterval   time.Duration
	execNoRetry            bool
	execAsync              bool
	execAsyncTimeout       time.Duration
	execAsyncPollInterval  time.Duration
	execAsyncMaxPollCount  int
	execAsyncRemoteDir     string
	execFormat             string
	execNoColor            bool
	execParallel           bool
	execSerial             bool
	execDebug              bool
	execForce              bool
	execSyncNodes          bool
	execSilent             bool
)

func NewRunCmd() *cobra.Command {
	runCmd := &cobra.Command{
		Use:   "run <command>",
		Short: "执行 Shell 命令",
		Long: `在指定节点上执行 Shell 命令，自动管理连接。

示例：
  owl exec run uptime --nodes node1,node2
  owl exec run "df -h" --group web
  owl exec run "systemctl status nginx" --label env=prod
  owl exec run uptime --status online
  owl exec run "sleep 30" --timeout 10s
  owl exec run "uptime" --format json
  owl exec run "df -h" --format detail
  owl exec run "sleep 5" --connect-timeout 5s --command-timeout 30s
  owl exec run "curl api.example.com" --retry 3 --retry-interval 2s
  owl exec run "long-running-script.sh" --async
  owl exec run "uptime" --nodes node1 --debug
  owl exec run "uptime" --nodes node1 --serial  # 串行执行`,
		Args: cobra.ExactArgs(1),
		Run:  runExecRun,
	}

	runCmd.Flags().StringVar(&execNodes, "nodes", "",
		"指定节点 ID (逗号分隔)")
	runCmd.Flags().StringVar(&execGroup, "group", "",
		"按分组选择节点")
	runCmd.Flags().StringSliceVarP(&execLabel, "label", "l", nil,
		"按标签选择节点 (格式: key=value)")
	runCmd.Flags().StringVar(&execStatus, "status", "",
		"按状态选择节点: online, offline")
	runCmd.Flags().DurationVar(&execTimeout, "timeout", 60*time.Second,
		"命令执行超时时间 (已废弃，推荐使用 --connect-timeout 和 --command-timeout)")
	runCmd.Flags().DurationVar(&execConnectTimeout, "connect-timeout", 10*time.Second,
		"SSH 连接超时时间")
	runCmd.Flags().DurationVar(&execCommandTimeout, "command-timeout", 30*time.Second,
		"命令执行超时时间")
	runCmd.Flags().BoolVar(&execParallel, "parallel", true,
		"并行执行 (默认启用)")
	runCmd.Flags().BoolVar(&execSerial, "serial", false,
		"串行执行 (禁用并行模式)")
	runCmd.Flags().IntVar(&execRetry, "retry", 3,
		"最大重试次数")
	runCmd.Flags().DurationVar(&execRetryInterval, "retry-interval", 1*time.Second,
		"初始重试间隔")
	runCmd.Flags().DurationVar(&execRetryMaxInterval, "retry-max-interval", 30*time.Second,
		"最大重试间隔")
	runCmd.Flags().BoolVar(&execNoRetry, "no-retry", false,
		"禁用重试")
	runCmd.Flags().BoolVar(&execAsync, "async", false,
		"异步执行")
	runCmd.Flags().DurationVar(&execAsyncTimeout, "async-timeout", 1*time.Hour,
		"异步任务超时时间")
	runCmd.Flags().DurationVar(&execAsyncPollInterval, "async-poll-interval", 10*time.Second,
		"异步任务轮询间隔 (0 表示 fire-and-forget)")
	runCmd.Flags().IntVar(&execAsyncMaxPollCount, "async-max-poll-count", 3600,
		"异步任务最大轮询次数")
	runCmd.Flags().StringVar(&execAsyncRemoteDir, "async-remote-dir", "/tmp/owl",
		"异步任务远程工作目录")
	runCmd.Flags().StringVarP(&execFormat, "format", "o", "simple",
		"输出格式: simple, detail, json")
	runCmd.Flags().BoolVar(&execNoColor, "no-color", false,
		"禁用颜色输出")
	runCmd.Flags().BoolVar(&execDebug, "debug", false,
		"Debug 模式，显示详细的执行过程和错误信息")
	runCmd.Flags().BoolVarP(&execForce, "force", "f", false,
		"跳过黑名单命令检查")
	runCmd.Flags().BoolVar(&execSyncNodes, "sync-nodes", false,
		"用 nodes.json 覆盖数据库中的节点数据")
	runCmd.Flags().BoolVarP(&execSilent, "silent", "s", false,
		"静默模式，仅以表格形式输出执行结果")

	return runCmd
}

func runExecRun(cmd *cobra.Command, args []string) {
	execmd := args[0]

	logger.Init(nil)
	defer logger.Sync()
	_, err := history.NewDB(history.DefaultConfig())
	if err != nil {
		fmt.Fprintf(os.Stderr, "警告: 无法初始化历史记录数据库: %v\n", err)
	}

	nodeLogWriter := logfile.NewNodeLogWriter("")

	handleExecNodeConflicts()

	nodeResolver := node.NewNodeResolver()

	var targetNodeIDs []string

	if execNodes != "" {
		targetNodeIDs = parseNodeList(execNodes)
	} else if execGroup != "" {
		nodes, err := nodeResolver.ListNodes(&node.ListOptions{
			Group: execGroup,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: 获取节点列表失败: %v\n", err)
			os.Exit(1)
		}
		for _, n := range nodes {
			targetNodeIDs = append(targetNodeIDs, n.ID)
		}
	} else if len(execLabel) > 0 {
		nodes, err := nodeResolver.ListNodes(&node.ListOptions{
			Label: execLabel[0],
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: 获取节点列表失败: %v\n", err)
			os.Exit(1)
		}
		for _, n := range nodes {
			targetNodeIDs = append(targetNodeIDs, n.ID)
		}
	} else {
		nodes, err := nodeResolver.ListNodes(&node.ListOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: 获取节点列表失败: %v\n", err)
			os.Exit(1)
		}
		for _, n := range nodes {
			targetNodeIDs = append(targetNodeIDs, n.ID)
		}
	}

	if len(targetNodeIDs) == 0 {
		fmt.Println("未找到目标节点")
		return
	}

	taskID := uuid.New().String()
	startTime := time.Now()

	// 处理并行模式：--serial 会覆盖 --parallel
	isParallel := execParallel && !execSerial

	silent := execSilent && execFormat == "simple"

	if !silent {
		fmt.Printf("🔧 命令: %s\n", execmd)
		fmt.Printf("🎯 节点: %d 个\n", len(targetNodeIDs))
		if isParallel {
			fmt.Println("⚡ 模式: 并行执行")
		} else {
			fmt.Println("⚡ 模式: 串行执行")
		}
		if execDebug {
			fmt.Println("🔍 Debug 模式: 启用")
		}
		fmt.Printf("🆔 任务ID: %s\n\n", taskID)
	}

	cfg, err := blacklist.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "警告: 加载黑名单配置失败: %v\n", err)
	}
	checker := blacklist.NewChecker(cfg)

	if execForce {
		fmt.Println("⚠️  已跳过黑名单检查 (--force)")
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
					fmt.Printf("警告: 解析节点 %s 失败，跳过黑名单检查: %v\n", nodeID, err)
				}
				continue
			}
			result := checker.Check(nodeInfo.User, execmd)
			if result.Blocked {
				blockedNodes = append(blockedNodes, blockedNode{nodeID, nodeInfo.User, result})
			}
		}

		if len(blockedNodes) > 0 {
			fmt.Println("⚠️  危险命令检测!")
			fmt.Printf("命令: %s\n", execmd)
			for _, bn := range blockedNodes {
				fmt.Printf("节点: %s (用户: %s)\n", bn.nodeID, bn.user)
				for _, match := range bn.result.Matches {
					fmt.Printf("  行: %s  匹配模式: %s\n", match.Line, match.Pattern)
				}
			}
			fmt.Print("⚠️  以上命令可能造成严重后果，确定要继续执行吗? (y/N): ")
			var input string
			fmt.Scanln(&input)
			if input != "y" && input != "Y" {
				fmt.Println("已取消执行")
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
			MaxRetries:    execRetry,
			InitialInterval: execRetryInterval,
			MaxInterval:   execRetryMaxInterval,
		}
	}

	if execAsync {
		asyncOpts := &async.AsyncOptions{
			Timeout:      execAsyncTimeout,
			PollInterval: execAsyncPollInterval,
			MaxPollCount: execAsyncMaxPollCount,
			RemoteBaseDir: execAsyncRemoteDir,
		}

		tasks, err := executor.RunAsync(ctx, targetNodeIDs, execmd, asyncOpts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: 启动异步任务失败: %v\n", err)
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
			fmt.Printf("🔄 [%s] 异步任务已启动 - ID: %s\n", task.NodeID, task.ID)
			if task.Error != nil {
				fmt.Printf("   错误: %v\n", task.Error)
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
		fmt.Printf("\n📊 总结: %d 成功, %d 失败\n", success, failed)
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
		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		fmt.Printf("节点: %s\n", result.NodeID)
		if result.Success {
			fmt.Printf("状态: ✅ 成功 (exit code: %d)\n", result.ExitCode)
		} else {
			fmt.Printf("状态: ❌ 失败\n")
			if result.ErrorType.String() != "" {
				fmt.Printf("错误类型: %s\n", result.ErrorType)
			}
			if result.ErrorDetail != "" {
				fmt.Printf("错误详情: %s\n", result.ErrorDetail)
			} else if result.Error != nil {
				fmt.Printf("错误: %v\n", result.Error)
			}
			if result.ErrorType.Suggestion() != "" {
				fmt.Printf("💡 建议: %s\n", result.ErrorType.Suggestion())
			}
		}
		fmt.Printf("\n输出:\n%s\n", result.Output)

		if execDebug && len(result.DebugInfo) > 0 {
			fmt.Println("\n🔍 Debug 信息:")
			for _, line := range result.DebugInfo {
				fmt.Printf("   - %s\n", line)
			}
		}
	} else {
		if result.Success {
			fmt.Printf("✅ [%s] 成功\n", result.NodeID)
			if result.Output != "" {
				for _, line := range strings.Split(result.Output, "\n") {
					fmt.Printf("   %s\n", line)
				}
			}
		} else {
			fmt.Printf("❌ [%s] 失败\n", result.NodeID)

			if result.ErrorType.String() != "" {
				fmt.Printf("   类型: %s\n", result.ErrorType)
			}

			if result.ErrorDetail != "" {
				fmt.Printf("   详情: %s\n", result.ErrorDetail)
			} else if result.Error != nil {
				fmt.Printf("   错误: %v\n", result.Error)
			}

			if result.ErrorType.Suggestion() != "" {
				fmt.Printf("   💡 建议: %s\n", result.ErrorType.Suggestion())
			}

			if result.Output != "" {
				fmt.Println("   输出:")
				for _, line := range strings.Split(result.Output, "\n") {
					fmt.Printf("      %s\n", line)
				}
			}

			if execDebug && len(result.DebugInfo) > 0 {
				fmt.Println("   🔍 Debug 信息:")
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
