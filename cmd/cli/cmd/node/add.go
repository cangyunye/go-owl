package node

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/i18n"
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
		Short: i18n.T("node.add.short"),
		Long:  i18n.T("node.add.long"),
		Args:  cobra.ExactArgs(1),
		Run:   runAdd,
	}

	addCmd.Flags().StringVarP(&addName, "name", "n", "",
		i18n.T("node.add.flag_name"))
	addCmd.Flags().StringVarP(&addAddress, "address", "a", "",
		i18n.T("node.add.flag_address"))
	addCmd.Flags().IntVarP(&addPort, "port", "p", 22,
		i18n.T("node.add.flag_port"))
	addCmd.Flags().StringVarP(&addUser, "user", "u", "",
		i18n.T("node.add.flag_user"))
	addCmd.Flags().StringVarP(&addPassword, "password", "P", "",
		i18n.T("node.add.flag_password"))
	addCmd.Flags().StringVar(&addSSHKey, "ssh-key", "",
		i18n.T("node.add.flag_ssh_key"))
	addCmd.Flags().StringVar(&addProxyJump, "proxy-jump", "",
		i18n.T("node.add.flag_proxy_jump"))
	addCmd.Flags().StringVar(&addGroups, "groups", "",
		i18n.T("node.add.flag_groups"))
	addCmd.Flags().StringSliceVarP(&addLabels, "labels", "l", nil,
		i18n.T("node.add.flag_labels"))
	addCmd.Flags().StringSliceVar(&addLabels, "label", nil,
		i18n.T("node.add.flag_labels_alias"))

	_ = addCmd.MarkFlagRequired("name")
	_ = addCmd.MarkFlagRequired("address")

	return addCmd
}

func runAdd(cmd *cobra.Command, args []string) {
	nodeID := args[0]
	store := common.GetNodeStore()

	// 检查节点是否已存在
	if _, err := store.Get(nodeID); err == nil {
		fmt.Fprintln(os.Stderr, i18n.T("node.add.err_exists", nodeID))
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
		fmt.Fprintln(os.Stderr, i18n.T("node.add.err_add", err))
		os.Exit(1)
	}

	// 持久化到文件
	store.Save()

	fmt.Printf("%s\n", i18n.T("node.add.ok", nodeID))
	fmt.Printf("%s\n", i18n.T("node.add.field_name", node.Name))
	fmt.Printf("%s\n", i18n.T("node.add.field_address", node.Address, i18n.F(node.Port)))
	if node.User != "" {
		fmt.Printf("%s\n", i18n.T("node.add.field_user", node.User))
	}
	if node.Password != "" {
		fmt.Println(i18n.T("node.add.field_password"))
	}
	if node.SSHKey != "" {
		fmt.Printf("%s\n", i18n.T("node.add.field_ssh_key", node.SSHKey))
	}
	if node.ProxyJump != "" {
		fmt.Printf("%s\n", i18n.T("node.add.field_proxyjump", node.ProxyJump))
	}
	if len(node.Groups) > 0 {
		fmt.Printf("%s\n", i18n.T("node.add.field_groups", joinStrings(node.Groups, ", ")))
	}
	if len(node.Labels) > 0 {
		labelStr := make([]string, 0, len(node.Labels))
		for k, v := range node.Labels {
			labelStr = append(labelStr, fmt.Sprintf("%s=%s", k, v))
		}
		fmt.Printf("%s\n", i18n.T("node.add.field_labels", joinStrings(labelStr, ", ")))
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
