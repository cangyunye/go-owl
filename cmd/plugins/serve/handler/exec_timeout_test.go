package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// 未指定 command_timeout 时必须回落兜底值：否则远端命令挂死会让该节点
// 永久停在"执行中"（服务端没有别的看门狗）。
func TestResolveCommandTimeout(t *testing.T) {
	d, label := resolveCommandTimeout("")
	assert.Equal(t, defaultCommandTimeout, d, "空值应回落默认超时")
	assert.Contains(t, label, "默认")

	d, label = resolveCommandTimeout("45s")
	assert.Equal(t, 45*time.Second, d, "显式超时应生效")
	assert.Equal(t, "45s", label)

	d, _ = resolveCommandTimeout("invalid")
	assert.Equal(t, defaultCommandTimeout, d, "非法值应回落默认超时")

	d, _ = resolveCommandTimeout("-5s")
	assert.Equal(t, defaultCommandTimeout, d, "非正超时应回落默认超时")
}
