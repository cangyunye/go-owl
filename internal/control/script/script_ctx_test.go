package script

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/internal/node"
)

// runCtx 取消后，未开始的脚本步骤不应再发起新的节点解析与 SSH 执行：
// 已取消的 ctx 在 executeScriptOnNode 入口直接短路，否则「取消运行」
// 对 script 步骤形同虚设。

func TestExecuteScript_CancelledCtxSkipsExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	e := NewScriptExecutor(node.NewNodeResolver(), nil)
	opts := &ScriptExecutionOptions{Inline: true, DestDir: t.TempDir(), Timeout: 5 * time.Second, Ctx: ctx}

	results, err := e.ExecuteScript("echo probe", []string{"__no_such_node__"}, opts)
	if err != nil {
		t.Fatalf("单节点取消不应整体报错: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("应返回 1 个节点结果，实际 %d", len(results))
	}
	if results[0].Error == nil || !strings.Contains(results[0].Error.Error(), "已取消") {
		t.Fatalf("已取消 ctx 应短路脚本执行，实际: %+v", results[0])
	}
}
