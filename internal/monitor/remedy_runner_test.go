package monitor

import (
	"context"
	"encoding/base64"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// remedyFakeExecer 模拟远端执行：解码 base64 命令体并记录，可注入失败。
type remedyFakeExecer struct {
	mu          sync.Mutex
	executed    []string // 已执行的脚本内容（解码后）
	failContent map[string]bool
}

func (f *remedyFakeExecer) Execute(command string, timeout time.Duration) (int, string, error) {
	// 命令形如: echo '<base64>' | base64 -d | bash
	parts := strings.SplitN(command, "'", 3)
	if len(parts) < 2 {
		return -1, "", &execErr{cmd: command}
	}
	content, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return -1, "", err
	}
	f.mu.Lock()
	f.executed = append(f.executed, string(content))
	shouldFail := f.failContent[string(content)]
	f.mu.Unlock()
	if shouldFail {
		return 1, "boom", nil
	}
	return 0, "ok: " + string(content), nil
}

type remedyFakeFactory struct {
	exec Execer
}

func (f *remedyFakeFactory) NewExecer(t *Target) (Execer, error) {
	return f.exec, nil
}

func scriptStep(order int, remedyID, content string) RemedyStep {
	return RemedyStep{Order: order, RemedyID: remedyID, Name: "步骤-" + remedyID,
		Kind: "script", Content: content, Status: StepPending}
}

func sopStep(order int, remedyID string) RemedyStep {
	return RemedyStep{Order: order, RemedyID: remedyID, Name: "指引-" + remedyID,
		Kind: "sop", Content: "1. du -h /", Status: StepPending}
}

func newRunExecutor(t *testing.T, fake *remedyFakeExecer) (*RunExecutor, *Store) {
	t.Helper()
	s := newTestStore(t)
	ex := NewRunExecutor(&remedyFakeFactory{exec: fake}, func(nodeID string) (*Target, error) {
		return &Target{ID: nodeID, Address: "127.0.0.1", Port: 22, User: "local"}, nil
	})
	ex.now = func() int64 { return 1000 }
	// 反馈闭环需要对策存在
	for _, id := range []string{"RM-1", "RM-2", "RM-3", "RM-SOP"} {
		require.NoError(t, s.UpsertRemedy(sampleRemedy(id, "OWL-DSK-001", "user", true)))
	}
	return ex, s
}

// TestRunExecutor_Sequential 验证多个脚本步骤按序执行、全部成功、整单完成。
func TestRunExecutor_Sequential(t *testing.T) {
	fake := &remedyFakeExecer{}
	ex, s := newRunExecutor(t, fake)

	run := &RemedyRun{ID: "RUN-1", AlertID: "AL-1", NodeID: "node-a",
		Status: RunPending, StopOnError: true, CreatedAt: 1000, UpdatedAt: 1000,
		Steps: []RemedyStep{
			scriptStep(0, "RM-1", "echo step1"),
			scriptStep(1, "RM-2", "echo step2"),
		}}
	require.NoError(t, s.CreateRemedyRun(run))

	require.NoError(t, ex.ExecuteRun(context.Background(), "RUN-1", s))

	got, _, _ := s.GetRemedyRun("RUN-1")
	require.Equal(t, RunDone, got.Status)
	require.Equal(t, StepSuccess, got.Steps[0].Status)
	require.Equal(t, StepSuccess, got.Steps[1].Status)
	fake.mu.Lock()
	require.Equal(t, []string{"echo step1", "echo step2"}, fake.executed, "应按序执行")
	fake.mu.Unlock()

	// 反馈闭环：对策执行计数累加
	rm, _, _ := s.GetRemedy("RM-1")
	require.Equal(t, 1, rm.ExecCount)
	require.Equal(t, 1, rm.SuccessCount)
}

// TestRunExecutor_FailStop 验证失败即停：后续步骤标记 skipped，整单 failed。
func TestRunExecutor_FailStop(t *testing.T) {
	fake := &remedyFakeExecer{failContent: map[string]bool{"echo boom": true}}
	ex, s := newRunExecutor(t, fake)

	run := &RemedyRun{ID: "RUN-1", AlertID: "AL-1", NodeID: "node-a",
		Status: RunPending, StopOnError: true, CreatedAt: 1000, UpdatedAt: 1000,
		Steps: []RemedyStep{
			scriptStep(0, "RM-1", "echo ok"),
			scriptStep(1, "RM-2", "echo boom"),
			scriptStep(2, "RM-3", "echo never"),
		}}
	require.NoError(t, s.CreateRemedyRun(run))

	require.NoError(t, ex.ExecuteRun(context.Background(), "RUN-1", s))

	got, _, _ := s.GetRemedyRun("RUN-1")
	require.Equal(t, RunFailed, got.Status)
	require.Equal(t, StepSuccess, got.Steps[0].Status)
	require.Equal(t, StepFailed, got.Steps[1].Status)
	require.Equal(t, StepSkipped, got.Steps[2].Status, "失败后不再执行后续步骤")
	fake.mu.Lock()
	require.Equal(t, []string{"echo ok", "echo boom"}, fake.executed)
	fake.mu.Unlock()
}

