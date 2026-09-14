package monitor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func memRuleTypes() []AlertType {
	return []AlertType{{
		ID: "OWL-MEM-001", Name: "内存使用率过高", DefaultSeverity: SeverityWarning,
		DefaultParams: RuleParams{Metric: "mem.used_pct", Op: ">", Value: 90, Duration: 1},
		Enabled:       true, Builtin: true,
	}}
}

// TestAlertManager_OpenRefreshResolve 验证完整生命周期：触发开告警 →
// 持续触发仅刷新 → 指标恢复连续 N 次自动解决。
func TestAlertManager_OpenRefreshResolve(t *testing.T) {
	s := newTestStore(t)
	m := NewAlertManager(s)
	m.recoverThreshold = 2
	m.now = func() int64 { return 1000 }
	types := diskRuleTypes()

	// duration=2：第 1 次满足不触发
	events, err := m.Tick("n1", highDiskSamples(93), types)
	require.NoError(t, err)
	require.Empty(t, events)

	// 第 2 次满足：开告警
	events, err = m.Tick("n1", highDiskSamples(94), types)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, EventOpened, events[0].Type)
	require.Equal(t, "n1", events[0].Alert.NodeID)

	// 继续触发：去重刷新，不新增
	events, err = m.Tick("n1", highDiskSamples(95), types)
	require.NoError(t, err)
	require.Empty(t, events, "去重：不应新增或重复事件")
	al, exists, err := s.GetActiveAlert("OWL-DSK-001", "n1")
	require.NoError(t, err)
	require.True(t, exists)

	// 恢复：连续 2 次不满足 → resolved
	events, err = m.Tick("n1", highDiskSamples(10), types)
	require.NoError(t, err)
	require.Empty(t, events)
	events, err = m.Tick("n1", highDiskSamples(11), types)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, EventResolved, events[0].Type)

	got, exists, err := s.GetAlert(al.ID)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, StatusResolved, got.Status)
	require.Equal(t, int64(1000), got.ResolvedAt)
}

// TestAlertManager_RealertWindow 验证重复告警合并窗口：告警解决后窗口期内
// 同类型再触发 → 重开原条目（不新建）；窗口关闭（0）→ 允许新建。
func TestAlertManager_RealertWindow(t *testing.T) {
	s := newTestStore(t)
	m := NewAlertManager(s)
	m.recoverThreshold = 1
	base := int64(1000)
	types := memRuleTypes()
	memSamples := func(v float64) []Sample {
		return []Sample{{NodeID: "n1", Metric: "mem.used_pct", TS: 1, Value: v}}
	}

	// 开告警 → 解决
	m.now = func() int64 { return base }
	events, err := m.Tick("n1", memSamples(95), types)
	require.NoError(t, err)
	require.Len(t, events, 1)
	originalID := events[0].Alert.ID
	m.now = func() int64 { return base + 60 }
	events, err = m.Tick("n1", memSamples(10), types)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, EventResolved, events[0].Type)

	// 默认窗口（24h）内再次触发：重开同一条目，不新建
	m.now = func() int64 { return base + 120 }
	events, err = m.Tick("n1", memSamples(95), types)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, EventOpened, events[0].Type)
	require.Equal(t, originalID, events[0].Alert.ID, "合并窗口内应重开原条目")
	got, exists, err := s.GetAlert(originalID)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, StatusOpen, got.Status)
	require.Equal(t, int64(0), got.ResolvedAt, "重开后应清除解决时间")
	require.Equal(t, int64(base+120), got.FirstSeen, "重开后 first_seen 重置，避免按旧时间立即升级")

	all, err := s.ListAlerts(AlertFilter{NodeID: "n1"})
	require.NoError(t, err)
	require.Len(t, all, 1, "窗口内重复触发不应产生第二条告警")

	// 窗口关闭（0）：解决后再触发 → 新建
	_, err = m.Resolve(originalID)
	require.NoError(t, err)
	m.SetRealertWindow(0)
	m.now = func() int64 { return base + 240 }
	events, err = m.Tick("n1", memSamples(96), types)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.NotEqual(t, originalID, events[0].Alert.ID, "窗口关闭时应新建告警")
}

// TestAlertManager_MarkCollectOK_KeepsDurationWindow 验证周期性采集成功回调
// 不清空规则连续命中计数：Engine 每个成功 tick 都会调 MarkCollectOK，若其
// 无条件 Reset 规则计数，duration≥2 的规则将永远无法满足持续窗口。
func TestAlertManager_MarkCollectOK_KeepsDurationWindow(t *testing.T) {
	s := newTestStore(t)
	m := NewAlertManager(s)
	types := diskRuleTypes() // duration=2
	samples := highDiskSamples(95)

	// 失联 → 恢复：转换瞬间允许 Reset 一次（清除断档期的陈旧计数）
	_, err := m.MarkCollectFail("n1")
	require.NoError(t, err)
	_, err = m.MarkCollectOK("n1")
	require.NoError(t, err)

	// 模拟 Engine 稳态循环：OK → Tick → OK → Tick ...
	_, err = m.MarkCollectOK("n1")
	require.NoError(t, err)
	_, err = m.Tick("n1", samples, types) // 计数 1，未达窗口
	require.NoError(t, err)
	_, err = m.MarkCollectOK("n1") // 稳态成功回调：不得清计数
	require.NoError(t, err)
	events, err := m.Tick("n1", samples, types) // 计数 2 → 触发
	require.NoError(t, err)
	require.Len(t, events, 1, "稳态 OK 回调不应清空 duration 窗口计数")
	require.Equal(t, EventOpened, events[0].Type)
}

