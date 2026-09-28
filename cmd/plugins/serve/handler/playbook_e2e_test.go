package handler

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/cangyunye/go-owl/internal/control/blacklist"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gossh "golang.org/x/crypto/ssh"
)

// 自动化 E2E：v1.9.1 发布说明的「五场景手动全 PASS」此前不可复跑
// （tests/e2e/test_sshd 无 runner）。这里用进程内真实执行 SSH server
// （任意密码登录，exec 经 /bin/sh -c 真实执行，回传合并输出与退出码）
// 走真实 sshExecutor → V2 引擎全链路，把五个场景固化为可回归测试。

// startRealExecSSHServer 起 exec 真实执行的最小 SSH server；
// 返回监听地址与执行计数（断言「发起/未发起 SSH」用）。
func startRealExecSSHServer(t *testing.T) (string, *atomic.Int64) {
	t.Helper()
	execCount := &atomic.Int64{}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
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
					ch, chReqs, err := newChan.Accept()
					if err != nil {
						continue
					}
					go handleE2ESession(ch, chReqs, execCount)
				}
			}(conn)
		}
	}()
	return ln.Addr().String(), execCount
}

func handleE2ESession(ch gossh.Channel, chReqs <-chan *gossh.Request, execCount *atomic.Int64) {
	defer ch.Close()
	for req := range chReqs {
		if req.Type != "exec" {
			req.Reply(false, nil)
			continue
		}
		if len(req.Payload) < 4 {
			req.Reply(false, nil)
			return
		}
		n := int(req.Payload[0])<<24 | int(req.Payload[1])<<16 | int(req.Payload[2])<<8 | int(req.Payload[3])
		if n > len(req.Payload)-4 {
			n = len(req.Payload) - 4
		}
		command := string(req.Payload[4 : 4+n])
		req.Reply(true, nil)
		execCount.Add(1)

		cmd := exec.Command("/bin/sh", "-c", command)
		output, cmdErr := cmd.CombinedOutput()
		if len(output) > 0 {
			_, _ = ch.Write(output)
		}
		exitCode := 0
		if cmdErr != nil {
			if ee, ok := cmdErr.(*exec.ExitError); ok {
				exitCode = ee.ExitCode()
			} else {
				exitCode = 127
			}
		}
		_, _ = ch.SendRequest("exit-status", false, gossh.Marshal(struct{ Code uint32 }{uint32(exitCode)}))
		return
	}
}

// newE2EHandler 建立带真实节点行（指向进程内 SSH server）的 handler；
// sshRunner 留空 → V2 走真实 sshExecutor 全链路。
func newE2EHandler(t *testing.T, addr, yaml string) (*PlaybookHandler, *store.PlaybookRunStore, string) {
	t.Helper()
	h, rs, pbFile := newPlaybookEngineTestHandler(t, yaml)
	// :memory: 连接池每连接独立成库，跨 goroutine（V2 执行 + 测试断言）
	// 读写必须单连接串行，否则偶发 no such table / 读不到写入
	h.db.SetMaxOpenConns(1)

	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	_, err = h.db.Exec(`INSERT INTO nodes (id, name, address, port, user, password, status, groups, labels) VALUES
		('e2e-node', 'e2e-node', ?, ?, 'root', 'any', 'online', '[]', '{}')`, host, port)
	require.NoError(t, err)

	require.NoError(t, h.playbooks.Upsert(t.Context(), &model.Playbook{
		ID: "pb-e2e", Name: "e2e", FilePath: pbFile, FileExists: true,
	}))
	h.checker = blacklist.NewChecker(&blacklist.Config{Rules: blacklist.DefaultRules()})
	return h, rs, pbFile
}

// ginCancel 经 HTTP 入口取消运行（与前端同一链路）。
func ginCancel(t *testing.T, h *PlaybookHandler, runID string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/api/v1/playbook/runs/"+runID, nil)
	c.Params = gin.Params{{Key: "id", Value: runID}}
	h.RunCancel(c)
	if w.Code != http.StatusOK {
		t.Fatalf("取消应成功，HTTP %d: %s", w.Code, w.Body.String())
	}
}

