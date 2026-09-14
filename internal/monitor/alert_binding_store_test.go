package monitor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestStore_AlertBindingCRUD 验证告警实例专属指令绑定：创建/更新/删除/按 seq 排序/
// auto_exec 过滤（问题：按告警 ID 指定 playbook 或脚本）。
func TestStore_AlertBindingCRUD(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().Unix()

	// 告警必须存在（外键语义由调用方保证，这里先建告警）
	require.NoError(t, s.InsertAlert(&Alert{ID: "AL-1", AlertTypeID: "OWL-MEM-001", NodeID: "n1",
		Severity: SeverityWarning, Status: StatusOpen, Message: "x", FirstSeen: now, LastSeen: now}))

	mk := func(id, kind, content string, seq int, auto bool) AlertBinding {
		return AlertBinding{ID: id, AlertID: "AL-1", Kind: kind, Name: "指令" + id,
			Content: content, Risk: "medium", AutoExec: auto, ExecMode: "sequential",
			Seq: seq, CreatedBy: "admin", CreatedAt: now}
	}
	require.NoError(t, s.CreateAlertBinding(mk("AB-2", "playbook", "restart-nginx", 2, false)))
	require.NoError(t, s.CreateAlertBinding(mk("AB-1", "script", "systemctl restart nginx", 1, true)))
	require.NoError(t, s.CreateAlertBinding(mk("AB-3", "playbook", "disk-clean", 3, true)))

	items, err := s.ListAlertBindings("AL-1")
	require.NoError(t, err)
	require.Len(t, items, 3)
	require.Equal(t, "AB-1", items[0].ID, "应按 seq 升序")

	autos, err := s.ListAutoAlertBindings("AL-1")
	require.NoError(t, err)
	require.Len(t, autos, 2, "只含 auto_exec=1")
	require.Equal(t, "AB-1", autos[0].ID)

	// 更新：改名/换自动开关
	got, ok, err := s.GetAlertBinding("AB-1")
	require.NoError(t, err)
	require.True(t, ok)
	got.Name = "重启 nginx"
	got.AutoExec = false
	require.NoError(t, s.UpdateAlertBinding(got))
	autos, err = s.ListAutoAlertBindings("AL-1")
	require.NoError(t, err)
	require.Len(t, autos, 1)

	// 删除
	require.NoError(t, s.DeleteAlertBinding("AB-2"))
	items, err = s.ListAlertBindings("AL-1")
	require.NoError(t, err)
	require.Len(t, items, 2)
}

// TestStore_AlertBinding_SurvivesReopen 验证绑定随告警存亡：
// 告警解决后重开（同 ID）绑定保留；告警被清理时级联删除。
func TestStore_AlertBinding_SurvivesReopen(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().Unix()

	require.NoError(t, s.InsertAlert(&Alert{ID: "AL-R", AlertTypeID: "OWL-MEM-001", NodeID: "n1",
		Severity: SeverityWarning, Status: StatusOpen, Message: "x", FirstSeen: now, LastSeen: now}))
	require.NoError(t, s.CreateAlertBinding(AlertBinding{ID: "AB-R1", AlertID: "AL-R",
		Kind: "playbook", Name: "处置", Content: "pb-1", Seq: 1, CreatedAt: now}))

	// 解决 → 合并窗口重开（同 ID 状态翻转），绑定仍在
	al, ok, err := s.GetAlert("AL-R")
	require.NoError(t, err)
	require.True(t, ok)
	al.Status = StatusResolved
	al.ResolvedAt = now
	require.NoError(t, s.UpdateAlert(al))
	al.Status = StatusOpen
	al.ResolvedAt = 0
	require.NoError(t, s.UpdateAlert(al))

	items, err := s.ListAlertBindings("AL-R")
	require.NoError(t, err)
	require.Len(t, items, 1, "重开告警应保留绑定")

	// 告警被清理（超期已解决）→ 绑定与执行记录级联删除
	require.NoError(t, s.InsertAlert(&Alert{ID: "AL-D", AlertTypeID: "OWL-MEM-001", NodeID: "n2",
		Severity: SeverityWarning, Status: StatusResolved, Message: "x",
		FirstSeen: now - 40*86400, LastSeen: now - 40*86400, ResolvedAt: now - 40*86400}))
	require.NoError(t, s.CreateAlertBinding(AlertBinding{ID: "AB-D1", AlertID: "AL-D",
		Kind: "script", Name: "清理", Content: "echo hi", Seq: 1, CreatedAt: now}))
	require.NoError(t, s.CreateAlertBindingRun(&AlertBindingRun{ID: "BR-1", AlertID: "AL-D",
		BindingID: "AB-D1", Kind: "script", Status: "success", CreatedAt: now}))

	require.NoError(t, s.CleanupAlerts(7))

	_, exists, err := s.GetAlertBinding("AB-D1")
	require.NoError(t, err)
	require.False(t, exists, "告警清理后绑定应级联删除")
	runs, err := s.ListAlertBindingRuns("AL-D")
	require.NoError(t, err)
	require.Empty(t, runs, "告警清理后执行记录应级联删除")
	items, err = s.ListAlertBindings("AL-R")
	require.NoError(t, err)
	require.Len(t, items, 1, "未清理告警的绑定不受影响")
}

// TestStore_AlertBindingRunLifecycle 验证绑定执行记录：创建 → 运行态 → 终态。
func TestStore_AlertBindingRunLifecycle(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().Unix()

	require.NoError(t, s.CreateAlertBindingRun(&AlertBindingRun{ID: "BR-1", AlertID: "AL-1",
		BindingID: "AB-1", Kind: "playbook", RefID: "PBR-9", Mode: "sequential",
		Status: "running", CreatedBy: "admin", CreatedAt: now}))

	runs, err := s.ListAlertBindingRuns("AL-1")
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, "running", runs[0].Status)

	require.NoError(t, s.FinishAlertBindingRun("BR-1", "success", ""))
	runs, err = s.ListAlertBindingRuns("AL-1")
	require.NoError(t, err)
	require.Equal(t, "success", runs[0].Status)
	require.NotZero(t, runs[0].FinishedAt)
}
