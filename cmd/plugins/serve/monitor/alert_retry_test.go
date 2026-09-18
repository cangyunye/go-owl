package monitor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	owlmonitor "github.com/cangyunye/go-owl/internal/monitor"
)

// retryFakeExecer 本地 fake 执行器工厂：重试计划异步执行时避免 nil runner panic
type retryFakeExecer struct{}

func (f *retryFakeExecer) NewExecer(t *owlmonitor.Target) (owlmonitor.Execer, error) {
	return execerFunc(func(command string, timeout time.Duration) (int, string, error) {
		return 0, "ok", nil
	}), nil
}

type execerFunc func(command string, timeout time.Duration) (int, string, error)

func (f execerFunc) Execute(command string, timeout time.Duration) (int, string, error) {
	return f(command, timeout)
}

func newRetryTestService(t *testing.T) (*Service, *owlmonitor.Store) {
	t.Helper()
	st, err := owlmonitor.OpenStore(t.TempDir() + "/m.db")
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })
	// StartRemedyRun 会异步 runner.ExecuteRun，给 fake 执行器避免 nil panic
	runner := owlmonitor.NewRunExecutor(&retryFakeExecer{},
		func(nodeID string) (*owlmonitor.Target, error) { return &owlmonitor.Target{ID: nodeID}, nil })
	svc := &Service{Store: st, db: nil, runner: runner} // db=nil：配置走保守默认（关闭）
	return svc, st
}

func seedRetryFixture(t *testing.T, st *owlmonitor.Store) *owlmonitor.Alert {
	t.Helper()
	now := time.Now().Unix()
	require.NoError(t, st.InsertAlert(&owlmonitor.Alert{ID: "AL-RT", AlertTypeID: "OWL-DSK-001",
		NodeID: "node-a", Severity: owlmonitor.SeverityWarning, Status: owlmonitor.StatusOpen,
		Message: "磁盘 95%", FirstSeen: now, LastSeen: now}))
	require.NoError(t, st.UpsertRemedy(sampleRemedyForRetry("RM-A", "low")))
	require.NoError(t, st.UpsertRemedy(sampleRemedyForRetry("RM-B", "low")))
	at, _, err := st.GetAlertType("OWL-DSK-001")
	require.NoError(t, err)
	alert, _, err := st.GetAlert("AL-RT")
	require.NoError(t, err)
	_ = at
	return alert
}

func sampleRemedyForRetry(id, risk string) owlmonitor.Remedy {
	return owlmonitor.Remedy{ID: id, AlertTypeID: "OWL-DSK-001", Name: "对策-" + id,
		Kind: "script", Content: "echo " + id, Risk: risk, Source: "user", Reviewed: true,
		CreatedAt: 1000, UpdatedAt: 1000}
}

func TestMaybeRetryRemedy_Disabled(t *testing.T) {
	svc, st := newRetryTestService(t)
	alert := seedRetryFixture(t, st)
	at, _, _ := st.GetAlertType("OWL-DSK-001")
	run := &owlmonitor.RemedyRun{ID: "RUN-D", AlertID: "AL-RT", NodeID: "node-a", Status: owlmonitor.RunDone}
	ver := &owlmonitor.RunVerification{RunID: "RUN-D", Status: owlmonitor.VerifyNotRecovered}

	// db=nil → 配置默认关闭 → 不创建重试
	require.NoError(t, st.CreateRemedyRun(run))
	svc.maybeRetryRemedy(alert, at, run, ver)

	runs, err := st.ListRemedyRunsByAlert("AL-RT")
	require.NoError(t, err)
	require.Len(t, runs, 1, "关闭时不得自动重试")
}

func TestMaybeRetryRemedy_EnabledPicksUntried(t *testing.T) {
	svc, st := newRetryTestService(t)
	svc.healRetryEnabled = true
	svc.healRetryLimit = 1
	alert := seedRetryFixture(t, st)
	at, _, _ := st.GetAlertType("OWL-DSK-001")

	// 第一次处置已用 RM-A，验证 not_recovered
	run := &owlmonitor.RemedyRun{ID: "RUN-1", AlertID: "AL-RT", NodeID: "node-a",
		Status: owlmonitor.RunDone, CreatedBy: "auto-heal", CreatedAt: 1000, UpdatedAt: 1000,
		Steps: []owlmonitor.RemedyStep{{Order: 0, RemedyID: "RM-A", Kind: "script", Status: owlmonitor.StepSuccess}}}
	require.NoError(t, st.CreateRemedyRun(run))
	ver := &owlmonitor.RunVerification{RunID: "RUN-1", Status: owlmonitor.VerifyNotRecovered}

	svc.maybeRetryRemedy(alert, at, run, ver)

	runs, err := st.ListRemedyRunsByAlert("AL-RT")
	require.NoError(t, err)
	require.Len(t, runs, 2, "应自动创建一次重试计划")
	var retryRunID string
	for _, r := range runs {
		if r.CreatedBy == "auto-heal-retry" {
			retryRunID = r.ID
		}
	}
	require.NotEmpty(t, retryRunID)
	fresh, _, err := st.GetRemedyRun(retryRunID)
	require.NoError(t, err)
	require.Equal(t, "RM-B", fresh.Steps[0].RemedyID, "应选用未尝试过的对策")
}

func TestMaybeRetryRemedy_LimitReached(t *testing.T) {
	svc, st := newRetryTestService(t)
	svc.healRetryEnabled = true
	svc.healRetryLimit = 1
	alert := seedRetryFixture(t, st)
	at, _, _ := st.GetAlertType("OWL-DSK-001")

	// 已有 1 次 auto-heal-retry → 达上限，不再重试
	require.NoError(t, st.CreateRemedyRun(&owlmonitor.RemedyRun{ID: "RUN-R1", AlertID: "AL-RT",
		NodeID: "node-a", Status: owlmonitor.RunDone, CreatedBy: "auto-heal-retry",
		CreatedAt: 1000, UpdatedAt: 1000,
		Steps: []owlmonitor.RemedyStep{{Order: 0, RemedyID: "RM-B", Kind: "script", Status: owlmonitor.StepSuccess}}}))

	svc.maybeRetryRemedy(alert, at, &owlmonitor.RemedyRun{ID: "RUN-0", AlertID: "AL-RT"},
		&owlmonitor.RunVerification{RunID: "RUN-0", Status: owlmonitor.VerifyNotRecovered})

	runs, _ := st.ListRemedyRunsByAlert("AL-RT")
	require.Len(t, runs, 1, "达到重试上限后不得再创建")
}

func TestMaybeRetryRemedy_IgnoresRecovered(t *testing.T) {
	svc, st := newRetryTestService(t)
	svc.healRetryEnabled = true
	alert := seedRetryFixture(t, st)
	at, _, _ := st.GetAlertType("OWL-DSK-001")

	svc.maybeRetryRemedy(alert, at, &owlmonitor.RemedyRun{ID: "RUN-OK", AlertID: "AL-RT"},
		&owlmonitor.RunVerification{RunID: "RUN-OK", Status: owlmonitor.VerifyRecovered})

	runs, _ := st.ListRemedyRunsByAlert("AL-RT")
	require.Len(t, runs, 0, "已恢复的验证不触发重试")
}
