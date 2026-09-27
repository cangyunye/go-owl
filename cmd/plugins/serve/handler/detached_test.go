package handler

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// 分离运行的命令包装：外层要立刻返回（只打印 pid 与日志路径），真正的进程 setsid+nohup 脱离。
func TestWrapDetachedCommand(t *testing.T) {
	logPath := "/tmp/owl-detached-node-1-123.log"
	wrapped := wrapDetachedCommand("for i in 1 2 3; do echo tick $i; sleep 1; done", logPath)

	for _, want := range []string{
		"LOG=" + logPath,
		"setsid nohup sh -c",
		`echo "detached_pid=$! log=$LOG"`,
		">> \"$LOG\" 2>&1 &",
	} {
		assert.Contains(t, wrapped, want, "包装后的命令缺少 %q", want)
	}

	// 命令里的单引号必须被 shell 转义，否则会截断命令
	quoted := wrapDetachedCommand(`echo 'hello world'`, logPath)
	assert.NotContains(t, strings.ReplaceAll(quoted, `'\''`, ""), "echo 'hello world'",
		"命令应被单引号包裹并转义内部单引号")
	assert.Contains(t, quoted, `'\''`, "内部单引号应转义为关闭+反斜杠+重开")
}

// 从任务输出取回 pid 与日志路径；普通任务输出应判定为「不是分离运行」
func TestParseDetachedMeta(t *testing.T) {
	pid, logPath, ok := parseDetachedMeta("detached_pid=4321 log=/tmp/owl-detached-web-1-9.log\n")
	assert.True(t, ok)
	assert.Equal(t, 4321, pid)
	assert.Equal(t, "/tmp/owl-detached-web-1-9.log", logPath)

	_, _, ok = parseDetachedMeta("uptime\n 12:00 up 3 days\n")
	assert.False(t, ok, "普通输出不应被当成分离运行")
}
