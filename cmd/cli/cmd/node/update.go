package node

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

var (
	updateName      string
	updateAddress   string
	updatePort      int
	updateUser      string
	updatePassword  string
	updateSSHKey    string
	updateProxyJump string
	updateGroups    string
	updateLabels    []string
	updateStatus    string
)

func NewUpdateCmd() *cobra.Command {
	updateCmd := &cobra.Command{
		Use:   "update <node-id>",
		Short: "更新节点信息",
		Long: `更新已存在节点的信息，支持部分字段更新。

示例：
  # 更新节点名称
  owl node update node1 --name new-name

  # 更新认证信息
  owl node update node1 --password "new-password"

  # 更新连接信息
  owl node update node1 --address 10.0.0.1 --port 2222

  # 更新分组和多标签
  owl node update node1 --groups web,prod --labels env=prod,appname=owl,tier=backend

  # 更新状态
  owl node update node1 --status online`,
		Args: cobra.ExactArgs(1),
		Run:  runUpdate,
	}

	updateCmd.Flags().StringVarP(&updateName, "name", "n", "",
		"节点名称")
	updateCmd.Flags().StringVarP(&updateAddress, "address", "a", "",
		"节点地址 IP")
	updateCmd.Flags().IntVarP(&updatePort, "port", "p", 0,
		"节点端口")
	updateCmd.Flags().StringVarP(&updateUser, "user", "u", "",
		"SSH 用户")
	updateCmd.Flags().StringVar(&updatePassword, "password", "",
		"SSH 密码")
	updateCmd.Flags().StringVar(&updateSSHKey, "ssh-key", "",
		"SSH 私钥文件路径")
	updateCmd.Flags().StringVar(&updateProxyJump, "proxy-jump", "",
		"跳板机地址")
	updateCmd.Flags().StringVar(&updateGroups, "groups", "",
		"分组列表 (逗号分隔)")
	updateCmd.Flags().StringSliceVarP(&updateLabels, "labels", "l", nil,
		"标签 (格式: key=value)")
	updateCmd.Flags().StringSliceVar(&updateLabels, "label", nil,
		"标签 (格式: key=value) (alias)")
	updateCmd.Flags().StringVar(&updateStatus, "status", "",
		"节点状态 (online/offline)")

	return updateCmd
}

func runUpdate(cmd *cobra.Command, args []string) {
	nodeID := args[0]
	store := common.GetNodeStore().(*common.InMemoryNodeStore)

	node, err := store.Get(nodeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: node not found: %s\n", nodeID)
		os.Exit(1)
	}

	hasUpdate := false

	if updateName != "" {
		node.Name = updateName
		hasUpdate = true
	}
	if updateAddress != "" {
		node.Address = updateAddress
		hasUpdate = true
	}
	if updatePort > 0 {
		node.Port = updatePort
		hasUpdate = true
	}
	if updateUser != "" {
		node.User = updateUser
		hasUpdate = true
	}
	if updatePassword != "" {
		node.Password = updatePassword
		hasUpdate = true
	}
	if updateSSHKey != "" {
		node.SSHKey = updateSSHKey
		hasUpdate = true
	}
	if updateProxyJump != "" {
		node.ProxyJump = updateProxyJump
		hasUpdate = true
	}
	if updateGroups != "" {
		groups := []string{}
		for _, g := range splitAndTrim(updateGroups, ",") {
			if g != "" {
				groups = append(groups, g)
			}
		}
		node.Groups = groups
		hasUpdate = true
	}
	if len(updateLabels) > 0 {
		if node.Labels == nil {
			node.Labels = make(map[string]string)
		}
		for _, label := range updateLabels {
			parts := splitAndTrim(label, "=")
			if len(parts) == 2 {
				node.Labels[parts[0]] = parts[1]
			}
		}
		hasUpdate = true
	}
	if updateStatus != "" {
		if updateStatus != "online" && updateStatus != "offline" {
			fmt.Fprintf(os.Stderr, "Error: invalid status, must be 'online' or 'offline'\n")
			os.Exit(1)
		}
		node.Status = updateStatus
		hasUpdate = true
	}

	if !hasUpdate {
		fmt.Println("No fields to update. Use --help to see available options.")
		return
	}

	node.UpdatedAt = time.Now().Format(time.RFC3339)

	if err := store.Update(node); err != nil {
		fmt.Fprintf(os.Stderr, "Error updating node: %v\n", err)
		os.Exit(1)
	}

	store.Save()

	fmt.Printf("✓ Node '%s' updated successfully\n", nodeID)
	fmt.Printf("  Name:       %s\n", node.Name)
	fmt.Printf("  Address:    %s:%d\n", node.Address, node.Port)
	if node.User != "" {
		fmt.Printf("  User:       %s\n", node.User)
	}
	if node.Password != "" {
		fmt.Printf("  Password:   [已设置]\n")
	}
	if node.SSHKey != "" {
		fmt.Printf("  SSH Key:    %s\n", node.SSHKey)
	}
	if node.ProxyJump != "" {
		fmt.Printf("  ProxyJump:  %s\n", node.ProxyJump)
	}
	fmt.Printf("  Status:     %s\n", node.Status)
	if len(node.Groups) > 0 {
		fmt.Printf("  Groups:     %s\n", joinStrings(node.Groups, ", "))
	}
	if len(node.Labels) > 0 {
		labelStr := make([]string, 0, len(node.Labels))
		for k, v := range node.Labels {
			labelStr = append(labelStr, fmt.Sprintf("%s=%s", k, v))
		}
		fmt.Printf("  Labels:     %s\n", joinStrings(labelStr, ", "))
	}
}
