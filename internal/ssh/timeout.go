package ssh

import (
	"fmt"
	"time"
)

// TimeoutConfig 超时配置
type TimeoutConfig struct {
	// ConnectTimeout 连接建立超时（默认 10 秒）
	ConnectTimeout time.Duration

	// CommandTimeout 命令执行超时（默认 30 秒）
	CommandTimeout time.Duration
}

// DefaultTimeoutConfig 默认超时配置
func DefaultTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{
		ConnectTimeout: 10 * time.Second,
		CommandTimeout: 30 * time.Second,
	}
}

// TimeoutType 超时类型
type TimeoutType string

const (
	// TimeoutConnect 连接超时
	TimeoutConnect TimeoutType = "connect"
	// TimeoutCommand 命令执行超时
	TimeoutCommand TimeoutType = "command"
)

// TimeoutError 超时错误
type TimeoutError struct {
	Type    TimeoutType
	NodeID  string
	Timeout time.Duration
	Cause   error
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("%s timeout after %v for node %s", e.Type, e.Timeout, e.NodeID)
}

func (e *TimeoutError) Unwrap() error {
	return e.Cause
}

// IsTimeoutError 判断错误是否为超时错误
func IsTimeoutError(err error) bool {
	_, ok := err.(*TimeoutError)
	return ok
}

// GetTimeoutType 获取超时类型
func GetTimeoutType(err error) (TimeoutType, bool) {
	if te, ok := err.(*TimeoutError); ok {
		return te.Type, true
	}
	return "", false
}