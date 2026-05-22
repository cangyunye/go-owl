package ssh

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// SSHAuthError SSH 认证失败错误
type SSHAuthError struct {
	ExitCode  int
	NodeID    string
	Stderr    string
	Cause     error
}

func (e *SSHAuthError) Error() string {
	return fmt.Sprintf("SSH 连接失败 (exit code %d) on node %s: %s", e.ExitCode, e.NodeID, strings.TrimSpace(e.Stderr))
}

func (e *SSHAuthError) Unwrap() error {
	return e.Cause
}

// NodeExecutorFactory 节点执行器工厂
type NodeExecutorFactory struct {
	sshConfigPath string
}

// NewNodeExecutorFactory 创建工厂
func NewNodeExecutorFactory() *NodeExecutorFactory {
	return &NodeExecutorFactory{}
}

// NewNodeExecutorFactoryWithSSHConfig 使用自定义 SSH config 路径创建工厂
func NewNodeExecutorFactoryWithSSHConfig(sshConfigPath string) *NodeExecutorFactory {
	return &NodeExecutorFactory{
		sshConfigPath: sshConfigPath,
	}
}

// GetExecutorForNode 获取适合指定节点的执行器
// 默认使用基于 crypto/ssh 的原生执行器（支持密钥优先、密码兜底）
func (f *NodeExecutorFactory) GetExecutorForNode(nodeID, nodeAddress string, nodePort int, nodeUser, nodeKeyFile, nodePassword string) (NodeExecutor, error) {
	// 1. 如果是本地节点（127.0.0.1 或 localhost），使用本地执行器
	if isLocalNode(nodeAddress) {
		return &LocalNodeExecutor{}, nil
	}

	// 2. 解析连接信息
	connInfo, err := ResolveConnection(nodeID, nodeAddress, nodePort, nodeUser, nodeKeyFile, nodePassword, f.sshConfigPath)
	if err != nil {
		return nil, err
	}

	// 3. 返回基于 crypto/ssh 的原生执行器
	return &NativeNodeExecutor{
		connInfo: connInfo,
	}, nil
}

// NodeExecutor 节点执行器接口
type NodeExecutor interface {
	Execute(command string, timeout time.Duration) (int, string, error)
	ExecuteWithConfig(command string, config *TimeoutConfig) (int, string, error)
}

// LocalNodeExecutor 本地节点执行器
type LocalNodeExecutor struct{}

func (e *LocalNodeExecutor) Execute(command string, timeout time.Duration) (int, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n" + stderr.String()
	}

	if ctx.Err() == context.DeadlineExceeded {
		return -1, output, fmt.Errorf("command timed out after %v", timeout)
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), output, nil
		}
		return -1, output, err
	}

	return 0, output, nil
}

// ExecuteWithConfig 执行命令（带超时配置）
func (e *LocalNodeExecutor) ExecuteWithConfig(command string, config *TimeoutConfig) (int, string, error) {
	if config == nil {
		config = &TimeoutConfig{
			ConnectTimeout: 0,
			CommandTimeout: 30 * time.Second,
		}
	}

	// 本地执行没有连接超时，直接使用命令超时
	timeout := config.CommandTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n" + stderr.String()
	}

	if ctx.Err() == context.DeadlineExceeded {
		return -1, output, &TimeoutError{
			Type:    TimeoutCommand,
			NodeID:  "localhost",
			Timeout: timeout,
			Cause:   ctx.Err(),
		}
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), output, nil
		}
		return -1, output, err
	}

	return 0, output, nil
}

// isLocalNode 判断是否为本地节点
func isLocalNode(address string) bool {
	localAddresses := []string{"127.0.0.1", "localhost", "::1", "0.0.0.0"}
	for _, local := range localAddresses {
		if address == local {
			return true
		}
	}
	return false
}

// GetSSHConfigForNode 获取节点的 SSH 配置信息
func (f *NodeExecutorFactory) GetSSHConfigForNode(nodeID, nodeAddress string) (*SSHConfig, bool) {
	configManager, err := NewConfigManagerWithPath(f.sshConfigPath)
	if err != nil {
		return nil, false
	}

	// 尝试多种匹配方式
	config := configManager.GetConfig(nodeID)
	if config != nil {
		return config, true
	}

	config = configManager.GetConfig(nodeAddress)
	if config != nil {
		return config, true
	}

	return nil, false
}

// ListSSHConfigs 列出所有 SSH 配置
func (f *NodeExecutorFactory) ListSSHConfigs() (map[string]*SSHConfig, error) {
	configManager, err := NewConfigManagerWithPath(f.sshConfigPath)
	if err != nil {
		return nil, err
	}
	return configManager.configs, nil
}
