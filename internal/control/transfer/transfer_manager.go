package transfer

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/cangyunye/go-owl/internal/node"
	"github.com/cangyunye/go-owl/internal/ssh"
)

type TransferManager struct {
	nodeResolver      *node.NodeResolver
	sshConfigPath     string
	rsyncAvailable    map[string]bool // nodeID -> available
	mu                sync.RWMutex
}

func NewTransferManager(nodeResolver *node.NodeResolver) *TransferManager {
	return &TransferManager{
		nodeResolver:   nodeResolver,
		rsyncAvailable: make(map[string]bool),
	}
}

// NewTransferManagerWithSSHConfig 创建带自定义 SSH config 路径的传输管理器
func NewTransferManagerWithSSHConfig(nodeResolver *node.NodeResolver, sshConfigPath string) *TransferManager {
	return &TransferManager{
		nodeResolver:   nodeResolver,
		sshConfigPath:  sshConfigPath,
		rsyncAvailable: make(map[string]bool),
	}
}

type TransferResult struct {
	NodeID     string
	Path       string
	Error      error
	Method     string // "rsync" or "scp"
	BytesTotal int64
	BytesTrans int64
	Speed      string
	Duration   time.Duration
}

type UploadOptions struct {
	Parallel     bool
	Overwrite    bool
	NoOverwrite  bool
	PreservePerm bool
	Resume       bool
	ChunkSize    int64
}

type DownloadOptions struct {
	Parallel   bool
	Subdir     bool
	NameFormat string
	Resume     bool
}

// CheckRsyncAvailable 检查 rsync 是否可用（同时获取连接信息）
func (tm *TransferManager) CheckRsyncAvailable(ctx context.Context, nodeID string) (*ssh.ConnectionInfo, bool) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if available, ok := tm.rsyncAvailable[nodeID]; ok {
		// 尝试再次获取连接信息
		nodeInfo, _ := tm.nodeResolver.Resolve(nodeID)
		if nodeInfo != nil {
			connInfo, _ := ssh.ResolveConnection(nodeID, nodeInfo.Address, nodeInfo.Port, nodeInfo.User, nodeInfo.SSHKey, nodeInfo.SSHPassword, tm.sshConfigPath)
			return connInfo, available
		}
		return nil, available
	}

	nodeInfo, err := tm.nodeResolver.Resolve(nodeID)
	if err != nil {
		tm.rsyncAvailable[nodeID] = false
		return nil, false
	}

	// 获取连接信息
	connInfo, err := ssh.ResolveConnection(nodeID, nodeInfo.Address, nodeInfo.Port, nodeInfo.User, nodeInfo.SSHKey, nodeInfo.SSHPassword, tm.sshConfigPath)
	if err != nil {
		tm.rsyncAvailable[nodeID] = false
		return nil, false
	}

	// 通过 SSH 远程检查 rsync 是否可用
	available := tm.checkRsyncRemotely(nodeInfo, connInfo)
	tm.rsyncAvailable[nodeID] = available

	if available {
		fmt.Printf("[%s] rsync 可用，将使用断点续传\n", nodeID)
	}

	return connInfo, available
}

// checkRsyncRemotely 通过 SSH 检查远程节点是否有 rsync
func (tm *TransferManager) checkRsyncRemotely(nodeInfo *node.ResolvedNode, connInfo *ssh.ConnectionInfo) bool {
	// 使用 SSH 执行 which rsync
	args := connInfo.BuildSSHCommand("which rsync")
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	cmd := exec.CommandContext(ctx, "ssh", args...)
	err := cmd.Run()
	
	return err == nil
}

func (tm *TransferManager) Upload(ctx context.Context, nodeIDs []string, localPath, remotePath string, opts *UploadOptions) []TransferResult {
	if opts == nil {
		opts = &UploadOptions{
			Parallel: true,
			Resume:   true,
		}
	}

	results := make([]TransferResult, len(nodeIDs))

	if opts.Parallel {
		var wg sync.WaitGroup
		wg.Add(len(nodeIDs))

		for i, nodeID := range nodeIDs {
			go func(idx int, id string) {
				defer wg.Done()
				results[idx] = tm.smartUpload(ctx, id, localPath, remotePath, opts)
			}(i, nodeID)
		}

		wg.Wait()
	} else {
		for i, nodeID := range nodeIDs {
			results[i] = tm.smartUpload(ctx, nodeID, localPath, remotePath, opts)
		}
	}

	return results
}

