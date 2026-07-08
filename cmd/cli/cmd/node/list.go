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
	listGroup   []string
	listLabel   []string
	listStatus  string
	listNoColor bool
	listHeader  string
)

// NewListCmd 创建节点列表命令
func NewListCmd() *cobra.Command {
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "列出所有节点",
		Long: `列出所有已注册的节点，支持按分组、标签、状态过滤。

可用字段：
  id, name, address, port, user, status, groups, labels, last_check

示例：
  owl node list                           # 列出所有节点
  owl node list --groups web              # 列出 web 分组的节点
  owl node list --label env=prod          # 列出 env=prod 的节点
  owl node list --status online           # 列出在线节点
  owl node list -o json                   # JSON 格式输出
  owl node list --header id,address       # 只显示 id 和 address 列
  owl node list --header id,name,labels:60  # 只显示3列，labels列宽度60
  owl node list --header *                # 显示默认8个字段
  owl node list --header *,id              # 默认字段 + id放最后
  owl node list --header labels:60,*        # labels先显示，然后其他默认字段`,
		Run: runList,
	}

	listCmd.Flags().StringVarP(&listFormat, "format", "o", "table",
		"输出格式: table, json, yaml")
	listCmd.Flags().StringSliceVarP(&listGroup, "groups", "g", nil, "按分组过滤 (多个分组用逗号分隔或多次使用 -g)")
	listCmd.Flags().StringSliceVar(&listGroup, "group", nil, "(已废弃，请使用 --groups)")
	listCmd.Flags().MarkHidden("group")
	listCmd.Flags().StringSliceVarP(&listLabel, "label", "l", nil,
		"按标签过滤 (格式: key=value)")
	listCmd.Flags().StringVarP(&listStatus, "status", "S", "",
		"按状态过滤: online, offline, unknown")
	listCmd.Flags().BoolVarP(&listNoColor, "no-color", "C", false,
		"禁用颜色输出")
	listCmd.Flags().StringVar(&listHeader, "header", "",
		"自定义显示字段和宽度 (格式: id,address,labels:60,*,id)")

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

	// 解析自定义字段
	var fields []common.HeaderField
	if listHeader != "" {
		fields = common.ParseHeaderFields(listHeader)
		if len(fields) == 0 {
			fmt.Fprintf(os.Stderr, "Invalid header format: %s\n", listHeader)
			os.Exit(1)
		}
	}

	// 输出
	if len(fields) > 0 {
		formatter.FormatNodesWithFields(modelNodes, fields)
	} else {
		formatter.FormatNodes(modelNodes)
	}
}

func filterNodes(nodes []*common.NodeInfo) []*common.NodeInfo {
	filtered := make([]*common.NodeInfo, 0)

	for _, n := range nodes {
		// 按分组过滤
		if len(listGroup) > 0 {
			if !containsAnyGroup(n.Groups, listGroup) {
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

func containsAnyGroup(groups []string, targets []string) bool {
	if len(targets) == 0 {
		return true
	}
	for _, g := range groups {
		for _, t := range targets {
			if g == t {
				return true
			}
		}
	}
	return false
}

func toModelNodes(nodes []*common.NodeInfo) []*model.Node {
	result := make([]*model.Node, len(nodes))
	for i, n := range nodes {
		result[i] = &model.Node{
			ID:          n.ID,
			Name:        n.Name,
			Address:     n.Address,
			Port:        n.Port,
			User:        n.User,
			Status:      model.NodeStatus(n.Status),
			Groups:      n.Groups,
			Labels:      n.Labels,
			LastCheckAt: n.LastCheckAt,
		}
	}
	return result
}

func toModelNode(n *common.NodeInfo) *model.Node {
	return &model.Node{
		ID:          n.ID,
		Name:        n.Name,
		Address:     n.Address,
		Port:        n.Port,
		User:        n.User,
		Status:      model.NodeStatus(n.Status),
		Groups:      n.Groups,
		Labels:      n.Labels,
		LastCheckAt: n.LastCheckAt,
	}
}
