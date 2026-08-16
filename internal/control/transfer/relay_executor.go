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
	remoteGscpPath     = "/tmp/gscp"
	timeoutOverheadSec = 30
)

type RelayExecutor struct {
	nodeResolver  *node.NodeResolver
	sshConfigPath string
	scriptPath    string
	gscpDir       string
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

func (e *RelayExecutor) detectRemoteArch(executor *ssh.NativeNodeExecutor) (string, string, error) {
	exitCode, output, err := executor.Execute("uname -sm", 10*time.Second)
	if err != nil {
		return "", "", fmt.Errorf("检测远程架构失败: %w", err)
	}
	if exitCode != 0 {
		return "", "", fmt.Errorf("检测远程架构失败，退出码: %d", exitCode)
	}

	parts := strings.Fields(strings.TrimSpace(output))
	if len(parts) < 2 {
		return "", "", fmt.Errorf("无法解析 uname 输出: %q", output)
	}

	sysName := parts[0]
	machine := parts[1]

	var goos string
	switch sysName {
	case "Linux":
		goos = "linux"
	case "Darwin":
		goos = "darwin"
	default:
		return "", "", fmt.Errorf("不支持的操作系统: %s", sysName)
	}

	var goarch string
	switch machine {
	case "x86_64", "amd64":
		goarch = "amd64"
	case "aarch64", "arm64":
		goarch = "arm64"
	default:
		return "", "", fmt.Errorf("不支持的架构: %s", machine)
	}

	return goos, goarch, nil
}

func (e *RelayExecutor) resolveGscpBinary(goos, goarch string) (string, error) {
	platformDir := fmt.Sprintf("%s-%s", goos, goarch)

	if e.gscpDir != "" {
		p := filepath.Join(e.gscpDir, "gscp")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		p = filepath.Join(e.gscpDir, platformDir, "gscp")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	if envDir := os.Getenv("OWL_GSCP_DIR"); envDir != "" {
		p := filepath.Join(envDir, "gscp")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		p = filepath.Join(envDir, platformDir, "gscp")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	if home, err := os.UserHomeDir(); err == nil {
		p := filepath.Join(home, ".owl", "gscp", platformDir, "gscp")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	execPath, err := os.Executable()
	if err == nil {
		execDir := filepath.Dir(execPath)

		p := filepath.Join(execDir, "gscp-"+platformDir)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}

		p = filepath.Join(execDir, platformDir, "gscp")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}

		p = filepath.Join(execDir, "..", platformDir, "gscp")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	p := filepath.Join("build", platformDir, "gscp")
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}

	return "", fmt.Errorf("未找到 gscp 二进制 (平台: %s)，请执行 make install-gscp 或设置 OWL_GSCP_DIR", platformDir)
}

func (e *RelayExecutor) DeployRelay(ctx context.Context, nodeID string) error {
	_, connInfo, err := e.resolveConnInfo(ctx, nodeID)
	if err != nil {
		return err
	}

	executor := ssh.NewNativeNodeExecutor(connInfo)

	goos, goarch, err := e.detectRemoteArch(executor)
	if err != nil {
		return fmt.Errorf("节点 %s: %w", nodeID, err)
	}

	gscpBinary, err := e.resolveGscpBinary(goos, goarch)
	if err != nil {
		return fmt.Errorf("节点 %s: %w", nodeID, err)
	}

	if err := executor.WriteFile(gscpBinary, remoteGscpPath); err != nil {
		return fmt.Errorf("上传 gscp 到节点 %s 失败: %w", nodeID, err)
	}

	scriptPath := e.resolveScriptPath()
	if _, err := os.Stat(scriptPath); err != nil {
		return fmt.Errorf("中继脚本未找到 (%s): %w", scriptPath, err)
	}

	if err := executor.WriteFile(scriptPath, remoteScriptPath); err != nil {
		return fmt.Errorf("上传中继脚本到节点 %s 失败: %w", nodeID, err)
	}

	exitCode, output, err := executor.Execute("chmod +x "+remoteGscpPath+" "+remoteScriptPath, 10*time.Second)
	if err != nil {
		return fmt.Errorf("设置权限失败: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("设置权限失败，退出码: %d, 输出: %s", exitCode, output)
	}

	exitCode, output, err = executor.Execute(remoteGscpPath+" --help", 10*time.Second)
	if err != nil {
		return fmt.Errorf("验证 gscp 失败: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("gscp 验证失败 (退出码 %d): %s", exitCode, output)
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

	results, parseErr := ParseRelayResults(output)
	if parseErr != nil {
		return nil, fmt.Errorf("解析中继结果失败: %w", parseErr)
	}

	if exitCode == 0 {
		return results, nil
	}

	successCount := 0
	failCount := 0
	var failedTargets []string
	for _, r := range results {
		if r.Status == "success" {
			successCount++
		} else {
			failCount++
			failedTargets = append(failedTargets, r.Target)
		}
	}

	if failCount > 0 && successCount > 0 {
		return results, fmt.Errorf("中继部分失败: %d/%d 个目标失败 (%s)", failCount, len(results), strings.Join(failedTargets, ","))
	}

	return results, fmt.Errorf("中继命令退出码非零 (%d)，全部 %d 个目标失败", exitCode, len(results))
}

func shellEscape(s string) string {
	if s == "" {
		return "''"
	}
	escaped := strings.ReplaceAll(s, "'", `'\''`)
	return "'" + escaped + "'"
}
