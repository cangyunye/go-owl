package async

import (
	"time"
)

// AsyncTask 异步任务
type AsyncTask struct {
	// ID 任务唯一标识
	ID string

	// NodeID 节点ID
	NodeID string

	// Command 执行的命令
	Command string

	// StartTime 开始时间
	StartTime time.Time

	// EndTime 结束时间（如果已完成）
	EndTime time.Time

	// Status 任务状态
	Status AsyncTaskStatus

	// Pid 远程进程 PID
	Pid int

	// OutputFile 输出文件路径
	OutputFile string

	// ExitCode 退出码
	ExitCode int

	// Error 错误信息
	Error error

	// AsyncTimeout 异步超时时间
	AsyncTimeout time.Duration

	// RemoteBaseDir 远程工作目录
	RemoteBaseDir string
}

// AsyncTaskStatus 异步任务状态
type AsyncTaskStatus string

const (
	AsyncTaskStatusPending   AsyncTaskStatus = "pending"    // 等待执行
	AsyncTaskStatusRunning   AsyncTaskStatus = "running"    // 执行中
	AsyncTaskStatusSuccess   AsyncTaskStatus = "success"    // 执行成功
	AsyncTaskStatusFailed    AsyncTaskStatus = "failed"     // 执行失败
	AsyncTaskStatusTimeout   AsyncTaskStatus = "timeout"    // 超时
	AsyncTaskStatusCanceled  AsyncTaskStatus = "canceled"   // 已取消
)

// AsyncOptions 异步执行选项
type AsyncOptions struct {
	// Timeout 异步任务最大运行时间（默认 1 小时）
	Timeout time.Duration

	// PollInterval 轮询间隔（0 表示 fire-and-forget）
	PollInterval time.Duration

	// MaxPollCount 最大轮询次数（默认 3600）
	MaxPollCount int

	// Async 是否异步执行（true）或同步执行（false）
	Async bool

	// RemoteBaseDir 远程工作目录（默认 /tmp/owl）
	RemoteBaseDir string
}

// DefaultAsyncOptions 默认配置
func DefaultAsyncOptions() AsyncOptions {
	return AsyncOptions{
		Timeout:      1 * time.Hour,
		PollInterval: 10 * time.Second,
		MaxPollCount: 3600,
		Async:        false,
		RemoteBaseDir: "/tmp/owl",
	}
}

// IsCompleted 判断任务是否已完成
func (task *AsyncTask) IsCompleted() bool {
	return task.Status == AsyncTaskStatusSuccess ||
		task.Status == AsyncTaskStatusFailed ||
		task.Status == AsyncTaskStatusTimeout ||
		task.Status == AsyncTaskStatusCanceled
}

// Duration 获取任务执行时长
func (task *AsyncTask) Duration() time.Duration {
	if task.EndTime.IsZero() {
		return time.Since(task.StartTime)
	}
	return task.EndTime.Sub(task.StartTime)
}