package handler

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	ai2 "github.com/cangyunye/go-owl/internal/ai"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func aiExecutorSetup(t *testing.T) *WebExecutor {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	key, err := os.ReadFile(filepath.Join(home, ".ssh", "id_rsa"))
	if err != nil {
		t.Skip("no ~/.ssh/id_rsa, skipping AI executor e2e")
	}
	user := os.Getenv("USER")
	if user == "" {
		user = "root"
	}

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY, name TEXT, address TEXT, port INTEGER DEFAULT 22,
		user TEXT, password TEXT, ssh_key TEXT, status TEXT DEFAULT 'unknown',
		groups TEXT DEFAULT '[]', labels TEXT DEFAULT '{}',
		proxy_jump TEXT DEFAULT '', created_at TIMESTAMP, updated_at TIMESTAMP)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO nodes (id, name, address, port, user, ssh_key, status, groups) VALUES (?,?,?,?,?,?,?,?)`,
		"localhost-ai", "localhost", "127.0.0.1", 22, user, string(key), "online", `["ai-test"]`)
	require.NoError(t, err)

	// verify SSH reachable
	info := &nodeSSHInfo{Address: "127.0.0.1", Port: 22, User: user, SSHKey: string(key)}
	c, sc, err := dialSFTP(info)
	if err != nil {
		t.Skipf("localhost SSH unavailable: %v", err)
	}
	c.Close()
	sc.Close()

	ts := store.NewTaskStore(db)
	require.NoError(t, ts.Init(context.Background()))
	trs := store.NewTransferRecordStore(db)
	require.NoError(t, trs.Init(context.Background()))
	prs := store.NewPlaybookRunStore(db)
	require.NoError(t, prs.Init(context.Background()))
	ns := store.NewNodeStore(db)
	pbs := store.NewPlaybookStore(db)
	require.NoError(t, pbs.Init(context.Background()))
	audit := store.NewAIAuditStore(db)
	require.NoError(t, audit.Init(context.Background()))
	hs := store.NewHistoryStore(db)
	require.NoError(t, hs.Init(context.Background()))

	e := NewWebExecutor(db, ts, trs, prs, ns, pbs, audit, NewKeyManager(), false)
	e.userRole = "admin"
	e.History = hs
	e.PlaybookHandler = NewPlaybookHandler(db, pbs, prs, ns, nil)
	return e
}

func TestWebExecutor_ExecuteCommand_E2E(t *testing.T) {
	e := aiExecutorSetup(t)
	ctx := context.Background()

	res, err := e.ExecuteCommand(ctx, ai2.ExecCommandParams{Nodes: []string{"localhost-ai"}, Command: "echo AI_EXEC_OK_123"})
	require.NoError(t, err)
	assert.Contains(t, res.Text, "AI_EXEC_OK_123")
	assert.Contains(t, res.Text, "1 成功")

	recs, total, err := e.History.Query(ctx, &store.QueryOptions{OpType: "command"})
	require.NoError(t, err)
	require.GreaterOrEqual(t, total, 1)
	assert.Equal(t, "echo AI_EXEC_OK_123", recs[0].Operation.Command)
	require.Len(t, recs[0].CommandExecutions, 1)
	assert.True(t, recs[0].CommandExecutions[0].Success)
}

func TestWebExecutor_NodeCheck_E2E(t *testing.T) {
	e := aiExecutorSetup(t)
	ctx := context.Background()

	res, err := e.NodeCheck(ctx, ai2.NodeCheckParams{Nodes: []string{"localhost-ai"}, Update: true})
	require.NoError(t, err)
	assert.Contains(t, res.Text, "1 在线")
	assert.Contains(t, res.Text, "localhost-ai")
}

func TestWebExecutor_TransferFile_E2E(t *testing.T) {
	e := aiExecutorSetup(t)
	ctx := context.Background()

	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "ai_transfer.txt")
	content := "ai transfer content\n"
	require.NoError(t, os.WriteFile(src, []byte(content), 0644))

	remotePath := fmt.Sprintf("/tmp/ai_transfer_%d.txt", os.Getpid())
	remoteCleanup(&nodeSSHInfo{Address: "127.0.0.1", Port: 22, User: os.Getenv("USER"), SSHKey: readKey(t)}, remotePath)
	defer remoteCleanup(&nodeSSHInfo{Address: "127.0.0.1", Port: 22, User: os.Getenv("USER"), SSHKey: readKey(t)}, remotePath)

	res, err := e.TransferFile(ctx, ai2.TransferFileParams{SourceFile: src, Nodes: []string{"localhost-ai"}, DestDir: remotePath, Permission: "0600"})
	require.NoError(t, err)
	assert.Contains(t, res.Text, "1 成功")

	info := &nodeSSHInfo{Address: "127.0.0.1", Port: 22, User: os.Getenv("USER"), SSHKey: readKey(t)}
	c, sc, err := dialSFTP(info)
	require.NoError(t, err)
	defer sc.Close()
	defer c.Close()
	fi, err := c.Stat(remotePath)
	require.NoError(t, err)
	assert.Equal(t, int64(len(content)), fi.Size())
	assert.Equal(t, os.FileMode(0600), fi.Mode().Perm())
}

func TestWebExecutor_RunPlaybook_E2E(t *testing.T) {
	e := aiExecutorSetup(t)
	ctx := context.Background()

	pbDir := t.TempDir()
	pbFile := filepath.Join(pbDir, "ai-test.yaml")
	pbYAML := "name: ai-test\ntasks:\n  - Say hello:\n      command: echo AI_PLAYBOOK_OK\n"
	require.NoError(t, os.WriteFile(pbFile, []byte(pbYAML), 0644))

	require.NoError(t, e.playbookStore.Upsert(ctx, &model.Playbook{
		ID: "ai-test", Name: "ai-test", FilePath: pbFile, FileExists: true, TasksCount: 1,
	}))

	res, err := e.RunPlaybook(ctx, ai2.RunPlaybookParams{Name: "ai-test", Nodes: []string{"localhost-ai"}})
	require.NoError(t, err)
	assert.Contains(t, res.Text, "ai-test")
	assert.Contains(t, res.Text, "Say hello")
}

func readKey(t *testing.T) string {
	t.Helper()
	home, _ := os.UserHomeDir()
	key, err := os.ReadFile(filepath.Join(home, ".ssh", "id_rsa"))
	if err != nil {
		t.Skip("no key")
	}
	return string(key)
}
