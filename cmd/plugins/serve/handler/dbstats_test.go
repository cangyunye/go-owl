package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func dbStatsTestSetup(t *testing.T) (*DBStatsHandler, *gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "owl.db")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`CREATE TABLE metrics_202609 (node_id TEXT, metric TEXT, ts INTEGER, value REAL)`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE nodes (id TEXT PRIMARY KEY, name TEXT)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO metrics_202609 SELECT 'n1', 'cpu.usage', i, 1.0 FROM (WITH RECURSIVE seq(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM seq WHERE i < 500) SELECT i FROM seq)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO nodes VALUES ('n1', 'node-1')`)
	require.NoError(t, err)

	h := NewDBStatsHandler(db, dbPath)
	r := gin.New()
	r.GET("/api/v1/db/stats", h.Stats)
	r.POST("/api/v1/db/vacuum", h.Vacuum)
	return h, r, dbPath
}

func doGet(t *testing.T, r *gin.Engine, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, path, nil)
	r.ServeHTTP(w, req)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return w, body
}

// 统计端点覆盖库内全部表：行数、字节占用与文件级信息。
func TestDBStats_ListsAllTablesWithRowsAndSizes(t *testing.T) {
	_, r, _ := dbStatsTestSetup(t)

	w, body := doGet(t, r, "/api/v1/db/stats")
	require.Equal(t, http.StatusOK, w.Code, body)

	data := body["data"].(map[string]any)
	tables := data["tables"].([]any)
	byName := map[string]map[string]any{}
	for _, tb := range tables {
		m := tb.(map[string]any)
		byName[m["name"].(string)] = m
	}
	metrics := byName["metrics_202609"]
	require.NotNil(t, metrics, "metrics table missing: %v", byName)
	assert.Equal(t, float64(500), metrics["rows"])
	assert.Greater(t, metrics["size_bytes"], float64(0))
	nodes := byName["nodes"]
	require.NotNil(t, nodes)
	assert.Equal(t, float64(1), nodes["rows"])

	file := data["file"].(map[string]any)
	assert.Greater(t, file["size_bytes"], float64(0))
	assert.Greater(t, file["page_size"], float64(0))
	assert.Greater(t, data["total_size_bytes"], float64(0))
}

// Vacuum 端点执行空间回收并成功返回（对已回收的库是无损幂等操作）。
func TestDBStats_Vacuum(t *testing.T) {
	_, r, dbPath := dbStatsTestSetup(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/db/vacuum", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body struct {
		Data struct {
			SizeBytesBefore int64 `json:"size_bytes_before"`
			SizeBytesAfter  int64 `json:"size_bytes_after"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Greater(t, body.Data.SizeBytesBefore, int64(0))
	assert.Greater(t, body.Data.SizeBytesAfter, int64(0))
	// VACUUM 重建后的文件仍在原位
	assert.FileExists(t, dbPath)
}

// 打不开 dbstat 等极端场景下统计端点降级为 0 而非 5xx，管理页仍可用。
func TestDBStats_EmptyDatabaseStillReports(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`CREATE TABLE t1 (a INTEGER)`)
	require.NoError(t, err)

	h := NewDBStatsHandler(db, filepath.Join(t.TempDir(), "x.db"))
	r := gin.New()
	r.GET("/api/v1/db/stats", h.Stats)

	w, body := doGet(t, r, "/api/v1/db/stats")
	require.Equal(t, http.StatusOK, w.Code, body)
	tables := body["data"].(map[string]any)["tables"].([]any)
	require.Len(t, tables, 1)
	assert.Equal(t, "t1", tables[0].(map[string]any)["name"])
}
