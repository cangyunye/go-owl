package script

import (
	"strings"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/internal/node"
)

// 回归:--inline 时 ExecuteScript 曾无视 Inline 选项无条件读取本地文件,
// 导致 "读取脚本文件失败: open echo xxx: no such file or directory"。
// 修复后内容应取自参数本身,流程推进到节点解析(不存在的节点返回单节点失败
// 结果而非整体错误)。
func TestExecuteScriptInlineUsesArgAsContent(t *testing.T) {
	e := NewScriptExecutor(node.NewNodeResolver(), nil)
	opts := &ScriptExecutionOptions{Inline: true, DestDir: t.TempDir(), Timeout: 5 * time.Second}

	results, err := e.ExecuteScript("echo inline-probe", []string{"__no_such_node__"}, opts)
	if err != nil {
		t.Fatalf("inline 模式不应在文件读取阶段失败: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("应返回 1 个节点结果, 实际 %d", len(results))
	}
	if results[0].Error == nil || !strings.Contains(results[0].Error.Error(), "解析节点失败") {
		t.Fatalf("应因节点解析失败而失败, 实际: %+v", results[0])
	}
}