func (tm *TransferManager) smartUpload(ctx context.Context, nodeID, localPath, remotePath string, opts *UploadOptions) TransferResult {
	startTime := time.Now()

	connInfo, rsyncOK := tm.CheckRsyncAvailable(ctx, nodeID)
	if opts.Resume && rsyncOK && connInfo != nil {
		return tm.rsyncUpload(ctx, nodeID, localPath, remotePath, opts, connInfo, startTime)
	}

	return tm.scpFallback(ctx, nodeID, localPath, remotePath, opts, startTime)
}

func (tm *TransferManager) rsyncUpload(ctx context.Context, nodeID, localPath, remotePath string, opts *UploadOptions, connInfo *ssh.ConnectionInfo, startTime time.Time) TransferResult {
	// 构建 rsync 参数
	otherArgs := []string{"-avz", "--partial", "--partial-dir=.rsync-partial", "--progress", "--stats"}
	
	if opts.NoOverwrite {
		otherArgs = append(otherArgs, "--update")
	}
	
	// 使用 ConnectionInfo 构建完整 rsync 命令（带认证信息）
	args := connInfo.BuildRsyncCommand(false, localPath, remotePath, otherArgs)
	
	cmd := exec.CommandContext(ctx, "rsync", args...)
	
	output, err := cmd.CombinedOutput()
	
	duration := time.Since(startTime)
	
	result := TransferResult{
		NodeID:   nodeID,
		Path:     remotePath,
		Method:   "rsync",
		Duration: duration,
	}
	
	if err != nil {
		result.Error = fmt.Errorf("rsync 上传失败: %w\n输出: %s", err, string(output))
		return result
	}
	
	result.Speed = extractSpeed(string(output))
	
	return result
}

func (tm *TransferManager) scpFallback(ctx context.Context, nodeID, localPath, remotePath string, opts *UploadOptions, startTime time.Time) TransferResult {
	// 这里我们使用 SSH 连接池的 scp 方式，保持向后兼容
	// 先获取 executor，然后通过 SSH 命令调用 scp
	executor, poolErr := tm.getExecutor(ctx, nodeID)
	if poolErr != nil {
		return TransferResult{
			NodeID: nodeID,
			Path:   remotePath,
			Error:  fmt.Errorf("获取连接失败: %w", poolErr),
			Method: "scp",
		}
	}
	
	nodeInfo, _ := tm.nodeResolver.Resolve(nodeID)
	scpCmd := fmt.Sprintf("scp -q %s %s@%s:%s", localPath, nodeInfo.User, nodeInfo.Address, remotePath)
	
	_, _, err := executor.Execute(scpCmd, 60*time.Second)
	
	duration := time.Since(startTime)
	return TransferResult{
		NodeID:   nodeID,
		Path:     remotePath,
		Error:    err,
		Method:   "scp",
		Duration: duration,
	}
}

// getExecutor 从连接池获取 executor
func (tm *TransferManager) getExecutor(ctx context.Context, nodeID string) (ssh.NodeExecutor, error) {
	nodeInfo, err := tm.nodeResolver.Resolve(nodeID)
	if err != nil {
		return nil, err
	}
	
	pool := ssh.NewConnectionPool(10, 5*time.Minute)
	executor, err := pool.Get(nodeInfo)
	return executor, err
}

func (tm *TransferManager) Download(ctx context.Context, nodeIDs []string, remotePath, localPath string, opts *DownloadOptions) []TransferResult {
	if opts == nil {
		opts = &DownloadOptions{
			Parallel: true,
			Resume:   true,
		}
	}

	results := make([]TransferResult, len(nodeIDs))

	if opts.Parallel {
		var wg sync.WaitGroup
		wg.Add(len(nodeIDs))

		for i, nodeID := range nodeIDs {
			go func(idx int, id string) {
				defer wg.Done()
				targetPath := tm.formatDownloadPath(localPath, id, remotePath, opts)
				results[idx] = tm.smartDownload(ctx, id, remotePath, targetPath, opts)
			}(i, nodeID)
		}

		wg.Wait()
	} else {
		for i, nodeID := range nodeIDs {
			targetPath := tm.formatDownloadPath(localPath, nodeID, remotePath, opts)
			results[i] = tm.smartDownload(ctx, nodeID, remotePath, targetPath, opts)
		}
	}

	return results
}

