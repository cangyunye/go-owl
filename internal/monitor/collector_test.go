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
	outputs  map[string]string
	fail     map[string]bool
	executed []string
}

func (f *fakeExecer) Execute(command string, timeout time.Duration) (int, string, error) {
	f.executed = append(f.executed, command)
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
		"cat /proc/loadavg": "0.52 0.47 0.41 2/345 12345\n",
		"LC_ALL=C df -P":    "Filesystem     1024-blocks    Used Available Capacity Mounted on\n/dev/sda1 205113712 85641132 108722116 45% /\n",
		"LC_ALL=C df -Pi":   "Filesystem     Inodes IUsed IFree IUse% Mounted on\n/dev/sda1 12845056 296247 12548809 3% /\n",
		"LC_ALL=C free -m":  "              total        used        free      shared  buff/cache   available\nMem:          15891        2352        2419         189       11120       12902\nSwap:          2047           0        2047\n",
		"cat /proc/net/dev": "Inter-|   Receive                                                |  Transmit\n face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed\n  eth0: 1000000000  500000    0    0    0     0          0         0  50000000  250000    0    0    0     0       0          0\n",
		"LC_ALL=C ss -s":    "Total: 128 (kernel 96)\nTCP:   12 (estab 4, closed 3, orphaned 0, timewait 5, transports 12), \n",
		"journalctl -p err -q --no-pager -o json --since=-5min | wc -l":                                           "3\n",
		"journalctl -k -q --no-pager --since=-5min | grep -ciE \"out of memory|oom-kill|killed process\" || true": "0\n",
		"cat /proc/uptime":          "12345.67 23456.78\n",
		"nproc":                     "8\n",
		"systemctl is-active nginx": "active\n",
	}
}

// TestCollector_CommandsLocalePinned 验证 locale 敏感的采集命令固定 C locale：
// 非 C locale 节点（如中文系统）的本地化表头会导致解析失败且被静默跳过。
func TestCollector_CommandsLocalePinned(t *testing.T) {
	pinned := map[string]bool{
		"LC_ALL=C df -P":   false,
		"LC_ALL=C df -Pi":  false,
		"LC_ALL=C free -m": false,
		"LC_ALL=C ss -s":   false,
	}
	for _, step := range collectSteps {
		if _, ok := pinned[step.command]; ok {
			pinned[step.command] = true
		}
	}
	for cmd, found := range pinned {
		require.True(t, found, "采集命令 %q 必须以固定 locale 的形式注册", cmd)
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
	require.InDelta(t, 8, byMetric["sys.cores"], 0.001)
	require.InDelta(t, 45, byMetric["disk.usage./"], 0.001)
	require.InDelta(t, 3, byMetric["disk.inodes./"], 0.001)
	require.InDelta(t, 18.81, byMetric["mem.used_pct"], 0.01)
	require.InDelta(t, 1000000000, byMetric["net.rx_bytes.eth0"], 0.001)
	require.InDelta(t, 50000000, byMetric["net.tx_bytes.eth0"], 0.001)
	require.InDelta(t, 4, byMetric["net.tcp_estab"], 0.001)
	require.InDelta(t, 5, byMetric["net.tcp_timewait"], 0.001)
	require.InDelta(t, 3, byMetric["err.journal_errors"], 0.001)
	require.InDelta(t, 0, byMetric["err.oom"], 0.001)
}

// TestCollector_SvcMetricsOnlyWithServices 验证 svc 步骤仅在目标配置了
// 受监控服务（Target.Services）时执行；默认附加 ssh/sshd，别名/坏名过滤。
func TestCollector_SvcMetricsOnlyWithServices(t *testing.T) {
	// 未配置服务：不执行 systemctl 命令、无 svc 指标
	exec := &fakeExecer{outputs: sampleOutputs()}
	f := &fakeFactory{exec: exec}
	c := NewCollector(f)
	c.now = func() int64 { return 1750000000 }

	samples, err := c.Collect(context.Background(), &Target{ID: "node-a"})
	require.NoError(t, err)
	for _, s := range samples {
		require.False(t, strings.HasPrefix(s.Metric, "svc."), "未配置服务不应产出 svc 指标")
	}
	for _, cmd := range exec.executed {
		require.NotContains(t, cmd, "systemctl", "未配置服务不应执行 systemctl 命令")
	}

	// 配置服务：产出 svc 指标；恶意 unit 名被白名单过滤
	exec2 := &fakeExecer{outputs: sampleOutputs()}
	c2 := NewCollector(&fakeFactory{exec: exec2})
	c2.now = func() int64 { return 1750000000 }

	samples, err = c2.Collect(context.Background(), &Target{ID: "node-a", Services: []string{" nginx "}})
	require.NoError(t, err)
	byMetric := map[string]float64{}
	for _, s := range samples {
		byMetric[s.Metric] = s.Value
	}
	_, hasSsh := byMetric["svc.active.ssh"]
	require.False(t, hasSsh, "未在列表中的 ssh 不应被采集（opt-in 语义）")

	// 恶意 unit 名被过滤；合法名进入命令
	exec3 := &fakeExecer{outputs: sampleOutputs()}
	c3 := NewCollector(&fakeFactory{exec: exec3})
	c3.now = func() int64 { return 1750000000 }
	_, err = c3.Collect(context.Background(), &Target{ID: "node-a", Services: []string{"cron", "bad name;rm -rf"}})
	require.NoError(t, err)

	var svcCmd string
	for _, cmd := range exec3.executed {
		if strings.Contains(cmd, "systemctl") {
			svcCmd = cmd
		}
	}
	require.NotEmpty(t, svcCmd)
	require.Contains(t, svcCmd, "cron")
	require.NotContains(t, svcCmd, "bad name", "非法 unit 名不得进入命令")
}

// TestCollector_OneCommandFails 验证单条命令失败不影响其余命令的采集：
// 错误携带失败命令，但成功命令的指标照常产出。
func TestCollector_OneCommandFails(t *testing.T) {
	f := &fakeFactory{exec: &fakeExecer{
		outputs: sampleOutputs(),
		fail:    map[string]bool{"LC_ALL=C ss -s": true},
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
