package monitor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSyntaxGate_Bash 验证 bash -n 校验：合法通过、非法拦截（含错误信息）。
func TestSyntaxGate_Bash(t *testing.T) {
	g := NewSyntaxGate()

	ok, reason := g.Validate("script", "echo hello\nls -la /tmp\n")
	require.True(t, ok, "合法脚本应通过: %s", reason)

	ok, reason = g.Validate("script", "if [ -f /tmp/x ]; then echo hi\n")
	require.False(t, ok, "非法脚本应拦截")
	require.NotEmpty(t, reason, "应返回错误原因")
}

// TestSyntaxGate_YAML 验证剧本 YAML 语法校验。
func TestSyntaxGate_YAML(t *testing.T) {
	g := NewSyntaxGate()

	ok, reason := g.Validate("playbook", "name: test\ntasks:\n  - name: ping\n    command: echo hi\n")
	require.True(t, ok, "合法 YAML 应通过: %s", reason)

	ok, _ = g.Validate("playbook", "name: broken\ntasks:\n  - name: [unclosed\n")
	require.False(t, ok, "非法 YAML 应拦截")
}

// TestSyntaxGate_SOP 验证 sop 人工指引无需校验。
func TestSyntaxGate_SOP(t *testing.T) {
	g := NewSyntaxGate()
	ok, _ := g.Validate("sop", "1. 查看 df -h\n2. 评估扩容\n")
	require.True(t, ok)
}

// TestSyntaxGate_BashMissing 验证 bash 不可用时失败关闭（AI 脚本不可执行）。
func TestSyntaxGate_BashMissing(t *testing.T) {
	g := &SyntaxGate{bashPath: "/nonexistent/bash"}
	ok, reason := g.Validate("script", "echo hi\n")
	require.False(t, ok, "bash 缺失必须失败关闭")
	require.Contains(t, reason, "bash")
}