func (tm *TransferManager) smartDownload(ctx context.Context, nodeID, remotePath, localPath string, opts *DownloadOptions) TransferResult {
	startTime := time.Now()

	connInfo, rsyncOK := tm.CheckRsyncAvailable(ctx, nodeID)
	if opts.Resume && rsyncOK && connInfo != nil {
		return tm.rsyncDownload(ctx, nodeID, remotePath, localPath, opts, connInfo, startTime)
	}

	return tm.scpDownloadFallback(ctx, nodeID, remotePath, localPath, opts, startTime)
}

func (tm *TransferManager) rsyncDownload(ctx context.Context, nodeID, remotePath, localPath string, opts *DownloadOptions, connInfo *ssh.ConnectionInfo, startTime time.Time) TransferResult {
	otherArgs := []string{"-avz", "--partial", "--partial-dir=.rsync-partial", "--progress", "--stats"}
	
	args := connInfo.BuildRsyncCommand(true, localPath, remotePath, otherArgs)
	
	cmd := exec.CommandContext(ctx, "rsync", args...)
	
	output, err := cmd.CombinedOutput()
	
	duration := time.Since(startTime)
	
	result := TransferResult{
		NodeID:   nodeID,
		Path:     localPath,
		Method:   "rsync",
		Duration: duration,
	}
	
	if err != nil {
		result.Error = fmt.Errorf("rsync 下载失败: %w\n输出: %s", err, string(output))
		return result
	}
	
	result.Speed = extractSpeed(string(output))
	
	return result
}

func (tm *TransferManager) scpDownloadFallback(ctx context.Context, nodeID, remotePath, localPath string, opts *DownloadOptions, startTime time.Time) TransferResult {
	nodeInfo, err := tm.nodeResolver.Resolve(nodeID)
	if err != nil {
		return TransferResult{
			NodeID: nodeID,
			Path:   localPath,
			Error:  fmt.Errorf("获取节点信息失败: %w", err),
			Method: "scp",
		}
	}
	
	scpCmd := fmt.Sprintf("scp -q %s@%s:%s %s", nodeInfo.User, nodeInfo.Address, remotePath, localPath)
	
	executor, err := tm.getExecutor(ctx, nodeID)
	if err != nil {
		return TransferResult{
			NodeID: nodeID,
			Path:   localPath,
			Error:  err,
			Method: "scp",
		}
	}
	
	_, _, err = executor.Execute(scpCmd, 60*time.Second)
	
	return TransferResult{
		NodeID:   nodeID,
		Path:     localPath,
		Error:    err,
		Method:   "scp",
		Duration: time.Since(startTime),
	}
}

func (tm *TransferManager) formatDownloadPath(localPath, nodeID, remotePath string, opts *DownloadOptions) string {
	fileName := getFileNameFromPath(remotePath)
	basePath := localPath

	if opts.Subdir {
		return fmt.Sprintf("%s/%s/%s", basePath, nodeID, fileName)
	}

	if opts.NameFormat != "" {
		formatted := opts.NameFormat
		formatted = replacePlaceholder(formatted, "{node}", nodeID)
		formatted = replacePlaceholder(formatted, "{file}", fileName)
		return fmt.Sprintf("%s/%s", basePath, formatted)
	}

	return fmt.Sprintf("%s/%s.%s", basePath, nodeID, fileName)
}

func getFileNameFromPath(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}

func replacePlaceholder(s, placeholder, value string) string {
	var result []rune
	placeholderRunes := []rune(placeholder)
	plen := len(placeholderRunes)
	sRunes := []rune(s)

	for i := 0; i < len(sRunes); i++ {
		if i+plen <= len(sRunes) && string(sRunes[i:i+plen]) == placeholder {
			result = append(result, []rune(value)...)
			i += plen - 1
		} else {
			result = append(result, sRunes[i])
		}
	}
	return string(result)
}

func extractSpeed(output string) string {
	// 简单的从 rsync 输出中提取速度的示例实现
	return "N/A"
}

func (tm *TransferManager) Close() {
	// 清理 rsync 缓存
	tm.mu.Lock()
	tm.rsyncAvailable = make(map[string]bool)
	tm.mu.Unlock()
}
