package monitor

import (
	"errors"
	"time"
)

// timeoutExecer 模拟执行超时。
type timeoutExecer struct{}

func (t *timeoutExecer) Execute(command string, timeout time.Duration) (int, string, error) {
	return -1, "", errors.New("命令执行超时")
}
