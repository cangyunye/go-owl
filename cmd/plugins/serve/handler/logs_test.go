package handler

import (
	"archive/zip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/internal/logfile"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func logsTestRouter(t *testing.T) (*gin.Engine, *LogHandler) {
	t.Helper()
	t.Setenv("OWL_LOG_DIR", t.TempDir())
	gin.SetMode(gin.TestMode)
	h := NewLogHandler()
	r := gin.New()
	r.GET("/api/v1/executions/:op_id/logs", h.List)
	r.GET("/api/v1/executions/:op_id/logs/archive", h.Archive)
	r.GET("/api/v1/executions/:op_id/logs/:node_id", h.Download)
	return r, h
}

func seedExecLog(t *testing.T, opID, nodeID string) {
	t.Helper()
	w := logfile.NewNodeLogWriter("")
	if _, err := w.WriteExecutionLog(opID, nodeID, "task-1", "uptime", 0, "hello output", "", 0); err != nil {
		t.Fatalf("seed log: %v", err)
	}
}

func TestLogList(t *testing.T) {
	r, _ := logsTestRouter(t)
	seedExecLog(t, "op-aaa", "web-01")
	seedExecLog(t, "op-aaa", "db-01")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/executions/op-aaa/logs", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)
	var resp struct {
		Data []logfile.ExecutionLogInfo `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	require.Len(t, resp.Data, 2)
	ids := []string{resp.Data[0].NodeID, resp.Data[1].NodeID}
	assert.ElementsMatch(t, []string{"web-01", "db-01"}, ids)
}

func TestLogList_UnknownOp_Empty(t *testing.T) {
	r, _ := logsTestRouter(t)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/executions/op-missing/logs", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), `"data":[]`)
}

func TestLogDownload(t *testing.T) {
	r, _ := logsTestRouter(t)
	seedExecLog(t, "op-aaa", "web-01")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/executions/op-aaa/logs/web-01", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)
	assert.Contains(t, w.Header().Get("Content-Disposition"), "attachment")
	assert.Contains(t, w.Body.String(), "hello output")
}

func TestLogDownload_NotFound(t *testing.T) {
	r, _ := logsTestRouter(t)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/executions/op-aaa/logs/nope", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, 404, w.Code)
}

func TestLogDownload_TraversalRejected(t *testing.T) {
	r, _ := logsTestRouter(t)
	seedExecLog(t, "op-aaa", "web-01")

	// node_id with path separators must be sanitized, not escape the dir
	for _, path := range []string{"/api/v1/executions/op-aaa/logs/..%2f..%2fetc%2fpasswd", "/api/v1/executions/op-aaa/logs/%2e%2e"} {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", path, nil)
		r.ServeHTTP(w, req)
		require.Equal(t, 404, w.Code, "path %s", path)
		assert.NotContains(t, w.Body.String(), "root:") // no /etc/passwd content
	}
}

func TestLogDownload_InvalidOp(t *testing.T) {
	r, _ := logsTestRouter(t)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/executions/op..id/logs/web-01", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, 400, w.Code)
}

func TestLogArchive(t *testing.T) {
	r, _ := logsTestRouter(t)
	seedExecLog(t, "op-aaa", "web-01")
	seedExecLog(t, "op-aaa", "db-01")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/executions/op-aaa/logs/archive", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)
	assert.Equal(t, "application/zip", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Header().Get("Content-Disposition"), "op-aaa.zip")

	zr, err := zip.NewReader(strings.NewReader(w.Body.String()), int64(w.Body.Len()))
	require.NoError(t, err)
	var names []string
	var contents []string
	for _, f := range zr.File {
		names = append(names, f.Name)
		rc, err := f.Open()
		require.NoError(t, err)
		b, _ := io.ReadAll(rc)
		rc.Close()
		contents = append(contents, string(b))
	}
	assert.ElementsMatch(t, []string{"manifest.json", "web-01.log", "db-01.log"}, names)
	joined := strings.Join(contents, "\n")
	assert.Contains(t, joined, "hello output")
}
