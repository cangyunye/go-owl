package node

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/i18n"
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
		Short: i18n.T("node.update.short"),
		Long:  i18n.T("node.update.long"),
		Args:  cobra.ExactArgs(1),
		Run:   runUpdate,
	}

	updateCmd.Flags().StringVarP(&updateName, "name", "n", "",
		i18n.T("node.update.flag_name"))
	updateCmd.Flags().StringVarP(&updateAddress, "address", "a", "",
		i18n.T("node.update.flag_address"))
	updateCmd.Flags().IntVarP(&updatePort, "port", "p", 0,
		i18n.T("node.update.flag_port"))
	updateCmd.Flags().StringVarP(&updateUser, "user", "u", "",
		i18n.T("node.update.flag_user"))
	updateCmd.Flags().StringVarP(&updatePassword, "password", "P", "",
		i18n.T("node.update.flag_password"))
	updateCmd.Flags().StringVar(&updateSSHKey, "ssh-key", "",
		i18n.T("node.update.flag_ssh_key"))
	updateCmd.Flags().StringVar(&updateProxyJump, "proxy-jump", "",
		i18n.T("node.update.flag_proxy_jump"))
	updateCmd.Flags().StringVar(&updateGroups, "groups", "",
		i18n.T("node.update.flag_groups"))
	updateCmd.Flags().StringSliceVarP(&updateLabels, "labels", "l", nil,
		i18n.T("node.update.flag_labels"))
	updateCmd.Flags().StringSliceVar(&updateLabels, "label", nil,
		i18n.T("node.update.flag_labels_alias"))
	updateCmd.Flags().StringVarP(&updateStatus, "status", "S", "",
		i18n.T("node.update.flag_status"))

	return updateCmd
}

func runUpdate(cmd *cobra.Command, args []string) {
	nodeID := args[0]
	store := common.GetNodeStore()

	node, err := store.Get(nodeID)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("node.update.err_not_found", nodeID))
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
			fmt.Fprintln(os.Stderr, i18n.T("node.update.err_invalid_status"))
			os.Exit(1)
		}
		node.Status = updateStatus
		hasUpdate = true
	}

	if !hasUpdate {
		fmt.Println(i18n.T("node.update.err_no_fields"))
		return
	}

	node.UpdatedAt = time.Now().Format(time.RFC3339)

	if err := store.Update(node); err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("node.update.err_update", err))
		os.Exit(1)
	}

	store.Save()

	fmt.Printf("%s\n", i18n.T("node.update.ok", nodeID))
	fmt.Printf("%s\n", i18n.T("node.update.field_name", node.Name))
	fmt.Printf("%s\n", i18n.T("node.update.field_address", node.Address, node.Port))
	if node.User != "" {
		fmt.Printf("%s\n", i18n.T("node.update.field_user", node.User))
	}
	if node.Password != "" {
		fmt.Println(i18n.T("node.update.field_password"))
	}
	if node.SSHKey != "" {
		fmt.Printf("%s\n", i18n.T("node.update.field_ssh_key", node.SSHKey))
	}
	if node.ProxyJump != "" {
		fmt.Printf("%s\n", i18n.T("node.update.field_proxyjump", node.ProxyJump))
	}
	fmt.Printf("%s\n", i18n.T("node.update.field_status", node.Status))
	if len(node.Groups) > 0 {
		fmt.Printf("%s\n", i18n.T("node.update.field_groups", joinStrings(node.Groups, ", ")))
	}
	if len(node.Labels) > 0 {
		labelStr := make([]string, 0, len(node.Labels))
		for k, v := range node.Labels {
			labelStr = append(labelStr, fmt.Sprintf("%s=%s", k, v))
		}
		fmt.Printf("%s\n", i18n.T("node.update.field_labels", joinStrings(labelStr, ", ")))
	}
}
