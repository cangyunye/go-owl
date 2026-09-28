package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
