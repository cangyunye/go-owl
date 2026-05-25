package command

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cangyunye/go-owl/internal/control/async"
	controlnode "github.com/cangyunye/go-owl/internal/control/node"
	"github.com/cangyunye/go-owl/internal/control/task"
	"github.com/cangyunye/go-owl/internal/node"
	"github.com/cangyunye/go-owl/internal/ssh"
	"github.com/cangyunye/go-owl/internal/logger"
	"go.uber.org/zap"
)

type Executor struct {
	nodeResolver *node.NodeResolver
	pool         *ssh.ConnectionPool
	debug        bool
}

func NewExecutor(nodeResolver *node.NodeResolver) *Executor {
	return &Executor{
		nodeResolver: nodeResolver,
		pool:         ssh.NewConnectionPool(10, 5*time.Minute),
		debug:        false,
	}
}

// SetDebug 设置 debug 模式
func (e *Executor) SetDebug(debug bool) {
	e.debug = debug
}

func (e *Executor) Run(ctx context.Context, nodeIDs []string, command string, opts *ExecuteOptions) []CommandResult {
	if opts == nil {
		opts = &ExecuteOptions{
			Parallel: true,
			Timeout:  30 * time.Second,
		}
	}

	if opts.RetryConfig != nil {
		return e.runWithRetry(ctx, nodeIDs, command, opts)
	}

	if opts.Parallel {
		return e.runParallel(ctx, nodeIDs, command, opts)
	}
	return e.runSequential(ctx, nodeIDs, command, opts)
}

func (e *Executor) runWithRetry(ctx context.Context, nodeIDs []string, command string, opts *ExecuteOptions) []CommandResult {
	retryExecutor := NewRetryExecutor(e, opts.RetryConfig)

	if opts.Parallel {
		retryResults := retryExecutor.RunParallelWithRetry(ctx, nodeIDs, command, opts)
		results := make([]CommandResult, len(retryResults))
		for i, r := range retryResults {
			results[i] = r.CommandResult
		}
		return results
	}

	retryResults := retryExecutor.RunWithRetry(ctx, nodeIDs, command, opts)
	results := make([]CommandResult, len(retryResults))
	for i, r := range retryResults {
		results[i] = r.CommandResult
	}
	return results
}

func (e *Executor) runParallel(ctx context.Context, nodeIDs []string, command string, opts *ExecuteOptions) []CommandResult {
	results := make([]CommandResult, len(nodeIDs))
	var wg sync.WaitGroup
	wg.Add(len(nodeIDs))

	for i, nodeID := range nodeIDs {
		go func(idx int, id string) {
			defer wg.Done()
			results[idx] = e.runOnNode(ctx, id, command, opts)
		}(i, nodeID)
	}

	wg.Wait()
	return results
}

func (e *Executor) runSequential(ctx context.Context, nodeIDs []string, command string, opts *ExecuteOptions) []CommandResult {
	results := make([]CommandResult, len(nodeIDs))

	for i, nodeID := range nodeIDs {
		results[i] = e.runOnNode(ctx, nodeID, command, opts)
	}

	return results
}

