package node

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/common/model"
)

var (
	listFormat  string
	listGroup   string
	listLabel   []string
	listStatus  string
	listNoColor bool
)

// NewListCmd 创建节点列表命令
func NewListCmd() *cobra.Command {
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "列出所有节点",
		Long: `列出所有已注册的节点，支持按分组、标签、状态过滤。

示例：
  owl node list                           # 列出所有节点
  owl node list --group web               # 列出 web 分组的节点
  owl node list --label env=prod          # 列出 env=prod 的节点
  owl node list --status online           # 列出在线节点
  owl node list -o json                   # JSON 格式输出`,
		Run: runList,
	}

	listCmd.Flags().StringVarP(&listFormat, "output", "o", "table",
		"输出格式: table, json, yaml")
	listCmd.Flags().StringVar(&listGroup, "group", "",
		"按分组过滤")
	listCmd.Flags().StringSliceVarP(&listLabel, "label", "l", nil,
		"按标签过滤 (格式: key=value)")
	listCmd.Flags().StringVar(&listStatus, "status", "",
		"按状态过滤: online, offline, unknown")
	listCmd.Flags().BoolVar(&listNoColor, "no-color", false,
		"禁用颜色输出")

	return listCmd
}

func runList(cmd *cobra.Command, args []string) {
	store := common.GetNodeStore()
	formatter := common.NewOutputFormatter(listFormat, !listNoColor)

	// 获取所有节点
	allNodes, err := store.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing nodes: %v\n", err)
		os.Exit(1)
	}

	// 过滤节点
	nodes := filterNodes(allNodes)

	// 转换为 model.Node 格式
	modelNodes := toModelNodes(nodes)

	// 输出
	formatter.FormatNodes(modelNodes)
}

func filterNodes(nodes []*common.NodeInfo) []*common.NodeInfo {
	filtered := make([]*common.NodeInfo, 0)

	for _, n := range nodes {
		// 按分组过滤
		if listGroup != "" {
			if !containsGroup(n.Groups, listGroup) {
				continue
			}
		}

		// 按标签过滤
		if len(listLabel) > 0 {
			match := true
			for _, label := range listLabel {
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
		if listStatus != "" {
			if strings.ToLower(n.Status) != strings.ToLower(listStatus) {
				continue
			}
		}

		filtered = append(filtered, n)
	}

	return filtered
}

func containsGroup(groups []string, group string) bool {
	for _, g := range groups {
		if g == group {
			return true
		}
	}
	return false
}

func toModelNodes(nodes []*common.NodeInfo) []*model.Node {
	result := make([]*model.Node, len(nodes))
	for i, n := range nodes {
		result[i] = &model.Node{
			ID:      n.ID,
			Name:    n.Name,
			Address: n.Address,
			Port:    n.Port,
			User:    n.User,
			Status:  model.NodeStatus(n.Status),
			Groups:  n.Groups,
			Labels:  n.Labels,
		}
	}
	return result
}

func toModelNode(n *common.NodeInfo) *model.Node {
	return &model.Node{
		ID:      n.ID,
		Name:    n.Name,
		Address: n.Address,
		Port:    n.Port,
		User:    n.User,
		Status:  model.NodeStatus(n.Status),
		Groups:  n.Groups,
		Labels:  n.Labels,
	}
}
