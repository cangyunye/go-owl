package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func featuresTestSetup(t *testing.T) (*sql.DB, *NodeHandler, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY, name TEXT, address TEXT, port INTEGER DEFAULT 22,
		user TEXT, password TEXT, ssh_key TEXT, status TEXT DEFAULT 'unknown',
		groups TEXT DEFAULT '[]', labels TEXT DEFAULT '{}',
		proxy_jump TEXT DEFAULT '', created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO nodes (id, name, address, port, user, status, groups, labels) VALUES
		('node-1', 'Node 1', '10.0.0.1', 22, 'root', 'online', '["web","prod"]', '{"env":"prod"}'),
		('node-2', 'Node 2', '10.0.0.2', 22, 'deploy', 'online', '["web"]', '{"env":"staging"}'),
		('node-3', 'Node 3', '10.0.0.3', 2222, 'admin', 'offline', '["db"]', '{"env":"prod"}')`)
	require.NoError(t, err)

	h := NewNodeHandler(db)

	router := gin.New()
	injectRBAC(db, router, "POST", "/api/v1/nodes/batch/groups", model.RoleEditor, h.BatchGroups)
	injectRBAC(db, router, "POST", "/api/v1/nodes/export", model.RoleEditor, h.Export)
	injectRBAC(db, router, "POST", "/api/v1/nodes/import", model.RoleEditor, h.Import)
	injectRBAC(db, router, "POST", "/api/v1/nodes/ping", model.RoleEditor, h.Ping)
	injectRBAC(db, router, "POST", "/api/v1/nodes/check", model.RoleEditor, h.Check)

	return db, h, router
}

func TestBatchGroups_AddGroups(t *testing.T) {
	_, h, router := featuresTestSetup(t)

	body := map[string]interface{}{
		"node_ids": []string{"node-1", "node-2"},
		"add":      []string{"cache", "monitor"},
		"remove":   []string{},
	}
	w := authRequest(t, router, "POST", "/api/v1/nodes/batch/groups", body, "editor")
	assert.Equal(t, 200, w.Code)

	var resp struct {
		Updated int      `json:"updated"`
		Errors  []string `json:"errors"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 2, resp.Updated)
	assert.Empty(t, resp.Errors)

	var groupsJSON string
	h.db.QueryRow("SELECT groups FROM nodes WHERE id = 'node-1'").Scan(&groupsJSON)
	var groups []string
	json.Unmarshal([]byte(groupsJSON), &groups)
	assert.Contains(t, groups, "web")
	assert.Contains(t, groups, "prod")
	assert.Contains(t, groups, "cache")
	assert.Contains(t, groups, "monitor")
}

func TestBatchGroups_RemoveGroups(t *testing.T) {
	_, h, router := featuresTestSetup(t)

	body := map[string]interface{}{
		"node_ids": []string{"node-1"},
		"add":      []string{},
		"remove":   []string{"web"},
	}
	w := authRequest(t, router, "POST", "/api/v1/nodes/batch/groups", body, "editor")
	assert.Equal(t, 200, w.Code)

	var groupsJSON string
	h.db.QueryRow("SELECT groups FROM nodes WHERE id = 'node-1'").Scan(&groupsJSON)
	var groups []string
	json.Unmarshal([]byte(groupsJSON), &groups)
	assert.NotContains(t, groups, "web")
	assert.Contains(t, groups, "prod")
}

func TestBatchGroups_AddAndRemove(t *testing.T) {
	_, h, router := featuresTestSetup(t)

	body := map[string]interface{}{
		"node_ids": []string{"node-1", "node-2", "node-3"},
		"add":      []string{"new-group"},
		"remove":   []string{"web", "db"},
	}
	w := authRequest(t, router, "POST", "/api/v1/nodes/batch/groups", body, "editor")
	assert.Equal(t, 200, w.Code)

	var resp struct {
		Updated int `json:"updated"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 3, resp.Updated)

	for _, id := range []string{"node-1", "node-2", "node-3"} {
		var groupsJSON string
		h.db.QueryRow("SELECT groups FROM nodes WHERE id = ?", id).Scan(&groupsJSON)
		var groups []string
		json.Unmarshal([]byte(groupsJSON), &groups)
		assert.Contains(t, groups, "new-group")
		assert.NotContains(t, groups, "web")
		assert.NotContains(t, groups, "db")
	}
}

func TestBatchGroups_EmptyNodeIDs(t *testing.T) {
	_, _, router := featuresTestSetup(t)

	body := map[string]interface{}{
		"node_ids": []string{},
		"add":      []string{"test"},
	}
	w := authRequest(t, router, "POST", "/api/v1/nodes/batch/groups", body, "editor")
	assert.Equal(t, 400, w.Code)
}

func TestBatchGroups_NonExistentNode(t *testing.T) {
	_, _, router := featuresTestSetup(t)

	body := map[string]interface{}{
		"node_ids": []string{"node-1", "nonexistent"},
		"add":      []string{"test"},
	}
	w := authRequest(t, router, "POST", "/api/v1/nodes/batch/groups", body, "editor")
	assert.Equal(t, 200, w.Code)

	var resp struct {
		Updated int      `json:"updated"`
		Errors  []string `json:"errors"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 1, resp.Updated)
	assert.Len(t, resp.Errors, 1)
	assert.Contains(t, resp.Errors[0], "nonexistent")
}