// TestAlertManager_SetEscalateAfter 验证升级时长可在运行期调整：
// 缩短后 warn 未处理按新时长升级；传 0/负值忽略（保留当前值）。
func TestAlertManager_SetEscalateAfter(t *testing.T) {
	s := newTestStore(t)
	m := NewAlertManager(s)
	base := int64(1000)
	m.now = func() int64 { return base }

	memSamples := func(v float64) []Sample {
		return []Sample{{NodeID: "n1", Metric: "mem.used_pct", TS: 1, Value: v}}
	}
	events, err := m.Tick("n1", memSamples(95), memRuleTypes())
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, EventOpened, events[0].Type)

	// 5 分钟后仍在触发：默认 1h 内不升级
	m.now = func() int64 { return base + 300 }
	events, err = m.Tick("n1", memSamples(96), memRuleTypes())
	require.NoError(t, err)
	require.Empty(t, events)

	// 缩短为 1 分钟后：累计超时 → 升级
	m.SetEscalateAfter(time.Minute)
	m.now = func() int64 { return base + 1300 }
	events, err = m.Tick("n1", memSamples(97), memRuleTypes())
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, EventEscalated, events[0].Type)

	// 非法值忽略：不改变当前配置
	m.SetEscalateAfter(0)
	m.SetEscalateAfter(-time.Minute)
}

// TestAlertManager_Ack 验证人工确认：open → acked。
func TestAlertManager_Ack(t *testing.T) {
	s := newTestStore(t)
	m := NewAlertManager(s)
	m.now = func() int64 { return 1000 }

	_, err := m.Tick("n1", highDiskSamples(93), diskRuleTypes())
	require.NoError(t, err)
	_, err = m.Tick("n1", highDiskSamples(94), diskRuleTypes())
	require.NoError(t, err)

	al, _, _ := s.GetActiveAlert("OWL-DSK-001", "n1")
	acked, err := m.Ack(al.ID)
	require.NoError(t, err)
	require.Equal(t, StatusAcked, acked.Status)

	// 已解决告警不可 ack
	_, err = m.Ack(al.ID) // acked 后再次 ack 允许（幂等）
	require.NoError(t, err)
	_, err = m.Resolve(al.ID)
	require.NoError(t, err)
	_, err = m.Ack(al.ID)
	require.Error(t, err, "已解决告警不能 ack")
}

// TestAlertManager_Escalate 验证 warn 超时未处理升级为 critical。
func TestAlertManager_Escalate(t *testing.T) {
	s := newTestStore(t)
	m := NewAlertManager(s)
	m.escalateAfter = 3600 // 1h
	base := int64(1000)
	m.now = func() int64 { return base }

	events, err := m.Tick("n1", []Sample{{NodeID: "n1", Metric: "mem.used_pct", TS: 1, Value: 95}}, memRuleTypes())
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, EventOpened, events[0].Type)

	// 2 小时后仍在触发 → 升级
	m.now = func() int64 { return base + 7200 }
	events, err = m.Tick("n1", []Sample{{NodeID: "n1", Metric: "mem.used_pct", TS: 2, Value: 96}}, memRuleTypes())
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, EventEscalated, events[0].Type)

	al, _, _ := s.GetActiveAlert("OWL-MEM-001", "n1")
	require.Equal(t, SeverityCritical, al.Severity)
}

// TestAlertManager_Unreachable 验证失联告警：连续失败达阈值开 OWL-OSS-001，
// 恢复后自动解决，可再次触发。
func TestAlertManager_Unreachable(t *testing.T) {
	s := newTestStore(t)
	m := NewAlertManager(s)
	m.failThreshold = 3
	m.now = func() int64 { return 1000 }

	events, err := m.MarkCollectFail("n1")
	require.NoError(t, err)
	require.Empty(t, events)
	events, err = m.MarkCollectFail("n1")
	require.NoError(t, err)
	require.Empty(t, events)

	events, err = m.MarkCollectFail("n1")
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, EventOpened, events[0].Type)
	require.Equal(t, "OWL-OSS-001", events[0].Alert.AlertTypeID)
	require.Equal(t, SeverityCritical, events[0].Alert.Severity)

	// 恢复
	events, err = m.MarkCollectOK("n1")
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, EventResolved, events[0].Type)

	// 再次失联可重开
	m.MarkCollectFail("n1")
	m.MarkCollectFail("n1")
	events, err = m.MarkCollectFail("n1")
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, EventOpened, events[0].Type)
}
