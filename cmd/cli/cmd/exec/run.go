package exec

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/common/model"
)

// execFlags
var (
	execNodes   string
	execGroup   string
	execLabel   []string
	execStatus  string
	execTimeout time.Duration
	execAsync   bool
	execFormat  string
	execNoColor bool
)

// NewRunCmd 创建执行命令子命令
func NewRunCmd() *cobra.Command {
	runCmd := &cobra.Command{
		Use:   "run <command>",
		Short: "执行 Shell 命令",
		Long: `在指定节点上执行 Shell 命令。

示例：
  owl exec run "uptime"                           # 在所有节点执行
  owl exec run "uptime" --nodes node1,node2      # 指定节点
  owl exec run "uptime" --group web              # 按分组执行
  owl exec run "df -h" --timeout 30s             # 超时设置
  owl exec run "service nginx restart" --async    # 异步执行`,
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
	runCmd.Flags().BoolVar(&execAsync, "async", false,
		"异步执行，不等待结果")
	runCmd.Flags().StringVarP(&execFormat, "output", "o", "simple",
		"输出格式: simple, detail, json")
	runCmd.Flags().BoolVar(&execNoColor, "no-color", false,
		"禁用颜色输出")

	return runCmd
}

func runExecRun(cmd *cobra.Command, args []string) {
	command := args[0]
	store := common.GetNodeStore()

	// 获取目标节点
	targetNodes := selectTargetNodes(store)
	if len(targetNodes) == 0 {
		fmt.Println("No target nodes found.")
		return
	}

	// 执行命令
	results := executeCommandOnNodes(command, targetNodes, execTimeout, execAsync)

	// 转换为 model.Node 格式输出
	formatter := common.NewOutputFormatter(execFormat, !execNoColor)
	modelResults := toModelResults(results)
	formatter.FormatTaskResults(modelResults)
}

func selectTargetNodes(store common.NodeStore) []*common.NodeInfo {
	var result []*common.NodeInfo
	allNodes, _ := store.List()

	for _, n := range allNodes {
		// 按节点 ID 过滤
		if execNodes != "" {
			nodeIDs := common.ParseNodeList(execNodes)
			if !containsString(nodeIDs, n.ID) {
				continue
			}
		}

		// 按分组过滤
		if execGroup != "" {
			if !containsString(n.Groups, execGroup) {
				continue
			}
		}

		// 按标签过滤
		if len(execLabel) > 0 {
			match := true
			for _, label := range execLabel {
				parts := strings.Split(label, "=")
				if len(parts) != 2 {
					continue
				}
				key, value := parts[0], parts[1]
				if v, ok := n.Labels[key]; !ok || v != value {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}

		// 按状态过滤
		if execStatus != "" {
			if strings.ToLower(n.Status) != strings.ToLower(execStatus) {
				continue
			}
		}

		result = append(result, n)
	}

	return result
}

func containsString(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

// executeCommandOnNodes 在多个节点上执行命令
// 这里使用模拟实现，实际会调用 gRPC 服务
func executeCommandOnNodes(command string, nodes []*common.NodeInfo, timeout time.Duration, async bool) map[string]*common.NodeInfo {
	results := make(map[string]*common.NodeInfo)

	for _, n := range nodes {
		// 模拟命令执行
		if async {
			go simulateAsyncExec(n.ID, command)
			results[n.ID] = n
		} else {
			// 同步执行
			if n.Status == "online" {
				fmt.Printf("[%s] Executing: %s\n", n.ID, command)
				// 模拟执行
				time.Sleep(100 * time.Millisecond)
				results[n.ID] = n
			} else {
				fmt.Printf("[%s] Node offline, skipping\n", n.ID)
				results[n.ID] = nil
			}
		}
	}

	return results
}

func simulateAsyncExec(nodeID, command string) {
	// 模拟异步执行
	fmt.Printf("[%s] Async execution started: %s\n", nodeID, command)
	time.Sleep(500 * time.Millisecond)
	fmt.Printf("[%s] Async execution completed\n", nodeID)
}

func toModelResults(results map[string]*common.NodeInfo) map[string]*model.Node {
	modelResults := make(map[string]*model.Node)
	for k, v := range results {
		if v != nil {
			modelResults[k] = &model.Node{
				ID:      v.ID,
				Name:    v.Name,
				Address: v.Address,
				Port:    v.Port,
				Status:  model.NodeStatus(v.Status),
				Groups:  v.Groups,
				Labels:  v.Labels,
			}
		} else {
			modelResults[k] = nil
		}
	}
	return modelResults
}
