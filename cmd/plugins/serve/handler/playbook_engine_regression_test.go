package handler

// 剧本 V2 引擎可见性行为的回归测试。
// V1→V2 改版时逐步推送、失败步骤记录、run.error 等行为被悄悄丢失且无测试报警，
// 本文件通过 fake hub / fake SSH 锁死这些行为。

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeHub 捕获所有广播，供断言推送时机与内容。
type fakeHub struct {
	mu   sync.Mutex
	msgs []WSMessage
}

func (f *fakeHub) Broadcast(msg WSMessage) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.msgs = append(f.msgs, msg)
}

func (f *fakeHub) BroadcastHistoryUpdate() {}

func (f *fakeHub) runUpdates() []WSMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []WSMessage
	for _, m := range f.msgs {
		if m.Type == "playbook_run_update" {
			out = append(out, m)
		}
	}
	return out
}

var _ playbookHub = (*fakeHub)(nil)

type fakeExecResult struct {
	output   string
	exitCode int
	err      error
}

// fakeSSHRunner 按 cmd→结果表返回，记录每次执行调用。
type fakeSSHRunner struct {
	mu      sync.Mutex
	user    string
	results map[string]fakeExecResult
	calls   []string
}

func (f *fakeSSHRunner) Execute(ctx context.Context, nodeID, command string) (string, int, error) {
	f.mu.Lock()
	f.calls = append(f.calls, command)
	r, ok := f.results[command]
	f.mu.Unlock()
	if !ok {
		r = fakeExecResult{output: "ok"}
	}
	return r.output, r.exitCode, r.err
}

func (f *fakeSSHRunner) getNodeInfo(nodeID string) (*nodeSSHInfo, error) {
	return &nodeSSHInfo{User: f.user}, nil
}

func (f *fakeSSHRunner) executedCommands() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

var _ playbookSSHRunner = (*fakeSSHRunner)(nil)

// 建立带 nodes / playbook_runs / history 表的内存库与 handler。
func newPlaybookEngineTestHandler(t *testing.T, yamlContent string) (*PlaybookHandler, *store.PlaybookRunStore, string) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY, name TEXT, address TEXT, port INTEGER DEFAULT 22,
		user TEXT, password TEXT, ssh_key TEXT, status TEXT DEFAULT 'unknown',
		groups TEXT DEFAULT '[]', labels TEXT DEFAULT '{}',
		proxy_jump TEXT DEFAULT '', created_at TIMESTAMP, updated_at TIMESTAMP)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO nodes (id, name, address, port, user, status, groups, labels) VALUES ('n1', 'node1', '127.0.0.1', 22, 'root', 'online', '[]', '{}')`)
	require.NoError(t, err)

	ps := store.NewPlaybookStore(db)
	require.NoError(t, ps.Init(t.Context()))
	rs := store.NewPlaybookRunStore(db)
	require.NoError(t, rs.Init(t.Context()))
	ns := store.NewNodeStore(db)
	hs := store.NewHistoryStore(db)
	require.NoError(t, hs.Init(t.Context()))

	pbFile := filepath.Join(t.TempDir(), "regression.yaml")
	require.NoError(t, os.WriteFile(pbFile, []byte(yamlContent), 0644))

	h := NewPlaybookHandler(db, ps, rs, ns, nil)
	h.History = hs
	return h, rs, pbFile
}

// 非零退出码必须以 error 的形式暴露给执行器：pipeline 的快停语义
// 完全依赖 err 非 nil。曾因返回 err=nil 导致 Web 端 pipeline 对
// 退出码失败"假装没失败"，与 CLI 行为分裂。
func TestExecuteOnNode_NonZeroExit_IsError(t *testing.T) {
	e := &webCommandExecutor{ssh: &fakeSSHRunner{results: map[string]fakeExecResult{
		"exit 1": {output: "boom", exitCode: 1},
	}}}

	result, err := e.ExecuteOnNode("n1", "exit 1", time.Second)

	require.Error(t, err, "非零退出码必须返回 error")
	require.NotNil(t, result, "即使出错也必须返回结果供记录")
	assert.NotNil(t, result.Error, "结果必须携带错误")
	assert.Equal(t, 1, result.ExitCode, "退出码必须保留供展示")

	result, err = e.ExecuteOnNode("n1", "echo ok", time.Second)
	require.NoError(t, err, "零退出码不是错误")
	assert.Nil(t, result.Error)
}

// pipeline 模式下失败步骤必须出现在 run.Results（状态 failed + 原因），
// 失败后后续步骤不得执行，run.Error 必须非空——"静默失败"是本次修复
// 要杜绝的核心症状。
func TestExecutePlaybookRunV2_FailedStepVisible(t *testing.T) {
	const pbYAML = `
name: failed-step-visible
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
	h, rs, pbFile := newPlaybookEngineTestHandler(t, pbYAML)
	ssh := &fakeSSHRunner{user: "root", results: map[string]fakeExecResult{
		"exit 1": {output: "boom", exitCode: 1},
	}}
	h.sshRunner = ssh

	run, err := rs.Create(t.Context(), "pb-1", "failed-step-visible", pbFile, []string{"n1"}, nil, "", false)
	require.NoError(t, err)

	h.executePlaybookRunV2(run.ID)

	run, err = rs.Get(t.Context(), run.ID)
	require.NoError(t, err)

	require.Equal(t, model.RunStatusFailed, run.Status, "有失败步骤时 run 必须是 failed")
	assert.NotEmpty(t, run.Error, "run 失败必须携带错误信息，不允许静默失败")

	byTask := map[string][]*model.StepResult{}
	for _, s := range run.Results {
		byTask[s.TaskName] = append(byTask[s.TaskName], s)
	}
	require.Len(t, byTask["step1_ok"], 1, "失败前的步骤必须有记录")
	assert.Equal(t, "completed", byTask["step1_ok"][0].Status)

	require.Len(t, byTask["step2_fail"], 1, "失败步骤自身的记录必须保留")
	assert.Equal(t, "failed", byTask["step2_fail"][0].Status, "失败步骤状态必须是 failed")
	assert.NotEmpty(t, byTask["step2_fail"][0].Error, "失败步骤必须携带失败原因")

	assert.Empty(t, byTask["step3_never"], "pipeline 模式失败后后续步骤不应执行")
}
