package monitor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseJournalCount 验证 wc -l / grep -c 风格计数值解析。
func TestParseJournalCount(t *testing.T) {
	samples, err := ParseJournalCount("42\n", "node-a", "err.journal_errors", 1750000000)
	require.NoError(t, err)
	require.Equal(t, []Sample{
		{NodeID: "node-a", Metric: "err.journal_errors", TS: 1750000000, Value: 42},
	}, samples)

	samples, err = ParseJournalCount("  0 \n", "node-a", "err.oom", 1750000000)
	require.NoError(t, err)
	require.Equal(t, []Sample{
		{NodeID: "node-a", Metric: "err.oom", TS: 1750000000, Value: 0},
	}, samples)
}

// TestParseJournalCount_Invalid 验证非整数输出报错（步骤失败被跳过）。
func TestParseJournalCount_Invalid(t *testing.T) {
	_, err := ParseJournalCount("bin/bash: journalctl: command not found\n", "node-a", "err.oom", 1750000000)
	require.Error(t, err)
}
