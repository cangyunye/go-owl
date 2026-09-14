package monitor

import (
	"context"
	"errors"
	"fmt"
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
	outputs["LC_ALL=C free -m"] = "              total        used        free      shared  buff/cache   available\nMem:          15891       15000        100         189         791         900\nSwap:          2047           0        2047\n"
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

// TestEngine_TickOnce_AutoHeal 验证告警触发 → 类型放行 → 自愈管线执行。
func TestEngine_TickOnce_AutoHeal(t *testing.T) {
	outputs := sampleOutputs()
	// 高内存触发 OWL-MEM-001（duration=1 已由 newTestEngine 设置）
	outputs["LC_ALL=C free -m"] = "              total        used        free      shared  buff/cache   available\nMem:          15891       15000        100         189         791         900\nSwap:          2047           0        2047\n"
	eng, s := newTestEngine(t, []Target{{ID: "node-a"}}, outputs)

	// 类型放行 + 低风险对策放行
	at, _, err := s.GetAlertType("OWL-MEM-001")
	require.NoError(t, err)
	at.AutoApprove = true
	require.NoError(t, s.UpsertAlertType(at))
	require.NoError(t, s.UpsertRemedy(Remedy{
		ID: "RM-HEAL", AlertTypeID: "OWL-MEM-001", Name: "清理缓存",
		Kind: "script", Content: "echo heal-ok", Risk: "low",
		Source: "user", Reviewed: true, AutoApprove: true,
	}))

	// 挂载自愈管线
	fake := &remedyFakeExecer{}
	runner := NewRunExecutor(&remedyFakeFactory{exec: fake}, func(id string) (*Target, error) {
		return &Target{ID: id}, nil
	})
	healer := NewAutoHealer(s, NewRuleBasedAdvisor(s, 3), runner)
	eng.SetAutoHealer(healer)

	require.NoError(t, eng.TickOnce(context.Background()))

	// 告警已开
	al, exists, err := s.GetActiveAlert("OWL-MEM-001", "node-a")
	require.NoError(t, err)
	require.True(t, exists)

	// 自愈计划已创建并执行完成
	deadline := time.Now().Add(5 * time.Second)
	var run RemedyRun
	for time.Now().Before(deadline) {
		runs, err := s.ListRemedyRunsByAlert(al.ID)
		require.NoError(t, err)
		if len(runs) > 0 {
			r, _, _ := s.GetRemedyRun(runs[0].ID)
			run = *r
			if run.IsTerminal() {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	require.Equal(t, RunDone, run.Status, "自愈计划应执行完成")
	require.Equal(t, StepSuccess, run.Steps[0].Status)
	fake.mu.Lock()
	require.Equal(t, []string{"echo heal-ok"}, fake.executed, "自愈脚本应真实执行")
	fake.mu.Unlock()
}

// TestEngine_TickOnce_NoAutoHealWithoutApproval 验证类型未放行时不触发自愈。
func TestEngine_TickOnce_NoAutoHealWithoutApproval(t *testing.T) {
	outputs := sampleOutputs()
	outputs["LC_ALL=C free -m"] = "              total        used        free      shared  buff/cache   available\nMem:          15891       15000        100         189         791         900\nSwap:          2047           0        2047\n"
	eng, s := newTestEngine(t, []Target{{ID: "node-a"}}, outputs)

	fake := &remedyFakeExecer{}
	runner := NewRunExecutor(&remedyFakeFactory{exec: fake}, func(id string) (*Target, error) {
		return &Target{ID: id}, nil
	})
	healer := NewAutoHealer(s, NewRuleBasedAdvisor(s, 3), runner)
	eng.SetAutoHealer(healer)

	require.NoError(t, eng.TickOnce(context.Background()))

	al, exists, err := s.GetActiveAlert("OWL-MEM-001", "node-a")
	require.NoError(t, err)
	require.True(t, exists, "告警照常触发")

	runs, err := s.ListRemedyRunsByAlert(al.ID)
	require.NoError(t, err)
	require.Empty(t, runs, "类型未放行不应产生自愈计划")
	fake.mu.Lock()
	require.Empty(t, fake.executed)
	fake.mu.Unlock()
}

// alwaysFailExecer 让全部采集命令失败，使 collectNode 走失联分支。
type alwaysFailExecer struct{}

func (alwaysFailExecer) Execute(command string, timeout time.Duration) (int, string, error) {
	return 1, "", errors.New("collect failed")
}

// TestEngine_TickOnce_ConcurrentAlertState 回归多节点并发采集下告警状态的并发安全：
// Engine.TickOnce 以 Concurrency 起 goroutine，collectNode 会并发调用
// AlertManager.MarkCollectFail/MarkCollectOK/Tick 与 RuleEngine.Tick，
// 二者的共享 map 若无保护会触发运行期 fatal error（concurrent map writes）。
// 需以 -race 运行才稳定暴露。
func TestEngine_TickOnce_ConcurrentAlertState(t *testing.T) {
	s := newTestStore(t)
	for _, id := range []string{"OWL-MEM-001", "OWL-DSK-001"} {
		at, _, err := s.GetAlertType(id)
		require.NoError(t, err)
		at.DefaultParams.Duration = 1
		require.NoError(t, s.UpsertAlertType(at))
	}

	c := NewCollector(&fakeFactory{exec: alwaysFailExecer{}})
	c.now = func() int64 { return 1750000000 }
	m := NewAlertManager(s)
	m.now = func() int64 { return 1750000000 }
	d := NewDispatcher(s)
	d.retries = 1
	d.delay = 0

	targets := make([]Target, 0, 16)
	for i := 0; i < 16; i++ {
		targets = append(targets, Target{ID: fmt.Sprintf("node-%02d", i), Name: fmt.Sprintf("n%d", i)})
	}
	eng := NewEngine(EngineConfig{
		Interval:      time.Minute,
		RetentionDays: 30,
		Concurrency:   10,
		SilenceUntil:  func() int64 { return 0 },
	}, s, c, fakeSource{targets: targets}, m, d)

	// 连续多轮：failCounts/recoverCounts 每轮均被并发写入。
	// 采集全失败会返回聚合错误，此处只关注并发安全（-race）。
	for i := 0; i < 5; i++ {
		_ = eng.TickOnce(context.Background())
	}
}

// TestEngine_TickOnce_Disabled 验证监控总开关：关闭时整轮跳过采集与告警。
func TestEngine_TickOnce_Disabled(t *testing.T) {
	eng, s := newTestEngine(t, []Target{{ID: "node-a", Name: "web-01"}}, sampleOutputs())
	eng.cfg.Enabled = func() bool { return false }

	require.NoError(t, eng.TickOnce(context.Background()))

	rows, err := s.QuerySamples("node-a", "load.load1", 0, 1<<62)
	require.NoError(t, err)
	require.Empty(t, rows, "开关关闭时不应采集入库")
	_, exists, err := s.GetActiveAlert("OWL-MEM-001", "node-a")
	require.NoError(t, err)
	require.False(t, exists, "开关关闭时不应产生告警")
}

// TestEngine_TickOnce_CollectWindow 验证采集时段窗口：窗口外整轮跳过。
func TestEngine_TickOnce_CollectWindow(t *testing.T) {
	eng, s := newTestEngine(t, []Target{{ID: "node-a", Name: "web-01"}}, sampleOutputs())
	eng.now = func() time.Time { return time.Date(2026, 9, 14, 7, 0, 0, 0, time.Local) }
	eng.cfg.CollectWindow = func() string { return "08:00-22:00" }

	require.NoError(t, eng.TickOnce(context.Background()))
	rows, err := s.QuerySamples("node-a", "load.load1", 0, 1<<62)
	require.NoError(t, err)
	require.Empty(t, rows, "窗口外不应采集入库")

	// 进入窗口后恢复采集
	eng.now = func() time.Time { return time.Date(2026, 9, 14, 9, 0, 0, 0, time.Local) }
	require.NoError(t, eng.TickOnce(context.Background()))
	rows, err = s.QuerySamples("node-a", "load.load1", 0, 1<<62)
	require.NoError(t, err)
	require.NotEmpty(t, rows, "窗口内应正常采集")
}

// TestEngine_InCollectWindow 验证时段窗口解析：空=全天、跨午夜、非法值放行。
func TestEngine_InCollectWindow(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 9, 14, h, m, 0, 0, time.Local) }
	require.True(t, inCollectWindow(at(12, 0), ""), "空窗口=全天采集")
	require.True(t, inCollectWindow(at(9, 0), "08:00-22:00"))
	require.False(t, inCollectWindow(at(7, 0), "08:00-22:00"))
	require.True(t, inCollectWindow(at(23, 0), "22:00-06:00"), "跨午夜窗口：23 点在窗口内")
	require.True(t, inCollectWindow(at(5, 0), "22:00-06:00"), "跨午夜窗口：凌晨 5 点在窗口内")
	require.False(t, inCollectWindow(at(12, 0), "22:00-06:00"), "跨午夜窗口：正午在窗口外")
	require.True(t, inCollectWindow(at(12, 0), "垃圾"), "非法窗口配置放行（不阻塞采集）")
}

// TestEngine_CleanupAlerts 验证告警保留期清理：只删已解决且超期的记录。
func TestEngine_CleanupAlerts(t *testing.T) {
	eng, s := newTestEngine(t, nil, sampleOutputs())
	eng.cfg.AlertRetentionDays = func() int { return 7 }

	now := time.Now().Unix()
	require.NoError(t, s.InsertAlert(&Alert{ID: "AL-old", AlertTypeID: "OWL-MEM-001", NodeID: "n1",
		Severity: SeverityWarning, Status: StatusResolved, Message: "x",
		FirstSeen: now - 30*86400, LastSeen: now - 30*86400, ResolvedAt: now - 30*86400}))
	require.NoError(t, s.InsertAlert(&Alert{ID: "AL-new", AlertTypeID: "OWL-MEM-001", NodeID: "n2",
		Severity: SeverityWarning, Status: StatusResolved, Message: "x",
		FirstSeen: now - 86400, LastSeen: now - 86400, ResolvedAt: now - 86400}))
	require.NoError(t, s.InsertAlert(&Alert{ID: "AL-open", AlertTypeID: "OWL-MEM-001", NodeID: "n3",
		Severity: SeverityWarning, Status: StatusOpen, Message: "x",
		FirstSeen: now - 30*86400, LastSeen: now - 30*86400}))

	require.NoError(t, eng.CleanupOnce())

	_, exists, err := s.GetAlert("AL-old")
	require.NoError(t, err)
	require.False(t, exists, "超期已解决告警应被删除")
	_, exists, err = s.GetAlert("AL-new")
	require.NoError(t, err)
	require.True(t, exists, "未超期告警应保留")
	_, exists, err = s.GetAlert("AL-open")
	require.NoError(t, err)
	require.True(t, exists, "未解决告警永不删除")
}

// TestEngine_OnAlertOpened 验证告警打开钩子：新建与合并窗口重开都会触发，
// 供 serve 侧执行告警绑定的自动处置指令。
func TestEngine_OnAlertOpened(t *testing.T) {
	outputs := sampleOutputs()
	// 高内存：used_pct > 90%（duration=1）→ 本轮即触发告警
	outputs["LC_ALL=C free -m"] = "              total        used        free      shared  buff/cache   available\nMem:          15891       15000        100         189         791         900\nSwap:          2047           0        2047\n"
	eng, s := newTestEngine(t, []Target{{ID: "node-a", Name: "web-01"}}, outputs)
	var opened []string
	eng.OnAlertOpened = func(ev AlertEvent, tg Target) {
		opened = append(opened, ev.Alert.ID+"|"+tg.ID)
	}

	require.NoError(t, eng.TickOnce(context.Background()))
	require.Eventually(t, func() bool { return len(opened) >= 1 }, time.Second, 5*time.Millisecond,
		"首次触发应异步回调 OnAlertOpened")

	// 重开场景：解决告警后再次入库触发（Alert ID 不变由 Manager 保证）
	al, exists, err := s.GetActiveAlert("OWL-MEM-001", "node-a")
	require.NoError(t, err)
	require.True(t, exists)
	al.Status = StatusResolved
	al.ResolvedAt = 1750000000
	require.NoError(t, s.UpdateAlert(al))
	eng.manager = NewAlertManager(s)
	eng.manager.now = func() int64 { return 1750000000 }

	require.NoError(t, eng.TickOnce(context.Background()))
	require.Eventually(t, func() bool { return len(opened) >= 2 }, time.Second, 5*time.Millisecond,
		"重开产生的 EventOpened 也应回调")
}
