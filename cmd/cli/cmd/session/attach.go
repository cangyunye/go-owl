package session

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/i18n"
	"github.com/cangyunye/go-owl/internal/session"
	"github.com/cangyunye/go-owl/internal/ssh"
	"github.com/spf13/cobra"
	gossh "golang.org/x/crypto/ssh"
)

var (
	attachNodes     string
	attachSSHConfig string
	attachKeyFile   string
)

func NewAttachCmd() *cobra.Command {
	attachCmd := &cobra.Command{
		Use:   "attach [node-id]",
		Short: i18n.T("session.attach.short"),
		Long:  i18n.T("session.attach.long"),
		Args:  cobra.MaximumNArgs(1),
		RunE:  runAttach,
	}

	attachCmd.Flags().StringVarP(&attachNodes, "nodes", "N", "",
		i18n.T("session.attach.flag_nodes"))
	attachCmd.Flags().StringVar(&attachSSHConfig, "ssh-config", "",
		i18n.T("session.attach.flag_ssh_config"))
	attachCmd.Flags().StringVar(&attachKeyFile, "key", "",
		i18n.T("session.attach.flag_key"))
	attachCmd.Flags().StringVar(&sessionTimeout, "timeout", "30m",
		i18n.T("session.attach.flag_timeout"))

	return attachCmd
}

func runAttach(cmd *cobra.Command, args []string) error {
	var nodeIDs []string
	var mode session.SessionMode

	// 解析节点
	if attachNodes != "" {
		nodeIDs = strings.Split(attachNodes, ",")
		for i := range nodeIDs {
			nodeIDs[i] = strings.TrimSpace(nodeIDs[i])
		}
		mode = session.SessionModeMultiple
	} else if len(args) > 0 {
		nodeIDs = []string{args[0]}
		mode = session.SessionModeSingle
	} else {
		return errors.New(i18n.T("session.attach.err_no_node"))
	}

	// 解析超时时间
	timeout, err := time.ParseDuration(sessionTimeout)
	if err != nil {
		return fmt.Errorf(i18n.Raw("session.attach.err_invalid_timeout"), err)
	}

	// 创建会话
	sess := session.NewSession(mode, nodeIDs, timeout)

	// 准备节点配置
	nodeConfigs, err := prepareNodeConfigs(nodeIDs)
	if err != nil {
		return fmt.Errorf(i18n.Raw("session.attach.err_prepare_config"), err)
	}

	// 连接
	fmt.Printf("%s", i18n.T("session.attach.connecting", i18n.F(len(nodeIDs))))
	if err := sess.Connect(nodeConfigs); err != nil {
		return fmt.Errorf(i18n.Raw("session.attach.err_connect"), err)
	}

	// 显示欢迎信息
	printWelcome(sess, len(nodeIDs))

	// 设置信号处理
	go sess.WaitForSignal()

	// 运行交互循环
	interactor := session.NewInteractiveLoop(sess)
	if err := interactor.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("session.attach.err_interactive", err))
	}

	// 关闭会话
	sess.Close()
	sess.PrintSummary()

	return nil
}

// prepareNodeConfigs 准备节点配置
func prepareNodeConfigs(nodeIDs []string) ([]*session.NodeConfig, error) {
	var configs []*session.NodeConfig

	configManager, err := getSSHConfigManager()
	if err != nil {
		return nil, err
	}

	for _, nodeID := range nodeIDs {
		config := parseNodeID(nodeID)

		if nodeInfo, err := getNodeInfo(nodeID); err == nil {
			if nodeInfo.Address != "" {
				config.Address = nodeInfo.Address
			}
			if nodeInfo.Port > 0 {
				config.Port = nodeInfo.Port
			}
			if nodeInfo.User != "" {
				config.User = nodeInfo.User
			}
			if nodeInfo.Password != "" {
				config.Password = nodeInfo.Password
			}
			if nodeInfo.SSHKey != "" {
				config.SSHKey = nodeInfo.SSHKey
			}
			if nodeInfo.ProxyJump != "" {
				config.ProxyJump = nodeInfo.ProxyJump
			}
		}

		var sshConfig *ssh.SSHConfig
		if configManager != nil {
			if cfg := configManager.GetConfig(nodeID); cfg != nil {
				sshConfig = cfg
			} else if cfg := configManager.GetConfig(config.Address); cfg != nil {
				sshConfig = cfg
			}
		}

		if sshConfig != nil {
			fmt.Printf("%s", i18n.T("session.attach.found_ssh_config", nodeID, sshConfig.HostName))

			if config.User == "" && sshConfig.User != "" {
				config.User = sshConfig.User
			}
			if sshConfig.Port > 0 {
				config.Port = sshConfig.Port
			}
			if sshConfig.HostName != "" {
				config.Address = sshConfig.HostName
			}
			if config.SSHKey == "" && sshConfig.IdentityFile != "" {
				config.SSHKey = sshConfig.IdentityFile
			}
		}

		authMethods, err := getAuthMethodsWithConfig(sshConfig, config.Password, config.SSHKey)
		if err != nil {
			return nil, err
		}
		config.Auth = authMethods

		configs = append(configs, config)
	}

	return configs, nil
}

