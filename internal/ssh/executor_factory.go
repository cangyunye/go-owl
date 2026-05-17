package ssh

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

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
func (f *NodeExecutorFactory) GetExecutorForNode(nodeID, nodeAddress string, nodePort int, nodeUser string) (NodeExecutor, error) {
	// 1. 解析连接信息
	connInfo, err := ResolveConnection(nodeID, nodeAddress, nodePort, nodeUser, f.sshConfigPath)
	if err != nil {
		return nil, err
	}

	// 2. 如果是本地节点（127.0.0.1 或 localhost），使用本地执行器
	if isLocalNode(nodeAddress) {
		return &LocalNodeExecutor{}, nil
	}

	// 3. 返回远程执行器
	return &RemoteNodeExecutorWithInfo{
		connInfo:      connInfo,
		sshConfigPath: f.sshConfigPath,
	}, nil
}

// RemoteNodeExecutorWithInfo 带连接信息的远程执行器
type RemoteNodeExecutorWithInfo struct {
	connInfo      *ConnectionInfo
	sshConfigPath string
}

// Execute 实现 NodeExecutor 接口
func (e *RemoteNodeExecutorWithInfo) Execute(command string, timeout time.Duration) (int, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := e.connInfo.BuildSSHCommand(command)
	cmd := exec.CommandContext(ctx, "ssh", args...)

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
func (e *RemoteNodeExecutorWithInfo) ExecuteWithConfig(command string, config *TimeoutConfig) (int, string, error) {
	if config == nil {
		config = &TimeoutConfig{
			ConnectTimeout: 10 * time.Second,
			CommandTimeout: 30 * time.Second,
		}
	}

	// 设置 SSH 参数（使用 ConnectTimeout）
	args := []string{
		"-o", fmt.Sprintf("ConnectTimeout=%d", int(config.ConnectTimeout.Seconds())),
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=no",
	}

	// 构建完整命令
	sshArgs := e.connInfo.BuildSSHCommand(command)
	args = append(args, sshArgs...)

	// 执行命令，总超时 = ConnectTimeout + CommandTimeout
	totalTimeout := config.ConnectTimeout + config.CommandTimeout
	ctx, cancel := context.WithTimeout(context.Background(), totalTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ssh", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n" + stderr.String()
	}

	// 解析超时类型
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), output, nil
		}

		if ctx.Err() == context.DeadlineExceeded {
			errMsg := output
			if strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "connect") {
				return -1, output, &TimeoutError{
					Type:    TimeoutConnect,
					NodeID:  e.connInfo.Address,
					Timeout: config.ConnectTimeout,
					Cause:   err,
				}
			}
			return -1, output, &TimeoutError{
				Type:    TimeoutCommand,
				NodeID:  e.connInfo.Address,
				Timeout: config.CommandTimeout,
				Cause:   err,
			}
		}

		return -1, output, err
	}

	return 0, output, nil
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
