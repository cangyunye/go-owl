package playbook

// 剧本执行核心可见性行为的回归测试。
// 背景：V1→V2 改版时逐步推送、失败步骤记录等行为被悄悄丢失且无测试报警，
// 本文件锁死这些行为，改版破坏任何一条都会在这里红。

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/internal/common/model"
	"github.com/cangyunye/go-owl/internal/control/command"
	controlnode "github.com/cangyunye/go-owl/internal/control/node"
	"github.com/cangyunye/go-owl/internal/control/task"
	"github.com/cangyunye/go-owl/internal/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCommandExecutor 按 cmd→退出码表返回；非零退出码同时返回 error，
// 模拟 CLI 链路 command.Executor 的语义（连接失败/被拦截等真实错误）。
type fakeCommandExecutor struct {
	exitCodes map[string]int
}

func (f *fakeCommandExecutor) Execute(tk *task.Task, nodeMgr controlnode.Manager) error {
	return fmt.Errorf("not supported")
}

func (f *fakeCommandExecutor) ExecuteOnNode(nodeID string, cmd string, timeout time.Duration) (*task.TaskResult, error) {
	now := time.Now()
	exitCode := f.exitCodes[cmd]
	result := &task.TaskResult{
		NodeID:    nodeID,
		ExitCode:  exitCode,
		Output:    fmt.Sprintf("ran: %s", cmd),
		StartTime: now,
		EndTime:   now,
	}
	var err error
	if exitCode != 0 {
		err = fmt.Errorf("exit code %d", exitCode)
		result.Error = err
	}
	return result, err
}

func (f *fakeCommandExecutor) ExecuteOnNodeWithConfig(nodeID string, cmd string, config *ssh.TimeoutConfig) (*task.TaskResult, error) {
	return f.ExecuteOnNode(nodeID, cmd, 0)
}

var _ command.CommandExecutor = (*fakeCommandExecutor)(nil)

// fakeNodeManager 最小节点管理器，节点表固定。
type fakeNodeManager struct {
	nodes []*model.Node
}

func (m *fakeNodeManager) Register(n *model.Node) error                          { return nil }
func (m *fakeNodeManager) Unregister(id string) error                            { return nil }
func (m *fakeNodeManager) UpdateStatus(id string, status model.NodeStatus) error { return nil }
func (m *fakeNodeManager) GetByID(id string) (*model.Node, error) {
	for _, n := range m.nodes {
		if n.ID == id {
			return n, nil
		}
	}
	return nil, fmt.Errorf("node %s not found", id)
}
func (m *fakeNodeManager) List() []*model.Node                                { return m.nodes }
func (m *fakeNodeManager) GetByGroup(group string) []*model.Node              { return nil }
func (m *fakeNodeManager) GetByLabels(labels map[string]string) []*model.Node { return nil }
func (m *fakeNodeManager) GetOnlineNodes() []*model.Node                      { return m.nodes }
func (m *fakeNodeManager) Count() int                                         { return len(m.nodes) }
func (m *fakeNodeManager) SearchByName(pattern string) []*model.Node          { return nil }
func (m *fakeNodeManager) SearchByAddress(pattern string) []*model.Node       { return nil }

var _ controlnode.Manager = (*fakeNodeManager)(nil)

const regressionPipelineYAML = `
name: pipeline-regression
execution_mode: pipeline
tasks:
  - name: step1_ok
    action: shell
    args:
      cmd: echo ok
  - name: step2_fail
    action: shell
    args:
      cmd: exit 1
  - name: step3_never
    action: shell
    args:
      cmd: echo never
`

// pipeline 模式失败即停，但失败步骤自身的执行结果必须保留在 Results 中——
// 否则前端只能看到失败之前的绿色步骤，用户既找不到失败步骤也找不到失败原因。
func TestPipeline_FailureKeepsFailedStepResult(t *testing.T) {
	mgr := &fakeNodeManager{nodes: []*model.Node{{ID: "n1", Name: "n1"}}}
	cmdExec := &fakeCommandExecutor{exitCodes: map[string]int{"exit 1": 1}}
	execer := NewExecutorWithOptions(mgr, cmdExec, nil, nil, &PlaybookOptions{})

	pb, err := NewParser().Parse(regressionPipelineYAML)
	require.NoError(t, err)

	exec, execErr := execer.Execute(pb, mgr.nodes, nil)

	require.Error(t, execErr, "pipeline 模式失败必须返回错误")
	assert.Equal(t, ExecutionStatusFailed, exec.Status)

	// 失败步骤自身的结果必须可见
	failed := exec.GetTaskResult("step2_fail")
	require.Len(t, failed, 1, "pipeline 失败步骤的结果必须保留在 Results 中")
	assert.NotNil(t, failed[0].Error, "失败步骤必须携带错误原因")

	// 失败前的步骤正常记录
	assert.Len(t, exec.GetTaskResult("step1_ok"), 1)

	// 失败后的步骤不得执行
	assert.Empty(t, exec.GetTaskResult("step3_never"), "pipeline 模式失败后后续步骤不应执行")
}

