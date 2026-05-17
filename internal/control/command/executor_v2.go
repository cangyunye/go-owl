package command

import (
	"context"
	"fmt"
	"sync"
	"time"

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
	Parallel   bool
	Timeout    time.Duration
	WorkingDir string
	Env        map[string]string
}

func (e *Executor) Run(ctx context.Context, nodeIDs []string, command string, opts *ExecuteOptions) []CommandResult {
	if opts == nil {
		opts = &ExecuteOptions{
			Parallel: true,
			Timeout:  30 * time.Second,
		}
	}

	if opts.Parallel {
		return e.runParallel(ctx, nodeIDs, command, opts)
	}
	return e.runSequential(ctx, nodeIDs, command, opts)
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

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	fullCommand := command
	if opts.WorkingDir != "" {
		fullCommand = fmt.Sprintf("cd %s && %s", opts.WorkingDir, command)
	}

	exitCode, output, err := executor.Execute(fullCommand, timeout)

	if err != nil {
		return CommandResult{
			NodeID:   nodeID,
			Output:   output,
			ExitCode: exitCode,
			Error:    err,
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
