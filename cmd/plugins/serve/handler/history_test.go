package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func historyTestSetup(t *testing.T) (*store.HistoryStore, *gin.Engine) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	hs := store.NewHistoryStore(db)
	require.NoError(t, hs.Init(context.Background()))

	gin.SetMode(gin.TestMode)
	us := store.NewUserStore(db)
	as := service.NewAuthService("test-secret-32byte-long-string!!")
	ah := NewAuthHandler(us, as)
	hh := NewHistoryHandler(hs)

	r := gin.New()
	auth := r.Group("/api/v1")
	auth.Use(ah.AuthMiddleware())
	reader := auth.Group("", ah.RBACMiddleware(model.RoleViewer))
	reader.GET("/history", hh.List)
	reader.GET("/history/stats", hh.Stats)
	reader.GET("/history/export", hh.Export)
	reader.GET("/history/detail/:task_id", hh.Get)
	admin := auth.Group("", ah.RBACMiddleware(model.RoleAdmin))
	admin.DELETE("/history", hh.Clean)
	return hs, r
}

func historyGET(t *testing.T, r *gin.Engine, path, tok string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", path, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)
	return w
}

func TestHistoryList_AndFilters(t *testing.T) {
	hs, r := historyTestSetup(t)
	ctx := context.Background()
	hs.RecordOperation(ctx, &store.Operation{TaskID: "a", OpType: "command", Command: "uptime", Targets: []string{"n1"}, Status: "completed", CreatedAt: time.Now().UTC()})
	hs.RecordOperation(ctx, &store.Operation{TaskID: "b", OpType: "playbook", Command: "deploy", Targets: []string{"n2"}, Status: "failed", CreatedAt: time.Now().UTC()})

	w := historyGET(t, r, "/api/v1/history", adminToken())
	require.Equal(t, 200, w.Code)
	var resp struct {
		Data []store.Record `json:"data"`
		Meta struct{ Total int `json:"total"` } `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.Meta.Total)
	assert.Len(t, resp.Data, 2)

	w2 := historyGET(t, r, "/api/v1/history?op_type=command", adminToken())
	var resp2 struct {
		Data []store.Record `json:"data"`
		Meta struct{ Total int `json:"total"` } `json:"meta"`
	}
	json.Unmarshal(w2.Body.Bytes(), &resp2)
	assert.Equal(t, 1, resp2.Meta.Total)
	assert.Equal(t, "command", resp2.Data[0].Operation.OpType)

	w3 := historyGET(t, r, "/api/v1/history?status=failed", adminToken())
	var resp3 struct {
		Meta struct{ Total int `json:"total"` } `json:"meta"`
	}
	json.Unmarshal(w3.Body.Bytes(), &resp3)
	assert.Equal(t, 1, resp3.Meta.Total)
}

func TestHistoryGet_Detail(t *testing.T) {
	hs, r := historyTestSetup(t)
	ctx := context.Background()
	hs.RecordOperation(ctx, &store.Operation{TaskID: "op-x", OpType: "command", Command: "uptime", Targets: []string{"n1"}, Status: "completed"})
	hs.RecordCommandExecution(ctx, &store.CommandExecution{TaskID: "op-x", NodeID: "n1", Command: "uptime", ExitCode: 0, Stdout: "ok", Success: true})

	w := historyGET(t, r, "/api/v1/history/detail/op-x", adminToken())
	require.Equal(t, 200, w.Code)
	var rec store.Record
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rec))
	assert.Equal(t, "op-x", rec.Operation.TaskID)
	require.Len(t, rec.CommandExecutions, 1)

	w404 := historyGET(t, r, "/api/v1/history/detail/nope", adminToken())
	assert.Equal(t, 404, w404.Code)
}

func TestHistoryStats(t *testing.T) {
	hs, r := historyTestSetup(t)
	ctx := context.Background()
	hs.RecordOperation(ctx, &store.Operation{TaskID: "a", OpType: "command", Status: "completed"})
	hs.RecordOperation(ctx, &store.Operation{TaskID: "b", OpType: "command", Status: "failed"})

	w := historyGET(t, r, "/api/v1/history/stats", adminToken())
	require.Equal(t, 200, w.Code)
	var st store.Stats
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &st))
	assert.Equal(t, 2, st.Total)
	assert.Equal(t, 2, st.ByOpType["command"])
}

func TestHistoryList_AsViewer_Allowed(t *testing.T) {
	_, r := historyTestSetup(t)
	w := historyGET(t, r, "/api/v1/history", viewerToken())
	assert.Equal(t, 200, w.Code)
}

func TestParseHistoryDuration(t *testing.T) {
	d, err := parseHistoryDuration("24h")
	require.NoError(t, err)
	assert.Equal(t, 24*time.Hour, d)
	d2, err := parseHistoryDuration("7d")
	require.NoError(t, err)
	assert.Equal(t, 7*24*time.Hour, d2)
	d3, err := parseHistoryDuration("30m")
	require.NoError(t, err)
	assert.Equal(t, 30*time.Minute, d3)
}

func TestHistoryExport_JSON(t *testing.T) {
	hs, r := historyTestSetup(t)
	hs.RecordOperation(context.Background(), &store.Operation{TaskID: "a", OpType: "command", Command: "uptime", Status: "completed"})

	w := historyGET(t, r, "/api/v1/history/export?format=json", adminToken())
	require.Equal(t, 200, w.Code)
	assert.Contains(t, w.Header().Get("Content-Disposition"), "history-")
	assert.Contains(t, w.Header().Get("Content-Disposition"), ".json")
	var recs []store.Record
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &recs))
	assert.Len(t, recs, 1)
}

func TestHistoryExport_YAML(t *testing.T) {
	hs, r := historyTestSetup(t)
	hs.RecordOperation(context.Background(), &store.Operation{TaskID: "a", OpType: "command", Command: "uptime", Status: "completed"})

	w := historyGET(t, r, "/api/v1/history/export?format=yaml", adminToken())
	require.Equal(t, 200, w.Code)
	assert.Contains(t, w.Header().Get("Content-Disposition"), ".yaml")
	assert.Contains(t, w.Body.String(), "uptime")
}

func TestHistoryClean_AdminOnly(t *testing.T) {
	hs, r := historyTestSetup(t)
	ctx := context.Background()
	hs.RecordOperation(ctx, &store.Operation{TaskID: "old", OpType: "command", Status: "completed", CreatedAt: time.Now().UTC().AddDate(0, 0, -100)})
	hs.RecordOperation(ctx, &store.Operation{TaskID: "new", OpType: "command", Status: "completed", CreatedAt: time.Now().UTC()})

	wv := httptest.NewRecorder()
	reqv, _ := http.NewRequest("DELETE", "/api/v1/history?days=30", nil)
	reqv.Header.Set("Authorization", "Bearer "+viewerToken())
	r.ServeHTTP(wv, reqv)
	assert.Equal(t, 403, wv.Code)

	wbad := httptest.NewRecorder()
	reqbad, _ := http.NewRequest("DELETE", "/api/v1/history?days=0", nil)
	reqbad.Header.Set("Authorization", "Bearer "+adminToken())
	r.ServeHTTP(wbad, reqbad)
	assert.Equal(t, 400, wbad.Code)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/api/v1/history?days=30", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken())
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
	var resp struct {
		Deleted int64 `json:"deleted"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, int64(1), resp.Deleted)
}
