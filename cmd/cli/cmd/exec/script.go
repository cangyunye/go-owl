package exec

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/settings"
	"github.com/cangyunye/go-owl/internal/control/blacklist"
	"github.com/cangyunye/go-owl/internal/control/script"
	"github.com/cangyunye/go-owl/internal/control/transfer"
	"github.com/cangyunye/go-owl/internal/history"
	"github.com/cangyunye/go-owl/internal/i18n"
	"github.com/cangyunye/go-owl/internal/logfile"
	"github.com/cangyunye/go-owl/internal/logger"
	"github.com/cangyunye/go-owl/internal/node"
)

// NewScriptCmd 创建脚本执行命令
func NewScriptCmd() *cobra.Command {
	scriptCmd := &cobra.Command{
		Use:   "script <script-file-or-url>",
		Short: i18n.T("exec.script.short"),
		Long:  i18n.T("exec.script.long"),
		Args: cobra.ExactArgs(1),
		Run:  runScript,
	}

	scriptCmd.Flags().StringVarP(&scriptNodes, "nodes", "N", "",
		i18n.T("exec.script.flag_nodes"))
	scriptCmd.Flags().StringSliceVarP(&scriptGroup, "groups", "g", nil, i18n.T("exec.script.flag_groups"))
	scriptCmd.Flags().StringSliceVar(&scriptGroup, "group", nil, i18n.T("exec.script.flag_group_deprecated"))
	scriptCmd.Flags().MarkHidden("group")
	scriptCmd.Flags().StringSliceVarP(&scriptLabel, "label", "l", nil,
		i18n.T("exec.script.flag_label"))
	scriptCmd.Flags().StringVar(&scriptDest, "dest", "/tmp",
		i18n.T("exec.script.flag_dest"))
	scriptCmd.Flags().StringVar(&scriptArgs, "args", "",
		i18n.T("exec.script.flag_args"))
	scriptCmd.Flags().DurationVarP(&scriptTimeout, "timeout", "t", 5*60*time.Second,
		i18n.T("exec.script.flag_timeout"))
	scriptCmd.Flags().BoolVar(&scriptInline, "inline", false,
		i18n.T("exec.script.flag_inline"))
	scriptCmd.Flags().BoolVar(&scriptKeep, "keep", false,
		i18n.T("exec.script.flag_keep"))
	scriptCmd.Flags().BoolVarP(&scriptForce, "force", "f", false,
		i18n.T("exec.script.flag_force"))
	scriptCmd.Flags().BoolVarP(&scriptSilent, "silent", "s", false,
		i18n.T("exec.script.flag_silent"))

	return scriptCmd
}

// scriptFlags
var (
	scriptNodes   string
	scriptGroup   []string
	scriptLabel   []string
	scriptDest    string
	scriptArgs    string
	scriptTimeout time.Duration
	scriptInline  bool
	scriptKeep    bool
	scriptForce   bool
	scriptSilent  bool
)

