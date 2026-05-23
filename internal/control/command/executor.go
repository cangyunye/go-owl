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
	"github.com/cangyunye/go-owl/internal/history"
	"github.com/cangyunye/go-owl/internal/logger"
	"github.com/cangyunye/go-owl/internal/ssh"
)

// ErrorType 错误类型
type ErrorType int

const (
	ErrorTypeUnknown    ErrorType = iota // 未知错误
	ErrorTypeNode                        // 节点相关错误
	ErrorTypeConnection                  // 连接失败
	ErrorTypeAuth                        // 认证失败
	ErrorTypeTimeout                     // 超时
	ErrorTypeCommand                     // 命令执行错误
)

// String 返回错误类型的可读字符串
func (t ErrorType) String() string {
	switch t {
	case ErrorTypeNode:
		return "节点错误"
	case ErrorTypeConnection:
		return "连接失败"
	case ErrorTypeAuth:
		return "认证失败"
	case ErrorTypeTimeout:
		return "超时"
	case ErrorTypeCommand:
		return "命令错误"
	default:
		return "未知错误"
	}
}

// Suggestion 返回对应错误的建议
func (t ErrorType) Suggestion() string {
	switch t {
	case ErrorTypeNode:
		return "请检查节点配置是否正确"
	case ErrorTypeConnection:
		return "请检查网络连接和节点地址"
	case ErrorTypeAuth:
		return "请检查用户名、密码或密钥配置"
	case ErrorTypeTimeout:
		return "请使用 --connect-timeout 或 --command-timeout 调整超时时间"
	case ErrorTypeCommand:
		return "请检查命令语法和脚本路径"
	default:
		return "请查看详细日志"
	}
}

type CommandResult struct {
	NodeID      string
	Output      string
	ExitCode    int
	Error       error
	ErrorType   ErrorType
	ErrorDetail string
	DebugInfo   []string
	Success     bool
	Duration    time.Duration
}

type ExecuteOptions struct {
	Parallel      bool
	Timeout       time.Duration
	TimeoutConfig *ssh.TimeoutConfig
	RetryConfig   *RetryConfig
	WorkingDir    string
	Env           map[string]string
}

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

	// 记录操作开始
	logger.Info("Starting command execution",
		logger.WithTaskID(tk.ID),
		logger.WithOperation("command"),
	)
	history.RecordOperation(&history.Operation{
		TaskID:    tk.ID,
		OpType:    "command",
		Command:   commandPayload.Command,
		Targets:   tk.Targets,
		Status:    "running",
		CreatedAt: time.Now(),
	})

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

		// 记录命令执行历史
		errorMsg := ""
		if result.Error != nil {
			errorMsg = result.Error.Error()
		}
		duration := taskResult.EndTime.Sub(taskResult.StartTime)
		history.RecordCommandExecution(&history.CommandExecution{
			TaskID:     tk.ID,
			NodeID:     result.NodeID,
			Command:    commandPayload.Command,
			ExitCode:   result.ExitCode,
			Stdout:     truncateString(result.Output, 4096),
			Stderr:     errorMsg,
			DurationMs: duration.Milliseconds(),
			Success:    result.ExitCode == 0,
			CreatedAt:  time.Now(),
		})
	}

	// 更新操作状态
	finalStatus := "completed"
	if len(tk.Results) > 0 && tk.FailureCount() > 0 {
		finalStatus = "partial_failure"
		if tk.SuccessCount() == 0 {
			finalStatus = "failed"
		}
	}
	logger.Info("Command execution completed",
		logger.WithTaskID(tk.ID),
		logger.WithOperation("command"),
	)
	history.RecordOperation(&history.Operation{
		TaskID:    tk.ID,
		OpType:    "command",
		Command:   commandPayload.Command,
		Targets:   tk.Targets,
		Status:    finalStatus,
		CreatedAt: time.Now(),
	})

	return nil
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
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
