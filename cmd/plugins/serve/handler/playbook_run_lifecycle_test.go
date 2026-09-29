package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/internal/control/blacklist"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// S13：取消与终态写入存在竞态（Get→UpdateStatus 非原子），已完成的
// run 可能被改写为 cancelled。Store 提供 CancelIfActive 单条条件
// UPDATE 消除竞态；handler 基于其返回值响应。

func TestPlaybookRunStore_CancelIfActive(t *testing.T) {
	_, rs, _ := newPlaybookEngineTestHandler(t, "name: demo\ntasks: []\n")

	run, err := rs.Create(t.Context(), "pb-1", "demo", "/nonexistent.yaml", []string{"n1"}, nil, "", false)
	require.NoError(t, err)

	// 活动 run：取消成功
	ok, err := rs.CancelIfActive(t.Context(), run.ID, "cancelled by user")
	require.NoError(t, err)
	assert.True(t, ok, "运行中的 run 应可取消")
	got, err := rs.Get(t.Context(), run.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RunStatusCancelled, got.Status)

	// 终态 run：不再改写
	run2, err := rs.Create(t.Context(), "pb-2", "demo", "/nonexistent.yaml", []string{"n1"}, nil, "", false)
	require.NoError(t, err)
	require.NoError(t, rs.UpdateStatus(t.Context(), run2.ID, model.RunStatusCompleted, ""))

	ok, err = rs.CancelIfActive(t.Context(), run2.ID, "cancelled by user")
	require.NoError(t, err)
	assert.False(t, ok, "已终态的 run 不得被取消改写")
	got, err = rs.Get(t.Context(), run2.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RunStatusCompleted, got.Status, "终态必须保持原样")
}

func TestRunCancel_FinishedRunRejected(t *testing.T) {
	h, rs, _ := newPlaybookEngineTestHandler(t, "name: demo\ntasks: []\n")
	run, err := rs.Create(t.Context(), "pb-1", "demo", "/nonexistent.yaml", []string{"n1"}, nil, "", false)
	require.NoError(t, err)
	require.NoError(t, rs.UpdateStatus(t.Context(), run.ID, model.RunStatusFailed, "boom"))

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/api/v1/playbook/runs/"+run.ID, nil)
	c.Params = gin.Params{{Key: "id", Value: run.ID}}
	h.RunCancel(c)

	require.Equal(t, http.StatusBadRequest, w.Code, "已终态 run 取消应拒绝")
	got, err := rs.Get(t.Context(), run.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RunStatusFailed, got.Status)
}

// S12：告警绑定触发此前无条件 danger_confirmed=true（operator 编写的
// 剧本 + admin 配置的绑定 + 无黑名单执行）。现经设置键
// monitor.playbook_force_danger_confirmed 控制（默认 true 保持兼容），
// 关闭后命中黑名单的剧本将被拦截。
func TestRunForAlert_ForceConfigurable(t *testing.T) {
	const pbYAML = `
name: evil-alert
execution_mode: fail_continue
tasks:
  - name: cleanup
    action: shell
    args:
      cmd: rm -rf /var/data
`
	h, rs, pbFile := newPlaybookEngineTestHandler(t, pbYAML)
	// :memory: 库的连接池每个连接是独立库，本测试多次读写 settings
	// 会跨连接，必须串行化到单连接
	h.db.SetMaxOpenConns(1)
	// 夹具不含 settings 表（生产由 initSettings 创建），upsertSetting
	// 依赖它
	_, err := h.db.Exec(`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT)`)
	require.NoError(t, err)
	h.sshRunner = &fakeSSHRunner{user: "root"}
	h.checker = blacklist.NewDefaultChecker()
	require.NoError(t, h.playbooks.Upsert(t.Context(), &model.Playbook{
		ID: "pb-file-id", Name: "evil-alert", FilePath: pbFile, FileExists: true,
	}))

	// 默认（未设置）：保持兼容，强制放行
	runID, err := h.RunForAlert(t.Context(), "pb-file-id", "n1", "admin")
	require.NoError(t, err)
	run, err := rs.Get(t.Context(), runID)
	require.NoError(t, err)
	assert.True(t, run.DangerConfirmed, "默认应强制放行（历史行为）")

	// 关闭强制放行：危险剧本被黑名单拦截
	upsertSetting(h.db, "monitor.playbook_force_danger_confirmed", "false")
	runID, err = h.RunForAlert(t.Context(), "pb-file-id", "n1", "admin")
	require.NoError(t, err)
	run, err = rs.Get(t.Context(), runID)
	require.NoError(t, err)
	assert.False(t, run.DangerConfirmed, "关闭后不得强制放行")

	h.executePlaybookRunV2(runID)
	run, err = rs.Get(t.Context(), runID)
	require.NoError(t, err)
	assert.Equal(t, model.RunStatusFailed, run.Status, "关闭强制放行后危险剧本应被拦截")

	banned := false
	for _, s := range run.Results {
		if s.TaskName == "cleanup" && s.Status == "failed" && strings.Contains(s.Error, "黑名单") {
			banned = true
		}
	}
	assert.True(t, banned, "危险步骤应以黑名单拦截失败收场，实际: %+v", run.Results)
}