func (e *Executor) runOnNode(ctx context.Context, nodeID, command string, opts *ExecuteOptions) CommandResult {
	result := CommandResult{
		NodeID: nodeID,
	}
	debugInfo := []string{}

	startTime := time.Now()
	if e.debug {
		logger.Debug("开始执行命令", zap.String("nodeID", nodeID), zap.String("command", command))
		debugInfo = append(debugInfo, fmt.Sprintf("开始时间: %s", startTime.Format(time.RFC3339)))
	}

	// 1. 解析节点信息
	if e.debug {
		logger.Debug("解析节点信息", zap.String("nodeID", nodeID))
	}
	nodeInfo, err := e.nodeResolver.Resolve(nodeID)
	if err != nil {
		if e.debug {
			logger.Debug("解析节点失败", zap.String("nodeID", nodeID), logger.WithError(err))
		}
		result.Error = fmt.Errorf("获取节点信息失败: %w", err)
		result.ErrorType = ErrorTypeNode
		result.ErrorDetail = err.Error()
		result.DebugInfo = append(debugInfo, fmt.Sprintf("错误: %v", err))
		return result
	}

	if e.debug {
		logger.Debug("节点信息解析成功", zap.String("nodeID", nodeID))
		debugInfo = append(debugInfo, fmt.Sprintf("节点地址: %s:%d", nodeInfo.Address, nodeInfo.Port))
		debugInfo = append(debugInfo, fmt.Sprintf("用户: %s", nodeInfo.User))
	}

	// 2. 建立连接
	connectStart := time.Now()
	if e.debug {
		logger.Debug("建立 SSH 连接", zap.String("nodeID", nodeID))
	}
	executor, err := e.pool.Get(nodeInfo)
	connectDuration := time.Since(connectStart)
	if err != nil {
		if e.debug {
			logger.Debug("SSH 连接失败", zap.String("nodeID", nodeID), logger.WithError(err))
		}
		// 分析错误类型
		errMsg := err.Error()
		errType := ErrorTypeConnection
		if containsAny(errMsg, "auth", "password", "key", "permission") {
			errType = ErrorTypeAuth
		} else if containsAny(errMsg, "timeout", "timed out") {
			errType = ErrorTypeTimeout
		}

		result.Error = fmt.Errorf("连接节点失败: %w", err)
		result.ErrorType = errType
		result.ErrorDetail = errMsg
		result.DebugInfo = append(debugInfo, fmt.Sprintf("连接耗时: %v", connectDuration))
		result.DebugInfo = append(debugInfo, fmt.Sprintf("错误: %v", err))
		return result
	}
	defer e.pool.Put(nodeID)

	if e.debug {
		logger.Debug("SSH 连接成功", zap.String("nodeID", nodeID), zap.String("duration", connectDuration.String()))
		debugInfo = append(debugInfo, fmt.Sprintf("连接成功, 耗时: %v", connectDuration))
	}

	// 3. 执行命令
	fullCommand := command
	if opts.WorkingDir != "" {
		fullCommand = fmt.Sprintf("cd %s && %s", opts.WorkingDir, command)
	}

	execStart := time.Now()
	if e.debug {
		logger.Debug("执行命令", zap.String("nodeID", nodeID), zap.String("command", fullCommand))
	}

	var exitCode int
	var output string
	var execErr error

	if opts.TimeoutConfig != nil {
		exitCode, output, execErr = executor.ExecuteWithConfig(fullCommand, opts.TimeoutConfig)
	} else {
		timeout := opts.Timeout
		if timeout == 0 {
			timeout = 30 * time.Second
		}
		exitCode, output, execErr = executor.Execute(fullCommand, timeout)
	}

	execDuration := time.Since(execStart)

	if e.debug {
		if execErr != nil {
			logger.Debug("命令执行失败", zap.String("nodeID", nodeID), logger.WithError(execErr))
		} else {
			logger.Debug("命令执行完成", zap.String("nodeID", nodeID), zap.String("duration", execDuration.String()), zap.Int("exitCode", exitCode))
		}
		debugInfo = append(debugInfo, fmt.Sprintf("执行命令: %s", fullCommand))
		debugInfo = append(debugInfo, fmt.Sprintf("执行耗时: %v", execDuration))
		debugInfo = append(debugInfo, fmt.Sprintf("退出码: %d", exitCode))
	}

	if execErr != nil {
		errType := ErrorTypeCommand
		errMsg := execErr.Error()

		// 检查是否为 SSH 认证错误
		var sshAuthErr *ssh.SSHAuthError
		if ok := errors.As(execErr, &sshAuthErr); ok {
			errType = ErrorTypeAuth
			if strings.Contains(sshAuthErr.Stderr, "timeout") || strings.Contains(sshAuthErr.Stderr, "refused") {
				errType = ErrorTypeConnection
			}
		} else if containsAny(errMsg, "timeout", "timed out") {
			errType = ErrorTypeTimeout
		}

		result.Output = output
		result.ExitCode = exitCode
		result.Error = execErr
		result.ErrorType = errType
		result.ErrorDetail = errMsg
		result.DebugInfo = debugInfo
		return result
	}

	// 4. 执行成功
	result.Output = output
	result.ExitCode = exitCode
	result.Success = exitCode == 0
	result.DebugInfo = debugInfo

	// 检查是否是命令级别的失败（比如命令不存在）
	if exitCode != 0 {
		if exitCode == 255 {
			// 255 是 SSH 特有的退出码，表示连接/认证失败
			result.ErrorType = ErrorTypeAuth
			if output != "" && (strings.Contains(output, "timeout") || strings.Contains(output, "refused")) {
				result.ErrorType = ErrorTypeConnection
			}
			result.ErrorDetail = fmt.Sprintf("SSH 连接失败（退出码 255）")
			result.Error = fmt.Errorf("SSH 连接失败，退出码 255: %s", truncateStr(output, 256))
		} else {
			result.ErrorType = ErrorTypeCommand
			result.ErrorDetail = fmt.Sprintf("命令执行失败，退出码 %d", exitCode)
			result.Error = fmt.Errorf("命令执行失败，退出码 %d", exitCode)
		}
		result.Success = false
	}

	return result
}