func runScript(cmd *cobra.Command, args []string) {
	scriptPath := args[0]

	// 从 owl settings 加载未显式指定的 flag 默认值
	applyScriptSettingsFallback(cmd)

	logger.Init(nil)
	defer logger.Sync()
	_, err := history.NewDB(history.DefaultConfig())
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("exec.script.warn_history_db", err))
	}

	nodeLogWriter := logfile.NewNodeLogWriter("")

	handleExecNodeConflicts()

	// 检查脚本文件是否存在
	if !(len(scriptPath) > 8 && (scriptPath[:7] == "http://" || scriptPath[:8] == "https://")) {
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, i18n.T("exec.script.err_script_not_found", scriptPath))
			os.Exit(1)
		}
	}

	// 获取目标节点
	nodeResolver := node.NewNodeResolver()
	targetNodes := selectScriptTargetNodesWithResolver(nodeResolver)
	if len(targetNodes) == 0 {
		fmt.Println(i18n.T("exec.script.no_target_nodes"))
		return
	}

	// 执行前显示信息
	if !scriptSilent {
		fmt.Println(i18n.T("exec.script.info_script", scriptPath))
		fmt.Println(i18n.T("exec.script.info_target_nodes", i18n.F(len(targetNodes))))
		if scriptInline {
			fmt.Println(i18n.T("exec.script.info_method_inline"))
		} else {
			fmt.Println(i18n.T("exec.script.info_method_transfer"))
			fmt.Println(i18n.T("exec.script.info_dest", scriptDest))
		}
		if scriptKeep {
			fmt.Println(i18n.T("exec.script.info_keep_yes"))
		}
		if scriptArgs != "" {
			fmt.Println(i18n.T("exec.script.info_args", scriptArgs))
		}
	}

	if scriptForce {
		fmt.Println(i18n.T("exec.script.skip_blacklist"))
	} else {
		var scriptContent []byte
		if len(scriptPath) > 8 && (scriptPath[:7] == "http://" || scriptPath[:8] == "https://") {
			resp, fetchErr := http.Get(scriptPath)
			if fetchErr == nil {
				defer resp.Body.Close()
				scriptContent, _ = io.ReadAll(resp.Body)
			}
		} else {
			scriptContent, _ = os.ReadFile(scriptPath)
		}

		if len(scriptContent) > 0 {
			cfg, err := blacklist.LoadConfig()
			if err != nil {
				fmt.Fprintln(os.Stderr, i18n.T("exec.script.blacklist_load_failed", err))
			} else {
				checker := blacklist.NewChecker(cfg)
				contentStr := string(scriptContent)

				blocked := false
				nodeResults := make([]*blacklist.CheckResult, len(targetNodes))
				for i, n := range targetNodes {
					result := checker.Check(n.User, contentStr)
					nodeResults[i] = result
					if result.Blocked {
						blocked = true
					}
				}

				if blocked {
					fmt.Println(i18n.T("exec.script.dangerous_detected"))
					fmt.Println(i18n.T("exec.script.dangerous_script", scriptPath))
					for i, n := range targetNodes {
						r := nodeResults[i]
						if len(r.Matches) > 0 {
							fmt.Println(i18n.T("exec.script.dangerous_node", n.ID, n.User))
							for _, m := range r.Matches {
								fmt.Println(i18n.T("exec.script.dangerous_line", m.Line))
								fmt.Println(i18n.T("exec.script.dangerous_pattern", m.Pattern))
							}
						}
					}
					fmt.Print(i18n.T("exec.script.dangerous_confirm"))
					var input string
					_, _ = fmt.Scanln(&input)
					if input != "y" && input != "Y" {
						fmt.Println(i18n.T("exec.script.cancelled"))
						return
					}
				}
			}
		}
	}

	if !scriptSilent {
		fmt.Println(i18n.T("exec.script.starting"))
	} else {
		printSilentHeader()
	}

	// 准备执行
	nodeIDs := make([]string, 0, len(targetNodes))
	for _, n := range targetNodes {
		nodeIDs = append(nodeIDs, n.ID)
	}

	// 记录操作开始
	taskID := uuid.New().String()
	history.RecordOperation(&history.Operation{
		TaskID:    taskID,
		OpType:    "script",
		Command:   scriptPath,
		Targets:   nodeIDs,
		Status:    "running",
		CreatedAt: time.Now(),
	})

	// 创建执行器
	transferMgr := transfer.NewTransferManager(nodeResolver)
	scriptExec := script.NewScriptExecutor(nodeResolver, transferMgr)

	opts := &script.ScriptExecutionOptions{
		DestDir: scriptDest,
		Args:    scriptArgs,
		Timeout: scriptTimeout,
		Inline:  scriptInline,
		Keep:    scriptKeep,
	}

	// 执行脚本
	results, execErr := scriptExec.ExecuteScript(scriptPath, nodeIDs, opts)

	// 处理结果
	success := 0
	failed := 0
	
	for _, result := range results {
		if result.Success() {
			if !scriptSilent {
				fmt.Println(i18n.T("exec.script.node_success", result.NodeID))
			}
			success++
		} else {
			if !scriptSilent {
				if result.Error != nil {
					fmt.Println(i18n.T("exec.script.node_failed_err", result.NodeID, result.Error))
				} else {
					fmt.Println(i18n.T("exec.script.node_failed_code", result.NodeID, i18n.F(result.ExitCode)))
				}
			}
			failed++
		}

		if scriptSilent {
			duration := result.EndTime.Sub(result.StartTime)
			printSilentRow(result.NodeID, result.Success(), result.ExitCode, duration)
		} else {
			// 显示输出
			if result.Output != "" {
				fmt.Println(i18n.T("exec.script.output_header"))
				for _, line := range splitLines(result.Output) {
					fmt.Printf("     %s\n", line)
				}
			}
		}

		// 记录历史
		errorMsg := ""
		if result.Error != nil {
			errorMsg = result.Error.Error()
		}
		history.RecordCommandExecution(&history.CommandExecution{
			TaskID:     taskID,
			NodeID:     result.NodeID,
			Command:    scriptPath,
			ExitCode:   result.ExitCode,
			Stdout:     truncateString(result.Output, 4096),
			Stderr:     errorMsg,
			DurationMs: result.EndTime.Sub(result.StartTime).Milliseconds(),
			Success:    result.Success(),
			CreatedAt:  time.Now(),
		})

		duration := result.EndTime.Sub(result.StartTime)
		nodeLogWriter.AppendEntry(result.NodeID, taskID, scriptPath, result.ExitCode, result.Output, errorMsg, duration)
	}

	// 更新操作状态
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
		OpType:    "script",
		Command:   scriptPath,
		Targets:   nodeIDs,
		Status:    finalStatus,
		CreatedAt: time.Now(),
	})

	// 显示总结
	if scriptSilent {
		printSilentSummary(success, failed)
	} else {
		fmt.Println(i18n.T("exec.script.summary", i18n.F(success), i18n.F(failed)))
	}
	
	if execErr != nil {
		fmt.Fprintln(os.Stderr, i18n.T("exec.script.exec_error", execErr))
		os.Exit(1)
	}
	
	if failed > 0 {
		os.Exit(1)
	}
}

