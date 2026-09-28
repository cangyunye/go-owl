package handler

// 剧本 V2 引擎可见性行为的回归测试。
// V1→V2 改版时逐步推送、失败步骤记录、run.error 等行为被悄悄丢失且无测试报警，
// 本文件通过 fake hub / fake SSH 锁死这些行为。

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/cangyunye/go-owl/internal/control/blacklist"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gossh "golang.org/x/crypto/ssh"
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
	mu        sync.Mutex
	user      string
	results   map[string]fakeExecResult
	calls     []string
	onExecute func(command string)     // 每次执行时回调（测试同步用）
	hangOn    map[string]chan struct{} // cmd → 释放门；阻塞至放行或 ctx 取消
}

func (f *fakeSSHRunner) Execute(ctx context.Context, nodeID, command string) (string, int, error) {
	// 入口检查 ctx：模拟真实链路在取消后连接即失败
	if err := ctx.Err(); err != nil {
		return "", -1, err
	}
	f.mu.Lock()
	f.calls = append(f.calls, command)
	onExec := f.onExecute
	r, ok := f.results[command]
	f.mu.Unlock()
	if release, ok := f.hangOn[command]; ok {
		select {
		case <-release:
		case <-ctx.Done():
			return "", -1, ctx.Err()
		}
	}
	if onExec != nil {
		onExec(command)
	}
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

// 逐步推送是本文件存在的理由：V1 引擎每执行一步就广播一次，
// V2 改版把它悄悄弄丢，退化成"终态一次性全量"，运行中什么都看不到。
// 本测试锁死：每步执行完必须立即广播 playbook_run_update 且结果渐进增长。
func TestExecutePlaybookRunV2_BroadcastsPerStep(t *testing.T) {
	const pbYAML = `
name: per-step-broadcast
tasks:
  - name: step1
    action: shell
    args:
      cmd: echo one
  - name: step2
    action: shell
    args:
      cmd: echo two
  - name: step3
    action: shell
    args:
      cmd: echo three
`
	h, rs, pbFile := newPlaybookEngineTestHandler(t, pbYAML)
	hub := &fakeHub{}
	h.hub = hub

	step1Done := make(chan struct{})
	release := make(chan struct{})
	ssh := &fakeSSHRunner{user: "root", onExecute: func(command string) {
		if command == "echo one" {
			close(step1Done)
		}
		if command == "echo three" {
			<-release // 卡住第三步：让断言有机会在"运行中"检查推送
		}
	}}
	h.sshRunner = ssh

	run, err := rs.Create(t.Context(), "pb-1", "per-step-broadcast", pbFile, []string{"n1"}, nil, "", false)
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		h.executePlaybookRunV2(run.ID)
		close(done)
	}()

	select {
	case <-step1Done:
	case <-time.After(5 * time.Second):
		t.Fatal("第一步长时间未执行")
	}
	// 等 step2 也执行完，此时 run 卡在 step3、终态未到
	require.Eventually(t, func() bool {
		for _, c := range ssh.executedCommands() {
			if c == "echo two" {
				return true
			}
		}
		return false
	}, 3*time.Second, 10*time.Millisecond)

	// 关键断言：run 仍在进行中（step3 未放行、终态广播未发），
	// step1/step2 的结果必须已经推送——逐步推送，不是终态一次性全量。
	sawPartialResults := false
	for _, m := range hub.runUpdates() {
		if r, ok := m.Data.(*model.PlaybookRun); ok && r.ID == run.ID && len(r.Results) >= 2 {
			sawPartialResults = true
			break
		}
	}
	assert.True(t, sawPartialResults, "运行中必须逐步广播已完成步骤的结果（V1→V2 曾退化为终态一次性全量）")

	close(release)
	<-done

	// 终态时三步全部推送过
	finalUpdates := hub.runUpdates()
	var lastRun *model.PlaybookRun
	for _, m := range finalUpdates {
		if r, ok := m.Data.(*model.PlaybookRun); ok && r.ID == run.ID {
			lastRun = r
		}
	}
	require.NotNil(t, lastRun)
	assert.Len(t, lastRun.Results, 3, "结束时全部步骤结果必须可见")
}

// 起 exec 后永久挂死的进程内 SSH server（不复用 internal/ssh 的 helper——
// 那些是包内私有），模拟远端命令阻塞（交互输入等）。
func startHangingSSHServer(t *testing.T) string {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer, err := gossh.NewSignerFromSigner(priv)
	require.NoError(t, err)

	cfg := &gossh.ServerConfig{PasswordCallback: func(conn gossh.ConnMetadata, password []byte) (*gossh.Permissions, error) {
		return &gossh.Permissions{}, nil
	}}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				sconn, chans, reqs, err := gossh.NewServerConn(c, cfg)
				if err != nil {
					c.Close()
					return
				}
				defer sconn.Close()
				go gossh.DiscardRequests(reqs)
				for newChan := range chans {
					if newChan.ChannelType() != "session" {
						newChan.Reject(gossh.UnknownChannelType, "unsupported")
						continue
					}
					_, chReqs, err := newChan.Accept()
					if err != nil {
						continue
					}
					go func() {
						for req := range chReqs {
							if req.Type == "exec" {
								req.Reply(true, nil)
								// 收到 exec 后什么都不做：远端命令挂死
								return
							}
							req.Reply(false, nil)
						}
					}()
				}
			}(conn)
		}
	}()
	return ln.Addr().String()
}

