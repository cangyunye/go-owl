package exec

import (
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/control/script"
	"github.com/cangyunye/go-owl/internal/control/transfer"
	"github.com/cangyunye/go-owl/internal/history"
	"github.com/cangyunye/go-owl/internal/logger"
	"github.com/cangyunye/go-owl/internal/node"
)

// NewScriptCmd 创建脚本执行命令
func NewScriptCmd() *cobra.Command {
	scriptCmd := &cobra.Command{
		Use:   "script <script-file-or-url>",
		Short: "执行脚本",
		Long: `在指定节点上传输并执行脚本。

支持本地脚本文件和 URL 远程脚本。

执行方式：
  默认：上传到远端文件执行（便于调试和审计）
  --inline：直接发送内容执行（不留痕迹）

示例：
  owl exec script deploy.sh --nodes web-01,web-02
  owl exec script ./scripts/install.sh --group web --dest /tmp
  owl exec script backup.sh --args "--env prod" --label env=prod
  owl exec script init.sh --inline --nodes test-01  # 直接内容执行
  owl exec script setup.sh --keep --nodes all  # 执行后保留脚本
  owl exec script deploy.sh --timeout 10m`,
		Args: cobra.ExactArgs(1),
		Run:  runScript,
	}

	scriptCmd.Flags().StringVar(&scriptNodes, "nodes", "",
		"指定节点 ID (逗号分隔)")
	scriptCmd.Flags().StringVar(&scriptGroup, "group", "",
		"按分组选择节点")
	scriptCmd.Flags().StringSliceVarP(&scriptLabel, "label", "l", nil,
		"按标签选择节点")
	scriptCmd.Flags().StringVar(&scriptDest, "dest", "/tmp",
		"目标目录")
	scriptCmd.Flags().StringVar(&scriptArgs, "args", "",
		"传递给脚本的参数")
	scriptCmd.Flags().DurationVar(&scriptTimeout, "timeout", 5*60*time.Second,
		"脚本执行超时时间")
	scriptCmd.Flags().BoolVar(&scriptInline, "inline", false,
		"直接发送内容执行，不保存为文件")
	scriptCmd.Flags().BoolVar(&scriptKeep, "keep", false,
		"执行后保留脚本文件（默认会删除）")

	return scriptCmd
}

// scriptFlags
var (
	scriptNodes   string
	scriptGroup   string
	scriptLabel   []string
	scriptDest    string
	scriptArgs    string
	scriptTimeout time.Duration
	scriptInline  bool
	scriptKeep    bool
)

func runScript(cmd *cobra.Command, args []string) {
	scriptPath := args[0]
	logger.Init(nil)
	defer logger.Sync()
	_, err := history.NewDB(history.DefaultConfig())
	if err != nil {
		fmt.Fprintf(os.Stderr, "警告: 无法初始化历史记录数据库: %v\n", err)
	}

	// 检查脚本文件是否存在
	if !(len(scriptPath) > 8 && (scriptPath[:7] == "http://" || scriptPath[:8] == "https://")) {
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "错误: 脚本文件不存在: %s\n", scriptPath)
			os.Exit(1)
		}
	}

	// 获取目标节点
	nodeResolver := node.NewNodeResolver()
	targetNodes := selectScriptTargetNodesWithResolver(nodeResolver)
	if len(targetNodes) == 0 {
		fmt.Println("未找到目标节点")
		return
	}

	// 执行前显示信息
	fmt.Printf("📜 脚本: %s\n", scriptPath)
	fmt.Printf("🎯 目标节点: %d 个\n", len(targetNodes))
	if scriptInline {
		fmt.Println("🚀 执行方式: 直接内容执行 (inline)")
	} else {
		fmt.Printf("🚀 执行方式: 文件传输 + 执行\n")
		fmt.Printf("📂 存放目录: %s\n", scriptDest)
	}
	if scriptKeep {
		fmt.Println("📝 保留脚本: 是")
	}
	if scriptArgs != "" {
		fmt.Printf("📋 参数: %s\n", scriptArgs)
	}

	fmt.Println("\n⏳ 开始执行...")

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
			fmt.Printf("✅ [%s] 成功\n", result.NodeID)
			success++
		} else {
			if result.Error != nil {
				fmt.Printf("❌ [%s] 失败: %v\n", result.NodeID, result.Error)
			} else {
				fmt.Printf("❌ [%s] 失败 (退出码: %d)\n", result.NodeID, result.ExitCode)
			}
			failed++
		}
		
		// 显示输出
		if result.Output != "" {
			fmt.Printf("   输出:\n")
			for _, line := range splitLines(result.Output) {
				fmt.Printf("     %s\n", line)
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
	fmt.Printf("\n📊 总结: %d 成功, %d 失败\n", success, failed)
	
	if execErr != nil {
		fmt.Fprintf(os.Stderr, "\n执行过程中出错: %v\n", execErr)
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

		// 检查 --group 筛选
		if scriptGroup != "" {
			if !containsStringList(n.Groups, scriptGroup) {
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
		if scriptNodes == "" && scriptGroup == "" && len(scriptLabel) == 0 {
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
