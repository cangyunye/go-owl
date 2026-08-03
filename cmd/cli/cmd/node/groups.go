package node

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/i18n"
)

// NewGroupsCmd 创建分组管理命令
func NewGroupsCmd() *cobra.Command {
	groupsCmd := &cobra.Command{
		Use:   "groups",
		Short: i18n.T("node.groups.short"),
		Long:  i18n.T("node.groups.long"),
	}

	groupsCmd.AddCommand(NewGroupsAddCmd())
	groupsCmd.AddCommand(NewGroupsRemoveCmd())
	groupsCmd.AddCommand(NewGroupsListCmd())
	groupsCmd.AddCommand(NewGroupsShowCmd())

	return groupsCmd
}

// NewGroupsAddCmd 添加分组
func NewGroupsAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <node-id> <group>",
		Short: i18n.T("node.groups.add.short"),
		Args:  cobra.ExactArgs(2),
		Run:   runGroupsAdd,
	}
}

func runGroupsAdd(cmd *cobra.Command, args []string) {
	nodeID, group := args[0], args[1]
	store := common.GetNodeStore()

	node, err := store.Get(nodeID)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("node.groups.err_get", err))
		os.Exit(1)
	}

	// 检查是否已在分组中
	for _, g := range node.Groups {
		if g == group {
			fmt.Printf("%s\n", i18n.T("node.groups.add.already", nodeID, group))
			return
		}
	}

	node.Groups = append(node.Groups, group)
	if err := store.Update(node); err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("node.groups.err_update", err))
		os.Exit(1)
	}

	store.Save()
	fmt.Printf("%s\n", i18n.T("node.groups.add.ok", nodeID, group))
}

// NewGroupsRemoveCmd 移除分组
func NewGroupsRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <node-id> <group>",
		Short: i18n.T("node.groups.remove.short"),
		Args:  cobra.ExactArgs(2),
		Run:   runGroupsRemove,
	}
}

func runGroupsRemove(cmd *cobra.Command, args []string) {
	nodeID, group := args[0], args[1]
	store := common.GetNodeStore()

	node, err := store.Get(nodeID)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("node.groups.err_get", err))
		os.Exit(1)
	}

	// 移除分组
	newGroups := make([]string, 0)
	found := false
	for _, g := range node.Groups {
		if g == group {
			found = true
		} else {
			newGroups = append(newGroups, g)
		}
	}

	if !found {
		fmt.Printf("%s\n", i18n.T("node.groups.remove.not_found", nodeID, group))
		return
	}

	node.Groups = newGroups
	if err := store.Update(node); err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("node.groups.err_update", err))
		os.Exit(1)
	}

	fmt.Printf("%s\n", i18n.T("node.groups.remove.ok", nodeID, group))
}

// NewGroupsListCmd 列出所有分组
func NewGroupsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: i18n.T("node.groups.list.short"),
		Run:   runGroupsList,
	}
}

func runGroupsList(cmd *cobra.Command, args []string) {
	store := common.GetNodeStore()
	nodes, err := store.List()
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("node.groups.err_list", err))
		os.Exit(1)
	}

	// 收集所有分组
	groupMap := make(map[string][]string)
	for _, n := range nodes {
		for _, g := range n.Groups {
			groupMap[g] = append(groupMap[g], n.ID)
		}
	}

	if len(groupMap) == 0 {
		fmt.Println(i18n.T("common.no_groups"))
		return
	}

	fmt.Println(i18n.T("node.groups.list.title"))
	fmt.Println("-------")
	for group, nodeIDs := range groupMap {
		fmt.Printf("%s\n", i18n.T("node.groups.list.item", group, len(nodeIDs), joinStrings(nodeIDs, ", ")))
	}
}

// NewGroupsShowCmd 显示分组详情
func NewGroupsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <group>",
		Short: i18n.T("node.groups.show.short"),
		Args:  cobra.ExactArgs(1),
		Run:   runGroupsShow,
	}
}

func runGroupsShow(cmd *cobra.Command, args []string) {
	group := args[0]
	store := common.GetNodeStore()
	nodes, err := store.List()
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("node.groups.err_list", err))
		os.Exit(1)
	}

	// 查找分组中的节点
	var groupNodes []*common.NodeInfo
	for _, n := range nodes {
		for _, g := range n.Groups {
			if g == group {
				groupNodes = append(groupNodes, n)
				break
			}
		}
	}

	if len(groupNodes) == 0 {
		fmt.Printf("%s\n", i18n.T("node.groups.show.empty", group))
		return
	}

	fmt.Printf("%s\n", i18n.T("node.groups.show.title", group, len(groupNodes)))
	fmt.Println("-------")
	for _, n := range groupNodes {
		fmt.Printf("%s\n", i18n.T("node.groups.show.item", n.ID, n.Name, n.Address, n.Port, n.Status))
	}
}
