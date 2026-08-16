package command

import (
	"time"

	"github.com/cangyunye/go-owl/internal/control/node"
	"github.com/cangyunye/go-owl/internal/control/task"
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
	ExecuteOnNodeWithConfig(nodeID string, command string, config *ssh.TimeoutConfig) (*task.TaskResult, error)
}

type NodeExecutor interface {
	Execute(command string, timeout time.Duration) (int, string, error)
}
