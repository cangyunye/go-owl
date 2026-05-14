package ssh

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SSHConfig SSH 配置条目
type SSHConfig struct {
	Host         string
	HostName     string
	User         string
	Port         int
	IdentityFile string
	ProxyCommand string
	ForwardAgent bool
}

// ConfigManager SSH 配置管理器
type ConfigManager struct {
	configs    map[string]*SSHConfig
	configPath string
}

// 全局单例
var globalConfigManager *ConfigManager

// NewConfigManager 创建配置管理器
func NewConfigManager() (*ConfigManager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("无法获取用户目录: %w", err)
	}

	configPath := filepath.Join(home, ".ssh", "config")
	return NewConfigManagerWithPath(configPath)
}

// NewConfigManagerWithPath 使用指定路径创建配置管理器
func NewConfigManagerWithPath(configPath string) (*ConfigManager, error) {
	cm := &ConfigManager{
		configs:    make(map[string]*SSHConfig),
		configPath: configPath,
	}

	if err := cm.LoadConfig(); err != nil {
		// 如果文件不存在，返回空配置而不是错误
		if os.IsNotExist(err) {
			return cm, nil
		}
		return nil, err
	}

	return cm, nil
}

// LoadConfig 加载 SSH 配置文件
func (cm *ConfigManager) LoadConfig() error {
	file, err := os.Open(cm.configPath)
	if err != nil {
		return err
	}
	defer file.Close()

	var currentHost string
	var currentConfig *SSHConfig

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 解析配置项
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		key := parts[0]
		value := strings.Join(parts[1:], " ")

		switch strings.ToLower(key) {
		case "host":
			// 保存上一个 host 的配置
			if currentHost != "" && currentConfig != nil {
				cm.configs[currentHost] = currentConfig
			}
			// 开始新的 host
			currentHost = value
			currentConfig = &SSHConfig{Host: currentHost}
			// 也保存别名
			for _, alias := range strings.Fields(value) {
				if alias != currentHost {
					cm.configs[alias] = currentConfig
				}
			}

		case "hostname":
			if currentConfig != nil {
				currentConfig.HostName = value
			}

		case "user":
			if currentConfig != nil {
				currentConfig.User = value
			}

		case "port":
			if currentConfig != nil {
				var port int
				fmt.Sscanf(value, "%d", &port)
				currentConfig.Port = port
			}

		case "identityfile":
			if currentConfig != nil {
				// 处理 ~/ 开头的路径
				if strings.HasPrefix(value, "~/") {
					home, _ := os.UserHomeDir()
					value = filepath.Join(home, value[2:])
				}
				currentConfig.IdentityFile = value
			}

		case "proxycommand":
			if currentConfig != nil {
				currentConfig.ProxyCommand = value
			}

		case "forwardagent":
			if currentConfig != nil {
				currentConfig.ForwardAgent = strings.ToLower(value) == "yes"
			}
		}
	}

	// 保存最后一个 host 的配置
	if currentHost != "" && currentConfig != nil {
		cm.configs[currentHost] = currentConfig
	}

	return scanner.Err()
}

// GetConfig 根据主机名获取 SSH 配置
func (cm *ConfigManager) GetConfig(host string) *SSHConfig {
	// 1. 精确匹配
	if config, ok := cm.configs[host]; ok {
		return config
	}

	// 2. 尝试从节点信息中获取
	// 如果 host 是 IP 或域名，可能需要查询节点管理器

	return nil
}

// HasConfig 检查是否存在主机配置
func (cm *ConfigManager) HasConfig(host string) bool {
	_, ok := cm.configs[host]
	return ok
}

// GetGlobalManager 获取全局配置管理器
func GetGlobalManager() (*ConfigManager, error) {
	if globalConfigManager == nil {
		var err error
		globalConfigManager, err = NewConfigManager()
		if err != nil {
			return nil, err
		}
	}
	return globalConfigManager, nil
}

// BuildSSHCommand 构建 SSH 命令参数
func BuildSSHCommand(config *SSHConfig, nodeUser, nodeAddress string, port int) []string {
	var args []string

	// 用户
	user := config.User
	if nodeUser != "" {
		user = nodeUser
	}
	if user != "" {
		args = append(args, "-l", user)
	}

	// 端口
	sshPort := config.Port
	if port > 0 {
		sshPort = port
	}
	if sshPort > 0 && sshPort != 22 {
		args = append(args, "-p", fmt.Sprintf("%d", sshPort))
	}

	// 密钥文件
	if config.IdentityFile != "" {
		args = append(args, "-i", config.IdentityFile)
	}

	// 代理命令
	if config.ProxyCommand != "" {
		args = append(args, "-o", fmt.Sprintf("ProxyCommand=%s", config.ProxyCommand))
	}

	// 转发 agent
	if config.ForwardAgent {
		args = append(args, "-A")
	}

	// 禁用主机密钥检查（谨慎使用）
	args = append(args, "-o", "StrictHostKeyChecking=no")
	args = append(args, "-o", "UserKnownHostsFile=/dev/null")

	// 目标主机
	address := config.HostName
	if nodeAddress != "" {
		address = nodeAddress
	}
	args = append(args, address)

	return args
}
