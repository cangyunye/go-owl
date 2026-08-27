package monitor

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "monitor.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// TestStore_SeedAlertTypes 验证首次打开自动写入内置注册表且幂等。
func TestStore_SeedAlertTypes(t *testing.T) {
	s := newTestStore(t)

	types, err := s.ListAlertTypes()
	require.NoError(t, err)
	require.Len(t, types, len(BuiltinAlertTypes()), "内置告警类型应全部落库")

	// 再次 seed 不重复
	require.NoError(t, s.SeedAlertTypesIfEmpty())
	types, err = s.ListAlertTypes()
	require.NoError(t, err)
	require.Len(t, types, len(BuiltinAlertTypes()))
}

// TestStore_UpsertAlertType 验证告警类型参数/开关可更新（阈值、放行）。
func TestStore_UpsertAlertType(t *testing.T) {
	s := newTestStore(t)

	at, ok, err := s.GetAlertType("OWL-DSK-001")
	require.NoError(t, err)
	require.True(t, ok)
	require.False(t, at.AutoApprove)

	at.DefaultParams.Value = 85
	at.AutoApprove = true
	require.NoError(t, s.UpsertAlertType(at))

	got, ok, err := s.GetAlertType("OWL-DSK-001")
	require.NoError(t, err)
	require.True(t, ok)
	require.InDelta(t, 85, got.DefaultParams.Value, 0.001)
	require.True(t, got.AutoApprove, "自动放行应持久化")
}

// TestStore_AlertLifecycle 验证告警实例：插入 → 去重 → 解决后允许新实例。
func TestStore_AlertLifecycle(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().Unix()

	al := &Alert{
		ID: "AL-1", AlertTypeID: "OWL-DSK-001", NodeID: "n1",
		Severity: SeverityWarning, Status: StatusOpen,
		Message: "磁盘使用率 93.5%", MetricSnapshot: `{"disk.usage./":93.5}`,
		FirstSeen: now, LastSeen: now,
	}
	require.NoError(t, s.InsertAlert(al))

	// 活跃去重：同 (类型, 节点) 的第二个活跃实例被唯一索引拒绝
	dup := *al
	dup.ID = "AL-2"
	require.Error(t, s.InsertAlert(&dup), "活跃告警去重索引应拒绝重复实例")

	// 解决后可再插入
	al.Status = StatusResolved
	al.ResolvedAt = now + 60
	require.NoError(t, s.UpdateAlert(al))

	require.NoError(t, s.InsertAlert(&dup))
}

// TestStore_ListAlerts_Filter 验证按状态/级别/节点筛选与排序（critical 置顶）。
func TestStore_ListAlerts_Filter(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().Unix()

	crit := &Alert{ID: "AL-C", AlertTypeID: "OWL-ERR-002", NodeID: "n1",
		Severity: SeverityCritical, Status: StatusOpen, Message: "OOM",
		FirstSeen: now - 100, LastSeen: now - 100}
	warn := &Alert{ID: "AL-W", AlertTypeID: "OWL-DSK-001", NodeID: "n2",
		Severity: SeverityWarning, Status: StatusOpen, Message: "磁盘",
		FirstSeen: now - 50, LastSeen: now - 50}
	done := &Alert{ID: "AL-D", AlertTypeID: "OWL-DSK-001", NodeID: "n3",
		Severity: SeverityWarning, Status: StatusResolved, Message: "已恢复",
		FirstSeen: now - 200, LastSeen: now - 100, ResolvedAt: now - 100}
	require.NoError(t, s.InsertAlert(crit))
	require.NoError(t, s.InsertAlert(warn))
	require.NoError(t, s.InsertAlert(done))

	// 全部按 severity 降序（critical 置顶）
	all, err := s.ListAlerts(AlertFilter{})
	require.NoError(t, err)
	require.Len(t, all, 3)
	require.Equal(t, "AL-C", all[0].ID, "critical 应置顶")

	// 只查活跃
	active, err := s.ListAlerts(AlertFilter{Status: StatusOpen})
	require.NoError(t, err)
	require.Len(t, active, 2)

	// 按节点
	n2, err := s.ListAlerts(AlertFilter{NodeID: "n2"})
	require.NoError(t, err)
	require.Len(t, n2, 1)
	require.Equal(t, "AL-W", n2[0].ID)

	// 活跃筛选（未解决）
	active, err = s.ListAlerts(AlertFilter{Status: "active"})
	require.NoError(t, err)
	require.Len(t, active, 2)
	total, err := s.CountAlerts(AlertFilter{Status: "active"})
	require.NoError(t, err)
	require.Equal(t, 2, total)

	// 分页：limit 1 → 只取最优先的一条（critical 置顶）
	page, err := s.ListAlerts(AlertFilter{Limit: 1})
	require.NoError(t, err)
	require.Len(t, page, 1)
	require.Equal(t, "AL-C", page[0].ID)
}
