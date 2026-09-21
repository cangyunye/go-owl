package transfer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/cangyunye/go-owl/internal/node"
	"github.com/cangyunye/go-owl/internal/ssh"
)

type TransferManager struct {
	nodeResolver   *node.NodeResolver
	sshConfigPath  string
	rsyncAvailable map[string]bool // nodeID -> available
	mu             sync.RWMutex
}

func NewTransferManager(nodeResolver *node.NodeResolver) *TransferManager {
	return &TransferManager{
		nodeResolver:   nodeResolver,
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

// defaultMaxConcurrency 并行上传的默认并发上限。
const defaultMaxConcurrency = 10

type UploadOptions struct {
	Parallel     bool
	Overwrite    bool
	NoOverwrite  bool
	PreservePerm bool
	Resume       bool
	ChunkSize    int64
	// MaxConcurrency 并行上传（Parallel=true）时的最大并发节点数。
	// 0 或负数 = 使用默认值 10。现有调用方无需修改即获得默认上限。
	MaxConcurrency int
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

	return connInfo, available
}

// checkRsyncRemotely 通过 SSH 检查远程节点是否有 rsync
func (tm *TransferManager) checkRsyncRemotely(nodeInfo *node.ResolvedNode, connInfo *ssh.ConnectionInfo) bool {
	executor := ssh.NewNativeNodeExecutor(connInfo)
	exitCode, _, err := executor.Execute("which rsync", 5*time.Second)
	return err == nil && exitCode == 0
}

// effectiveUploadConcurrency 从上传选项解析并发上限（nil/0/负数 → 默认值）。
func effectiveUploadConcurrency(opts *UploadOptions) int {
	if opts != nil && opts.MaxConcurrency > 0 {
		return opts.MaxConcurrency
	}
	return defaultMaxConcurrency
}

// parallelLimited 以固定 worker 池限并发执行 fn（至多 limit 个同时执行），
// 结果严格按输入顺序返回且与输入一一对应。limit <= 0 时按默认上限处理。
func parallelLimited[T any](ctx context.Context, ids []string, limit int, fn func(context.Context, string) T) []T {
	if limit <= 0 {
		limit = defaultMaxConcurrency
	}
	out := make([]T, len(ids))
	if len(ids) == 0 {
		return out
	}
	jobs := make(chan int)

	workers := limit
	if workers > len(ids) {
		workers = len(ids)
	}
	if workers < 1 {
		workers = 1
	}

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for idx := range jobs {
				// 各 worker 写互不相同的下标，无数据竞争
				out[idx] = fn(ctx, ids[idx])
			}
		}()
	}

	for idx := range ids {
		jobs <- idx
	}
	close(jobs)
	wg.Wait()
	return out
}

func (tm *TransferManager) Upload(ctx context.Context, nodeIDs []string, localPath, remotePath string, opts *UploadOptions) []TransferResult {
	if opts == nil {
		opts = &UploadOptions{
			Parallel: true,
			Resume:   true,
		}
	}

	if opts.Parallel {
		return parallelLimited(ctx, nodeIDs, effectiveUploadConcurrency(opts),
			func(ctx context.Context, id string) TransferResult {
				return tm.smartUpload(ctx, id, localPath, remotePath, opts)
			})
	}

	results := make([]TransferResult, len(nodeIDs))
	for i, nodeID := range nodeIDs {
		results[i] = tm.smartUpload(ctx, nodeID, localPath, remotePath, opts)
	}
	return results
}

