package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlaybookRun_RecordsHistory(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY, name TEXT, address TEXT, port INTEGER DEFAULT 22,
		user TEXT, password TEXT, ssh_key TEXT, status TEXT DEFAULT 'unknown',
		groups TEXT DEFAULT '[]', labels TEXT DEFAULT '{}',
		proxy_jump TEXT DEFAULT '', created_at TIMESTAMP, updated_at TIMESTAMP)`)
	require.NoError(t, err)

	ps := store.NewPlaybookStore(db)
	require.NoError(t, ps.Init(t.Context()))
	rs := store.NewPlaybookRunStore(db)
	require.NoError(t, rs.Init(t.Context()))
	ns := store.NewNodeStore(db)
	hs := store.NewHistoryStore(db)
	require.NoError(t, hs.Init(t.Context()))

	h := NewPlaybookHandler(db, ps, rs, ns, nil)
	h.History = hs

	run, err := rs.Create(t.Context(), "pb-1", "demo", "/nonexistent.yaml", []string{"n1"}, nil, "", false)
	require.NoError(t, err)

	op := &store.Operation{TaskID: run.ID, OpType: "playbook", Command: "playbook run demo", Targets: []string{"n1"}, PlaybookPath: "/nonexistent.yaml", Status: "running"}
	require.NoError(t, hs.RecordOperation(t.Context(), op))

	recs, total, err := hs.Query(t.Context(), &store.QueryOptions{OpType: "playbook"})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	assert.Equal(t, "/nonexistent.yaml", recs[0].Operation.PlaybookPath)
}

func TestPlaybookRun_ForcedAudit(t *testing.T) {
	runPlaybookAndReadForced := func(confirmed bool) bool {
		db, err := sql.Open("sqlite", ":memory:")
		require.NoError(t, err)
		db.SetMaxOpenConns(1)

		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS nodes (
			id TEXT PRIMARY KEY, name TEXT, address TEXT, port INTEGER DEFAULT 22,
			user TEXT, password TEXT, ssh_key TEXT, status TEXT DEFAULT 'unknown',
			groups TEXT DEFAULT '[]', labels TEXT DEFAULT '{}',
			proxy_jump TEXT DEFAULT '', created_at TIMESTAMP, updated_at TIMESTAMP)`)
		require.NoError(t, err)

		ps := store.NewPlaybookStore(db)
		require.NoError(t, ps.Init(t.Context()))
		rs := store.NewPlaybookRunStore(db)
		require.NoError(t, rs.Init(t.Context()))
		ns := store.NewNodeStore(db)
		hs := store.NewHistoryStore(db)
		require.NoError(t, hs.Init(t.Context()))

		pbFile := filepath.Join(t.TempDir(), "forced.yaml")
		require.NoError(t, os.WriteFile(pbFile, []byte("name: forced-test\ntasks: []\n"), 0644))
		require.NoError(t, ps.Upsert(t.Context(), &model.Playbook{ID: "forced-pb", Name: "forced-test", FilePath: pbFile, FileExists: true}))

		h := NewPlaybookHandler(db, ps, rs, ns, nil)
		h.History = hs

		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		body := fmt.Sprintf(`{"target_nodes":["n1"],"danger_confirmed":%t}`, confirmed)
		c.Request = httptest.NewRequest("POST", "/api/v1/playbooks/forced-pb/run", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Params = gin.Params{{Key: "id", Value: "forced-pb"}}
		h.Run(c)
		require.Equal(t, http.StatusAccepted, w.Code, "danger_confirmed=%t", confirmed)

		var run model.PlaybookRun
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &run))

		var forced int
		require.NoError(t, db.QueryRow(`SELECT forced FROM operations WHERE task_id = ? AND op_type = 'playbook'`, run.ID).Scan(&forced))
		return forced == 1
	}

	assert.False(t, runPlaybookAndReadForced(false), "plain run must not be audited as forced")
	assert.True(t, runPlaybookAndReadForced(true), "danger_confirmed run must be audited as forced")
}
