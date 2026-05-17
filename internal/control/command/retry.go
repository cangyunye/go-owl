package command

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// RetryConfig 重试配置
type RetryConfig struct {
	// MaxRetries 最大重试次数（默认 3）
	MaxRetries int

	// InitialInterval 初始重试间隔（默认 1 秒）
	InitialInterval time.Duration

	// MaxInterval 最大重试间隔（默认 30 秒）
	MaxInterval time.Duration

	// BackoffMultiplier 退避乘数（默认 2.0）
	BackoffMultiplier float64

	// RetryableErrors 可重试的错误列表
	RetryableErrors []string

	// EnableExponentialBackoff 是否启用指数退避
	EnableExponentialBackoff bool
}

// DefaultRetryConfig 默认重试配置
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:              3,
		InitialInterval:         1 * time.Second,
		MaxInterval:             30 * time.Second,
		BackoffMultiplier:       2.0,
		RetryableErrors:         []string{"connection refused", "timeout", "temporary failure"},
		EnableExponentialBackoff: true,
	}
}

// RetryResult 重试结果
type RetryResult struct {
	// CommandResult 最终的命令结果
	CommandResult

	// TotalAttempts 总尝试次数
	TotalAttempts int

	// RetryHistory 重试历史
	RetryHistory []RetryAttempt

	// FinalError 最终错误（如果全部失败）
	FinalError error

	// Retried 是否进行了重试
	Retried bool
}

// RetryAttempt 单次重试尝试
type RetryAttempt struct {
	Attempt    int
	StartTime  time.Time
	EndTime    time.Time
	Error      error
	Duration   time.Duration
}

// RetryableError 可重试的错误
type RetryableError struct {
	OriginalError error
	RetryCount    int
	NextRetryAt   time.Time
}

func (e *RetryableError) Error() string {
	return fmt.Sprintf("retryable error after %d attempts: %v", e.RetryCount, e.OriginalError)
}

// IsRetryable 判断错误是否可重试
func IsRetryable(err error, config *RetryConfig) bool {
	if err == nil {
		return false
	}

	errMsg := strings.ToLower(err.Error())

	// 检查自定义可重试错误列表
	if config != nil {
		for _, pattern := range config.RetryableErrors {
			if strings.Contains(errMsg, strings.ToLower(pattern)) {
				return true
			}
		}
	}

	// 内置可重试错误检测
	switch {
	case strings.Contains(errMsg, "connection refused"):
		return true
	case strings.Contains(errMsg, "timeout"):
		return true
	case strings.Contains(errMsg, "temporary failure"):
		return true
	case strings.Contains(errMsg, "i/o timeout"):
		return true
	case strings.Contains(errMsg, "network is unreachable"):
		return true
	case strings.Contains(errMsg, "no route to host"):
		return true
	case strings.Contains(errMsg, "host is unreachable"):
		return true
	case strings.Contains(errMsg, "connection timed out"):
		return true
	case strings.Contains(errMsg, "broken pipe"):
		return true
	case strings.Contains(errMsg, "reset by peer"):
		return true
	default:
		return false
	}
}

// calculateInterval 计算重试间隔
func calculateInterval(attempt int, config *RetryConfig) time.Duration {
	if config == nil {
		defaultConfig := DefaultRetryConfig()
		config = &defaultConfig
	}

	interval := float64(config.InitialInterval)

	if config.EnableExponentialBackoff {
		interval *= math.Pow(config.BackoffMultiplier, float64(attempt))
	} else {
		interval *= float64(attempt + 1)
	}

	// 添加抖动（10%）避免惊群效应
	jitter := time.Duration(rand.Float64() * float64(interval) * 0.1)
	interval += float64(jitter)

	// 限制最大间隔
	if time.Duration(interval) > config.MaxInterval {
		interval = float64(config.MaxInterval)
	}

	return time.Duration(interval)
}

// RetryExecutor 带重试的执行器
type RetryExecutor struct {
	executor *Executor
	config   *RetryConfig
}

// NewRetryExecutor 创建重试执行器
func NewRetryExecutor(executor *Executor, config *RetryConfig) *RetryExecutor {
	if config == nil {
		defaultConfig := DefaultRetryConfig()
		config = &defaultConfig
	}
	return &RetryExecutor{
		executor: executor,
		config:   config,
	}
}

// RunWithRetry 执行命令并重试
func (e *RetryExecutor) RunWithRetry(ctx context.Context, nodeIDs []string, command string, opts *ExecuteOptions) []RetryResult {
	results := make([]RetryResult, len(nodeIDs))

	for i, nodeID := range nodeIDs {
		results[i] = e.runOnNodeWithRetry(ctx, nodeID, command, opts)
	}

	return results
}

func (e *RetryExecutor) runOnNodeWithRetry(ctx context.Context, nodeID, command string, opts *ExecuteOptions) RetryResult {
	var history []RetryAttempt
	totalAttempts := 0

	for attempt := 0; attempt <= e.config.MaxRetries; attempt++ {
		totalAttempts++
		startTime := time.Now()

		// 执行命令
		result := e.executor.runOnNode(ctx, nodeID, command, opts)

		endTime := time.Now()
		duration := endTime.Sub(startTime)

		history = append(history, RetryAttempt{
			Attempt:   totalAttempts,
			StartTime: startTime,
			EndTime:   endTime,
			Error:     result.Error,
			Duration:  duration,
		})

		// 检查是否成功
		if result.Success {
			return RetryResult{
				CommandResult: result,
				TotalAttempts: totalAttempts,
				RetryHistory:  history,
				Retried:       attempt > 0,
			}
		}

		// 检查是否可以重试
		if attempt < e.config.MaxRetries && IsRetryable(result.Error, e.config) {
			// 计算等待时间
			interval := calculateInterval(attempt, e.config)

			select {
			case <-time.After(interval):
				continue
			case <-ctx.Done():
				return RetryResult{
					CommandResult: result,
					TotalAttempts: totalAttempts,
					RetryHistory:  history,
					FinalError:    ctx.Err(),
					Retried:       attempt > 0,
				}
			}
		} else {
			// 不可重试或已达最大次数
			return RetryResult{
				CommandResult: result,
				TotalAttempts: totalAttempts,
				RetryHistory:  history,
				FinalError:    result.Error,
				Retried:       attempt > 0,
			}
		}
	}

	return RetryResult{
		CommandResult: CommandResult{Success: false, Error: fmt.Errorf("max retries exceeded")},
		TotalAttempts: totalAttempts,
		RetryHistory:  history,
		FinalError:   fmt.Errorf("max retries exceeded"),
		Retried:      true,
	}
}

// RunParallelWithRetry 并行执行命令并重试
func (e *RetryExecutor) RunParallelWithRetry(ctx context.Context, nodeIDs []string, command string, opts *ExecuteOptions) []RetryResult {
	results := make([]RetryResult, len(nodeIDs))
	var wg sync.WaitGroup
	wg.Add(len(nodeIDs))

	for i, nodeID := range nodeIDs {
		go func(idx int, id string) {
			defer wg.Done()
			results[idx] = e.runOnNodeWithRetry(ctx, id, command, opts)
		}(i, nodeID)
	}

	wg.Wait()
	return results
}