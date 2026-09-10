package main

import (
	"bytes"
	"testing"

	"github.com/cangyunye/go-owl/internal/i18n"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRootCmd_VersionFlag --version 输出构建注入的版本信息。
func TestRootCmd_VersionFlag(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--version"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})

	require.NoError(t, cmd.Execute())
	got := out.String()
	assert.Contains(t, got, "owl-serve")
	assert.Contains(t, got, "dev", "默认构建应为 dev")
}

// TestRootCmd_HelpTextFromI18n 帮助文案与 CLI 共用同一 i18n 目录，
// 不再是 main 里硬编码的中文（上轮评审 P1-3 遗留项）。
func TestRootCmd_HelpTextFromI18n(t *testing.T) {
	cmd := newRootCmd()
	assert.Equal(t, i18n.T("serve.cmd.short"), cmd.Short)
	assert.Equal(t, i18n.T("serve.cmd.long"), cmd.Long)
}

// TestRootCmd_FlagsAlignedWithCLI 与 owl serve 包装器的 flag 集合保持一致：
// 两端靠字符串拼接传参（上轮评审 P2-2），新增 flag 必须两端同步。
func TestRootCmd_FlagsAlignedWithCLI(t *testing.T) {
	cmd := newRootCmd()
	for _, name := range []string{"port", "host", "dev", "reset-admin", "ai-debug"} {
		assert.NotNil(t, cmd.Flags().Lookup(name), "缺少 flag --%s", name)
	}
}
