package handler

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// 重试期间节点会长时间静默（connect/command 超时 × 重试次数），用户无法区分
// "还在重试"与"卡死"，必须把重试进度播报到实时终端。
func TestRetryNotice(t *testing.T) {
	msg := retryNotice(1, 4, 2*time.Second, errors.New("ssh dial: i/o timeout"))
	assert.Contains(t, msg, "1/4", "应显示第几次尝试")
	assert.Contains(t, msg, "ssh dial: i/o timeout", "应带上失败原因")
	assert.Contains(t, msg, "2s 后重试", "应提示下次重试等待时长")

	// 最后一次失败（不再重试）不应提示等待
	msg = retryNotice(4, 4, 0, errors.New("ssh dial: i/o timeout"))
	assert.Contains(t, msg, "4/4")
	assert.NotContains(t, msg, "后重试")
}
