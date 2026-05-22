package ssh

import (
	"fmt"
	"os/user"
	"strings"
)

// ConnectionInfo 连接信息
type ConnectionInfo struct {
	User      string
	Address   string
	Port      int
	KeyFile   string
	Password  string // SSH 密码
	UseConfig bool   // 是否使用 SSH config 中的配置
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
func ResolveConnection(nodeID, nodeAddress string, nodePort int, nodeUser, nodeKeyFile, nodePassword string, sshConfigPath string) (*ConnectionInfo, error) {
	info := &ConnectionInfo{
		Address:  nodeAddress,
		Port:     nodePort,
		User:     nodeUser,
		KeyFile:  nodeKeyFile,
		Password: nodePassword,
	}

	// 1. 如果节点配置了密钥或用户，直接返回（节点配置优先级最高）
	if nodeKeyFile != "" || nodeUser != "" {
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

// BuildRsyncCommand 构建 rsync 命令参数（使用相同的认证信息）
func (ci *ConnectionInfo) BuildRsyncCommand(isDownload bool, localPath, remotePath string, otherArgs []string) []string {
	user := ci.GetUser()
	address := ci.Address

	// 构建 rsh 参数（指定 rsync 使用的 ssh 命令和认证选项）
	rshArgs := []string{"ssh"}
	rshArgs = append(rshArgs, "-o", "StrictHostKeyChecking=no")
	rshArgs = append(rshArgs, "-o", "UserKnownHostsFile=/dev/null")
	rshArgs = append(rshArgs, "-o", "LogLevel=ERROR")

	if ci.Port > 0 && ci.Port != 22 {
		rshArgs = append(rshArgs, "-p", fmt.Sprintf("%d", ci.Port))
	}
	if ci.KeyFile != "" {
		rshArgs = append(rshArgs, "-i", ci.KeyFile)
	}
	rshFlag := fmt.Sprintf("--rsh=%s", strings.Join(rshArgs, " "))

	// 构建完整 rsync 命令
	rsyncArgs := make([]string, 0)
	rsyncArgs = append(rsyncArgs, otherArgs...)
	rsyncArgs = append(rsyncArgs, rshFlag)

	if isDownload {
		// 下载: remote -> local
		rsyncArgs = append(rsyncArgs, fmt.Sprintf("%s@%s:%s", user, address, remotePath))
		rsyncArgs = append(rsyncArgs, localPath)
	} else {
		// 上传: local -> remote
		rsyncArgs = append(rsyncArgs, localPath)
		rsyncArgs = append(rsyncArgs, fmt.Sprintf("%s@%s:%s", user, address, remotePath))
	}

	return rsyncArgs
}

// osUserHomeDir 获取用户目录
func osUserHomeDir() (string, error) {
	if u, err := user.Current(); err == nil {
		return u.HomeDir, nil
	}
	return "", fmt.Errorf("无法获取用户目录")
}