func TestBatchGroups_AsViewer_Forbidden(t *testing.T) {
	_, _, router := featuresTestSetup(t)

	body := map[string]interface{}{
		"node_ids": []string{"node-1"},
		"add":      []string{"test"},
	}
	w := authRequest(t, router, "POST", "/api/v1/nodes/batch/groups", body, "viewer")
	assert.Equal(t, 403, w.Code)
}

func TestExport_AllNodes_YAML(t *testing.T) {
	_, _, router := featuresTestSetup(t)

	body := map[string]interface{}{
		"format": "yaml",
	}
	w := authRequest(t, router, "POST", "/api/v1/nodes/export", body, "editor")
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "yaml")
	assert.Contains(t, w.Header().Get("Content-Disposition"), "nodes-")

	var nf nodeExportFile
	err := yaml.Unmarshal(w.Body.Bytes(), &nf)
	require.NoError(t, err)
	assert.Equal(t, "1.0", nf.Version)
	assert.Len(t, nf.Nodes, 3)
}

func TestExport_AllNodes_JSON(t *testing.T) {
	_, _, router := featuresTestSetup(t)

	body := map[string]interface{}{
		"format": "json",
	}
	w := authRequest(t, router, "POST", "/api/v1/nodes/export", body, "editor")
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "json")

	var nf nodeExportFile
	err := json.Unmarshal(w.Body.Bytes(), &nf)
	require.NoError(t, err)
	assert.Equal(t, "1.0", nf.Version)
	assert.Len(t, nf.Nodes, 3)
}

func TestExport_FilterByNodeIDs(t *testing.T) {
	_, _, router := featuresTestSetup(t)

	body := map[string]interface{}{
		"node_ids": []string{"node-1", "node-2"},
		"format":   "json",
	}
	w := authRequest(t, router, "POST", "/api/v1/nodes/export", body, "editor")
	assert.Equal(t, 200, w.Code)

	var nf nodeExportFile
	json.Unmarshal(w.Body.Bytes(), &nf)
	assert.Len(t, nf.Nodes, 2)
}

func TestExport_FilterByGroups(t *testing.T) {
	_, _, router := featuresTestSetup(t)

	body := map[string]interface{}{
		"groups": []string{"web"},
		"format": "json",
	}
	w := authRequest(t, router, "POST", "/api/v1/nodes/export", body, "editor")
	assert.Equal(t, 200, w.Code)

	var nf nodeExportFile
	json.Unmarshal(w.Body.Bytes(), &nf)
	assert.Len(t, nf.Nodes, 2)
}

func TestExport_NoMatchingNodes(t *testing.T) {
	_, _, router := featuresTestSetup(t)

	body := map[string]interface{}{
		"groups": []string{"nonexistent"},
		"format": "json",
	}
	w := authRequest(t, router, "POST", "/api/v1/nodes/export", body, "editor")
	assert.Equal(t, 200, w.Code)

	var nf nodeExportFile
	json.Unmarshal(w.Body.Bytes(), &nf)
	assert.Len(t, nf.Nodes, 0)
}

func TestExport_AsViewer_Forbidden(t *testing.T) {
	_, _, router := featuresTestSetup(t)

	body := map[string]interface{}{"format": "json"}
	w := authRequest(t, router, "POST", "/api/v1/nodes/export", body, "viewer")
	assert.Equal(t, 403, w.Code)
}

