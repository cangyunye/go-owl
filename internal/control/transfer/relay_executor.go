package transfer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cangyunye/go-owl/internal/node"
	"github.com/cangyunye/go-owl/internal/ssh"
)

const (
	defaultScriptPath  = "scripts/owl-relay.sh"
	remoteScriptPath   = "/tmp/owl-relay.sh"
	timeoutOverheadSec = 30
)

type RelayExecutor struct {
	nodeResolver  *node.NodeResolver
	sshConfigPath string
	scriptPath    string
}

func NewRelayExecutor(nodeResolver *node.NodeResolver) *RelayExecutor {
	return &RelayExecutor{
		nodeResolver: nodeResolver,
		scriptPath:   defaultScriptPath,
	}
}

func (e *RelayExecutor) resolveConnInfo(ctx context.Context, nodeID string) (*node.ResolvedNode, *ssh.ConnectionInfo, error) {
	nodeInfo, err := e.nodeResolver.Resolve(nodeID)
	if err != nil {
		return nil, nil, fmt.Errorf("解析节点 %s 失败: %w", nodeID, err)
	}

	connInfo, err := ssh.ResolveConnection(
		nodeInfo.ID,
		nodeInfo.Address,
		nodeInfo.Port,
		nodeInfo.User,
		nodeInfo.SSHKey,
		nodeInfo.SSHPassword,
		e.sshConfigPath,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("解析节点 %s 的连接信息失败: %w", nodeID, err)
	}

	return nodeInfo, connInfo, nil
}

func (e *RelayExecutor) resolveScriptPath() string {
	if _, err := os.Stat(e.scriptPath); err == nil {
		return e.scriptPath
	}
	altPath := filepath.Join("..", defaultScriptPath)
	if _, err := os.Stat(altPath); err == nil {
		return altPath
	}
	return e.scriptPath
}

func (e *RelayExecutor) DeployScript(ctx context.Context, nodeID string) error {
	_, connInfo, err := e.resolveConnInfo(ctx, nodeID)
	if err != nil {
		return err
	}

	executor := ssh.NewNativeNodeExecutor(connInfo)

	scriptPath := e.resolveScriptPath()
	if _, err := os.Stat(scriptPath); err != nil {
		return fmt.Errorf("中继脚本未找到 (%s): %w", scriptPath, err)
	}

	if err := executor.WriteFile(scriptPath, remoteScriptPath); err != nil {
		return fmt.Errorf("上传中继脚本到节点 %s 失败: %w", nodeID, err)
	}

	exitCode, output, err := executor.Execute("chmod +x "+remoteScriptPath, 10*time.Second)
	if err != nil {
		return fmt.Errorf("设置中继脚本权限失败: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("设置中继脚本权限失败，退出码: %d, 输出: %s", exitCode, output)
	}

	return nil
}

func (e *RelayExecutor) ExecuteRelay(ctx context.Context, nodeID string, task *RelaySubTask) ([]RelayTargetResult, error) {
	_, connInfo, err := e.resolveConnInfo(ctx, nodeID)
	if err != nil {
		return nil, err
	}

	executor := ssh.NewNativeNodeExecutor(connInfo)

	args := task.ToShellArgs()
	escapedArgs := make([]string, len(args))
	for i, arg := range args {
		escapedArgs[i] = shellEscape(arg)
	}

	command := remoteScriptPath + " " + strings.Join(escapedArgs, " ")

	timeout := time.Duration(task.TimeoutSec+timeoutOverheadSec) * time.Second
	exitCode, output, err := executor.Execute(command, timeout)
	if err != nil {
		return nil, fmt.Errorf("节点 %s 执行中继命令失败: %w", nodeID, err)
	}
	if exitCode != 0 {
		return nil, fmt.Errorf("节点 %s 中继命令退出码非零 (%d): %s", nodeID, exitCode, output)
	}

	results, err := ParseRelayResults(output)
	if err != nil {
		return nil, fmt.Errorf("解析中继结果失败: %w", err)
	}

	return results, nil
}

func shellEscape(s string) string {
	if s == "" {
		return "''"
	}
	escaped := strings.ReplaceAll(s, "'", `'\''`)
	return "'" + escaped + "'"
}
