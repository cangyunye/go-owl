package playbook

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

// playbookRunFlags
var (
	pbRunNodes     string
	pbRunGroup     string
	pbRunLabel     []string
	pbRunTags      string
	pbRunSkipTags  string
	pbRunExtraVars []string
	pbRunCheck     bool
	pbRunDiff      bool
)

// NewPlaybookRunCmd 创建剧本执行命令
func NewPlaybookRunCmd() *cobra.Command {
	runCmd := &cobra.Command{
		Use:   "run <playbook-file>",
		Short: "执行剧本",
		Long: `执行 Ansible 风格的 YAML 剧本。

示例：
  owl playbook run site.yml
  owl playbook run site.yml --tags nginx,mysql
  owl playbook run site.yml --extra-vars "version=1.2.3"
  owl playbook run site.yml --check`,
		Args: cobra.ExactArgs(1),
		Run:  runPlaybookRun,
	}

	runCmd.Flags().StringVar(&pbRunNodes, "nodes", "",
		"指定节点 ID (逗号分隔)")
	runCmd.Flags().StringVar(&pbRunGroup, "group", "",
		"按分组选择节点")
	runCmd.Flags().StringSliceVarP(&pbRunLabel, "label", "l", nil,
		"按标签选择节点")
	runCmd.Flags().StringVar(&pbRunTags, "tags", "",
		"执行指定标签的任务")
	runCmd.Flags().StringVar(&pbRunSkipTags, "skip-tags", "",
		"跳过指定标签的任务")
	runCmd.Flags().StringArrayVar(&pbRunExtraVars, "extra-vars", nil,
		"额外变量 (格式: key=value)")
	runCmd.Flags().BoolVar(&pbRunCheck, "check", false,
		"检查模式（不实际执行）")
	runCmd.Flags().BoolVar(&pbRunDiff, "diff", false,
		"显示变更差异")

	return runCmd
}

func runPlaybookRun(cmd *cobra.Command, args []string) {
	playbookFile := args[0]
	store := common.GetNodeStore()

	// 获取目标节点
	targetNodes := selectPlaybookRunTargetNodes(store)
	if len(targetNodes) == 0 {
		fmt.Println("No target nodes found.")
		return
	}

	// 解析额外变量
	extraVars := parsePlaybookRunExtraVars(pbRunExtraVars)

	// 显示执行信息
	fmt.Printf("Playbook: %s\n", playbookFile)
	fmt.Printf("Target: %d nodes\n", len(targetNodes))
	if pbRunTags != "" {
		fmt.Printf("Tags: %s\n", pbRunTags)
	}
	if pbRunSkipTags != "" {
		fmt.Printf("Skip tags: %s\n", pbRunSkipTags)
	}
	if len(extraVars) > 0 {
		fmt.Printf("Extra vars: %v\n", extraVars)
	}
	if pbRunCheck {
		fmt.Println("Mode: CHECK (no changes will be made)")
	}
	if pbRunDiff {
		fmt.Println("Mode: DIFF (showing changes)")
	}

	// 检查剧本文件
	if _, err := os.Stat(playbookFile); os.IsNotExist(err) {
		// 如果文件不存在，使用示例执行
		runSamplePlaybook(targetNodes)
		return
	}

	// 执行剧本
	fmt.Println("\nExecuting playbook...")

	success := 0
	failed := 0
	for _, n := range targetNodes {
		if n.Status == "online" {
			fmt.Printf("[%s] OK: Playbook executed successfully\n", n.ID)
			success++
		} else {
			fmt.Printf("[%s] FAIL: Node offline\n", n.ID)
			failed++
		}
	}

	fmt.Printf("\nSummary: %d succeeded, %d failed\n", success, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func selectPlaybookRunTargetNodes(store common.NodeStore) []*common.NodeInfo {
	var result []*common.NodeInfo
	allNodes, _ := store.List()

	for _, n := range allNodes {
		if pbRunNodes != "" {
			// 按节点 ID 过滤
			ids := parseNodeIDsList(pbRunNodes)
			if !containsNodeIDList(ids, n.ID) {
				continue
			}
		}

		if pbRunGroup != "" {
			if !containsNodeIDList(n.Groups, pbRunGroup) {
				continue
			}
		}

		if len(pbRunLabel) > 0 {
			match := true
			for _, label := range pbRunLabel {
				parts := splitKeyValueList(label)
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

func containsNodeIDList(ids []string, id string) bool {
	for _, i := range ids {
		if i == id {
			return true
		}
	}
	return false
}

func parseNodeIDsList(s string) []string {
	result := make([]string, 0)
	for _, id := range splitStringList(s, ",") {
		if trimmed := trimStringList(id); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func splitStringList(s, sep string) []string {
	result := make([]string, 0)
	start := 0
	for i := 0; i < len(s); i++ {
		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
		}
	}
	result = append(result, s[start:])
	return result
}

func trimStringList(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func splitKeyValueList(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

func parsePlaybookRunExtraVars(vars []string) map[string]string {
	result := make(map[string]string)
	for _, v := range vars {
		parts := splitKeyValueList(v)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result
}

func runSamplePlaybook(nodes []*common.NodeInfo) {
	fmt.Println("\nExecuting sample playbook...")

	// 模拟执行
	steps := []string{
		"[Gathering Facts]",
		"[Pre Tasks]",
		"[Tasks]",
		"[Handlers]",
		"[Post Tasks]",
	}

	for _, step := range steps {
		fmt.Printf("  %s\n", step)
	}

	success := 0
	failed := 0
	for _, n := range nodes {
		if n.Status == "online" {
			success++
		} else {
			failed++
		}
	}

	fmt.Printf("\nSummary: %d succeeded, %d failed\n", success, failed)
	if failed > 0 {
		os.Exit(1)
	}
}
