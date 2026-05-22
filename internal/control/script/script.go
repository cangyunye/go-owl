package script

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cangyunye/go-owl/internal/control/transfer"
	"github.com/cangyunye/go-owl/internal/node"
	"github.com/cangyunye/go-owl/internal/ssh"
)

// ScriptExecutor 脚本执行器
type ScriptExecutor struct {
	nodeResolver *node.NodeResolver
	transferMgr  *transfer.TransferManager
}

// ScriptExecutionOptions 脚本执行选项
type ScriptExecutionOptions struct {
	DestDir string // 远端存放目录
	Args    string // 传递给脚本的参数
	Timeout time.Duration
	Inline  bool // 是否直接发送内容执行
	Keep    bool // 是否保留脚本文件
}

// NewScriptExecutor 创建脚本执行器
func NewScriptExecutor(nodeResolver *node.NodeResolver, transferMgr *transfer.TransferManager) *ScriptExecutor {
	if transferMgr == nil {
		transferMgr = transfer.NewTransferManager(nodeResolver)
	}
	return &ScriptExecutor{
		nodeResolver: nodeResolver,
		transferMgr:  transferMgr,
	}
}

// ExecuteScript 执行脚本
func (e *ScriptExecutor) ExecuteScript(scriptPath string, targets []string, opts *ScriptExecutionOptions) ([]*ScriptExecutionResult, error) {
	if opts == nil {
		opts = &ScriptExecutionOptions{
			DestDir: "/tmp",
			Timeout: 5 * time.Minute,
		}
	}

	// 检查脚本类型
	isURL := strings.HasPrefix(scriptPath, "http://") || strings.HasPrefix(scriptPath, "https://")
	var content []byte
	var err error

	if isURL {
		return nil, fmt.Errorf("URL 脚本尚未支持")
	} else {
		content, err = os.ReadFile(scriptPath)
		if err != nil {
			return nil, fmt.Errorf("读取脚本文件失败: %w", err)
		}
	}

	scriptName := filepath.Base(scriptPath)
	results := make([]*ScriptExecutionResult, 0, len(targets))
	var wg sync.WaitGroup
	resultChan := make(chan *ScriptExecutionResult, len(targets))

	for _, nodeID := range targets {
		wg.Add(1)
		go func(nid string) {
			defer wg.Done()
			result := e.executeScriptOnNode(nid, scriptPath, scriptName, content, opts)
			resultChan <- result
		}(nodeID)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	for result := range resultChan {
		results = append(results, result)
	}

	return results, nil
}

// executeScriptOnNode 在单个节点上执行脚本
func (e *ScriptExecutor) executeScriptOnNode(nodeID, scriptPath, scriptName string, content []byte, opts *ScriptExecutionOptions) *ScriptExecutionResult {
	result := &ScriptExecutionResult{
		NodeID: nodeID,
		Script: scriptPath,
		Method: "file",
	}
	if opts.Inline {
		result.Method = "inline"
	}
	startTime := time.Now()

	nodeInfo, err := e.nodeResolver.Resolve(nodeID)
	if err != nil {
		result.Error = fmt.Errorf("解析节点失败: %w", err)
		result.EndTime = time.Now()
		return result
	}

	var exitCode int
	var output string

	if opts.Inline {
		exitCode, output, err = e.executeInline(nodeInfo, content, opts)
	} else {
		exitCode, output, err = e.executeViaFile(nodeInfo, scriptName, content, opts)
	}

	result.ExitCode = exitCode
	result.Output = output
	result.Error = err
	result.StartTime = startTime
	result.EndTime = time.Now()

	return result
}

// newNodeExecutor 从节点信息创建原生 SSH 执行器
func (e *ScriptExecutor) newNodeExecutor(nodeInfo *node.ResolvedNode) (*ssh.NativeNodeExecutor, error) {
	connInfo, err := ssh.ResolveConnection(
		nodeInfo.ID,
		nodeInfo.Address,
		nodeInfo.Port,
		nodeInfo.User,
		nodeInfo.SSHKey,
		nodeInfo.SSHPassword,
		"",
	)
	if err != nil {
		return nil, fmt.Errorf("解析连接信息失败: %w", err)
	}
	return ssh.NewNativeNodeExecutor(connInfo), nil
}

// executeViaFile 通过文件方式执行
func (e *ScriptExecutor) executeViaFile(nodeInfo *node.ResolvedNode, scriptName string, content []byte, opts *ScriptExecutionOptions) (int, string, error) {
	ctx := context.Background()

	// 1. 保存为临时文件
	tmpFile, err := os.CreateTemp("", "owl-script-*.sh")
	if err != nil {
		return -1, "", fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.Write(content)
	if err != nil {
		return -1, "", fmt.Errorf("写入临时文件失败: %w", err)
	}
	tmpFile.Close()

	// 2. 上传到远程节点
	remotePath := filepath.Join(opts.DestDir, scriptName)
	uploadOpts := &transfer.UploadOptions{
		Overwrite: true,
	}
	uploadResults := e.transferMgr.Upload(ctx, []string{nodeInfo.ID}, tmpFile.Name(), remotePath, uploadOpts)
	if len(uploadResults) > 0 && uploadResults[0].Error != nil {
		return -1, "", fmt.Errorf("上传脚本失败: %w", uploadResults[0].Error)
	}

	// 3. 创建原生执行器并执行脚本
	executor, err := e.newNodeExecutor(nodeInfo)
	if err != nil {
		return -1, "", err
	}

	var cmd string
	if opts.Args != "" {
		cmd = fmt.Sprintf("chmod +x %s && %s %s", remotePath, remotePath, opts.Args)
	} else {
		cmd = fmt.Sprintf("chmod +x %s && %s", remotePath, remotePath)
	}

	exitCode, output, execErr := executor.Execute(cmd, opts.Timeout)

	// 4. 清理脚本文件（除非 --keep）
	if !opts.Keep {
		cleanCmd := fmt.Sprintf("rm -f %s", remotePath)
		executor.Execute(cleanCmd, 30*time.Second)
	}

	return exitCode, output, execErr
}

// executeInline 内联方式执行
func (e *ScriptExecutor) executeInline(nodeInfo *node.ResolvedNode, content []byte, opts *ScriptExecutionOptions) (int, string, error) {
	ctx := context.Background()

	tmpFile, err := os.CreateTemp("", "owl-script-*.sh")
	if err != nil {
		return -1, "", fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.Write(content)
	if err != nil {
		return -1, "", fmt.Errorf("写入临时文件失败: %w", err)
	}
	tmpFile.Close()

	remotePath := filepath.Join(opts.DestDir, filepath.Base(tmpFile.Name()))
	uploadOpts := &transfer.UploadOptions{
		Overwrite: true,
	}
	uploadResults := e.transferMgr.Upload(ctx, []string{nodeInfo.ID}, tmpFile.Name(), remotePath, uploadOpts)
	if len(uploadResults) > 0 && uploadResults[0].Error != nil {
		return -1, "", fmt.Errorf("上传脚本失败: %w", uploadResults[0].Error)
	}

	// 创建原生执行器并执行
	executor, err := e.newNodeExecutor(nodeInfo)
	if err != nil {
		return -1, "", err
	}

	var execCmd string
	if opts.Args != "" {
		execCmd = fmt.Sprintf("bash %s %s", remotePath, opts.Args)
	} else {
		execCmd = fmt.Sprintf("bash %s", remotePath)
	}

	exitCode, output, execErr := executor.Execute(execCmd, opts.Timeout)

	// 立即清理
	cleanCmd := fmt.Sprintf("rm -f %s", remotePath)
	executor.Execute(cleanCmd, 30*time.Second)

	return exitCode, output, execErr
}

// ScriptExecutionResult 脚本执行结果
type ScriptExecutionResult struct {
	NodeID    string
	Script    string
	Method    string // "file" or "inline"
	ExitCode  int
	Output    string
	Error     error
	StartTime time.Time
	EndTime   time.Time
}

// Success 是否成功
func (r *ScriptExecutionResult) Success() bool {
	return r.Error == nil && r.ExitCode == 0
}
