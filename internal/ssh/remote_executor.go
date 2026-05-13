package ssh

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// RemoteNodeExecutor 远程节点执行器
type RemoteNodeExecutor struct {
	sshConfigPath string
}

// NewRemoteNodeExecutor 创建远程节点执行器
func NewRemoteNodeExecutor(sshConfigPath string) *RemoteNodeExecutor {
	return &RemoteNodeExecutor{
		sshConfigPath: sshConfigPath,
	}
}

// Execute 在远程节点执行命令
func (e *RemoteNodeExecutor) Execute(nodeID, nodeAddress string, nodePort int, nodeUser, command string, timeout time.Duration) (int, string, error) {
	// 解析连接信息
	connInfo, err := ResolveConnection(nodeID, nodeAddress, nodePort, nodeUser, e.sshConfigPath)
	if err != nil {
		return -1, "", fmt.Errorf("解析连接信息失败: %w", err)
	}

	// 构建 SSH 命令
	args := connInfo.BuildSSHCommand(command)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ssh", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
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

// ExecuteWithConfig 使用指定的 SSH 配置执行命令
func (e *RemoteNodeExecutor) ExecuteWithConfig(config *SSHConfig, command string, timeout time.Duration) (int, string, error) {
	var user, address string
	var port int

	if config != nil {
		user = config.User
		address = config.HostName
		port = config.Port
	}

	return e.Execute(config.Host, address, port, user, command, timeout)
}
