package command

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cangyunye/go-owl/internal/control/async"
	"github.com/cangyunye/go-owl/internal/node"
	"github.com/cangyunye/go-owl/internal/ssh"
)

type Executor struct {
	nodeResolver *node.NodeResolver
	pool         *ssh.ConnectionPool
}

func NewExecutor(nodeResolver *node.NodeResolver) *Executor {
	return &Executor{
		nodeResolver: nodeResolver,
		pool:         ssh.NewConnectionPool(10, 5*time.Minute),
	}
}

type CommandResult struct {
	NodeID   string
	Output   string
	ExitCode int
	Error    error
	Success  bool
}

type ExecuteOptions struct {
	Parallel      bool
	Timeout       time.Duration
	TimeoutConfig *ssh.TimeoutConfig
	RetryConfig   *RetryConfig
	WorkingDir    string
	Env           map[string]string
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
	nodeInfo, err := e.nodeResolver.Resolve(nodeID)
	if err != nil {
		return CommandResult{
			NodeID:   nodeID,
			Output:   "",
			ExitCode: -1,
			Error:    fmt.Errorf("获取节点信息失败: %w", err),
			Success:  false,
		}
	}

	executor, err := e.pool.Get(nodeInfo)
	if err != nil {
		return CommandResult{
			NodeID:   nodeID,
			Output:   "",
			ExitCode: -1,
			Error:    fmt.Errorf("连接节点失败: %w", err),
			Success:  false,
		}
	}
	defer e.pool.Put(nodeID)

	fullCommand := command
	if opts.WorkingDir != "" {
		fullCommand = fmt.Sprintf("cd %s && %s", opts.WorkingDir, command)
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

	if execErr != nil {
		return CommandResult{
			NodeID:   nodeID,
			Output:   output,
			ExitCode: exitCode,
			Error:    execErr,
			Success:  false,
		}
	}

	return CommandResult{
		NodeID:   nodeID,
		Output:   output,
		ExitCode: exitCode,
		Error:    nil,
		Success:  exitCode == 0,
	}
}

func (e *Executor) Close() {
	if e.pool != nil {
		e.pool.Close()
	}
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
