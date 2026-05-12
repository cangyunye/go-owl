package command

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/cangyunye/go-owl/internal/common/model"
	"github.com/cangyunye/go-owl/internal/control/node"
	"github.com/cangyunye/go-owl/internal/control/task"
)

type CommandExecutor interface {
	Execute(tk *task.Task, nodeMgr node.Manager) error
	ExecuteOnNode(nodeID string, command string, timeout time.Duration) (*task.TaskResult, error)
}

type NodeExecutor interface {
	Execute(command string, timeout time.Duration) (int, string, error)
}

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

type commandExecutor struct {
	nodeMgr node.Manager
	exec    NodeExecutor
}

func NewCommandExecutor(nodeMgr node.Manager, exec NodeExecutor) CommandExecutor {
	if exec == nil {
		exec = &LocalNodeExecutor{}
	}
	return &commandExecutor{
		nodeMgr: nodeMgr,
		exec:    exec,
	}
}

type ExecutionResult struct {
	NodeID   string
	ExitCode int
	Output   string
	Error    error
}

func (e *commandExecutor) Execute(tk *task.Task, nodeMgr node.Manager) error {
	commandPayload, ok := tk.Payload.(*task.CommandPayload)
	if !ok {
		return fmt.Errorf("invalid task payload type")
	}

	timeout := commandPayload.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	var wg sync.WaitGroup
	resultChan := make(chan *ExecutionResult, len(tk.Targets))

	for _, nodeID := range tk.Targets {
		wg.Add(1)
		go func(nid string) {
			defer wg.Done()

			result, err := e.ExecuteOnNode(nid, commandPayload.Command, timeout)
			execResult := &ExecutionResult{
				NodeID: nid,
			}
			if err != nil {
				execResult.Error = err
				execResult.ExitCode = -1
			} else {
				execResult.ExitCode = result.ExitCode
				execResult.Output = result.Output
				execResult.Error = result.Error
			}
			resultChan <- execResult
		}(nodeID)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	for result := range resultChan {
		taskResult := &task.TaskResult{
			NodeID:    result.NodeID,
			ExitCode:  result.ExitCode,
			Output:    result.Output,
			Error:     result.Error,
			StartTime: time.Now().Add(-timeout),
			EndTime:   time.Now(),
		}
		tk.SetResult(result.NodeID, taskResult)
	}

	return nil
}

func (e *commandExecutor) ExecuteOnNode(nodeID string, command string, timeout time.Duration) (*task.TaskResult, error) {
	nodeInfo, err := e.nodeMgr.GetByID(nodeID)
	if err != nil {
		return nil, fmt.Errorf("node not found: %w", err)
	}

	if nodeInfo.Status != model.NodeStatusOnline {
		return nil, fmt.Errorf("node %s is not online (status: %s)", nodeID, nodeInfo.Status)
	}

	startTime := time.Now()
	exitCode, output, err := e.exec.Execute(command, timeout)
	endTime := time.Now()

	return &task.TaskResult{
		NodeID:    nodeID,
		ExitCode:  exitCode,
		Output:    output,
		Error:     err,
		StartTime: startTime,
		EndTime:   endTime,
	}, nil
}

type CommandBuilder struct {
	command strings.Builder
}

func NewCommandBuilder() *CommandBuilder {
	return &CommandBuilder{}
}

func (b *CommandBuilder) Append(cmd string) *CommandBuilder {
	b.command.WriteString(cmd)
	return b
}

func (b *CommandBuilder) Appendf(format string, args ...interface{}) *CommandBuilder {
	b.command.WriteString(fmt.Sprintf(format, args...))
	return b
}

func (b *CommandBuilder) String() string {
	return b.command.String()
}

func ParseCommandArgs(input string) ([]string, error) {
	var args []string
	var current strings.Builder
	inQuote := false
	quoteChar := byte(0)

	for i := 0; i < len(input); i++ {
		c := input[i]

		if !inQuote && (c == '"' || c == '\'') {
			inQuote = true
			quoteChar = c
			continue
		}

		if inQuote && c == quoteChar {
			inQuote = false
			continue
		}

		if !inQuote && c == ' ' {
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
			continue
		}

		current.WriteByte(c)
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	if inQuote {
		return nil, fmt.Errorf("unclosed quote: %c", quoteChar)
	}

	return args, nil
}
