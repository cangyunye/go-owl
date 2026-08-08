package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
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

func playbookUploadRouter(t *testing.T) (*gin.Engine, *PlaybookHandler, *sql.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`)
	require.NoError(t, err)
	library := t.TempDir()
	_, err = db.Exec(`INSERT INTO settings (key, value) VALUES ('playbook_library_path', ?)`, library)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO settings (key, value) VALUES ('staging_min_free', '1')`)
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

	us := store.NewUserStore(db)
	as := service.NewAuthService("test-secret-32byte-long-string!!")
	ah := NewAuthHandler(us, as)

	r := gin.New()
	auth := r.Group("/api/v1")
	auth.Use(ah.AuthMiddleware())
	op := auth.Group("", ah.RBACMiddleware(model.RoleOperator))
	op.POST("/playbooks/upload", h.Upload)
	op.GET("/playbooks/:id/download", h.Download)
	op.GET("/playbooks", h.List)

	return r, h, db
}

func multipartUploadTo(t *testing.T, router *gin.Engine, token, url, filename, content string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = io.Copy(part, strings.NewReader(content))
	require.NoError(t, err)
	w.Close()

	req, _ := http.NewRequest("POST", url, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestPlaybookUpload_Basic(t *testing.T) {
	router, h, _ := playbookUploadRouter(t)
	token := testTokenOp(t)

	rec := multipartUploadTo(t, router, token, "/api/v1/playbooks/upload", "demo.yaml", "name: demo\nversion: 1.0\ntasks:\n  - name: ping\n    command: echo hi\n")
	assert.Equal(t, http.StatusCreated, rec.Code)

	library := h.getPlaybookLibraryPath()
	_, err := os.Stat(filepath.Join(library, "demo.yaml"))
	assert.NoError(t, err, "uploaded file must exist in library")

	var resp struct {
		Data model.Playbook `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "demo", resp.Data.Name)
	assert.True(t, resp.Data.FileExists)
}

func TestPlaybookUpload_BadExtension(t *testing.T) {
	router, _, _ := playbookUploadRouter(t)
	token := testTokenOp(t)

	rec := multipartUploadTo(t, router, token, "/api/v1/playbooks/upload", "demo.txt", "not a playbook")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPlaybookUpload_SameNameOverwrites(t *testing.T) {
	router, h, _ := playbookUploadRouter(t)
	token := testTokenOp(t)

	rec1 := multipartUploadTo(t, router, token, "/api/v1/playbooks/upload", "demo.yaml", "name: v1\ntasks: []\n")
	assert.Equal(t, http.StatusCreated, rec1.Code)

	var resp1 struct {
		Data model.Playbook `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &resp1))

	rec2 := multipartUploadTo(t, router, token, "/api/v1/playbooks/upload", "demo.yaml", "name: v2\ntasks: []\n")
	assert.Equal(t, http.StatusCreated, rec2.Code)

	var resp2 struct {
		Data model.Playbook `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp2))
	assert.Equal(t, resp1.Data.ID, resp2.Data.ID, "overwrite must keep stable ID")
	assert.Equal(t, "v2", resp2.Data.Name, "content must be replaced")

	library := h.getPlaybookLibraryPath()
	data, err := os.ReadFile(filepath.Join(library, "demo.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "name: v2")
}

func TestPlaybookUpload_InsufficientDisk(t *testing.T) {
	router, _, db := playbookUploadRouter(t)
	_, err := db.Exec(`UPDATE settings SET value = '999999999' WHERE key = 'staging_min_free'`)
	require.NoError(t, err)
	token := testTokenOp(t)

	rec := multipartUploadTo(t, router, token, "/api/v1/playbooks/upload", "demo.yaml", "name: demo\ntasks: []\n")
	assert.Equal(t, http.StatusInsufficientStorage, rec.Code)
}

func TestPlaybookDownload_Basic(t *testing.T) {
	router, h, _ := playbookUploadRouter(t)
	token := testTokenOp(t)

	uploaded := multipartUploadTo(t, router, token, "/api/v1/playbooks/upload", "demo.yaml", "name: demo\nversion: 1.0\ntasks: []\n")
	require.Equal(t, http.StatusCreated, uploaded.Code)
	var resp struct {
		Data model.Playbook `json:"data"`
	}
	require.NoError(t, json.Unmarshal(uploaded.Body.Bytes(), &resp))

	req, _ := http.NewRequest("GET", "/api/v1/playbooks/"+resp.Data.ID+"/download", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Disposition"), "attachment")
	assert.Contains(t, string(rec.Body.Bytes()), "name: demo")

	_ = h
}

func TestPlaybookDownload_NotFound(t *testing.T) {
	router, _, _ := playbookUploadRouter(t)
	token := testTokenOp(t)

	req, _ := http.NewRequest("GET", "/api/v1/playbooks/ghost/download", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestPlaybookRun_PreflightWarnings(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY, name TEXT, address TEXT, port INTEGER DEFAULT 22,
		user TEXT, password TEXT, ssh_key TEXT, status TEXT DEFAULT 'unknown',
		groups TEXT DEFAULT '[]', labels TEXT DEFAULT '{}',
		proxy_jump TEXT DEFAULT '', created_at TIMESTAMP, updated_at TIMESTAMP)`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`)
	require.NoError(t, err)

	library := t.TempDir()
	_, err = db.Exec(`INSERT INTO settings (key, value) VALUES ('playbook_library_path', ?)`, library)
	require.NoError(t, err)

	ps := store.NewPlaybookStore(db)
	require.NoError(t, ps.Init(t.Context()))
	rs := store.NewPlaybookRunStore(db)
	require.NoError(t, rs.Init(t.Context()))
	ns := store.NewNodeStore(db)
	hs := store.NewHistoryStore(db)
	require.NoError(t, hs.Init(t.Context()))

	pbFile := filepath.Join(library, "preflight.yaml")
	pbYAML := `name: preflight
tasks:
  - name: upload-missing
    action: upload
    args:
      src: "{{PLAYBOOK_DIR}}/missing.bin"
      dest: /tmp/missing.bin
  - name: upload-ok
    action: upload
    args:
      src: "{{PLAYBOOK_DIR}}/exists.txt"
      dest: /tmp/exists.txt
`
	require.NoError(t, os.WriteFile(pbFile, []byte(pbYAML), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(library, "exists.txt"), []byte("x"), 0644))
	require.NoError(t, ps.Upsert(t.Context(), &model.Playbook{ID: "preflight-pb", Name: "preflight", FilePath: pbFile, FileExists: true}))

	h := NewPlaybookHandler(db, ps, rs, ns, nil)
	h.History = hs

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/v1/playbooks/preflight-pb/run", strings.NewReader(`{"target_nodes":["n1"]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "preflight-pb"}}
	h.Run(c)
	require.Equal(t, http.StatusAccepted, w.Code)

	var run model.PlaybookRun
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &run))
	require.Len(t, run.Warnings, 1, "only the missing src should be warned")
	assert.Contains(t, run.Warnings[0], "upload-missing")
	assert.Contains(t, run.Warnings[0], "missing.bin")
}
