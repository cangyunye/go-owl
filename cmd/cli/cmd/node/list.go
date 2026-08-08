package node

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/settings"
	"github.com/cangyunye/go-owl/internal/common/model"
	"github.com/cangyunye/go-owl/internal/i18n"
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
		Short: i18n.T("node.list.short"),
		Long:  i18n.T("node.list.long"),
		Run:   runList,
	}

	listCmd.Flags().StringVarP(&listFormat, "format", "o", "table",
		i18n.T("node.list.flag_format"))
	listCmd.Flags().StringSliceVarP(&listGroup, "groups", "g", nil, i18n.T("node.list.flag_groups"))
	listCmd.Flags().StringSliceVar(&listGroup, "group", nil, i18n.T("node.list.flag_group_deprecated"))
	listCmd.Flags().MarkHidden("group")
	listCmd.Flags().StringSliceVarP(&listLabel, "label", "l", nil,
		i18n.T("node.list.flag_label"))
	listCmd.Flags().StringVarP(&listStatus, "status", "S", "",
		i18n.T("node.list.flag_status"))
	listCmd.Flags().BoolVarP(&listNoColor, "no-color", "C", false,
		i18n.T("node.list.flag_no_color"))
	listCmd.Flags().StringVar(&listHeader, "header", "",
		i18n.T("node.list.flag_header"))

	return listCmd
}

func runList(cmd *cobra.Command, args []string) {
	// 从 owl settings 加载未显式指定的 flag 默认值
	applyListSettingsFallback(cmd)

	store := common.GetNodeStore()
	formatter := common.NewOutputFormatter(listFormat, !listNoColor)

	// 获取所有节点
	allNodes, err := store.List()
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("node.list.err_list", err))
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
			fmt.Fprintln(os.Stderr, i18n.T("node.list.err_header", listHeader))
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

// applyListSettingsFallback 从 owl settings 加载未显式指定的 node list flag 默认值
func applyListSettingsFallback(cmd *cobra.Command) {
	s := settings.GetCurrentSettings()

	// --groups: 如果用户未指定，使用 settings 中的 default.group 或 target.groups
	if !cmd.Flags().Changed("groups") {
		group := s.Default.Group
		if group == "" {
			group = s.Target.Groups
		}
		if group != "" {
			listGroup = strings.Split(group, ",")
		}
	}

	// --format: 如果用户未指定，使用 settings 中的 output.format
	if !cmd.Flags().Changed("format") && s.Output.Format != "" {
		listFormat = s.Output.Format
	}

	// --no-color: 如果用户未指定，从 settings output.color 取反
	if !cmd.Flags().Changed("no-color") {
		listNoColor = !s.Output.Color
	}
}
