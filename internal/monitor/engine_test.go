package monitor

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// memRuleDuration1 内存规则（持续 1 次采样即触发），便于单轮 Tick 断言。
func memRuleDuration1() []AlertType {
	types := memRuleTypes()
	types[0].DefaultParams.Duration = 1
	return types
}

func newTestEngine(t *testing.T, targets []Target, outputs map[string]string) (*Engine, *Store) {
	t.Helper()
	s := newTestStore(t)

	// 将内存规则改为持续 1 次采样触发
	for _, id := range []string{"OWL-MEM-001", "OWL-DSK-001"} {
		at, _, err := s.GetAlertType(id)
		require.NoError(t, err)
		at.DefaultParams.Duration = 1
		require.NoError(t, s.UpsertAlertType(at))
	}

	f := &fakeFactory{exec: &fakeExecer{outputs: outputs}}
	c := NewCollector(f)
	c.now = func() int64 { return 1750000000 }

	m := NewAlertManager(s)
	m.now = func() int64 { return 1750000000 }

	d := NewDispatcher(s)
	d.retries = 1
	d.delay = 0

	eng := NewEngine(EngineConfig{
		Interval:      time.Minute,
		RetentionDays: 30,
		Concurrency:   4,
		SilenceUntil:  func() int64 { return 0 },
	}, s, c, fakeSource{targets: targets}, m, d)
	return eng, s
}

type fakeSource struct{ targets []Target }

func (f fakeSource) ListTargets() ([]Target, error) { return f.targets, nil }

// TestEngine_TickOnce_CollectAndAlert 验证一轮采集：指标入库 + 规则触发开告警。
func TestEngine_TickOnce_CollectAndAlert(t *testing.T) {
	outputs := sampleOutputs()
	// 高内存：used_pct = (15891-900)/15891 = 94.3% > 90%
	outputs["free -m"] = "              total        used        free      shared  buff/cache   available\nMem:          15891       15000        100         189         791         900\nSwap:          2047           0        2047\n"
	eng, s := newTestEngine(t, []Target{{ID: "node-a", Name: "web-01"}}, outputs)

	require.NoError(t, eng.TickOnce(context.Background()))

	// 指标已入库
	rows, err := s.QuerySamples("node-a", "load.load1", 1750000000-1, 1750000000+1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.InDelta(t, 0.52, rows[0].Value, 0.001)

	// 内存 95% > 90%（duration=1）→ 告警已开
	al, exists, err := s.GetActiveAlert("OWL-MEM-001", "node-a")
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, StatusOpen, al.Status)
	require.Equal(t, "node-a", al.NodeID)
}

// TestEngine_TickOnce_NetRate 验证网卡速率跨轮合成（net.rx_rate.<iface>）。
func TestEngine_TickOnce_NetRate(t *testing.T) {
	outputs := sampleOutputs()
	eng, s := newTestEngine(t, []Target{{ID: "node-a"}}, outputs)

	// 第一轮：计数 1000000000，无前值 → 不出速率
	require.NoError(t, eng.TickOnce(context.Background()))
	rows, err := s.QuerySamples("node-a", "net.rx_rate.eth0", 0, 1<<62)
	require.NoError(t, err)
	require.Empty(t, rows, "首轮无前值不应产出速率")

	// 第二轮：计数增加 1000000000，间隔 60s → 速率约 16666666 B/s
	outputs["cat /proc/net/dev"] = "Inter-|   Receive                                                |  Transmit\n face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed\n  eth0: 2000000000  500000    0    0    0     0          0         0  50000000  250000    0    0    0     0       0          0\n"
	eng.collector.now = func() int64 { return 1750000060 }
	require.NoError(t, eng.TickOnce(context.Background()))

	rows, err = s.QuerySamples("node-a", "net.rx_rate.eth0", 0, 1<<62)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.InDelta(t, 1000000000.0/60, rows[0].Value, 1000)
}

// TestEngine_TickOnce_Unreachable 验证连续采集失败触发失联告警。
func TestEngine_TickOnce_Unreachable(t *testing.T) {
	outputs := sampleOutputs()
	eng, s := newTestEngine(t, []Target{{ID: "node-a"}}, outputs)
	eng.manager.failThreshold = 2

	// 让 loadavg（必选命令）失败
	eng.collector = NewCollector(&fakeFactory{exec: &fakeExecer{
		outputs: outputs,
		fail:    map[string]bool{"cat /proc/loadavg": true},
	}})

	require.Error(t, eng.TickOnce(context.Background())) // 失败 1（返回采集错误）
	_, exists, err := s.GetActiveAlert("OWL-OSS-001", "node-a")
	require.NoError(t, err)
	require.False(t, exists, "未达阈值不应失联告警")

	require.Error(t, eng.TickOnce(context.Background())) // 失败 2 → 触发
	al, exists, err := s.GetActiveAlert("OWL-OSS-001", "node-a")
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, SeverityCritical, al.Severity)

	// 恢复：采集成功 → 失联告警自动解决
	eng.collector = NewCollector(&fakeFactory{exec: &fakeExecer{outputs: outputs}})
	require.NoError(t, eng.TickOnce(context.Background()))
	_, exists, err = s.GetActiveAlert("OWL-OSS-001", "node-a")
	require.NoError(t, err)
	require.False(t, exists, "恢复后失联告警应解决")
}

// TestEngine_TickOnce_Silence 验证静默期内不新建告警。
func TestEngine_TickOnce_Silence(t *testing.T) {
	eng, s := newTestEngine(t, []Target{{ID: "node-a"}}, sampleOutputs())
	eng.cfg.SilenceUntil = func() int64 { return 1750000001 } // 静默到下一秒之后

	require.NoError(t, eng.TickOnce(context.Background()))

	_, exists, err := s.GetActiveAlert("OWL-MEM-001", "node-a")
	require.NoError(t, err)
	require.False(t, exists, "静默期内不应新建告警")
}

// TestEngine_CleanupOnce 验证引擎清理按保留天数执行。
func TestEngine_CleanupOnce(t *testing.T) {
	eng, s := newTestEngine(t, nil, sampleOutputs())

	// 手工插入 40 天前的过期采样与近期采样
	now := time.Now().Unix()
	require.NoError(t, s.InsertSamples([]Sample{
		{NodeID: "n1", Metric: "load.load1", TS: now - 40*86400, Value: 1},
		{NodeID: "n1", Metric: "load.load1", TS: now - 86400, Value: 2},
	}))

	require.NoError(t, eng.CleanupOnce())

	rows, err := s.QuerySamples("n1", "load.load1", 0, 1<<62)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.InDelta(t, 2, rows[0].Value, 0.001)
}