// Web 链路的命令超时必须真实生效：CombinedOutput 不响应 ctx，
// 曾导致交互式命令把 run 永久卡在 running。回归测试锁死：
// ctx 超时后 Execute 必须返回（watchdog 断连）。
func TestSSHExecutor_Execute_ContextTimeoutTerminates(t *testing.T) {
	addr := startHangingSSHServer(t)
	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`CREATE TABLE nodes (
		id TEXT PRIMARY KEY, name TEXT, address TEXT, port INTEGER DEFAULT 22,
		user TEXT, password TEXT, ssh_key TEXT, status TEXT DEFAULT 'unknown',
		groups TEXT DEFAULT '[]', labels TEXT DEFAULT '{}',
		proxy_jump TEXT DEFAULT '', created_at TIMESTAMP, updated_at TIMESTAMP)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO nodes (id, name, address, port, user, password, status, groups, labels)
		VALUES ('n1', 'n1', ?, ?, 'root', 'pw', 'online', '[]', '{}')`, host, port)
	require.NoError(t, err)

	e := &sshExecutor{db: db}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	type execOutcome struct {
		out  string
		code int
		err  error
	}
	ch := make(chan execOutcome, 1)
	start := time.Now()
	go func() {
		out, code, execErr := e.Execute(ctx, "n1", "hang-forever")
		ch <- execOutcome{out, code, execErr}
	}()

	select {
	case r := <-ch:
		require.Error(t, r.err, "ctx 超时后必须返回错误")
		assert.Less(t, time.Since(start), 5*time.Second, "超时必须在秒级返回")
	case <-time.After(5 * time.Second):
		t.Fatal("ctx 超时后 Execute 仍未返回：超时形同虚设，命令挂死会永久卡住 run")
	}
}

// 运行中取消必须能终止执行（包括卡在交互命令上的步骤），
// 终态必须是 cancelled 而不是永远 running 或误标 failed。
func TestExecutePlaybookRunV2_CancelTerminates(t *testing.T) {
	const pbYAML = `
name: cancel-terminates
tasks:
  - name: step1
    action: shell
    args:
      cmd: echo one
  - name: step2
    action: shell
    args:
      cmd: echo two
  - name: step3
    action: shell
    args:
      cmd: echo three
`
	h, rs, pbFile := newPlaybookEngineTestHandler(t, pbYAML)
	hub := &fakeHub{}
	h.hub = hub

	release := make(chan struct{})
	ssh := &fakeSSHRunner{user: "root", hangOn: map[string]chan struct{}{
		"echo two": release, // step2 模拟卡死，且不放行——只能靠取消终止
	}}
	h.sshRunner = ssh

	run, err := rs.Create(t.Context(), "pb-1", "cancel-terminates", pbFile, []string{"n1"}, nil, "", false)
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		h.executePlaybookRunV2(run.ID)
		close(done)
	}()

	// 等 step2 真正卡住
	require.Eventually(t, func() bool {
		return len(ssh.executedCommands()) >= 2
	}, 3*time.Second, 10*time.Millisecond)

	// 用户取消：不释放 step2，运行必须因此终止
	h.cancelRun(run.ID)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("取消后运行未终止：卡住的命令无法被中断，run 会永远停在 running")
	}

	got, err := rs.Get(t.Context(), run.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RunStatusCancelled, got.Status, "取消后终态必须是 cancelled")
}

// 预检必须指出剧本里哪些步骤会命中危险命令黑名单——
// 否则用户要到运行失败且端上无任何提示时才知道被拦了。
func TestPreflightPlaybook_BlacklistWarning(t *testing.T) {
	const dangerYAML = `
name: danger-preflight
tasks:
  - name: cleanup
    action: shell
    args:
      cmd: rm -rf /tmp/data
  - name: safe_step
    action: shell
    args:
      cmd: echo ok
`
	h, _, pbFile := newPlaybookEngineTestHandler(t, dangerYAML)
	// 固定用默认黑名单规则，避免本机 ~/.owl/blacklist.yaml 影响断言
	h.checker = blacklist.NewChecker(&blacklist.Config{Rules: blacklist.DefaultRules()})

	warnings := h.preflightPlaybook(pbFile)
	require.NotEmpty(t, warnings, "危险命令必须产生预检警告")

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "cleanup") && strings.Contains(w, "rm -rf") {
			found = true
		}
	}
	assert.True(t, found, "警告必须指出步骤名与命中的命令，实际警告: %v", warnings)

	const safeYAML = `
name: safe-preflight
tasks:
  - name: safe_step
    action: shell
    args:
      cmd: echo ok
`
	h2, _, safeFile := newPlaybookEngineTestHandler(t, safeYAML)
	h2.checker = h.checker
	assert.Empty(t, h2.preflightPlaybook(safeFile), "安全剧本不应产生黑名单警告")
}

// script 动作同样要预检：inline 的内容即脚本本身，文件脚本读内容检查，
// 否则用户要到运行被拦才知道（预检的存在意义）。
func TestPreflightPlaybook_ScriptActionWarning(t *testing.T) {
	const dangerYAML = `
name: danger-script-preflight
tasks:
  - name: evil_inline
    action: script
    args:
      script: "echo start && rm -rf /data"
      inline: true
`
	h, _, pbFile := newPlaybookEngineTestHandler(t, dangerYAML)
	h.checker = blacklist.NewChecker(&blacklist.Config{Rules: blacklist.DefaultRules()})

	warnings := h.preflightPlaybook(pbFile)
	require.NotEmpty(t, warnings, "危险 inline 脚本必须产生预检警告")

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "evil_inline") && strings.Contains(w, "rm -rf") {
			found = true
		}
	}
	assert.True(t, found, "警告必须指出步骤名与命中的内容，实际警告: %v", warnings)
}