func waitForStatus(t *testing.T, h *PlaybookHandler, runID string, statuses ...model.PlaybookRunStatus) *model.PlaybookRun {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		run, err := h.runs.Get(t.Context(), runID)
		if err == nil {
			for _, s := range statuses {
				if run.Status == s {
					return run
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("20s 内未到达状态 %v", statuses)
	return nil
}

// 场景 1：命令成功 → completed、退出码 0、输出可见。
func TestE2E_CommandSuccess(t *testing.T) {
	addr, execCount := startRealExecSSHServer(t)
	const pbYAML = `
name: e2e-ok
execution_mode: fail_continue
tasks:
  - name: hello
    action: shell
    args:
      cmd: echo E2E_OK_42
`
	h, rs, pbFile := newE2EHandler(t, addr, pbYAML)

	run, err := rs.Create(t.Context(), "pb-e2e", "e2e", pbFile, []string{"e2e-node"}, nil, "", false)
	require.NoError(t, err)

	h.executePlaybookRunV2(run.ID)
	got := waitForStatus(t, h, run.ID, model.RunStatusCompleted)

	assert.Equal(t, int64(1), execCount.Load(), "应恰好发起一次 SSH 执行")
	require.Len(t, got.Results, 1)
	assert.Equal(t, "completed", got.Results[0].Status)
	assert.Equal(t, 0, got.Results[0].ExitCode)
	assert.Contains(t, got.Results[0].Output, "E2E_OK_42", "远端真实输出应回传")
}

// 场景 2：非零退出码必须透传为失败（946542b 的 E2E 等价物）。
func TestE2E_NonZeroExitPreserved(t *testing.T) {
	addr, _ := startRealExecSSHServer(t)
	const pbYAML = `
name: e2e-fail
execution_mode: fail_continue
tasks:
  - name: fail_step
    action: shell
    args:
      cmd: echo before-fail; exit 3
  - name: after
    action: shell
    args:
      cmd: echo after
`
	h, rs, pbFile := newE2EHandler(t, addr, pbYAML)

	run, err := rs.Create(t.Context(), "pb-e2e", "e2e", pbFile, []string{"e2e-node"}, nil, "", false)
	require.NoError(t, err)

	h.executePlaybookRunV2(run.ID)
	got := waitForStatus(t, h, run.ID, model.RunStatusFailed)

	require.NotEmpty(t, got.Results)
	var failStep *model.StepResult
	for _, s := range got.Results {
		if s.TaskName == "fail_step" {
			failStep = s
		}
	}
	require.NotNil(t, failStep, "失败步骤必须保留在结果里")
	assert.Equal(t, "failed", failStep.Status)
	assert.Equal(t, 3, failStep.ExitCode, "底层退出码必须原样保留")
	assert.NotEmpty(t, got.Error, "run 失败必须携带原因")
}

// 场景 3：危险命令被黑名单拦截（root 视角），不发起任何 SSH。
func TestE2E_BlacklistBlocked(t *testing.T) {
	addr, execCount := startRealExecSSHServer(t)
	const pbYAML = `
name: e2e-evil
execution_mode: fail_continue
tasks:
  - name: cleanup
    action: shell
    args:
      cmd: rm -rf /tmp/e2e-data
`
	h, rs, pbFile := newE2EHandler(t, addr, pbYAML)

	run, err := rs.Create(t.Context(), "pb-e2e", "e2e", pbFile, []string{"e2e-node"}, nil, "", false)
	require.NoError(t, err)

	h.executePlaybookRunV2(run.ID)
	got := waitForStatus(t, h, run.ID, model.RunStatusFailed)

	assert.Equal(t, int64(0), execCount.Load(), "被拦截的命令不得发起 SSH")
	require.NotEmpty(t, got.Results)
	banned := false
	for _, s := range got.Results {
		if s.Status == "failed" && strings.Contains(s.Error, "黑名单") {
			banned = true
		}
	}
	assert.True(t, banned, "失败原因必须是黑名单拦截，实际: %+v", got.Results)
}

// 场景 4：任务级超时真实生效（真实 SSH 挂死 server，命令超时后断连返回）。
func TestE2E_TaskTimeoutTerminates(t *testing.T) {
	hangAddr := startHangingSSHServer(t)
	const pbYAML = `
name: e2e-timeout
execution_mode: fail_continue
tasks:
  - name: hang
    action: shell
    args:
      cmd: sleep 1000
    timeout:
      command: 500ms
`
	h, rs, pbFile := newE2EHandler(t, hangAddr, pbYAML)

	run, err := rs.Create(t.Context(), "pb-e2e", "e2e", pbFile, []string{"e2e-node"}, nil, "", false)
	require.NoError(t, err)

	start := time.Now()
	h.executePlaybookRunV2(run.ID)
	elapsed := time.Since(start)

	got := waitForStatus(t, h, run.ID, model.RunStatusFailed)
	assert.Less(t, elapsed, 15*time.Second, "超时必须真实生效，不得挂满 1000s")
	assert.Equal(t, model.RunStatusFailed, got.Status)
}

// 场景 5：运行中取消真实终止（真实 SSH 挂死 + HTTP 取消入口）。
func TestE2E_CancelTerminates(t *testing.T) {
	hangAddr := startHangingSSHServer(t)
	const pbYAML = `
name: e2e-cancel
execution_mode: fail_continue
tasks:
  - name: hang
    action: shell
    args:
      cmd: sleep 1000
`
	h, rs, pbFile := newE2EHandler(t, hangAddr, pbYAML)

	run, err := rs.Create(t.Context(), "pb-e2e", "e2e", pbFile, []string{"e2e-node"}, nil, "", false)
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.executePlaybookRunV2(run.ID)
	}()

	// 等进入 running 后经 HTTP 入口取消
	waitForStatus(t, h, run.ID, model.RunStatusRunning)
	ginCancel(t, h, run.ID)

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("取消后引擎 15s 内未返回，取消链路失效")
	}
	got := waitForStatus(t, h, run.ID, model.RunStatusCancelled)
	assert.Equal(t, model.RunStatusCancelled, got.Status)
}
