package monitor

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeAdvisor 可控建议器。
type fakeAdvisor struct {
	plan *DisposalPlan
	err  error
}

func (f fakeAdvisor) Advise(ctx context.Context, req DisposalRequest) (*DisposalPlan, error) {
	return f.plan, f.err
}

func autoHealRemedy(t *testing.T, s *Store, id string, approved bool) {
	t.Helper()
	require.NoError(t, s.UpsertRemedy(Remedy{
		ID: id, AlertTypeID: "OWL-DSK-001", Name: "对策-" + id,
		Kind: "script", Content: "echo " + id, Risk: "low",
		Source: "user", Reviewed: true, AutoApprove: approved,
	}))
}

func newTestAutoHealer(t *testing.T, fake *remedyFakeExecer, advisor Advisor) (*AutoHealer, *Store) {
	t.Helper()
	s := newTestStore(t)
	runner := NewRunExecutor(&remedyFakeFactory{exec: fake}, func(nodeID string) (*Target, error) {
		return &Target{ID: nodeID}, nil
	})
	runner.now = func() int64 { return 1000 }
	return NewAutoHealer(s, advisor, runner), s
}

func waitPlanDone(t *testing.T, s *Store, runID string) RemedyRun {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		run, _, err := s.GetRemedyRun(runID)
		require.NoError(t, err)
		if run.IsTerminal() {
			return *run
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("计划未在时限内完成")
	return RemedyRun{}
}

// TestAutoHealer_AllApproved 验证全放行低风险单节点 → 自动执行成功。
func TestAutoHealer_AllApproved(t *testing.T) {
	fake := &remedyFakeExecer{}
	advisor := fakeAdvisor{plan: &DisposalPlan{Reasoning: "test", Steps: []DisposalStep{
		{RemedyID: "RM-1", Name: "清理", Kind: "script", Content: "echo RM-1", Risk: "low", Source: "user", Reviewed: true},
	}}}
	h, s := newTestAutoHealer(t, fake, advisor)
	autoHealRemedy(t, s, "RM-1", true)

	at := AlertType{ID: "OWL-DSK-001", AutoApprove: true}
	run, err := h.Heal(context.Background(), &Alert{ID: "AL-1", AlertTypeID: "OWL-DSK-001", NodeID: "n1"}, at, &Target{ID: "n1"})
	require.NoError(t, err)
	require.NotNil(t, run)

	got := waitPlanDone(t, s, run.ID)
	require.Equal(t, RunDone, got.Status)
	require.Equal(t, StepSuccess, got.Steps[0].Status)
}

// TestAutoHealer_SyntaxGate 验证 AI 生成脚本语法不合格被拦截，其余照常。
func TestAutoHealer_SyntaxGate(t *testing.T) {
	fake := &remedyFakeExecer{}
	advisor := fakeAdvisor{plan: &DisposalPlan{Steps: []DisposalStep{
		{RemedyID: "RM-OK", Name: "合法", Kind: "script", Content: "echo ok", Risk: "low", Source: "ai", Reviewed: true},
		{RemedyID: "", Name: "AI 脚本", Kind: "script", Content: "if [ -f /x ]; then echo broken", Risk: "low", Source: "ai", Reviewed: true, Generated: true},
	}}}
	h, s := newTestAutoHealer(t, fake, advisor)
	autoHealRemedy(t, s, "RM-OK", true)

	at := AlertType{ID: "OWL-DSK-001", AutoApprove: true}
	run, err := h.Heal(context.Background(), &Alert{ID: "AL-1", AlertTypeID: "OWL-DSK-001", NodeID: "n1"}, at, &Target{ID: "n1"})
	require.NoError(t, err)
	require.NotNil(t, run)

	got := waitPlanDone(t, s, run.ID)
	require.Equal(t, StepSkipped, got.Steps[1].Status, "语法不合格应被拦截")
	require.Contains(t, got.Steps[1].Output, "语法", "应说明语法校验失败")
	require.Equal(t, StepSuccess, got.Steps[0].Status, "合法步骤照常执行")
}

// TestAutoHealer_HighRiskHuman 验证高风险仅人工（不进入自动执行）。
func TestAutoHealer_HighRiskHuman(t *testing.T) {
	fake := &remedyFakeExecer{}
	advisor := fakeAdvisor{plan: &DisposalPlan{Steps: []DisposalStep{
		{RemedyID: "RM-H", Name: "高风险", Kind: "script", Content: "echo risky", Risk: "high", Source: "user", Reviewed: true},
	}}}
	h, s := newTestAutoHealer(t, fake, advisor)
	autoHealRemedy(t, s, "RM-H", true)

	at := AlertType{ID: "OWL-DSK-001", AutoApprove: true}
	run, err := h.Heal(context.Background(), &Alert{ID: "AL-1", AlertTypeID: "OWL-DSK-001", NodeID: "n1"}, at, &Target{ID: "n1"})
	require.NoError(t, err)
	require.NotNil(t, run)

	got := waitPlanDone(t, s, run.ID)
	require.Equal(t, StepSkipped, got.Steps[0].Status)
	require.Contains(t, got.Steps[0].Output, "仅人工", "高风险应仅人工")
	fake.mu.Lock()
	require.Empty(t, fake.executed, "高风险不应自动执行")
	fake.mu.Unlock()
}

// TestAutoHealer_TypeNotApproved 验证类型未放行 → 全部仅人工。
func TestAutoHealer_TypeNotApproved(t *testing.T) {
	fake := &remedyFakeExecer{}
	advisor := fakeAdvisor{plan: &DisposalPlan{Steps: []DisposalStep{
		{RemedyID: "RM-1", Name: "清理", Kind: "script", Content: "echo hi", Risk: "low", Source: "user", Reviewed: true},
	}}}
	h, s := newTestAutoHealer(t, fake, advisor)
	autoHealRemedy(t, s, "RM-1", true)

	at := AlertType{ID: "OWL-DSK-001", AutoApprove: false} // 未放行
	run, err := h.Heal(context.Background(), &Alert{ID: "AL-1", AlertTypeID: "OWL-DSK-001", NodeID: "n1"}, at, &Target{ID: "n1"})
	require.NoError(t, err)
	require.NotNil(t, run)

	got := waitPlanDone(t, s, run.ID)
	require.Equal(t, StepSkipped, got.Steps[0].Status)
	require.Contains(t, got.Steps[0].Output, "放行", "未放行应说明")
}

// TestAutoHealer_EmptyPlan 验证空建议 → 不创建计划。
func TestAutoHealer_EmptyPlan(t *testing.T) {
	h, _ := newTestAutoHealer(t, &remedyFakeExecer{}, fakeAdvisor{plan: &DisposalPlan{}})
	run, err := h.Heal(context.Background(), &Alert{ID: "AL-1", AlertTypeID: "OWL-DSK-001", NodeID: "n1"},
		AlertType{ID: "OWL-DSK-001", AutoApprove: true}, &Target{ID: "n1"})
	require.NoError(t, err)
	require.Nil(t, run, "无步骤时不创建计划")
}
