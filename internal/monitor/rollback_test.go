package monitor

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func rollbackStep(order int, remedyID, content, rollback string) RemedyStep {
	st := scriptStep(order, remedyID, content)
	st.Rollback = rollback
	return st
}

// TestRollback_ReverseOrderOnFailStop 失败即停时，已成功 script 步骤按逆序执行回滚，
// 失败步骤自身的 rollback 不执行，整单仍置 failed。
func TestRollback_ReverseOrderOnFailStop(t *testing.T) {
	fake := &remedyFakeExecer{failContent: map[string]bool{"echo boom": true}}
	ex, s := newRunExecutor(t, fake)

	run := &RemedyRun{ID: "RUN-RB1", AlertID: "AL-1", NodeID: "node-a",
		Status: RunPending, StopOnError: true, CreatedAt: 1000, UpdatedAt: 1000,
		Steps: []RemedyStep{
			rollbackStep(0, "RM-1", "echo step1", "undo step1"),
			rollbackStep(1, "RM-2", "echo boom", "undo boom"),
			scriptStep(2, "RM-3", "echo step3"),
		}}
	require.NoError(t, s.CreateRemedyRun(run))
	require.NoError(t, ex.ExecuteRun(context.Background(), "RUN-RB1", s))

	got, _, _ := s.GetRemedyRun("RUN-RB1")
	require.Equal(t, RunFailed, got.Status)
	require.Equal(t, StepFailed, got.Steps[1].Status)

	// step0 成功且被逆序回滚
	require.Contains(t, got.Steps[0].Output, "回滚执行")
	require.Contains(t, got.Steps[0].Output, "undo step1")

	// 失败步骤自身不回滚
	require.NotContains(t, got.Steps[1].Output, "undo boom")

	// 回滚脚本确实在远端执行过（step1 的 boom 之后是 step0 的 undo）
	fake.mu.Lock()
	executed := append([]string{}, fake.executed...)
	fake.mu.Unlock()
	require.GreaterOrEqual(t, len(executed), 3)
	require.Equal(t, "undo step1", executed[len(executed)-1], "回滚应逆序且最后执行")
}

// TestRollback_DisabledBySwitch rollbackEnabled=false 时不执行回滚。
func TestRollback_DisabledBySwitch(t *testing.T) {
	fake := &remedyFakeExecer{failContent: map[string]bool{"echo boom": true}}
	ex, s := newRunExecutor(t, fake)
	ex.SetRollbackEnabled(false)

	run := &RemedyRun{ID: "RUN-RB2", AlertID: "AL-1", NodeID: "node-a",
		Status: RunPending, StopOnError: true, CreatedAt: 1000, UpdatedAt: 1000,
		Steps: []RemedyStep{
			rollbackStep(0, "RM-1", "echo ok", "undo ok"),
			rollbackStep(1, "RM-2", "echo boom", "undo boom"),
		}}
	require.NoError(t, s.CreateRemedyRun(run))
	require.NoError(t, ex.ExecuteRun(context.Background(), "RUN-RB2", s))

	got, _, _ := s.GetRemedyRun("RUN-RB2")
	require.Equal(t, RunFailed, got.Status)
	require.NotContains(t, got.Steps[0].Output, "回滚执行")
}

// TestRollback_BlacklistedRollbackSkipped 回滚内容命中黑名单：跳过并记录，不阻断。
func TestRollback_BlacklistedRollbackSkipped(t *testing.T) {
	fake := &remedyFakeExecer{failContent: map[string]bool{"echo boom": true}}
	ex, s := newRunExecutor(t, fake)

	run := &RemedyRun{ID: "RUN-RB3", AlertID: "AL-1", NodeID: "node-a",
		Status: RunPending, StopOnError: true, CreatedAt: 1000, UpdatedAt: 1000,
		Steps: []RemedyStep{
			rollbackStep(0, "RM-1", "echo ok", "rm -rf /data/important"),
			rollbackStep(1, "RM-2", "echo boom", "undo boom"),
		}}
	require.NoError(t, s.CreateRemedyRun(run))
	require.NoError(t, ex.ExecuteRun(context.Background(), "RUN-RB3", s))

	got, _, _ := s.GetRemedyRun("RUN-RB3")
	require.Equal(t, RunFailed, got.Status)
	require.Contains(t, got.Steps[0].Output, "回滚")
	require.Contains(t, got.Steps[0].Output, "黑名单")
	// 黑名单内容不得被执行
	fake.mu.Lock()
	for _, c := range fake.executed {
		require.NotContains(t, c, "rm -rf /data/important")
	}
	fake.mu.Unlock()
}

