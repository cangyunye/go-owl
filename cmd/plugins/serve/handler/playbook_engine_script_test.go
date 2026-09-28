package handler

import (
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/internal/control/blacklist"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// script 动作由 ScriptExecutor 自行拨 SSH，不经过 webCommandExecutor
// 的黑名单检查点：此前剧本里写一段 rm -rf 脚本即可绕过「确认危险命令」
// 交互。运行时脚本内容必须与命令一视同仁地过黑名单。

func TestExecutePlaybookRunV2_ScriptActionBlockedByBlacklist(t *testing.T) {
	const pbYAML = `
name: evil-script
execution_mode: pipeline
tasks:
  - name: cleanup
    action: script
    args:
      script: "echo start\nrm -rf /data"
      inline: true
`
	h, rs, pbFile := newPlaybookEngineTestHandler(t, pbYAML)
	ssh := &fakeSSHRunner{user: "root"}
	h.sshRunner = ssh
	h.checker = blacklist.NewDefaultChecker()

	run, err := rs.Create(t.Context(), "pb-1", "evil-script", pbFile, []string{"n1"}, nil, "", false)
	require.NoError(t, err)

	h.executePlaybookRunV2(run.ID)

	run, err = rs.Get(t.Context(), run.ID)
	require.NoError(t, err)
	require.Equal(t, model.RunStatusFailed, run.Status, "命中黑名单的脚本步骤必须让 run 失败")
	assert.Empty(t, ssh.executedCommands(), "被拦截的脚本不得发起任何 SSH 命令")

	found := false
	for _, s := range run.Results {
		if s.TaskName == "cleanup" && strings.Contains(s.Error, "黑名单") {
			found = true
		}
	}
	assert.True(t, found, "脚本步骤的失败原因必须是黑名单拦截，实际结果: %+v", run.Results)
}

func TestExecutePlaybookRunV2_ScriptActionSafeAllowed(t *testing.T) {
	const pbYAML = `
name: safe-script
execution_mode: fail_continue
tasks:
  - name: probe
    action: script
    args:
      script: "echo probe"
      inline: true
`
	h, rs, pbFile := newPlaybookEngineTestHandler(t, pbYAML)
	ssh := &fakeSSHRunner{user: "root"}
	h.sshRunner = ssh
	h.checker = blacklist.NewDefaultChecker()

	run, err := rs.Create(t.Context(), "pb-1", "safe-script", pbFile, []string{"n1"}, nil, "", false)
	require.NoError(t, err)

	h.executePlaybookRunV2(run.ID)

	run, err = rs.Get(t.Context(), run.ID)
	require.NoError(t, err)
	// 安全脚本放行：检查通过后继续走脚本执行链（本测试环境无真实节点，
	// 以节点解析失败收场），不得出现黑名单拦截。
	for _, s := range run.Results {
		if s.TaskName == "probe" {
			assert.NotContains(t, s.Error, "黑名单", "安全脚本不应被拦截")
		}
	}
}
