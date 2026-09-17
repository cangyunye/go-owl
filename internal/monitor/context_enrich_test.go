package monitor

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ---- store 查询 ----

// TestListRemedyRunsByAlertType 按告警类型检索历史处置计划（join alerts）。
func TestListRemedyRunsByAlertType(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().Unix()

	mkAlert := func(id, typeID, node string) {
		require.NoError(t, s.InsertAlert(&Alert{ID: id, AlertTypeID: typeID, NodeID: node,
			Severity: SeverityWarning, Status: StatusOpen, FirstSeen: now, LastSeen: now}))
	}
	mkAlert("AL-H1", "OWL-DSK-001", "node-a")
	mkAlert("AL-H2", "OWL-DSK-001", "node-b")
	mkAlert("AL-H3", "OWL-MEM-001", "node-a")

	mkRun := func(id, alertID string, status RemedyRunStatus) {
		require.NoError(t, s.CreateRemedyRun(&RemedyRun{ID: id, AlertID: alertID, NodeID: "node-x",
			Status: status, StopOnError: true, CreatedAt: now, UpdatedAt: now,
			Steps: []RemedyStep{scriptStep(0, "RM-1", "echo fix")}}))
	}
	mkRun("RUN-H1", "AL-H1", RunDone)
	mkRun("RUN-H2", "AL-H2", RunFailed)
	mkRun("RUN-H3", "AL-H3", RunDone) // 其他类型，不应出现

	runs, err := s.ListRemedyRunsByAlertType("OWL-DSK-001", 10)
	require.NoError(t, err)
	require.Len(t, runs, 2)
	require.Equal(t, "RUN-H1", runs[0].RunID) // created_at 相同则按插入序，至少都在集合内
	ids := map[string]bool{}
	for _, r := range runs {
		ids[r.RunID] = true
	}
	require.True(t, ids["RUN-H1"] && ids["RUN-H2"])
	require.False(t, ids["RUN-H3"], "其他告警类型的历史不得混入")
}

// ---- Enricher ----

func newTestEnricher(s *Store) *DisposalContextEnricher {
	return &DisposalContextEnricher{store: s, now: func() time.Time { return time.Unix(1750000000, 0) }}
}

func seedEnrichAlert(t *testing.T, s *Store) *Alert {
	now := time.Now().Unix()
	require.NoError(t, s.InsertAlert(&Alert{ID: "AL-E1", AlertTypeID: "OWL-DSK-001", NodeID: "node-e",
		Severity: SeverityWarning, Status: StatusOpen, Message: "磁盘 95%", FirstSeen: now, LastSeen: now}))
	at, _, err := s.GetAlertType("OWL-DSK-001")
	require.NoError(t, err)
	return &Alert{ID: "AL-E1", AlertTypeID: at.ID, NodeID: "node-e"}
}

func TestEnrich_HistoryBlock(t *testing.T) {
	s := newTestStore(t)
	e := newTestEnricher(s)

	// 历史同类处置：AL-HIST 同类型 + RunDone + 步骤
	now := time.Now().Unix()
	require.NoError(t, s.InsertAlert(&Alert{ID: "AL-HIST", AlertTypeID: "OWL-DSK-001", NodeID: "node-old",
		Severity: SeverityWarning, Status: StatusResolved, FirstSeen: now - 86400, LastSeen: now - 86400}))
	require.NoError(t, s.CreateRemedyRun(&RemedyRun{ID: "RUN-HIST", AlertID: "AL-HIST", NodeID: "node-old",
		Status: RunDone, StopOnError: true, CreatedAt: now - 86000, UpdatedAt: now - 86000,
		Steps: []RemedyStep{scriptStep(0, "RM-1", "echo cleanup")}}))

	// 该 run 的疗效验证：recovered
	require.NoError(t, s.SaveVerification(&RunVerification{RunID: "RUN-HIST", AlertID: "AL-HIST",
		NodeID: "node-old", AlertTypeID: "OWL-DSK-001", Status: VerifyRecovered, VerifiedAt: now - 85000}))

	alert := seedEnrichAlert(t, s)
	at, _, _ := s.GetAlertType("OWL-DSK-001")
	out := e.Enrich(alert, at)

	require.Contains(t, out, "[历史同类处置]")
	require.Contains(t, out, "RUN-HIST")
	require.Contains(t, out, "recovered")
}