func TestImport_YAML(t *testing.T) {
	db, h, router := featuresTestSetup(t)

	yamlContent := `version: "1.0"
nodes:
  - id: imported-1
    name: Imported Node 1
    address: 192.168.1.100
    port: 22
    user: root
    status: unknown
    groups: [web, new]
    labels: {env: test}
  - id: imported-2
    name: Imported Node 2
    address: 192.168.1.101
    port: 2222
    user: deploy
    status: online
    groups: [db]
    labels: {env: staging}
`

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "nodes.yaml")
	part.Write([]byte(yamlContent))
	writer.Close()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/nodes/import", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, _ := as.GenerateToken("testuser", "editor")
	req.Header.Set("Authorization", "Bearer "+token)

	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)

	var resp importResult
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 2, resp.Success)
	assert.Equal(t, 0, resp.Failed)

	var count int
	db.QueryRow("SELECT COUNT(*) FROM nodes WHERE id = 'imported-1'").Scan(&count)
	assert.Equal(t, 1, count)

	var groupsJSON string
	h.db.QueryRow("SELECT groups FROM nodes WHERE id = 'imported-1'").Scan(&groupsJSON)
	var groups []string
	json.Unmarshal([]byte(groupsJSON), &groups)
	assert.Contains(t, groups, "web")
	assert.Contains(t, groups, "new")
}

func TestImport_JSON(t *testing.T) {
	db, _, router := featuresTestSetup(t)

	jsonContent := `{
		"version": "1.0",
		"nodes": [
			{"id": "json-import-1", "name": "JSON Import", "address": "10.10.10.10", "port": 22, "user": "root", "groups": ["api"], "labels": {}}
		]
	}`

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "nodes.json")
	part.Write([]byte(jsonContent))
	writer.Close()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/nodes/import", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, _ := as.GenerateToken("testuser", "editor")
	req.Header.Set("Authorization", "Bearer "+token)

	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)

	var resp importResult
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 1, resp.Success)

	var count int
	db.QueryRow("SELECT COUNT(*) FROM nodes WHERE id = 'json-import-1'").Scan(&count)
	assert.Equal(t, 1, count)
}

func TestImport_SkipExisting(t *testing.T) {
	_, _, router := featuresTestSetup(t)

	yamlContent := `version: "1.0"
nodes:
  - id: node-1
    name: Should Be Skipped
    address: 10.0.0.1
    port: 22
    user: root
  - id: new-node
    name: New Node
    address: 10.0.0.99
    port: 22
    user: root
`

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "nodes.yaml")
	part.Write([]byte(yamlContent))
	writer.WriteField("skip_existing", "true")
	writer.Close()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/nodes/import", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, _ := as.GenerateToken("testuser", "editor")
	req.Header.Set("Authorization", "Bearer "+token)

	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)

	var resp importResult
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 1, resp.Success)
	assert.Equal(t, 1, resp.Skipped)
}

func TestImport_Overwrite(t *testing.T) {
	_, h, router := featuresTestSetup(t)

	yamlContent := `version: "1.0"
nodes:
  - id: node-1
    name: Overwritten Name
    address: 10.0.0.1
    port: 22
    user: newuser
    groups: [overwritten]
    labels: {}
`

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "nodes.yaml")
	part.Write([]byte(yamlContent))
	writer.WriteField("overwrite", "true")
	writer.Close()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/nodes/import", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, _ := as.GenerateToken("testuser", "editor")
	req.Header.Set("Authorization", "Bearer "+token)

	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)

	var resp importResult
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 1, resp.Success)

	var name, user string
	h.db.QueryRow("SELECT name, user FROM nodes WHERE id = 'node-1'").Scan(&name, &user)
	assert.Equal(t, "Overwritten Name", name)
	assert.Equal(t, "newuser", user)
}

func TestImport_DryRun(t *testing.T) {
	db, _, router := featuresTestSetup(t)

	yamlContent := `version: "1.0"
nodes:
  - id: dry-run-1
    name: Dry Run Node
    address: 10.0.0.200
    port: 22
    user: root
`

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "nodes.yaml")
	part.Write([]byte(yamlContent))
	writer.WriteField("dry_run", "true")
	writer.Close()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/nodes/import", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, _ := as.GenerateToken("testuser", "editor")
	req.Header.Set("Authorization", "Bearer "+token)

	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)

	var resp importResult
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 1, resp.Success)
	assert.Len(t, resp.Errors, 1)
	assert.Contains(t, resp.Errors[0], "[preview]")

	var count int
	db.QueryRow("SELECT COUNT(*) FROM nodes WHERE id = 'dry-run-1'").Scan(&count)
	assert.Equal(t, 0, count)
}

