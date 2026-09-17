package monitor

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestVerificationStore_RoundTrip(t *testing.T) {
	s := newTestStore(t)

	rec := &RunVerification{
		RunID: "RUN-1", AlertID: "AL-1", NodeID: "node-a",
		AlertTypeID: "OWL-DSK-001", Status: VerifyRecovered,
		Metric: "disk.usage./", Value: 71.2,
		Message: "处置后定向采集：指标已回落", VerifiedAt: 1750000000,
	}
	require.NoError(t, s.SaveVerification(rec))

	got, err := s.GetVerificationByRun("RUN-1")
	require.NoError(t, err)
	require.Equal(t, VerifyRecovered, got.Status)
	require.Equal(t, "OWL-DSK-001", got.AlertTypeID)
	require.InDelta(t, 71.2, got.Value, 0.001)

	// upsert：同 run 重复验证覆盖
	rec.Status = VerifyNotRecovered
	rec.VerifiedAt = 1750000100
	require.NoError(t, s.SaveVerification(rec))
	got, _ = s.GetVerificationByRun("RUN-1")
	require.Equal(t, VerifyNotRecovered, got.Status)

	// ListVerificationsByAlert（verified_at 倒序）
	rec2 := &RunVerification{RunID: "RUN-2", AlertID: "AL-1", NodeID: "node-a",
		AlertTypeID: "OWL-DSK-001", Status: VerifyInconclusive, VerifiedAt: 1750000200}
	require.NoError(t, s.SaveVerification(rec2))
	list, err := s.ListVerificationsByAlert("AL-1")
	require.NoError(t, err)
	require.Len(t, list, 2)
	require.Equal(t, "RUN-2", list[0].RunID)

	// 未验证的 run 查不到
	if _, err := s.GetVerificationByRun("RUN-NONE"); err == nil {
		t.Fatal("expected not-found error")
	}
}

// TestRunVerifier_Recovered 处置后定向采集未再命中规则 → recovered（sampleOutputs 磁盘 45% < 90%）。
func TestRunVerifier_Recovered(t *testing.T) {
	fake := &fakeExecer{outputs: sampleOutputs()}
	s := newTestStore(t)
	seedVerifyFixture(t, s, "AL-V1", "RUN-V1")

	v := newTestVerifier(t, s, fake)
	res, err := v.VerifyRun(context.Background(), "RUN-V1")
	require.NoError(t, err)
	require.Equal(t, VerifyRecovered, res.Status)

	got, err := s.GetVerificationByRun("RUN-V1")
	require.NoError(t, err)
	require.Equal(t, VerifyRecovered, got.Status)
	require.Equal(t, "AL-V1", got.AlertID)
}

// TestRunVerifier_NotRecovered 处置后指标仍命中规则 → not_recovered。
func TestRunVerifier_NotRecovered(t *testing.T) {
	fake := &fakeExecer{outputs: withDiskUsage("95%")}
	s := newTestStore(t)
	seedVerifyFixture(t, s, "AL-V2", "RUN-V2")

	v := newTestVerifier(t, s, fake)
	res, err := v.VerifyRun(context.Background(), "RUN-V2")
	require.NoError(t, err)
	require.Equal(t, VerifyNotRecovered, res.Status)
	require.Greater(t, res.Value, 90.0)
}

// TestRunVerifier_Inconclusive 必需采集步骤失败与"已恢复"严格区分 → inconclusive。
func TestRunVerifier_Inconclusive(t *testing.T) {
	fake := &fakeExecer{outputs: sampleOutputs(), fail: map[string]bool{"cat /proc/loadavg": true}}
	s := newTestStore(t)
	seedVerifyFixture(t, s, "AL-V3", "RUN-V3")

	v := newTestVerifier(t, s, fake)
	res, err := v.VerifyRun(context.Background(), "RUN-V3")
	require.NoError(t, err)
	require.Equal(t, VerifyInconclusive, res.Status)
	require.NotEmpty(t, res.Message)
}

// TestRunVerifier_RejectsNonDoneRun 非终态/失败单不得触发验证。
func TestRunVerifier_RejectsNonDoneRun(t *testing.T) {
	fake := &fakeExecer{outputs: sampleOutputs()}
	s := newTestStore(t)
	seedVerifyFixture(t, s, "AL-V4", "RUN-V4")
	require.NoError(t, s.UpdateRemedyRunStatus("RUN-V4", RunFailed))

	v := newTestVerifier(t, s, fake)
	if _, err := v.VerifyRun(context.Background(), "RUN-V4"); err == nil {
		t.Fatal("expected error for non-done run")
	}
}

// TestRunExecutor_OnRunFinished 终态 RunDone 触发收尾回调（执行器侧 go 异步）。
func TestRunExecutor_OnRunFinished(t *testing.T) {
	fake := &remedyFakeExecer{}
	ex, s := newRunExecutor(t, fake)

	done := make(chan *RemedyRun, 1)
	ex.OnRunFinished = func(run *RemedyRun) { done <- run }

	run := &RemedyRun{ID: "RUN-CB", AlertID: "AL-1", NodeID: "node-a",
		Status: RunPending, StopOnError: true, CreatedAt: 1000, UpdatedAt: 1000,
		Steps: []RemedyStep{scriptStep(0, "RM-1", "echo step1")}}
	require.NoError(t, s.CreateRemedyRun(run))
	require.NoError(t, ex.ExecuteRun(context.Background(), "RUN-CB", s))

	select {
	case r := <-done:
		require.Equal(t, RunDone, r.Status)
		require.Equal(t, "RUN-CB", r.ID)
	case <-time.After(2 * time.Second):
		t.Fatal("OnRunFinished not called")
	}
}

