package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"

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
	e.userRole = "admin"

	res, err := e.ExecuteCommand(t.Context(), ai2.ExecCommandParams{
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
	e.userRole = "admin"

	res, err := e.ExecuteScript(t.Context(), ai2.ExecScriptParams{
		Nodes:  []string{"test-node"},
		Script: "#!/bin/bash\nrm -rf /var/data",
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Contains(t, res.Text, "黑名单")
}