// TestRunExecutor_ContinueOnError 验证 stop_on_error=false 时失败后继续。
func TestRunExecutor_ContinueOnError(t *testing.T) {
	fake := &remedyFakeExecer{failContent: map[string]bool{"echo boom": true}}
	_, s := newTestRunnerContinue(t, fake)

	got, _, _ := s.GetRemedyRun("RUN-1")
	require.Equal(t, RunDone, got.Status)
	require.Equal(t, StepFailed, got.Steps[1].Status)
	require.Equal(t, StepSuccess, got.Steps[2].Status, "失败后仍继续后续步骤")
}

// TestRunExecutor_SopSkipped 验证 sop 人工步骤自动跳过，脚本照常执行。
func TestRunExecutor_SopSkipped(t *testing.T) {
	fake := &remedyFakeExecer{}
	ex, s := newRunExecutor(t, fake)

	run := &RemedyRun{ID: "RUN-1", AlertID: "AL-1", NodeID: "node-a",
		Status: RunPending, StopOnError: true, CreatedAt: 1000, UpdatedAt: 1000,
		Steps: []RemedyStep{
			sopStep(0, "RM-SOP"),
			scriptStep(1, "RM-1", "echo hi"),
		}}
	require.NoError(t, s.CreateRemedyRun(run))

	require.NoError(t, ex.ExecuteRun(context.Background(), "RUN-1", s))

	got, _, _ := s.GetRemedyRun("RUN-1")
	require.Equal(t, StepSkipped, got.Steps[0].Status, "sop 人工步骤应跳过")
	require.Equal(t, StepSuccess, got.Steps[1].Status)
	require.Contains(t, got.Steps[0].Output, "人工", "跳过原因应说明")
}

// TestRunExecutor_BlacklistBlocked 验证命中危险命令黑名单时不执行、步骤失败。
func TestRunExecutor_BlacklistBlocked(t *testing.T) {
	fake := &remedyFakeExecer{}
	ex, s := newRunExecutor(t, fake)

	run := &RemedyRun{ID: "RUN-1", AlertID: "AL-1", NodeID: "node-a",
		Status: RunPending, StopOnError: true, CreatedAt: 1000, UpdatedAt: 1000,
		Steps: []RemedyStep{
			scriptStep(0, "RM-DANGER", "rm -rf /"),
		}}
	require.NoError(t, s.CreateRemedyRun(run))

	require.NoError(t, ex.ExecuteRun(context.Background(), "RUN-1", s))

	got, _, _ := s.GetRemedyRun("RUN-1")
	require.Equal(t, StepFailed, got.Steps[0].Status)
	require.Contains(t, got.Steps[0].Output, "黑名单", "应说明黑名单拦截原因")
	fake.mu.Lock()
	require.Empty(t, fake.executed, "危险命令不应真正执行")
	fake.mu.Unlock()
}

// TestRunExecutor_Stop 验证执行中停止：后续步骤 skipped，整单 stopped。
func TestRunExecutor_Stop(t *testing.T) {
	// 阻塞式 fake：第一步真正开始执行后通知测试，等待放行
	blocker := &blockingExecer{started: make(chan struct{}), release: make(chan struct{})}
	s := newTestStore(t)
	ex := NewRunExecutor(&remedyFakeFactory{exec: blocker}, func(nodeID string) (*Target, error) {
		return &Target{ID: nodeID}, nil
	})
	ex.now = func() int64 { return 1000 }

	run := &RemedyRun{ID: "RUN-1", AlertID: "AL-1", NodeID: "node-a",
		Status: RunPending, StopOnError: true, CreatedAt: 1000, UpdatedAt: 1000,
		Steps: []RemedyStep{
			scriptStep(0, "RM-1", "echo slow"),
			scriptStep(1, "RM-2", "echo after"),
		}}
	require.NoError(t, s.CreateRemedyRun(run))

	done := make(chan error, 1)
	go func() { done <- ex.ExecuteRun(context.Background(), "RUN-1", s) }()

	// 等第一步真正进入执行
	<-blocker.started
	require.NoError(t, s.UpdateRemedyRunStatus("RUN-1", RunStopped))
	close(blocker.release)

	require.NoError(t, <-done)

	got, _, _ := s.GetRemedyRun("RUN-1")
	require.Equal(t, RunStopped, got.Status)
	require.Equal(t, StepSkipped, got.Steps[1].Status, "停止后剩余步骤应跳过")
}

type blockingExecer struct {
	started chan struct{}
	release chan struct{}
}

func (f *blockingExecer) Execute(command string, timeout time.Duration) (int, string, error) {
	select {
	case <-f.started:
	default:
		close(f.started)
	}
	<-f.release
	return 0, "ok", nil
}

// 辅助：失败继续场景
func newTestRunnerContinue(t *testing.T, fake *remedyFakeExecer) (*RunExecutor, *Store) {
	t.Helper()
	ex, s := newRunExecutor(t, fake)
	run := &RemedyRun{ID: "RUN-1", AlertID: "AL-1", NodeID: "node-a",
		Status: RunPending, StopOnError: false, CreatedAt: 1000, UpdatedAt: 1000,
		Steps: []RemedyStep{
			scriptStep(0, "RM-1", "echo ok"),
			scriptStep(1, "RM-2", "echo boom"),
			scriptStep(2, "RM-3", "echo after"),
		}}
	require.NoError(t, s.CreateRemedyRun(run))
	require.NoError(t, ex.ExecuteRun(context.Background(), "RUN-1", s))
	return ex, s
}
