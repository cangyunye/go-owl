package playbook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/internal/node"
)

// script 动作此前由 ScriptExecutor 自行拨 SSH，完全绕过 Web 端
// webCommandExecutor 的黑名单检查点：剧本用 script 写一段 rm -rf
// 即可绕过「确认危险命令」交互。注入 ScriptCheckFunc 后，
// 脚本内容（文件与 inline）必须与命令一视同仁地过检查。

func writeTempScript(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "step.sh")
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunAction_ScriptFileBlockedByScriptCheck(t *testing.T) {
	runner := NewDefaultActionRunnerWithOptions(nil, nil, &PlaybookOptions{})
	runner.SetScriptCheckFunc(func(nodeID, content string) error {
		if strings.Contains(content, "rm -rf") {
			return &mockBlockedErr{}
		}
		return nil
	})

	scriptPath := writeTempScript(t, "echo start\nrm -rf /data\n")
	result, err := runner.RunAction("script", map[string]interface{}{
		"script": scriptPath,
	}, "node-1", nil, nil)

	if err == nil || !strings.Contains(err.Error(), "黑名单") {
		t.Fatalf("命中黑名单的脚本文件应被拦截，实际 err=%v", err)
	}
	if result.ExitCode != -1 {
		t.Fatalf("拦截步骤退出码应为 -1，实际 %d", result.ExitCode)
	}
	if result.Error == nil {
		t.Fatal("拦截步骤应携带 Error 供上层判定失败")
	}
}

func TestRunAction_ScriptInlineBlockedByScriptCheck(t *testing.T) {
	runner := NewDefaultActionRunnerWithOptions(nil, nil, &PlaybookOptions{})
	var gotContent string
	runner.SetScriptCheckFunc(func(nodeID, content string) error {
		gotContent = content
		if strings.Contains(content, "rm -rf") {
			return &mockBlockedErr{}
		}
		return nil
	})

	// inline 脚本内容即参数本身：文件存在性检查不得先于内容检查
	// 把 inline 误判成缺失文件（此前 inline 在剧本里完全不可用）。
	result, err := runner.RunAction("script", map[string]interface{}{
		"script": "echo start && rm -rf /data",
		"inline": true,
	}, "node-1", nil, nil)

	if err == nil || !strings.Contains(err.Error(), "黑名单") {
		t.Fatalf("inline 危险脚本应被黑名单拦截，实际 err=%v", err)
	}
	if !strings.Contains(gotContent, "rm -rf") {
		t.Fatalf("ScriptCheckFunc 应收到 inline 脚本内容，实际 %q", gotContent)
	}
	if result.ExitCode != -1 {
		t.Fatalf("拦截步骤退出码应为 -1，实际 %d", result.ExitCode)
	}
}

func TestRunAction_ScriptFileCheckedThenExecuted(t *testing.T) {
	runner := NewDefaultActionRunnerWithOptions(nil, node.NewNodeResolver(), &PlaybookOptions{})
	var checkedNode, checkedContent string
	runner.SetScriptCheckFunc(func(nodeID, content string) error {
		checkedNode = nodeID
		checkedContent = content
		return nil
	})

	scriptPath := writeTempScript(t, "echo inline-probe\n")
	// 检查放行后继续走原执行链：node 不存在时按单节点解析失败返回。
	_, err := runner.RunAction("script", map[string]interface{}{
		"script": scriptPath,
	}, "__no_such_node__", nil, nil)

	if err == nil || !strings.Contains(err.Error(), "解析节点失败") {
		t.Fatalf("放行后应继续执行链并在节点解析处失败，实际 err=%v", err)
	}
	if checkedNode != "__no_such_node__" {
		t.Fatalf("ScriptCheckFunc 应收到节点 ID，实际 %q", checkedNode)
	}
	if !strings.Contains(checkedContent, "inline-probe") {
		t.Fatalf("ScriptCheckFunc 应收到脚本文件内容，实际 %q", checkedContent)
	}
}

func TestRunAction_ScriptCheckNotCalledWhenNil(t *testing.T) {
	// 未注入检查器（CLI 路径自带交互确认）时行为不变：
	// node 解析失败返回单节点失败，而不是整体报错。
	runner := NewDefaultActionRunnerWithOptions(nil, node.NewNodeResolver(), &PlaybookOptions{})
	scriptPath := writeTempScript(t, "echo inline-probe\n")
	_, err := runner.RunAction("script", map[string]interface{}{
		"script": scriptPath,
	}, "__no_such_node__", nil, nil)

	if err == nil || !strings.Contains(err.Error(), "解析节点失败") {
		t.Fatalf("应因节点解析失败而失败，实际 err=%v", err)
	}
}

type mockBlockedErr struct{}

func (*mockBlockedErr) Error() string { return "危险命令已被黑名单拦截" }
