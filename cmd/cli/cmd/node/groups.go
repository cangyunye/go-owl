package node

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

// NewGroupsCmd 创建分组管理命令
func NewGroupsCmd() *cobra.Command {
	groupsCmd := &cobra.Command{
		Use:   "groups",
		Short: "管理节点分组",
		Long: `管理节点的分组，支持添加、删除、列出分组。

示例：
  owl node groups add node1 web
  owl node groups remove node1 web
  owl node groups list
  owl node groups show web`,
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
		Short: "添加节点到分组",
		Args:  cobra.ExactArgs(2),
		Run:   runGroupsAdd,
	}
}

func runGroupsAdd(cmd *cobra.Command, args []string) {
	nodeID, group := args[0], args[1]
	store := common.GetNodeStore()

	node, err := store.Get(nodeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// 检查是否已在分组中
	for _, g := range node.Groups {
		if g == group {
			fmt.Printf("Node '%s' is already in group '%s'\n", nodeID, group)
			return
		}
	}

	node.Groups = append(node.Groups, group)
	if err := store.Update(node); err != nil {
		fmt.Fprintf(os.Stderr, "Error updating node: %v\n", err)
		os.Exit(1)
	}

	store.Save()
	fmt.Printf("Node '%s' added to group '%s'\n", nodeID, group)
}

// NewGroupsRemoveCmd 移除分组
func NewGroupsRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <node-id> <group>",
		Short: "从分组移除节点",
		Args:  cobra.ExactArgs(2),
		Run:   runGroupsRemove,
	}
}

func runGroupsRemove(cmd *cobra.Command, args []string) {
	nodeID, group := args[0], args[1]
	store := common.GetNodeStore()

	node, err := store.Get(nodeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
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
		fmt.Printf("Node '%s' is not in group '%s'\n", nodeID, group)
		return
	}

	node.Groups = newGroups
	if err := store.Update(node); err != nil {
		fmt.Fprintf(os.Stderr, "Error updating node: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Node '%s' removed from group '%s'\n", nodeID, group)
}

// NewGroupsListCmd 列出所有分组
func NewGroupsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "列出所有分组",
		Run:   runGroupsList,
	}
}

func runGroupsList(cmd *cobra.Command, args []string) {
	store := common.GetNodeStore()
	nodes, err := store.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing nodes: %v\n", err)
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
		fmt.Println("No groups found.")
		return
	}

	fmt.Println("Groups:")
	fmt.Println("-------")
	for group, nodeIDs := range groupMap {
		fmt.Printf("  %s: %d nodes [%s]\n", group, len(nodeIDs), joinStrings(nodeIDs, ", "))
	}
}

// NewGroupsShowCmd 显示分组详情
func NewGroupsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <group>",
		Short: "显示分组中的节点",
		Args:  cobra.ExactArgs(1),
		Run:   runGroupsShow,
	}
}

func runGroupsShow(cmd *cobra.Command, args []string) {
	group := args[0]
	store := common.GetNodeStore()
	nodes, err := store.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing nodes: %v\n", err)
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
		fmt.Printf("No nodes in group '%s'\n", group)
		return
	}

	fmt.Printf("Group: %s (%d nodes)\n", group, len(groupNodes))
	fmt.Println("-------")
	for _, n := range groupNodes {
		fmt.Printf("  %s (%s) - %s:%d [%s]\n", n.ID, n.Name, n.Address, n.Port, n.Status)
	}
}
