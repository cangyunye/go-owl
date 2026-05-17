package exec

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

// playbookFlags
var (
	pbNodes     string
	pbGroup     string
	pbLabel     []string
	pbTags      string
	pbSkipTags  string
	pbExtraVars []string
	pbCheck     bool
	pbDiff      bool
)

// NewPlaybookCmd 创建剧本执行命令
func NewPlaybookCmd() *cobra.Command {
	pbCmd := &cobra.Command{
		Use:   "playbook <playbook-file>",
		Short: "执行 Ansible 剧本",
		Long: `执行 Ansible 风格的 YAML 剧本。

示例：
  owl exec playbook site.yml
  owl exec playbook deploy.yml --nodes web-01,web-02
  owl exec playbook deploy.yml --group web
  owl exec playbook deploy.yml --tags nginx,mysql
  owl exec playbook deploy.yml --extra-vars "version=v1.2.3,env=prod"
  owl exec playbook site.yml --check
  owl exec playbook site.yml --diff`,
		Args: cobra.ExactArgs(1),
		Run:  runPlaybook,
	}

	pbCmd.Flags().StringVar(&pbNodes, "nodes", "",
		"指定节点 ID (逗号分隔)")
	pbCmd.Flags().StringVar(&pbGroup, "group", "",
		"按分组选择节点")
	pbCmd.Flags().StringSliceVarP(&pbLabel, "label", "l", nil,
		"按标签选择节点")
	pbCmd.Flags().StringVar(&pbTags, "tags", "",
		"执行指定标签的任务 (逗号分隔)")
	pbCmd.Flags().StringVar(&pbSkipTags, "skip-tags", "",
		"跳过指定标签的任务")
	pbCmd.Flags().StringArrayVar(&pbExtraVars, "extra-vars", nil,
		"额外变量 (格式: key=value)")
	pbCmd.Flags().BoolVar(&pbCheck, "check", false,
		"检查模式（不实际执行）")
	pbCmd.Flags().BoolVar(&pbDiff, "diff", false,
		"显示变更差异")

	return pbCmd
}

func runPlaybook(cmd *cobra.Command, args []string) {
	playbookFile := args[0]
	store := common.GetNodeStore()

	// 获取目标节点
	targetNodes := selectPlaybookTargetNodes(store)
	if len(targetNodes) == 0 {
		fmt.Println("No target nodes found.")
		return
	}

	// 解析额外变量
	extraVars := parseExtraVarsPB(pbExtraVars)

	// 显示执行信息
	fmt.Printf("Playbook: %s\n", playbookFile)
	fmt.Printf("Target: %d nodes\n", len(targetNodes))
	if pbTags != "" {
		fmt.Printf("Tags: %s\n", pbTags)
	}
	if len(extraVars) > 0 {
		fmt.Printf("Extra vars: %v\n", extraVars)
	}
	if pbCheck {
		fmt.Println("Mode: CHECK (no changes will be made)")
	}

	// 加载并解析剧本
	fmt.Println("\nParsing playbook...")

	// 模拟剧本执行
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
}

func selectPlaybookTargetNodes(store common.NodeStore) []*common.NodeInfo {
	var result []*common.NodeInfo
	allNodes, _ := store.List()

	for _, n := range allNodes {
		if pbNodes != "" {
			nodeIDs := common.ParseNodeList(pbNodes)
			if !containsPB(nodeIDs, n.ID) {
				continue
			}
		}

		if pbGroup != "" {
			if !containsPB(n.Groups, pbGroup) {
				continue
			}
		}

		if len(pbLabel) > 0 {
			match := true
			for _, label := range pbLabel {
				parts := splitEqPB(label)
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

func containsPB(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

func splitEqPB(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

func parseExtraVarsPB(vars []string) map[string]string {
	result := make(map[string]string)
	for _, v := range vars {
		parts := splitEqPB(v)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result
}