// TestRunExecutor_OnRunFinished_SkipWaitingApproval 等待审批不触发回调。
func TestRunExecutor_OnRunFinished_SkipWaitingApproval(t *testing.T) {
	fake := &remedyFakeExecer{}
	ex, s := newRunExecutor(t, fake)

	called := make(chan *RemedyRun, 1)
	ex.OnRunFinished = func(run *RemedyRun) { called <- run }

	run := &RemedyRun{ID: "RUN-WA", AlertID: "AL-1", NodeID: "node-a",
		Status: RunPending, StopOnError: true, CreatedAt: 1000, UpdatedAt: 1000,
		Steps: []RemedyStep{func() RemedyStep {
			st := scriptStep(0, "RM-1", "echo x")
			st.Status = StepPendingApproval
			return st
		}()}}
	require.NoError(t, s.CreateRemedyRun(run))
	require.NoError(t, ex.ExecuteRun(context.Background(), "RUN-WA", s))

	got, _, _ := s.GetRemedyRun("RUN-WA")
	require.Equal(t, RunWaitingApproval, got.Status)
	select {
	case <-called:
		t.Fatal("waiting_approval must not trigger OnRunFinished")
	default:
	}
}

// ---- 帮手 ----

// withDiskUsage 把磁盘使用率替换为指定百分比（df -P 与 df -Pi 同步）。
func withDiskUsage(pct string) map[string]string {
	out := sampleOutputs()
	out["LC_ALL=C df -P"] = "Filesystem     1024-blocks    Used Available Capacity Mounted on\n/dev/sda1 205113712 999999 108722116 " + pct + " /\n"
	out["LC_ALL=C df -Pi"] = "Filesystem     Inodes IUsed IFree IUse% Mounted on\n/dev/sda1 12845056 296247 12548809 3% /\n"
	return out
}

// newTestVerifier 构造验证器：delay=0、单节点 fake 采集。
func newTestVerifier(t *testing.T, s *Store, fake *fakeExecer) *RunVerifier {
	t.Helper()
	v := NewRunVerifier(NewCollector(&fakeFactory{exec: fake}),
		func(nodeID string) (*Target, error) {
			return &Target{ID: nodeID, Address: "127.0.0.1", Port: 22, User: "local"}, nil
		}, s)
	v.SetDelay(0)
	v.now = func() time.Time { return time.Unix(1750000000, 0) }
	return v
}

// seedVerifyFixture 预置：活跃磁盘告警 + 一个 RunDone 的执行计划（一个成功 script 步骤）。
func seedVerifyFixture(t *testing.T, s *Store, alertID, runID string) {
	t.Helper()
	now := time.Now().Unix()
	require.NoError(t, s.InsertAlert(&Alert{
		ID: alertID, AlertTypeID: "OWL-DSK-001", NodeID: "node-a",
		Severity: SeverityWarning, Status: StatusOpen, Message: "磁盘使用率过高",
		MetricSnapshot: `{"disk.usage./":95}`, FirstSeen: now, LastSeen: now,
	}))
	require.NoError(t, s.CreateRemedyRun(&RemedyRun{ID: runID, AlertID: alertID, NodeID: "node-a",
		Status: RunDone, StopOnError: true, CreatedAt: now, UpdatedAt: now,
		Steps: []RemedyStep{func() RemedyStep {
			st := scriptStep(0, "RM-1", "echo fix")
			st.Status = StepSuccess
			st.ExitCode = 0
			return st
		}()}}))
	// 反馈计数需要对策存在
	require.NoError(t, s.UpsertRemedy(sampleRemedy("RM-1", "OWL-DSK-001", "user", true)))
}

// TestClosedLoop_EndToEnd 闭环贯通：执行计划完成 → OnRunFinished 回调 →
// 定向采集验证 → 结果落库（C1 核心链路的组合集成验证）。
func TestClosedLoop_EndToEnd(t *testing.T) {
	// 指标已回落（磁盘 45% < 90%）→ 处置后验证应为 recovered
	fake := &fakeExecer{outputs: sampleOutputs()}
	s := newTestStore(t)

	verifier := NewRunVerifier(NewCollector(&fakeFactory{exec: fake}),
		func(nodeID string) (*Target, error) {
			return &Target{ID: nodeID, Address: "127.0.0.1", Port: 22, User: "local"}, nil
		}, s)
	verifier.SetDelay(0)
	verifier.now = func() time.Time { return time.Unix(1750000000, 0) }

	ex := NewRunExecutor(&remedyFakeFactory{exec: fake},
		func(nodeID string) (*Target, error) {
			return &Target{ID: nodeID, Address: "127.0.0.1", Port: 22, User: "local"}, nil
		})
	ex.OnRunFinished = func(run *RemedyRun) {
		if _, err := verifier.VerifyRun(context.Background(), run.ID); err != nil {
			t.Errorf("verify failed: %v", err)
		}
	}

	seedVerifyFixture(t, s, "AL-CL", "RUN-CL")
	// seed 的 run 已是 RunDone，重置为 pending 走真实执行链
	require.NoError(t, s.UpdateRemedyRunStatus("RUN-CL", RunPending))

	require.NoError(t, ex.ExecuteRun(context.Background(), "RUN-CL", s))

	// OnRunFinished 异步触发，轮询等待验证落库
	var v *RunVerification
	var err error
	for i := 0; i < 40; i++ {
		if v, err = s.GetVerificationByRun("RUN-CL"); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.NoError(t, err, "执行完成后验证记录应落库")
	require.Equal(t, VerifyRecovered, v.Status)
	require.Equal(t, "AL-CL", v.AlertID)
}