func TestEnrich_ChangeRecords(t *testing.T) {
	s := newTestStore(t)
	e := newTestEnricher(s)
	e.history = func(nodeID string, since time.Time, limit int) ([]ChangeRecord, error) {
		return []ChangeRecord{
			{Time: "2026-09-17 10:00:00", User: "alice", Origin: "web", OpType: "command", Command: "systemctl restart nginx"},
		}, nil
	}
	alert := seedEnrichAlert(t, s)
	at, _, _ := s.GetAlertType("OWL-DSK-001")
	out := e.Enrich(alert, at)

	require.Contains(t, out, "[告警前变更]")
	require.Contains(t, out, "systemctl restart nginx")
	require.Contains(t, out, "alice")
}

func TestEnrich_RelatedAlerts(t *testing.T) {
	s := newTestStore(t)
	e := newTestEnricher(s)
	now := time.Now().Unix()
	alert := seedEnrichAlert(t, s)

	// 同节点其他类型活跃告警
	require.NoError(t, s.InsertAlert(&Alert{ID: "AL-MEM", AlertTypeID: "OWL-MEM-001", NodeID: "node-e",
		Severity: SeverityCritical, Status: StatusOpen, Message: "内存 95%", FirstSeen: now, LastSeen: now}))
	// 同类型其他节点活跃（群发信号）
	require.NoError(t, s.InsertAlert(&Alert{ID: "AL-DSK2", AlertTypeID: "OWL-DSK-001", NodeID: "node-f",
		Severity: SeverityWarning, Status: StatusOpen, Message: "磁盘 91%", FirstSeen: now, LastSeen: now}))

	at, _, _ := s.GetAlertType("OWL-DSK-001")
	out := e.Enrich(alert, at)

	require.Contains(t, out, "[相关告警]")
	require.Contains(t, out, "OWL-MEM-001")
	require.Contains(t, out, "node-f")
}

func TestEnrich_NoDataNoBlocks(t *testing.T) {
	s := newTestStore(t)
	e := newTestEnricher(s)
	alert := seedEnrichAlert(t, s)
	at, _, _ := s.GetAlertType("OWL-DSK-001")
	out := e.Enrich(alert, at)
	require.Empty(t, strings.TrimSpace(out), "无任何数据时不得输出空块")
}

// ---- AutoHealer 接线 ----

// captureAdvisor 捕获 Advise 收到的 Context
type captureAdvisor struct{ got string }

func (a *captureAdvisor) Advise(_ context.Context, req DisposalRequest) (*DisposalPlan, error) {
	a.got = req.Context
	return &DisposalPlan{Reasoning: "test"}, nil
}

type fakeEnricher struct{ out string }

func (f *fakeEnricher) Enrich(_ *Alert, _ AlertType) string { return f.out }

func TestAutoHealer_ContextEnricher(t *testing.T) {
	s := newTestStore(t)
	advisor := &captureAdvisor{}
	runner := NewRunExecutor(&remedyFakeFactory{exec: &remedyFakeExecer{}},
		func(nodeID string) (*Target, error) { return &Target{ID: nodeID}, nil })
	healer := NewAutoHealer(s, advisor, runner)
	healer.SetContextEnricher(&fakeEnricher{out: "[历史同类处置] RUN-X recovered"})

	now := time.Now().Unix()
	require.NoError(t, s.InsertAlert(&Alert{ID: "AL-AH", AlertTypeID: "OWL-DSK-001", NodeID: "node-a",
		Severity: SeverityWarning, Status: StatusOpen, Message: "磁盘", FirstSeen: now, LastSeen: now}))
	at, _, _ := s.GetAlertType("OWL-DSK-001")

	_, err := healer.Heal(context.Background(), &Alert{ID: "AL-AH", AlertTypeID: "OWL-DSK-001",
		NodeID: "node-a", Severity: SeverityWarning, Status: StatusOpen}, at, &Target{ID: "node-a"})
	require.NoError(t, err)
	require.Contains(t, advisor.got, "[历史同类处置] RUN-X recovered")
}