func getNodeInfo(nodeID string) (*common.NodeInfo, error) {
	store := common.GetNodeStore()
	return store.Get(nodeID)
}

// getSSHConfigManager 获取 SSH 配置管理器
func getSSHConfigManager() (*ssh.ConfigManager, error) {
	if attachSSHConfig != "" {
		return ssh.NewConfigManagerWithPath(attachSSHConfig)
	}
	return ssh.NewConfigManager()
}

// parseNodeID 解析节点 ID
func parseNodeID(nodeID string) *session.NodeConfig {
	// 默认配置
	config := &session.NodeConfig{
		ID:      nodeID,
		Address: nodeID,
		Port:    22,
		User:    "root",
	}

	// 解析 user@host:port 格式
	parts := strings.Split(nodeID, "@")
	if len(parts) == 2 {
		config.User = parts[0]
		hostPort := parts[1]

		hostParts := strings.Split(hostPort, ":")
		config.Address = hostParts[0]
		if len(hostParts) == 2 {
			fmt.Sscanf(hostParts[1], "%d", &config.Port)
		}
	}

	return config
}

// getAuthMethods 获取认证方法
func getAuthMethods() ([]gossh.AuthMethod, error) {
	return getAuthMethodsWithConfig(nil, "", "")
}

// getAuthMethodsWithConfig 根据 SSH 配置获取认证方法
func getAuthMethodsWithConfig(sshConfig *ssh.SSHConfig, password, sshKey string) ([]gossh.AuthMethod, error) {
	var authMethods []gossh.AuthMethod

	if password != "" {
		authMethods = append(authMethods, gossh.Password(password))
	}

	if sshKey != "" {
		if _, err := os.Stat(sshKey); err == nil {
			auth, err := publicKeyAuth(sshKey)
			if err == nil {
				authMethods = append(authMethods, auth)
			}
		}
	}

	if sshConfig != nil && sshConfig.IdentityFile != "" {
		if _, err := os.Stat(sshConfig.IdentityFile); err == nil {
			auth, err := publicKeyAuth(sshConfig.IdentityFile)
			if err == nil {
				authMethods = append(authMethods, auth)
			}
		}
	}

	if sshKey == "" && (sshConfig == nil || sshConfig.IdentityFile == "") {
		home, err := os.UserHomeDir()
		if err == nil {
			defaultKeys := []string{
				filepath.Join(home, ".ssh", "id_rsa"),
				filepath.Join(home, ".ssh", "id_ed25519"),
				filepath.Join(home, ".ssh", "id_ecdsa"),
			}

			for _, keyFile := range defaultKeys {
				if _, err := os.Stat(keyFile); err == nil {
					auth, err := publicKeyAuth(keyFile)
					if err == nil {
						authMethods = append(authMethods, auth)
						break
					}
				}
			}
		}
	}

	if len(authMethods) == 0 {
		return nil, errors.New(i18n.T("session.attach.err_no_auth"))
	}

	return authMethods, nil
}

// publicKeyAuth 公钥认证
func publicKeyAuth(keyFile string) (gossh.AuthMethod, error) {
	key, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf(i18n.Raw("session.attach.err_read_key"), err)
	}

	signer, err := gossh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf(i18n.Raw("session.attach.err_parse_key"), err)
	}

	return gossh.PublicKeys(signer), nil
}

// sshAgentAuth SSH Agent 认证
func sshAgentAuth() gossh.AuthMethod {
	// 简化实现：返回 nil
	// 实际应该连接到 SSH Agent
	return nil
}

// printWelcome 显示欢迎信息
func printWelcome(sess *session.Session, nodeCount int) {
	fmt.Println("─────────────────────────────────────")
	fmt.Printf("%s", i18n.T("session.attach.connected_count", i18n.F(nodeCount)))
	fmt.Printf("%s", i18n.T("session.attach.session_id", sess.ID))
	fmt.Printf("%s", i18n.T("session.attach.session_timeout", sess.Timeout.String()))
	fmt.Println("─────────────────────────────────────")
	fmt.Println()
	fmt.Println(i18n.T("session.attach.help_title"))
	fmt.Println(i18n.T("session.attach.help_help"))
	fmt.Println(i18n.T("session.attach.help_exit"))
	fmt.Println(i18n.T("session.attach.help_status"))
	fmt.Println(i18n.T("session.attach.help_clear"))
	fmt.Println(i18n.T("session.attach.help_broadcast"))
	fmt.Println()
	fmt.Println(i18n.T("session.attach.hint_local"))
	fmt.Println(i18n.T("session.attach.hint_remote"))
	fmt.Println()
}