// 进度回调必须每个节点步骤完成即触发（含失败步骤），且未执行的步骤不触发——
// 上层逐步推送完全依赖它。
func TestProgressFunc_CalledPerStepIncludingFailure(t *testing.T) {
	mgr := &fakeNodeManager{nodes: []*model.Node{{ID: "n1", Name: "n1"}}}
	cmdExec := &fakeCommandExecutor{exitCodes: map[string]int{"exit 1": 1}}
	execer := NewExecutorWithOptions(mgr, cmdExec, nil, nil, &PlaybookOptions{})

	var mu sync.Mutex
	var progress []string
	if setter, ok := execer.(interface{ SetProgressFunc(func(*TaskResult)) }); ok {
		setter.SetProgressFunc(func(r *TaskResult) {
			mu.Lock()
			defer mu.Unlock()
			progress = append(progress, r.TaskName)
		})
	} else {
		t.Fatal("executor 必须支持 SetProgressFunc（逐步推送依赖此回调）")
	}

	pb, err := NewParser().Parse(regressionPipelineYAML)
	require.NoError(t, err)

	execer.Execute(pb, mgr.nodes, nil)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"step1_ok", "step2_fail"}, progress,
		"进度回调必须覆盖已执行的每一步（含失败步），未执行的步骤不得出现")
}

// fail_continue 模式必须跑完所有任务，失败步骤与后续步骤都要有记录。
func TestFailContinue_RecordsFailedAndSubsequentSteps(t *testing.T) {
	mgr := &fakeNodeManager{nodes: []*model.Node{{ID: "n1", Name: "n1"}}}
	cmdExec := &fakeCommandExecutor{exitCodes: map[string]int{"exit 1": 1}}
	execer := NewExecutorWithOptions(mgr, cmdExec, nil, nil, &PlaybookOptions{})

	yaml := `
name: fail-continue-regression
execution_mode: fail_continue
tasks:
  - name: step1_ok
    action: shell
    args:
      cmd: echo ok
  - name: step2_fail
    action: shell
    args:
      cmd: exit 1
  - name: step3_still_runs
    action: shell
    args:
      cmd: echo still running
`
	pb, err := NewParser().Parse(yaml)
	require.NoError(t, err)

	exec, execErr := execer.Execute(pb, mgr.nodes, nil)

	assert.NoError(t, execErr, "fail_continue 模式错误不外抛")
	assert.Len(t, exec.GetTaskResult("step2_fail"), 1, "失败步骤必须有记录")
	failed := exec.GetTaskResult("step2_fail")
	assert.NotNil(t, failed[0].Error)
	// 退出码必须保留：FailureCount（run 终态判定）依赖 ExitCode != 0，
	// 丢失会把失败误判成成功（E2E 曾抓到 run completed 的退化）。
	assert.Equal(t, 1, failed[0].ExitCode, "失败步骤的退出码必须从底层结果保留")
	assert.Len(t, exec.GetTaskResult("step3_still_runs"), 1, "失败后后续步骤必须继续执行")
}

// 中止（pipeline 快停 / pre、post 任务失败）时，失败步骤的结果同样必须保留，
// 且 Execute 必须返回非 nil error——handler 靠它写 run.error，丢了错误就"静默失败"。
func TestPreAndPostTaskFailure_KeepsFailedStepResult(t *testing.T) {
	mgr := &fakeNodeManager{nodes: []*model.Node{{ID: "n1", Name: "n1"}}}
	cmdExec := &fakeCommandExecutor{exitCodes: map[string]int{"exit 1": 1}}
	execer := NewExecutorWithOptions(mgr, cmdExec, nil, nil, &PlaybookOptions{})

	preYAML := `
name: pre-fail-regression
execution_mode: pipeline
pre_tasks:
  - name: pre_ok
    action: shell
    args:
      cmd: echo pre
  - name: pre_fail
    action: shell
    args:
      cmd: exit 1
tasks:
  - name: main_never
    action: shell
    args:
      cmd: echo main
`
	pb, err := NewParser().Parse(preYAML)
	require.NoError(t, err)

	exec, execErr := execer.Execute(pb, mgr.nodes, nil)

	require.Error(t, execErr)
	require.Len(t, exec.GetTaskResult("pre_fail"), 1, "pre_tasks 失败步骤的结果必须保留")
	assert.NotNil(t, exec.GetTaskResult("pre_fail")[0].Error)
	assert.Empty(t, exec.GetTaskResult("main_never"), "pre_tasks 失败后主任务不应执行")

	// post_tasks 中止路径：fail_continue 模式下用 any_errors_fatal 触发
	// （pipeline 模式被 parser 禁止携带 post_tasks）。
	postYAML := `
name: post-fail-regression
execution_mode: fail_continue
tasks:
  - name: main_ok
    action: shell
    args:
      cmd: echo main
post_tasks:
  - name: post_fail
    action: shell
    args:
      cmd: exit 1
    any_errors_fatal: true
  - name: post_never
    action: shell
    args:
      cmd: echo post
`
	pb2, err := NewParser().Parse(postYAML)
	require.NoError(t, err)

	exec2, execErr2 := execer.Execute(pb2, mgr.nodes, nil)

	require.Error(t, execErr2)
	require.Len(t, exec2.GetTaskResult("post_fail"), 1, "post_tasks 失败步骤的结果必须保留")
	assert.Empty(t, exec2.GetTaskResult("post_never"), "post_tasks 失败后后续 post 任务不应执行")
}