func selectScriptTargetNodesWithResolver(resolver *node.NodeResolver) []*node.ResolvedNode {
	var result []*node.ResolvedNode
	allNodes, _ := resolver.ListNodes(&node.ListOptions{})

	for _, n := range allNodes {
		included := false
		
		// 检查 --nodes 筛选
		if scriptNodes != "" {
			nodeIDs := common.ParseNodeList(scriptNodes)
			if !containsStringList(nodeIDs, n.ID) {
				continue
			}
			included = true
		}

		// 检查 --groups 筛选
		if len(scriptGroup) > 0 {
			if !node.ContainsAny(n.Groups, scriptGroup) {
				continue
			}
			included = true
		}

		// 检查 --label 筛选
		if len(scriptLabel) > 0 {
			match := true
			for _, label := range scriptLabel {
				parts := splitLabelEq(label)
				if len(parts) == 2 {
					key, value := parts[0], parts[1]
					if v, ok := n.Labels[key]; !ok || v != value {
						match = false
						break
					}
				}
			}
			if !match {
				continue
			}
			included = true
		}

		// 如果没有指定任何筛选条件，默认包含所有
		if scriptNodes == "" && len(scriptGroup) == 0 && len(scriptLabel) == 0 {
			included = true
		}

		if included {
			result = append(result, n)
		}
	}

	return result
}

func splitLines(s string) []string {
	var lines []string
	current := ""
	for _, c := range s {
		if c == '\n' {
			lines = append(lines, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func splitLabelEq(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

func containsStringList(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

// applyScriptSettingsFallback 从 owl settings 加载未显式指定的脚本 flag 默认值
func applyScriptSettingsFallback(cmd *cobra.Command) {
	s := settings.GetCurrentSettings()

	// --groups: 如果用户未指定，使用 settings 中的 default.group 或 target.groups
	if !cmd.Flags().Changed("groups") {
		group := s.Default.Group
		if group == "" {
			group = s.Target.Groups
		}
		if group != "" {
			scriptGroup = strings.Split(group, ",")
		}
	}

	// --label: 如果用户未指定，使用 settings 中的 default.labels
	if !cmd.Flags().Changed("label") {
		for k, v := range s.Default.Labels {
			scriptLabel = append(scriptLabel, k+"="+v)
		}
	}
}
