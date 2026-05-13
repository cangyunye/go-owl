package session

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

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
		Short: "连接到交互式会话",
		Long:  `建立持久 SSH 会话，支持单节点实时交互和多节点批量管理`,
		Args:  cobra.MaximumNArgs(1),
		RunE:  runAttach,
	}

	attachCmd.Flags().StringVar(&attachNodes, "nodes", "",
		"多节点模式，指定节点列表（逗号分隔）")
	attachCmd.Flags().StringVar(&attachSSHConfig, "ssh-config", "",
		"SSH config 路径（默认: ~/.ssh/config）")
	attachCmd.Flags().StringVar(&attachKeyFile, "key", "",
		"SSH 私钥文件路径")
	attachCmd.Flags().StringVar(&sessionTimeout, "timeout", "30m",
		"会话超时时间（如: 30m, 1h）")

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
		return fmt.Errorf("请指定节点: <node-id> 或 --nodes <node1>,<node2>")
	}

	// 解析超时时间
	timeout, err := time.ParseDuration(sessionTimeout)
	if err != nil {
		return fmt.Errorf("无效的超时时间: %w", err)
	}

	// 创建会话
	sess := session.NewSession(mode, nodeIDs, timeout)

	// 准备节点配置
	nodeConfigs, err := prepareNodeConfigs(nodeIDs)
	if err != nil {
		return fmt.Errorf("准备节点配置失败: %w", err)
	}

	// 连接
	fmt.Printf("正在连接到 %d 个节点...\n", len(nodeIDs))
	if err := sess.Connect(nodeConfigs); err != nil {
		return fmt.Errorf("连接失败: %w", err)
	}

	// 显示欢迎信息
	printWelcome(sess, len(nodeIDs))

	// 设置信号处理
	go sess.WaitForSignal()

	// 运行交互循环
	interactor := session.NewInteractiveLoop(sess)
	if err := interactor.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "交互失败: %v\n", err)
	}

	// 关闭会话
	sess.Close()
	sess.PrintSummary()

	return nil
}

// prepareNodeConfigs 准备节点配置
func prepareNodeConfigs(nodeIDs []string) ([]*session.NodeConfig, error) {
	var configs []*session.NodeConfig

	// 加载 SSH 配置
	configManager, err := getSSHConfigManager()
	if err != nil {
		return nil, err
	}

	for _, nodeID := range nodeIDs {
		// 解析节点 ID
		config := parseNodeID(nodeID)

		// 从 SSH config 中查找配置
		var sshConfig *ssh.SSHConfig
		if configManager != nil {
			// 优先按节点ID匹配
			if cfg := configManager.GetConfig(nodeID); cfg != nil {
				sshConfig = cfg
			} else if cfg := configManager.GetConfig(config.Address); cfg != nil {
				// 按地址匹配
				sshConfig = cfg
			}
		}

		// 应用 SSH config 配置
		if sshConfig != nil {
			fmt.Printf("找到 SSH 配置: %s -> %s\n", nodeID, sshConfig.HostName)

			// 用户优先级：节点配置 > SSH config
			if config.User == "" && sshConfig.User != "" {
				config.User = sshConfig.User
			}

			// 端口优先级：节点配置 > SSH config
			if config.Port == 22 && sshConfig.Port > 0 {
				config.Port = sshConfig.Port
			}

			// 地址
			if sshConfig.HostName != "" {
				config.Address = sshConfig.HostName
			}

			// 设置密钥文件
			if sshConfig.IdentityFile != "" {
				attachKeyFile = sshConfig.IdentityFile
			}
		}

		// 获取认证方法（优先使用密钥认证）
		authMethods, err := getAuthMethodsWithConfig(sshConfig)
		if err != nil {
			return nil, err
		}
		config.Auth = authMethods

		configs = append(configs, config)
	}

	return configs, nil
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
	return getAuthMethodsWithConfig(nil)
}

// getAuthMethodsWithConfig 根据 SSH 配置获取认证方法
func getAuthMethodsWithConfig(sshConfig *ssh.SSHConfig) ([]gossh.AuthMethod, error) {
	var authMethods []gossh.AuthMethod

	// 1. 尝试 SSH config 中的密钥文件
	if sshConfig != nil && sshConfig.IdentityFile != "" {
		if _, err := os.Stat(sshConfig.IdentityFile); err == nil {
			auth, err := publicKeyAuth(sshConfig.IdentityFile)
			if err == nil {
				authMethods = append(authMethods, auth)
			}
		}
	}

	// 2. 尝试指定的密钥文件
	if attachKeyFile != "" {
		auth, err := publicKeyAuth(attachKeyFile)
		if err == nil {
			authMethods = append(authMethods, auth)
		}
	}

	// 3. 尝试默认密钥文件
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
				}
			}
		}
	}

	// 4. 尝试 SSH Agent
	if auth := sshAgentAuth(); auth != nil {
		authMethods = append(authMethods, auth)
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("未找到可用的 SSH 认证方法")
	}

	return authMethods, nil
}

// publicKeyAuth 公钥认证
func publicKeyAuth(keyFile string) (gossh.AuthMethod, error) {
	key, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("读取密钥文件失败: %w", err)
	}

	signer, err := gossh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("解析私钥失败: %w", err)
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
	fmt.Printf("已连接到 %d 个节点\n", nodeCount)
	fmt.Printf("会话 ID: %s\n", sess.ID)
	fmt.Printf("会话超时: %s\n", sess.Timeout.String())
	fmt.Println("─────────────────────────────────────")
	fmt.Println("输入 'help' 查看可用命令")
	fmt.Println("输入 'exit' 或按 Ctrl+C 退出会话")
	fmt.Println()
}
