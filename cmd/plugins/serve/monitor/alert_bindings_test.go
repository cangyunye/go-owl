package monitor

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	owlmonitor "github.com/cangyunye/go-owl/internal/monitor"
	"github.com/stretchr/testify/require"
)

// fakePlaybookRunner 记录启动顺序，可手动放行"运行完成"，用于验证
// 顺序串行（前一未完成不启动后一）与并发（同时启动）两种模式。
type fakePlaybookRunner struct {
	mu      sync.Mutex
	started []string // playbook 名（按启动顺序）
	held    map[string]bool
	seq     int
}

func newFakeRunner() *fakePlaybookRunner {
	return &fakePlaybookRunner{held: map[string]bool{}}
}

func (f *fakePlaybookRunner) RunForAlert(_ context.Context, playbookID, nodeID, createdBy string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	id := fmt.Sprintf("PBR-%d", f.seq)
	f.started = append(f.started, playbookID+"|"+nodeID)
	f.held[id] = true
	return id, nil
}

func (f *fakePlaybookRunner) PlaybookRunFinished(_ context.Context, runID string) (bool, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.held[runID] {
		return true, "completed", nil
	}
	return false, "running", nil
}

func (f *fakePlaybookRunner) release(runID string) {
	f.mu.Lock()
	delete(f.held, runID)
	f.mu.Unlock()
}

func (f *fakePlaybookRunner) startedSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.started...)
}

func newBindingTestService(t *testing.T) (*Service, *owlmonitor.Store) {
	t.Helper()
	st, err := owlmonitor.OpenStore(filepath.Join(t.TempDir(), "monitor.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	now := time.Now().Unix()
	require.NoError(t, st.InsertAlert(&owlmonitor.Alert{ID: "AL-1", AlertTypeID: "OWL-MEM-001",
		NodeID: "n1", Severity: owlmonitor.SeverityWarning, Status: owlmonitor.StatusOpen,
		Message: "x", FirstSeen: now, LastSeen: now}))
	svc := &Service{Store: st}
	return svc, st
}

func mkBinding(id, pbID string, seq int, auto bool, mode string) owlmonitor.AlertBinding {
	return owlmonitor.AlertBinding{ID: id, AlertID: "AL-1", Kind: "playbook",
		Name: id, Content: pbID, AutoExec: auto, ExecMode: mode, Seq: seq, CreatedBy: "admin"}
}

// TestService_RunAlertBindings_Sequential 验证顺序串行：前一条未完成不启动后一条。
func TestService_RunAlertBindings_Sequential(t *testing.T) {
	svc, st := newBindingTestService(t)
	runner := newFakeRunner()
	svc.SetPlaybookRunner(runner)

	recs, err := svc.RunAlertBindings("AL-1", []owlmonitor.AlertBinding{
		mkBinding("AB-1", "pb-first", 1, false, ""),
		mkBinding("AB-2", "pb-second", 2, false, ""),
	}, "sequential", "n1", "admin")
	require.NoError(t, err)
	require.Len(t, recs, 2)

	require.Eventually(t, func() bool {
		return len(runner.startedSnapshot()) == 1
	}, time.Second, 5*time.Millisecond, "第一条应启动")
	require.Equal(t, []string{"pb-first|n1"}, runner.startedSnapshot())
	time.Sleep(50 * time.Millisecond)
	require.Len(t, runner.startedSnapshot(), 1, "第一条未完成时不得启动第二条")

	runner.release("PBR-1")
	require.Eventually(t, func() bool {
		return len(runner.startedSnapshot()) == 2
	}, 2*time.Second, 5*time.Millisecond, "第一条完成后启动第二条")
	require.Equal(t, []string{"pb-first|n1", "pb-second|n1"}, runner.startedSnapshot())

	// 关联运行 ID 应已回写到执行记录
	runs, _ := st.ListAlertBindingRuns("AL-1")
	require.Len(t, runs, 2)
	require.NotEmpty(t, runs[0].RefID, "执行记录应回写 playbook_run ID")
	require.NotEmpty(t, runs[1].RefID)

	runner.release("PBR-2")
	require.Eventually(t, func() bool {
		runs, _ := st.ListAlertBindingRuns("AL-1")
		return len(runs) == 2 && runs[0].Status == "success" && runs[1].Status == "success"
	}, 6*time.Second, 20*time.Millisecond, "全部完成后执行记录应为 success")
}

// TestService_RunAlertBindings_Concurrent 验证并发模式：全部同时启动。
func TestService_RunAlertBindings_Concurrent(t *testing.T) {
	svc, _ := newBindingTestService(t)
	runner := newFakeRunner()
	svc.SetPlaybookRunner(runner)

	_, err := svc.RunAlertBindings("AL-1", []owlmonitor.AlertBinding{
		mkBinding("AB-1", "pb-a", 1, false, ""),
		mkBinding("AB-2", "pb-b", 2, false, ""),
	}, "concurrent", "n1", "admin")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return len(runner.startedSnapshot()) == 2
	}, time.Second, 5*time.Millisecond, "并发模式应同时启动全部")
}

// TestService_HandleAlertOpened_AutoExec 验证告警打开钩子只执行 auto_exec 绑定。
func TestService_HandleAlertOpened_AutoExec(t *testing.T) {
	svc, st := newBindingTestService(t)
	runner := newFakeRunner()
	svc.SetPlaybookRunner(runner)
	now := time.Now().Unix()
	require.NoError(t, st.CreateAlertBinding(mkBinding("AB-A", "pb-auto", 1, true, "sequential")))
	require.NoError(t, st.CreateAlertBinding(mkBinding("AB-M", "pb-manual", 2, false, "sequential")))

	// 非本告警的绑定不应执行
	require.NoError(t, st.CreateAlertBinding(owlmonitor.AlertBinding{ID: "AB-X", AlertID: "AL-OTHER",
		Kind: "playbook", Name: "x", Content: "pb-x", Seq: 1, CreatedAt: now}))

	svc.handleAlertOpened(owlmonitor.AlertEvent{Type: owlmonitor.EventOpened,
		Alert: &owlmonitor.Alert{ID: "AL-1", NodeID: "n1"}}, owlmonitor.Target{ID: "n1"})

	require.Eventually(t, func() bool {
		return len(runner.startedSnapshot()) == 1
	}, time.Second, 5*time.Millisecond, "只执行 auto_exec=1 的绑定")
	require.Equal(t, []string{"pb-auto|n1"}, runner.startedSnapshot())
	runner.release("PBR-1")
}