func (e *Executor) Execute(tk *task.Task, nodeMgr controlnode.Manager) error {
	return nil
}

func (e *Executor) ExecuteOnNode(nodeID string, cmd string, timeout time.Duration) (*task.TaskResult, error) {
	ctx := context.Background()
	opts := &ExecuteOptions{
		Parallel: false,
		Timeout:  timeout,
	}
	results := e.Run(ctx, []string{nodeID}, cmd, opts)
	if len(results) == 0 {
		return nil, fmt.Errorf("no result from node %s", nodeID)
	}
	r := results[0]
	startTime := time.Now()
	return &task.TaskResult{
		NodeID:    r.NodeID,
		ExitCode:  r.ExitCode,
		Output:    r.Output,
		StartTime: startTime,
		EndTime:   startTime,
	}, r.Error
}

func (e *Executor) Close() {
	if e.pool != nil {
		e.pool.Close()
	}
}

func (e *Executor) RunStreaming(ctx context.Context, nodeIDs []string, command string, opts *ExecuteOptions) <-chan CommandResult {
	results := make(chan CommandResult, len(nodeIDs))

	go func() {
		defer close(results)
		for _, nodeID := range nodeIDs {
			result := e.runOnNode(ctx, nodeID, command, opts)
			results <- result
		}
	}()

	return results
}

// RunAsync 异步执行命令
func (e *Executor) RunAsync(ctx context.Context, nodeIDs []string, command string, asyncOpts *async.AsyncOptions) ([]*async.AsyncTask, error) {
	if asyncOpts == nil {
		asyncOpts = &async.AsyncOptions{
			Timeout:      1 * time.Hour,
			PollInterval: 10 * time.Second,
			MaxPollCount: 3600,
		}
	}

	manager := async.NewAsyncTaskManager(asyncOpts)
	tasks := make([]*async.AsyncTask, len(nodeIDs))
	var wg sync.WaitGroup
	wg.Add(len(nodeIDs))

	for i, nodeID := range nodeIDs {
		go func(idx int, id string) {
			defer wg.Done()
			task, err := manager.StartAsync(ctx, id, command, asyncOpts)
			if err != nil {
				tasks[idx] = &async.AsyncTask{
					NodeID: id,
					Status: async.AsyncTaskStatusFailed,
					Error:  err,
				}
				return
			}
			tasks[idx] = task
		}(i, nodeID)
	}

	wg.Wait()
	return tasks, nil
}

func containsAny(s string, substrs ...string) bool {
	sLower := strings.ToLower(s)
	for _, substr := range substrs {
		if strings.Contains(sLower, strings.ToLower(substr)) {
			return true
		}
	}
	return false
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