func (tm *TransferManager) smartUpload(ctx context.Context, nodeID, localPath, remotePath string, opts *UploadOptions) TransferResult {
	startTime := time.Now()

	connInfo, rsyncOK := tm.CheckRsyncAvailable(ctx, nodeID)
	// 密码认证时跳过 rsync（rsync CLI 不支持密码传递）
	if opts.Resume && rsyncOK && connInfo != nil && connInfo.Password == "" {
		fmt.Printf("[%s] rsync 可用，将使用断点续传\n", nodeID)
		result := tm.rsyncUpload(ctx, nodeID, localPath, remotePath, opts, connInfo, startTime)
		if result.Error == nil {
			return result
		}
		// rsync 失败（如本机无 rsync 可执行文件）时降级 scp，保证文件仍能送达
		fmt.Printf("[%s] rsync 失败（%v），降级为 scp\n", nodeID, result.Error)
		return tm.scpFallback(ctx, nodeID, localPath, remotePath, opts, startTime)
	}

	if rsyncOK && connInfo != nil && connInfo.Password != "" {
		fmt.Printf("[%s] 节点使用密码认证，改用 SSH 原生传输\n", nodeID)
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
	nodeInfo, err := tm.nodeResolver.Resolve(nodeID)
	if err != nil {
		return TransferResult{
			NodeID: nodeID,
			Path:   remotePath,
			Error:  fmt.Errorf("获取节点信息失败: %w", err),
			Method: "scp",
		}
	}

	connInfo, err := ssh.ResolveConnection(nodeInfo.ID, nodeInfo.Address, nodeInfo.Port, nodeInfo.User, nodeInfo.SSHKey, nodeInfo.SSHPassword, tm.sshConfigPath)
	if err != nil {
		return TransferResult{
			NodeID: nodeID,
			Path:   remotePath,
			Error:  fmt.Errorf("解析连接信息失败: %w", err),
			Method: "scp",
		}
	}

	executor := ssh.NewNativeNodeExecutor(connInfo)
	if err := executor.WriteFile(localPath, remotePath); err != nil {
		return TransferResult{
			NodeID:   nodeID,
			Path:     remotePath,
			Error:    fmt.Errorf("文件传输失败: %w", err),
			Method:   "scp",
			Duration: time.Since(startTime),
		}
	}

	return TransferResult{
		NodeID:   nodeID,
		Path:     remotePath,
		Method:   "scp",
		Duration: time.Since(startTime),
	}
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
	// 密码认证时跳过 rsync（rsync CLI 不支持密码传递）
	if opts.Resume && rsyncOK && connInfo != nil && connInfo.Password == "" {
		fmt.Printf("[%s] rsync 可用，将使用断点续传\n", nodeID)
		return tm.rsyncDownload(ctx, nodeID, remotePath, localPath, opts, connInfo, startTime)
	}

	if rsyncOK && connInfo != nil && connInfo.Password != "" {
		fmt.Printf("[%s] 节点使用密码认证，改用 SSH 原生传输\n", nodeID)
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

	connInfo, err := ssh.ResolveConnection(nodeInfo.ID, nodeInfo.Address, nodeInfo.Port, nodeInfo.User, nodeInfo.SSHKey, nodeInfo.SSHPassword, tm.sshConfigPath)
	if err != nil {
		return TransferResult{
			NodeID: nodeID,
			Path:   localPath,
			Error:  fmt.Errorf("解析连接信息失败: %w", err),
			Method: "scp",
		}
	}

	executor := ssh.NewNativeNodeExecutor(connInfo)
	exitCode, output, execErr := executor.Execute(fmt.Sprintf("cat '%s'", remotePath), 60*time.Second)
	if execErr != nil {
		return TransferResult{
			NodeID:   nodeID,
			Path:     localPath,
			Error:    fmt.Errorf("读取远程文件失败: %w", execErr),
			Method:   "scp",
			Duration: time.Since(startTime),
		}
	}
	if exitCode != 0 {
		return TransferResult{
			NodeID:   nodeID,
			Path:     localPath,
			Error:    fmt.Errorf("读取远程文件失败，退出码: %d", exitCode),
			Method:   "scp",
			Duration: time.Since(startTime),
		}
	}

	if err := os.WriteFile(localPath, []byte(output), 0644); err != nil {
		return TransferResult{
			NodeID:   nodeID,
			Path:     localPath,
			Error:    fmt.Errorf("写入本地文件失败: %w", err),
			Method:   "scp",
			Duration: time.Since(startTime),
		}
	}

	return TransferResult{
		NodeID:   nodeID,
		Path:     localPath,
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
