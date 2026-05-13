package ssh

import (
	"fmt"
	"os/user"
)

// ConnectionInfo 连接信息
type ConnectionInfo struct {
	User      string
	Address   string
	Port      int
	KeyFile   string
	UseConfig bool // 是否使用 SSH config 中的配置
}

// GetUser 获取连接用户
func (ci *ConnectionInfo) GetUser() string {
	if ci.User != "" {
		return ci.User
	}
	// 如果没指定，尝试获取当前用户名
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return "root"
}

// ResolveConnection 解析连接信息
// 优先级：节点配置 > SSH config > 当前用户
func ResolveConnection(nodeID, nodeAddress string, nodePort int, nodeUser string, sshConfigPath string) (*ConnectionInfo, error) {
	info := &ConnectionInfo{
		Address: nodeAddress,
		Port:    nodePort,
		User:    nodeUser,
	}

	// 1. 如果节点配置了用户，优先使用
	if nodeUser != "" {
		info.UseConfig = false
		return info, nil
	}

	// 2. 查找 SSH config
	if sshConfigPath == "" {
		home, err := osUserHomeDir()
		if err == nil {
			sshConfigPath = home + "/.ssh/config"
		}
	}

	config, err := NewConfigManagerWithPath(sshConfigPath)
	if err != nil {
		// SSH config 不存在或解析失败，使用默认方式
		info.UseConfig = false
		return info, nil
	}

	// 尝试多种方式匹配 SSH config
	var matchedConfig *SSHConfig

	// 2.1 按节点 ID 匹配
	if config.HasConfig(nodeID) {
		matchedConfig = config.GetConfig(nodeID)
	}

	// 2.2 按节点地址匹配
	if matchedConfig == nil && config.HasConfig(nodeAddress) {
		matchedConfig = config.GetConfig(nodeAddress)
	}

	// 2.3 尝试匹配 Host 别名
	if matchedConfig == nil {
		// 遍历所有配置，检查 HostName 是否匹配
		for _, cfg := range config.configs {
			if cfg.HostName == nodeAddress || cfg.HostName == nodeID {
				matchedConfig = cfg
				break
			}
		}
	}

	if matchedConfig != nil {
		info.User = matchedConfig.User
		info.KeyFile = matchedConfig.IdentityFile
		info.UseConfig = true
	}

	return info, nil
}

// BuildSSHCommand 构建 SSH 命令
func (ci *ConnectionInfo) BuildSSHCommand(command string) []string {
	var args []string

	user := ci.GetUser()
	address := ci.Address
	port := ci.Port

	// SSH 基本选项
	args = append(args, "-o", "StrictHostKeyChecking=no")
	args = append(args, "-o", "UserKnownHostsFile=/dev/null")
	args = append(args, "-o", "LogLevel=ERROR")

	// 用户
	if user != "" {
		args = append(args, "-l", user)
	}

	// 端口
	if port > 0 && port != 22 {
		args = append(args, "-p", fmt.Sprintf("%d", port))
	}

	// 密钥文件
	if ci.KeyFile != "" {
		args = append(args, "-i", ci.KeyFile)
	}

	// 远程命令
	args = append(args, address)
	args = append(args, command)

	return args
}

// osUserHomeDir 获取用户目录
func osUserHomeDir() (string, error) {
	if u, err := user.Current(); err == nil {
		return u.HomeDir, nil
	}
	return "", fmt.Errorf("无法获取用户目录")
}