// TestRollback_StopOnErrorFalse 全部执行完但含失败步骤：收尾时同样回滚已成功步骤。
func TestRollback_StopOnErrorFalse(t *testing.T) {
	fake := &remedyFakeExecer{failContent: map[string]bool{"echo boom": true}}
	ex, s := newRunExecutor(t, fake)

	run := &RemedyRun{ID: "RUN-RB4", AlertID: "AL-1", NodeID: "node-a",
		Status: RunPending, StopOnError: false, CreatedAt: 1000, UpdatedAt: 1000,
		Steps: []RemedyStep{
			rollbackStep(0, "RM-1", "echo ok", "undo ok"),
			rollbackStep(1, "RM-2", "echo boom", "undo boom"),
			rollbackStep(2, "RM-3", "echo ok3", "undo ok3"),
		}}
	require.NoError(t, s.CreateRemedyRun(run))
	require.NoError(t, ex.ExecuteRun(context.Background(), "RUN-RB4", s))

	got, _, _ := s.GetRemedyRun("RUN-RB4")
	require.Equal(t, StepFailed, got.Steps[1].Status)
	require.Equal(t, StepSuccess, got.Steps[2].Status)
	// step0 与 step2 都被回滚（逆序：step2 在前）
	require.Contains(t, got.Steps[0].Output, "undo ok")
	require.Contains(t, got.Steps[2].Output, "undo ok3")
	fake.mu.Lock()
	executed := append([]string{}, fake.executed...)
	fake.mu.Unlock()
	lastTwo := executed[len(executed)-2:]
	require.Equal(t, "undo ok3", lastTwo[0])
	require.Equal(t, "undo ok", lastTwo[1])
}

// TestRollback_RollbackFailureNotBlocking 回滚自身失败只记录，不影响整单终态。
func TestRollback_RollbackFailureNotBlocking(t *testing.T) {
	fake := &remedyFakeExecer{failContent: map[string]bool{"echo boom": true, "undo ok": true}}
	ex, s := newRunExecutor(t, fake)

	run := &RemedyRun{ID: "RUN-RB5", AlertID: "AL-1", NodeID: "node-a",
		Status: RunPending, StopOnError: true, CreatedAt: 1000, UpdatedAt: 1000,
		Steps: []RemedyStep{
			rollbackStep(0, "RM-1", "echo ok", "undo ok"),
			rollbackStep(1, "RM-2", "echo boom", "undo boom"),
		}}
	require.NoError(t, s.CreateRemedyRun(run))
	require.NoError(t, ex.ExecuteRun(context.Background(), "RUN-RB5", s))

	got, _, _ := s.GetRemedyRun("RUN-RB5")
	require.Equal(t, RunFailed, got.Status)
	require.Contains(t, got.Steps[0].Output, "回滚执行: 失败")
}

// TestRollback_NoSuccessStepsNoop 无已成功步骤（第一步就失败）时不触发任何回滚。
func TestRollback_NoSuccessStepsNoop(t *testing.T) {
	fake := &remedyFakeExecer{failContent: map[string]bool{"echo boom": true}}
	ex, s := newRunExecutor(t, fake)

	run := &RemedyRun{ID: "RUN-RB6", AlertID: "AL-1", NodeID: "node-a",
		Status: RunPending, StopOnError: true, CreatedAt: 1000, UpdatedAt: 1000,
		Steps: []RemedyStep{rollbackStep(0, "RM-1", "echo boom", "undo boom")}}
	require.NoError(t, s.CreateRemedyRun(run))
	require.NoError(t, ex.ExecuteRun(context.Background(), "RUN-RB6", s))

	got, _, _ := s.GetRemedyRun("RUN-RB6")
	require.Equal(t, RunFailed, got.Status)
	require.NotContains(t, got.Steps[0].Output, "回滚")
}

// TestRollback_Idempotent 重复执行同一 run（审批恢复场景重入）不重复回滚。
func TestRollback_Idempotent(t *testing.T) {
	fake := &remedyFakeExecer{failContent: map[string]bool{"echo boom": true}}
	ex, s := newRunExecutor(t, fake)

	run := &RemedyRun{ID: "RUN-RB7", AlertID: "AL-1", NodeID: "node-a",
		Status: RunPending, StopOnError: true, CreatedAt: 1000, UpdatedAt: 1000,
		Steps: []RemedyStep{
			rollbackStep(0, "RM-1", "echo ok", "undo ok"),
			rollbackStep(1, "RM-2", "echo boom", "undo boom"),
		}}
	require.NoError(t, s.CreateRemedyRun(run))
	require.NoError(t, ex.ExecuteRun(context.Background(), "RUN-RB7", s))
	_ = ex.ExecuteRun(context.Background(), "RUN-RB7", s) // 幂等重入

	got, _, _ := s.GetRemedyRun("RUN-RB7")
	require.Equal(t, 1, strings.Count(got.Steps[0].Output, "回滚执行"))
}