func TestImport_InvalidFile(t *testing.T) {
	_, _, router := featuresTestSetup(t)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "invalid.txt")
	part.Write([]byte("this is not valid yaml or json"))
	writer.Close()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/nodes/import", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, _ := as.GenerateToken("testuser", "editor")
	req.Header.Set("Authorization", "Bearer "+token)

	router.ServeHTTP(w, req)
	assert.Equal(t, 400, w.Code)
}

func TestImport_MissingFile(t *testing.T) {
	_, _, router := featuresTestSetup(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/nodes/import", strings.NewReader(""))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=xxx")

	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, _ := as.GenerateToken("testuser", "editor")
	req.Header.Set("Authorization", "Bearer "+token)

	router.ServeHTTP(w, req)
	assert.Equal(t, 400, w.Code)
}

func TestImport_AsViewer_Forbidden(t *testing.T) {
	_, _, router := featuresTestSetup(t)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "nodes.yaml")
	part.Write([]byte("version: 1.0\nnodes: []"))
	writer.Close()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/nodes/import", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, _ := as.GenerateToken("testuser", "viewer")
	req.Header.Set("Authorization", "Bearer "+token)

	router.ServeHTTP(w, req)
	assert.Equal(t, 403, w.Code)
}

func TestPing_UnreachableNodes(t *testing.T) {
	_, _, router := featuresTestSetup(t)

	body := map[string]interface{}{
		"node_ids": []string{"node-1", "node-2"},
	}
	w := authRequest(t, router, "POST", "/api/v1/nodes/ping", body, "editor")
	assert.Equal(t, 200, w.Code)

	var resp struct {
		Results []pingResult `json:"results"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Len(t, resp.Results, 2)

	for _, r := range resp.Results {
		assert.False(t, r.Success)
		assert.NotEmpty(t, r.Error)
	}
}

func TestPing_EmptyNodeIDs(t *testing.T) {
	_, _, router := featuresTestSetup(t)

	body := map[string]interface{}{
		"node_ids": []string{},
	}
	w := authRequest(t, router, "POST", "/api/v1/nodes/ping", body, "editor")
	assert.Equal(t, 400, w.Code)
}

func TestPing_NonExistentNode(t *testing.T) {
	_, _, router := featuresTestSetup(t)

	body := map[string]interface{}{
		"node_ids": []string{"nonexistent"},
	}
	w := authRequest(t, router, "POST", "/api/v1/nodes/ping", body, "editor")
	assert.Equal(t, 200, w.Code)

	var resp struct {
		Results []pingResult `json:"results"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Len(t, resp.Results, 0)
}

func TestPing_AsViewer_Forbidden(t *testing.T) {
	_, _, router := featuresTestSetup(t)

	body := map[string]interface{}{
		"node_ids": []string{"node-1"},
	}
	w := authRequest(t, router, "POST", "/api/v1/nodes/ping", body, "viewer")
	assert.Equal(t, 403, w.Code)
}

func TestCheck_NoCredentials(t *testing.T) {
	_, _, router := featuresTestSetup(t)

	body := map[string]interface{}{
		"node_ids": []string{"node-1", "node-2"},
	}
	w := authRequest(t, router, "POST", "/api/v1/nodes/check", body, "editor")
	assert.Equal(t, 200, w.Code)

	var resp struct {
		Results []checkResult `json:"results"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Len(t, resp.Results, 2)

	for _, r := range resp.Results {
		assert.False(t, r.Success)
		assert.Contains(t, r.Error, "no credentials")
	}
}

func TestCheck_EmptyNodeIDs(t *testing.T) {
	_, _, router := featuresTestSetup(t)

	body := map[string]interface{}{
		"node_ids": []string{},
	}
	w := authRequest(t, router, "POST", "/api/v1/nodes/check", body, "editor")
	assert.Equal(t, 400, w.Code)
}

func TestCheck_UpdatesNodeStatus(t *testing.T) {
	_, h, router := featuresTestSetup(t)

	body := map[string]interface{}{
		"node_ids": []string{"node-1"},
	}
	w := authRequest(t, router, "POST", "/api/v1/nodes/check", body, "editor")
	assert.Equal(t, 200, w.Code)

	var status string
	h.db.QueryRow("SELECT status FROM nodes WHERE id = 'node-1'").Scan(&status)
	assert.Equal(t, "offline", status)
}

func TestCheck_AsViewer_Forbidden(t *testing.T) {
	_, _, router := featuresTestSetup(t)

	body := map[string]interface{}{
		"node_ids": []string{"node-1"},
	}
	w := authRequest(t, router, "POST", "/api/v1/nodes/check", body, "viewer")
	assert.Equal(t, 403, w.Code)
}

func TestExportImport_RoundTrip(t *testing.T) {
	db, _, router := featuresTestSetup(t)

	exportBody := map[string]interface{}{
		"node_ids": []string{"node-1", "node-2"},
		"format":   "yaml",
	}
	w := authRequest(t, router, "POST", "/api/v1/nodes/export", exportBody, "editor")
	assert.Equal(t, 200, w.Code)
	exportedData := w.Body.Bytes()

	db.Exec("DELETE FROM nodes WHERE id IN ('node-1', 'node-2')")

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "nodes.yaml")
	part.Write(exportedData)
	writer.Close()

	w2 := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/nodes/import", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, _ := as.GenerateToken("testuser", "editor")
	req.Header.Set("Authorization", "Bearer "+token)

	router.ServeHTTP(w2, req)
	assert.Equal(t, 200, w2.Code)

	var resp importResult
	json.Unmarshal(w2.Body.Bytes(), &resp)
	assert.Equal(t, 2, resp.Success)

	var count int
	db.QueryRow("SELECT COUNT(*) FROM nodes WHERE id IN ('node-1', 'node-2')").Scan(&count)
	assert.Equal(t, 2, count)
}

func TestPing_WithRealServer(t *testing.T) {
	db, _, router := featuresTestSetup(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	addr := ln.Addr().String()
	parts := strings.Split(addr, ":")
	port, _ := strconv.Atoi(parts[1])

	db.Exec("INSERT INTO nodes (id, name, address, port, user, status) VALUES ('ping-test', 'Ping Test', '127.0.0.1', ?, 'root', 'unknown')", port)

	body := map[string]interface{}{
		"node_ids": []string{"ping-test"},
	}
	w := authRequest(t, router, "POST", "/api/v1/nodes/ping", body, "editor")
	assert.Equal(t, 200, w.Code)

	var resp struct {
		Results []pingResult `json:"results"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	require.Len(t, resp.Results, 1)
	assert.True(t, resp.Results[0].Success)
	assert.GreaterOrEqual(t, resp.Results[0].LatencyMs, 0.0)
}

func TestPing_IPv6Address(t *testing.T) {
	db, _, router := featuresTestSetup(t)

	ln, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	_, portStr, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)
	port, _ := strconv.Atoi(portStr)

	db.Exec("INSERT INTO nodes (id, name, address, port, user, status) VALUES ('ping-v6', 'Ping V6', '::1', ?, 'root', 'unknown')", port)

	body := map[string]interface{}{
		"node_ids": []string{"ping-v6"},
	}
	w := authRequest(t, router, "POST", "/api/v1/nodes/ping", body, "editor")
	assert.Equal(t, 200, w.Code)

	var resp struct {
		Results []pingResult `json:"results"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	require.Len(t, resp.Results, 1)
	assert.True(t, resp.Results[0].Success, "IPv6 ping failed: %s", resp.Results[0].Error)
}

func TestImport_EmptyNodes(t *testing.T) {
	_, _, router := featuresTestSetup(t)

	yamlContent := `version: "1.0"
nodes: []
`

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "nodes.yaml")
	part.Write([]byte(yamlContent))
	writer.Close()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/nodes/import", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, _ := as.GenerateToken("testuser", "editor")
	req.Header.Set("Authorization", "Bearer "+token)

	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)

	var resp importResult
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Success)
	assert.Equal(t, 0, resp.Failed)
}

func TestImport_InvalidNodeData(t *testing.T) {
	_, _, router := featuresTestSetup(t)

	yamlContent := `version: "1.0"
nodes:
  - id: ""
    name: No ID
    address: 10.0.0.1
  - id: no-address
    name: No Address
    address: ""
`

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, _ := writer.CreateFormFile("file", "nodes.yaml")
	part.Write([]byte(yamlContent))
	writer.Close()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/nodes/import", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, _ := as.GenerateToken("testuser", "editor")
	req.Header.Set("Authorization", "Bearer "+token)

	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)

	var resp importResult
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, 0, resp.Success)
	assert.Equal(t, 2, resp.Failed)
}
