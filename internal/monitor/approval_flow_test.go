package monitor

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestRunExecutor_WaitingApproval 验证审批步骤不执行、整单进入待审批，
// 批准后恢复执行，拒绝后跳过。
func TestRunExecutor_WaitingApproval(t *testing.T) {
	fake := &remedyFakeExecer{}
	ex, s := newRunExecutor(t, fake)

	run := &RemedyRun{ID: "RUN-1", AlertID: "AL-1", NodeID: "node-a",
		Status: RunPending, StopOnError: true, CreatedAt: 1000, UpdatedAt: 1000,
		Steps: []RemedyStep{
			scriptStep(0, "RM-1", "echo first"),
			{Order: 1, RemedyID: "RM-2", Name: "待审批", Kind: "script",
				Content: "echo second", Status: StepPendingApproval},
			scriptStep(2, "RM-3", "echo third"),
		}}
	require.NoError(t, s.CreateRemedyRun(run))

	require.NoError(t, ex.ExecuteRun(context.Background(), "RUN-1", s))

	got, _, _ := s.GetRemedyRun("RUN-1")
	require.Equal(t, RunWaitingApproval, got.Status, "有待审批步骤时整单应等待审批")
	require.Equal(t, StepSuccess, got.Steps[0].Status, "获准步骤照常执行")
	require.Equal(t, StepPendingApproval, got.Steps[1].Status, "待审批步骤不应执行")
	require.Equal(t, StepPending, got.Steps[2].Status, "审批屏障后的步骤等待恢复执行")
	fake.mu.Lock()
	require.Equal(t, []string{"echo first"}, fake.executed, "只执行获准步骤")
	fake.mu.Unlock()
}

// TestRunExecutor_ApproveResume 验证批准后恢复执行剩余步骤。
func TestRunExecutor_ApproveResume(t *testing.T) {
	fake := &remedyFakeExecer{}
	ex, s := newRunExecutor(t, fake)

	run := &RemedyRun{ID: "RUN-1", AlertID: "AL-1", NodeID: "node-a",
		Status: RunPending, StopOnError: true, CreatedAt: 1000, UpdatedAt: 1000,
		Steps: []RemedyStep{
			{Order: 0, RemedyID: "RM-2", Name: "待审批", Kind: "script",
				Content: "echo approved", Status: StepPendingApproval},
		}}
	require.NoError(t, s.CreateRemedyRun(run))

	// 第一轮：等待审批
	require.NoError(t, ex.ExecuteRun(context.Background(), "RUN-1", s))
	got, _, _ := s.GetRemedyRun("RUN-1")
	require.Equal(t, RunWaitingApproval, got.Status)

	// 批准：待审批 → 待执行，恢复执行
	require.NoError(t, s.ApproveRemedySteps("RUN-1", time.Now().Unix()))
	require.NoError(t, ex.ExecuteRun(context.Background(), "RUN-1", s))

	got, _, _ = s.GetRemedyRun("RUN-1")
	require.Equal(t, RunDone, got.Status)
	require.Equal(t, StepSuccess, got.Steps[0].Status)
	fake.mu.Lock()
	require.Equal(t, []string{"echo approved"}, fake.executed)
	fake.mu.Unlock()
}

// TestRunExecutor_RejectApproval 验证拒绝后步骤跳过、整单完成。
func TestRunExecutor_RejectApproval(t *testing.T) {
	fake := &remedyFakeExecer{}
	ex, s := newRunExecutor(t, fake)

	run := &RemedyRun{ID: "RUN-1", AlertID: "AL-1", NodeID: "node-a",
		Status: RunPending, StopOnError: true, CreatedAt: 1000, UpdatedAt: 1000,
		Steps: []RemedyStep{
			{Order: 0, RemedyID: "RM-2", Name: "待审批", Kind: "script",
				Content: "echo x", Status: StepPendingApproval},
		}}
	require.NoError(t, s.CreateRemedyRun(run))
	require.NoError(t, ex.ExecuteRun(context.Background(), "RUN-1", s))

	require.NoError(t, s.RejectRemedySteps("RUN-1", time.Now().Unix()))
	require.NoError(t, ex.ExecuteRun(context.Background(), "RUN-1", s))

	got, _, _ := s.GetRemedyRun("RUN-1")
	require.Equal(t, RunDone, got.Status)
	require.Equal(t, StepSkipped, got.Steps[0].Status, "拒绝后应跳过")
	require.Contains(t, got.Steps[0].Output, "拒绝")
}

// TestAutoHealer_ApprovalFlow 验证审批矩阵决策 → 待审批步骤 + 整单等待审批。
func TestAutoHealer_ApprovalFlow(t *testing.T) {
	fake := &remedyFakeExecer{}
	advisor := fakeAdvisor{plan: &DisposalPlan{Steps: []DisposalStep{
		// low 风险 → 自动（advisor 按风险排序，低风险在前）
		{RemedyID: "RM-L", Name: "低风险", Kind: "script", Content: "echo low", Risk: "low", Source: "user", Reviewed: true},
		// medium 风险单节点 → 审批屏障
		{RemedyID: "RM-M", Name: "中风险", Kind: "script", Content: "echo mid", Risk: "medium", Source: "user", Reviewed: true},
	}}}
	h, s := newTestAutoHealer(t, fake, advisor)
	for _, id := range []string{"RM-M", "RM-L"} {
		autoHealRemedy(t, s, id, true)
	}
	at := AlertType{ID: "OWL-DSK-001", AutoApprove: true}
	run, err := h.Heal(context.Background(), &Alert{ID: "AL-1", AlertTypeID: "OWL-DSK-001", NodeID: "n1"}, at, &Target{ID: "n1"})
	require.NoError(t, err)
	require.NotNil(t, run)

	got := waitPlanDone(t, s, run.ID)
	require.Equal(t, RunWaitingApproval, got.Status, "中风险步骤待审批")
	require.Equal(t, StepSuccess, got.Steps[0].Status, "低风险自动执行")
	require.Equal(t, StepPendingApproval, got.Steps[1].Status, "中风险步骤待审批")
}
