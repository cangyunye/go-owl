package monitor

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCollector_LocalIntegration 在本地 Linux 上跑一轮真实采集（SSH 工厂的
// 本地执行器路径）。非 Linux（无 /proc）环境跳过。
func TestCollector_LocalIntegration(t *testing.T) {
	if _, err := os.Stat("/proc/loadavg"); err != nil {
		t.Skip("非 Linux 环境，跳过本地集成采集测试")
	}

	c := NewCollector(NewSSHExecerFactory())
	samples, err := c.Collect(context.Background(), &Target{
		ID: "local-test", Address: "127.0.0.1", Port: 22, User: "local",
	})
	// 部分命令（如 ss）缺失时仍返回错误但保留有效指标；只校验必达指标。
	found := false
	for _, s := range samples {
		require.Equal(t, "local-test", s.NodeID)
		if s.Metric == "load.load1" {
			found = true
			require.Positive(t, s.Value, "本机负载应 > 0")
		}
	}
	require.True(t, found, "应采集到 load.load1 指标")
	_ = err
}
