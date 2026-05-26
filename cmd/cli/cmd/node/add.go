package node

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

// addFlags
var (
	addName      string
	addAddress   string
	addPort      int
	addUser      string
	addPassword  string
	addSSHKey    string
	addProxyJump string
	addGroups    string
	addLabels    []string
)

// NewAddCmd 创建添加节点命令
func NewAddCmd() *cobra.Command {
	addCmd := &cobra.Command{
		Use:   "add <node-id>",
		Short: "添加新节点",
		Long: `添加一个新节点到管理列表。

示例：
  # 基本用法
  owl node add node1 --name web-server-1 --address 192.168.1.10

  # 指定端口和用户
  owl node add node2 --name db-server-1 --address 192.168.1.20 --port 22 --user admin

  # 添加分组和多标签
  owl node add node3 --name app-server --address 192.168.1.30 \
    --groups web,production --labels env=prod,appname=owl,region=us-east

  # 使用 SSH 密钥认证
  owl node add node4 --name remote-server --address 192.168.1.40 \
    --ssh-key ~/.ssh/id_rsa --labels env=staging,tier=backend`,
		Args: cobra.ExactArgs(1),
		Run:  runAdd,
	}

	addCmd.Flags().StringVarP(&addName, "name", "n", "",
		"节点名称 (必需)")
	addCmd.Flags().StringVarP(&addAddress, "address", "a", "",
		"节点地址 IP (必需)")
	addCmd.Flags().IntVarP(&addPort, "port", "p", 22,
		"节点端口 (默认: 22)")
	addCmd.Flags().StringVarP(&addUser, "user", "u", "",
		"SSH 用户 (默认: 当前用户)")
	addCmd.Flags().StringVar(&addPassword, "password", "",
		"SSH 密码")
	addCmd.Flags().StringVar(&addSSHKey, "ssh-key", "",
		"SSH 私钥文件路径")
	addCmd.Flags().StringVar(&addProxyJump, "proxy-jump", "",
		"跳板机地址")
	addCmd.Flags().StringVar(&addGroups, "groups", "",
		"分组列表 (逗号分隔)")
	addCmd.Flags().StringSliceVarP(&addLabels, "labels", "l", nil,
		"标签 (格式: key=value)")
	addCmd.Flags().StringSliceVar(&addLabels, "label", nil,
		"标签 (格式: key=value) (alias)")

	_ = addCmd.MarkFlagRequired("name")
	_ = addCmd.MarkFlagRequired("address")

	return addCmd
}

func runAdd(cmd *cobra.Command, args []string) {
	nodeID := args[0]
	store := common.GetNodeStore()

	// 检查节点是否已存在
	if _, err := store.Get(nodeID); err == nil {
		fmt.Fprintf(os.Stderr, "Error: node already exists: %s\n", nodeID)
		os.Exit(1)
	}

	// 解析分组
	groups := []string{}
	if addGroups != "" {
		for _, g := range splitAndTrim(addGroups, ",") {
			if g != "" {
				groups = append(groups, g)
			}
		}
	}

	// 解析标签
	labels := make(map[string]string)
	for _, label := range addLabels {
		parts := splitAndTrim(label, "=")
		if len(parts) == 2 {
			labels[parts[0]] = parts[1]
		}
	}

	// 创建节点
	now := time.Now().Format(time.RFC3339)
	node := &common.NodeInfo{
		ID:        nodeID,
		Name:      addName,
		Address:   addAddress,
		Port:      addPort,
		User:      addUser,
		Password:  addPassword,
		SSHKey:    addSSHKey,
		ProxyJump: addProxyJump,
		Status:    "offline", // 新添加节点默认离线
		Groups:    groups,
		Labels:    labels,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// 保存节点
	if err := store.Add(node); err != nil {
		fmt.Fprintf(os.Stderr, "Error adding node: %v\n", err)
		os.Exit(1)
	}

	// 持久化到文件
	store.Save()

	fmt.Printf("✓ Node '%s' added successfully\n", nodeID)
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
	if len(node.Groups) > 0 {
		fmt.Printf("  Groups:  %s\n", joinStrings(node.Groups, ", "))
	}
	if len(node.Labels) > 0 {
		labelStr := make([]string, 0, len(node.Labels))
		for k, v := range node.Labels {
			labelStr = append(labelStr, fmt.Sprintf("%s=%s", k, v))
		}
		fmt.Printf("  Labels:  %s\n", joinStrings(labelStr, ", "))
	}
}

// Helper functions
func splitAndTrim(s string, sep string) []string {
	parts := make([]string, 0)
	for _, p := range split(s, sep) {
		if trimmed := trim(p); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

func split(s string, sep string) []string {
	result := make([]string, 0)
	start := 0
	for i := 0; i < len(s); i++ {
		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	result = append(result, s[start:])
	return result
}

func trim(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for _, s := range strs[1:] {
		result += sep + s
	}
	return result
}
