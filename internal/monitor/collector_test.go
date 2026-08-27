package monitor

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeExecer 按命令返回固定输出，模拟远端节点。
type fakeExecer struct {
	outputs map[string]string
	fail    map[string]bool
}

func (f *fakeExecer) Execute(command string, timeout time.Duration) (int, string, error) {
	if f.fail[command] {
		return 1, "", &execErr{cmd: command}
	}
	return 0, f.outputs[command], nil
}

type execErr struct{ cmd string }

func (e *execErr) Error() string { return "exec failed: " + e.cmd }

type fakeFactory struct {
	exec Execer
}

func (f *fakeFactory) NewExecer(t *Target) (Execer, error) {
	return f.exec, nil
}

func sampleOutputs() map[string]string {
	return map[string]string{
		"cat /proc/loadavg":       "0.52 0.47 0.41 2/345 12345\n",
		"df -P":                   "Filesystem     1024-blocks    Used Available Capacity Mounted on\n/dev/sda1 205113712 85641132 108722116 45% /\n",
		"df -Pi":                  "Filesystem     Inodes IUsed IFree IUse% Mounted on\n/dev/sda1 12845056 296247 12548809 3% /\n",
		"free -m":                 "              total        used        free      shared  buff/cache   available\nMem:          15891        2352        2419         189       11120       12902\nSwap:          2047           0        2047\n",
		"cat /proc/net/dev":       "Inter-|   Receive                                                |  Transmit\n face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed\n  eth0: 1000000000  500000    0    0    0     0          0         0  50000000  250000    0    0    0     0       0          0\n",
		"ss -s":                   "Total: 128 (kernel 96)\nTCP:   12 (estab 4, closed 3, orphaned 0, timewait 5, transports 12), \n",
		"cat /proc/uptime":        "12345.67 23456.78\n",
		"nproc":                   "8\n",
		"systemctl is-active nginx": "active\n",
	}
}

// TestCollector_CollectAll 验证采集器在一次轮询中解析出全部核心指标。
func TestCollector_CollectAll(t *testing.T) {
	f := &fakeFactory{exec: &fakeExecer{outputs: sampleOutputs()}}
	c := NewCollector(f)
	c.now = func() int64 { return 1750000000 }

	samples, err := c.Collect(context.Background(), &Target{ID: "node-a"})
	require.NoError(t, err)

	byMetric := map[string]float64{}
	for _, s := range samples {
		require.Equal(t, "node-a", s.NodeID)
		require.Equal(t, int64(1750000000), s.TS)
		byMetric[s.Metric] = s.Value
	}
	require.InDelta(t, 0.52, byMetric["load.load1"], 0.001)
	require.InDelta(t, 45, byMetric["disk.usage./"], 0.001)
	require.InDelta(t, 3, byMetric["disk.inodes./"], 0.001)
	require.InDelta(t, 18.81, byMetric["mem.used_pct"], 0.01)
	require.InDelta(t, 1000000000, byMetric["net.rx_bytes.eth0"], 0.001)
	require.InDelta(t, 50000000, byMetric["net.tx_bytes.eth0"], 0.001)
	require.InDelta(t, 4, byMetric["net.tcp_estab"], 0.001)
	require.InDelta(t, 5, byMetric["net.tcp_timewait"], 0.001)
}

// TestCollector_OneCommandFails 验证单条命令失败不影响其余命令的采集：
// 错误携带失败命令，但成功命令的指标照常产出。
func TestCollector_OneCommandFails(t *testing.T) {
	f := &fakeFactory{exec: &fakeExecer{
		outputs: sampleOutputs(),
		fail:    map[string]bool{"ss -s": true},
	}}
	c := NewCollector(f)
	c.now = func() int64 { return 1750000000 }

	samples, err := c.Collect(context.Background(), &Target{ID: "node-a"})
	require.Error(t, err, "部分失败应返回错误以暴露失败命令")
	require.Contains(t, err.Error(), "ss -s")

	byMetric := map[string]float64{}
	for _, s := range samples {
		byMetric[s.Metric] = s.Value
	}
	require.InDelta(t, 45, byMetric["disk.usage./"], 0.001)
	_, hasTCP := byMetric["net.tcp_estab"]
	require.False(t, hasTCP, "ss 失败时不应产出 tcp 指标")
}

// TestCollector_AllCommandsFail 验证全部命令失败返回错误（节点失联信号）。
func TestCollector_AllCommandsFail(t *testing.T) {
	f := &fakeFactory{exec: &fakeExecer{
		outputs: sampleOutputs(),
		fail:    map[string]bool{"cat /proc/loadavg": true},
	}}
	c := NewCollector(f)
	c.now = func() int64 { return 1750000000 }

	// 只有 loadavg 一个必选命令失败 → 整体失败
	_, err := c.Collect(context.Background(), &Target{ID: "node-a"})
	require.Error(t, err)
}

// TestRatePerSec 验证累计计数 → 每秒速率；同时间戳或计数器重置（重启）时返回 0。
func TestRatePerSec(t *testing.T) {
	prev := Sample{NodeID: "n", Metric: "net.rx_bytes.eth0", TS: 1000, Value: 100}
	cur := Sample{NodeID: "n", Metric: "net.rx_bytes.eth0", TS: 1010, Value: 250}
	require.InDelta(t, 15, RatePerSec(prev, cur), 0.001)

	require.InDelta(t, 0, RatePerSec(cur, cur), 0.001)

	reset := Sample{NodeID: "n", Metric: "net.rx_bytes.eth0", TS: 1020, Value: 50}
	require.InDelta(t, 0, RatePerSec(cur, reset), 0.001)
}

// TestCollector_Timeout 验证命令执行超时被采集器透传为整体错误。
func TestCollector_Timeout(t *testing.T) {
	f := &fakeFactory{exec: &timeoutExecer{}}
	c := NewCollector(f)
	c.now = func() int64 { return 1750000000 }

	_, err := c.Collect(context.Background(), &Target{ID: "node-a"})
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "超时"), "错误应包含超时信息: %v", err)
}