// S15：黑名单检查前 getNodeInfo 失败（节点行缺失/凭据解密失败）时
// user 被静默置空，root 规则组全部跳过——root 危险命令被放行且无任何
// 提示。改为 fail-closed：解析失败即拦截，错误可见、不发起执行。
func TestExecuteOnNode_NodeResolveFailureFailsClosed(t *testing.T) {
	ssh := &fakeSSHRunner{nodeErr: errors.New("node row gone")}
	exec := &webCommandExecutor{ssh: ssh, check: blacklist.NewDefaultChecker()}

	result, err := exec.ExecuteOnNode("n1", "uptime", time.Second)

	require.Error(t, err, "节点解析失败必须 fail-closed，不得以空用户降级检查")
	require.NotNil(t, result)
	assert.Equal(t, -1, result.ExitCode)
	assert.Empty(t, ssh.executedCommands(), "fail-closed 后不得发起任何执行")
}

// R7：黑名单用户解析按 run 缓存——多节点多步骤时此前每步都查库并
// 解密凭据。缓存后同一节点只解析一次，结果一致。
func TestWebCommandExecutor_NodeUserCached(t *testing.T) {
	ssh := &fakeSSHRunner{user: "root"}
	exec := &webCommandExecutor{ssh: ssh, check: blacklist.NewDefaultChecker()}

	for i := 0; i < 3; i++ {
		u, err := exec.nodeUser("n1")
		require.NoError(t, err)
		assert.Equal(t, "root", u)
	}
}

// R9：run.Warnings 此前只写在内存对象上，store 无对应列——刷新后
// RunGet 警告消失。预检警告必须落库并随查询返回。
func TestRun_PersistsPreflightWarnings(t *testing.T) {
	const pbYAML = `
name: warned
execution_mode: fail_continue
tasks:
  - name: cleanup
    action: shell
    args:
      cmd: rm -rf /tmp/data
`
	h, rs, pbFile := newPlaybookEngineTestHandler(t, pbYAML)
	h.db.SetMaxOpenConns(1)
	h.checker = blacklist.NewDefaultChecker()
	require.NoError(t, h.playbooks.Upsert(t.Context(), &model.Playbook{
		ID: "pb-file-id", Name: "warned", FilePath: pbFile, FileExists: true,
	}))

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/v1/playbooks/pb-file-id/run", strings.NewReader(`{"target_nodes":["n1"],"danger_confirmed":true}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "pb-file-id"}}
	h.Run(c)
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())

	var run model.PlaybookRun
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &run))

	got, err := rs.Get(t.Context(), run.ID)
	require.NoError(t, err)
	found := false
	for _, warn := range got.Warnings {
		if strings.Contains(warn, "黑名单") {
			found = true
		}
	}
	assert.True(t, found, "落库的 Warnings 应包含预检黑名单警告，实际: %v", got.Warnings)
}
