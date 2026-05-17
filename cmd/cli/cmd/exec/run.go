package exec

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/internal/control/command"
	"github.com/cangyunye/go-owl/internal/node"
)

var (
	execNodes    string
	execGroup    string
	execLabel    []string
	execStatus   string
	execTimeout  time.Duration
	execAsync    bool
	execFormat   string
	execNoColor  bool
	execParallel bool
)

func NewRunCmd() *cobra.Command {
	runCmd := &cobra.Command{
		Use:   "run <command>",
		Short: "执行 Shell 命令",
		Long: `在指定节点上执行 Shell 命令，自动管理连接。

示例：
  owl exec run "uptime" --nodes node1,node2
  owl exec run "df -h" --group web
  owl exec run "service nginx restart" --timeout 60s`,
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
		"命令执行超时时间")
	runCmd.Flags().BoolVar(&execParallel, "parallel", true,
		"并行执行")
	runCmd.Flags().BoolVar(&execAsync, "async", false,
		"异步执行，不等待结果")
	runCmd.Flags().StringVarP(&execFormat, "output", "o", "simple",
		"输出格式: simple, detail, json")
	runCmd.Flags().BoolVar(&execNoColor, "no-color", false,
		"禁用颜色输出")

	return runCmd
}

func runExecRun(cmd *cobra.Command, args []string) {
	execmd := args[0]

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

	fmt.Printf("🔧 命令: %s\n", execmd)
	fmt.Printf("🎯 节点: %d 个\n", len(targetNodeIDs))
	if execParallel {
		fmt.Println("⚡ 模式: 并行执行")
	}
	fmt.Println()

	executor := command.NewExecutor(nodeResolver)
	defer executor.Close()

	ctx, cancel := context.WithTimeout(context.Background(), execTimeout*time.Duration(len(targetNodeIDs)))
	defer cancel()

	opts := &command.ExecuteOptions{
		Parallel: execParallel,
		Timeout:  execTimeout,
	}

	results := executor.Run(ctx, targetNodeIDs, execmd, opts)

	success := 0
	failed := 0

	for _, result := range results {
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
				if result.Error != nil {
					fmt.Printf("错误: %v\n", result.Error)
				}
			}
			fmt.Printf("\n输出:\n%s\n", result.Output)
		} else {
			if result.Success {
				fmt.Printf("✅ [%s] 成功\n", result.NodeID)
				if result.Output != "" {
					for _, line := range strings.Split(result.Output, "\n") {
						fmt.Printf("   %s\n", line)
					}
				}
				success++
			} else {
				fmt.Printf("❌ [%s] 失败", result.NodeID)
				if result.Error != nil {
					fmt.Printf(": %v", result.Error)
				}
				fmt.Println()
				failed++
			}
		}
	}

	if execFormat != "json" {
		fmt.Printf("\n📊 总结: %d 成功, %d 失败\n", success, failed)
	}

	if failed > 0 {
		os.Exit(1)
	}
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

func parseLabels(labels []string) map[string]string {
	result := make(map[string]string)
	for _, label := range labels {
		for i := 0; i < len(label); i++ {
			if label[i] == '=' {
				result[label[:i]] = label[i+1:]
				break
			}
		}
	}
	return result
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
