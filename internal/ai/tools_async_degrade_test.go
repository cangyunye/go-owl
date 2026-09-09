package ai

import (
	"context"
	"strings"
	"testing"
)

// owl async 子命令已随 2026-09-09 评审移除(其内存态实现跨进程不可见,从未真正可用)。
// CLI AI 的 async_* 工具必须优雅降级:向 LLM 返回可操作的指引,
// 而不是让 LLM 收到子进程 "unknown command" 之类的报错。
// 与 web 端 WebExecutor 的存根风格(handler/aiexecutor.go)保持一致。
func TestCLIExecutorAsyncToolsDegradeGracefully(t *testing.T) {
	e := &CLIExecutor{}
	ctx := context.Background()

	t.Run("async_list", func(t *testing.T) {
		_, err := e.AsyncList(ctx)
		assertAsyncDegrade(t, err)
	})

	t.Run("async_status", func(t *testing.T) {
		_, err := e.AsyncStatus(ctx, AsyncStatusParams{TaskID: "task-123"})
		assertAsyncDegrade(t, err)
	})

	t.Run("async_cancel", func(t *testing.T) {
		_, err := e.AsyncCancel(ctx, AsyncStatusParams{TaskID: "task-123"})
		assertAsyncDegrade(t, err)
	})
}

func assertAsyncDegrade(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected degradation error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "owl exec --async") {
		t.Errorf("degradation message should mention `owl exec --async`, got: %s", msg)
	}
	if strings.Contains(msg, "unknown command") {
		t.Errorf("degradation message should not leak subprocess error, got: %s", msg)
	}
}
