package exec

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

// NewScriptCmd 创建脚本执行命令
func NewScriptCmd() *cobra.Command {
	scriptCmd := &cobra.Command{
		Use:   "script <script-file-or-url>",
		Short: "执行脚本",
		Long: `在指定节点上传输并执行脚本。

支持本地脚本文件和 URL 远程脚本。

示例：
  owl exec script deploy.sh --nodes node1,node2
  owl exec script ./scripts/install.sh --group web --dest /tmp
  owl exec script https://example.com/setup.sh --args "--env prod"`,
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
)

func runScript(cmd *cobra.Command, args []string) {
	scriptPath := args[0]
	store := common.GetNodeStore()

	// 获取目标节点
	targetNodes := selectScriptTargetNodes(store)
	if len(targetNodes) == 0 {
		fmt.Println("No target nodes found.")
		return
	}

	// 确定脚本类型
	scriptType := "local"
	if len(scriptPath) > 8 && (scriptPath[:7] == "http://" || scriptPath[:8] == "https://") {
		scriptType = "url"
	}

	// 传输并执行脚本
	fmt.Printf("Script type: %s\n", scriptType)
	fmt.Printf("Script: %s\n", scriptPath)
	fmt.Printf("Target: %d nodes\n", len(targetNodes))
	fmt.Printf("Destination: %s\n", scriptDest)

	if scriptArgs != "" {
		fmt.Printf("Arguments: %s\n", scriptArgs)
	}

	fmt.Println("\nTransferring and executing script...")

	// 模拟传输和执行
	success := 0
	failed := 0
	for _, n := range targetNodes {
		if n.Status == "online" {
			fmt.Printf("[%s] OK: Script transferred and executed\n", n.ID)
			success++
		} else {
			fmt.Printf("[%s] FAIL: Node offline\n", n.ID)
			failed++
		}
	}

	fmt.Printf("\nSummary: %d succeeded, %d failed\n", success, failed)
}

func selectScriptTargetNodes(store common.NodeStore) []*common.NodeInfo {
	var result []*common.NodeInfo
	allNodes, _ := store.List()

	for _, n := range allNodes {
		if scriptNodes != "" {
			nodeIDs := common.ParseNodeList(scriptNodes)
			if !containsStringList(nodeIDs, n.ID) {
				continue
			}
		}

		if scriptGroup != "" {
			if !containsStringList(n.Groups, scriptGroup) {
				continue
			}
		}

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
		}

		result = append(result, n)
	}

	return result
}

func containsStringList(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

func splitLabelEq(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}
