package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	monitorSvc "github.com/cangyunye/go-owl/cmd/plugins/serve/monitor"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	owlmonitor "github.com/cangyunye/go-owl/internal/monitor"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

const bindingTestSecret = "test-secret-32byte-long-string!!"

func alertBindingTestSetup(t *testing.T) (*gin.Engine, *owlmonitor.Store, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	file := filepath.Join(t.TempDir(), "owl.db")
	st, err := owlmonitor.OpenStore(file)
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	db, err := sql.Open("sqlite", file)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	now := time.Now().Unix()
	for _, a := range []*owlmonitor.Alert{
		{ID: "AL-A", AlertTypeID: "OWL-MEM-001", NodeID: "n1", Severity: owlmonitor.SeverityWarning,
			Status: owlmonitor.StatusOpen, Message: "x", FirstSeen: now, LastSeen: now},
		{ID: "AL-B", AlertTypeID: "OWL-MEM-001", NodeID: "n2", Severity: owlmonitor.SeverityWarning,
			Status: owlmonitor.StatusOpen, Message: "x", FirstSeen: now, LastSeen: now},
	} {
		require.NoError(t, st.InsertAlert(a))
	}

	svc := &monitorSvc.Service{Store: st}
	h := NewMonitorHandler(db, svc)

	us := store.NewUserStore(db)
	as := service.NewAuthService(bindingTestSecret)
	ah := NewAuthHandler(us, as)

	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(ah.AuthMiddleware())
	{
		viewer := api.Group("", ah.RBACMiddleware("viewer", "editor", "operator", "admin"))
		viewer.GET("/alerts/:id/bindings", h.ListAlertBindings)
		viewer.GET("/alerts/:id/binding-runs", h.ListAlertBindingRuns)
		operator := api.Group("", ah.RBACMiddleware("operator", "admin"))
		operator.POST("/alerts/:id/bindings/run", h.RunAlertBindings)
		admin := api.Group("", ah.RBACMiddleware("admin"))
		admin.POST("/alerts/:id/bindings", h.CreateAlertBinding)
		admin.PUT("/alert-bindings/:id", h.UpdateAlertBinding)
		admin.DELETE("/alert-bindings/:id", h.DeleteAlertBinding)
	}

	token, _ := as.GenerateToken("admin", "admin")
	return r, st, token
}

func doReq(r *gin.Engine, token, method, path, body string) *httptest.ResponseRecorder {
	var buf *bytes.Buffer
	if body != "" {
		buf = bytes.NewBufferString(body)
	} else {
		buf = &bytes.Buffer{}
	}
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(method, path, buf)
	req.Header.Set("Authorization", "Bearer "+token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	r.ServeHTTP(w, req)
	return w
}

func TestAlertBindingHandler_RBACAndCRUD(t *testing.T) {
	r, st, adminToken := alertBindingTestSetup(t)
	as := service.NewAuthService(bindingTestSecret)
	viewerToken, _ := as.GenerateToken("viewer", "viewer")

	// viewer 不得创建
	w := doReq(r, viewerToken, "POST", "/api/v1/alerts/AL-A/bindings",
		`{"kind":"script","name":"x","content":"echo hi"}`)
	assert.Equal(t, 403, w.Code, "viewer 创建绑定应 403")

	// admin 创建（script）
	w = doReq(r, adminToken, "POST", "/api/v1/alerts/AL-A/bindings",
		`{"kind":"script","name":"重启服务","content":"systemctl restart nginx","auto_exec":true,"exec_mode":"sequential","seq":1}`)
	require.Equal(t, 200, w.Code)
	var created struct {
		Item owlmonitor.AlertBinding `json:"item"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.Equal(t, "重启服务", created.Item.Name)
	require.True(t, created.Item.AutoExec)
	require.NotEmpty(t, created.Item.CreatedBy, "应记录创建人")

	// viewer 可查看
	w = doReq(r, viewerToken, "GET", "/api/v1/alerts/AL-A/bindings", "")
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "重启服务")

	// viewer 不得执行
	w = doReq(r, viewerToken, "POST", "/api/v1/alerts/AL-A/bindings/run",
		`{"binding_ids":["`+created.Item.ID+`"],"mode":"sequential"}`)
	assert.Equal(t, 403, w.Code)

	// admin 更新（关闭自动执行）
	w = doReq(r, adminToken, "PUT", "/api/v1/alert-bindings/"+created.Item.ID, `{"auto_exec":false}`)
	assert.Equal(t, 200, w.Code)

	// 非法 exec_mode
	w = doReq(r, adminToken, "PUT", "/api/v1/alert-bindings/"+created.Item.ID, `{"exec_mode":"parallel"}`)
	assert.Equal(t, 400, w.Code)

	// 删除
	w = doReq(r, adminToken, "DELETE", "/api/v1/alert-bindings/"+created.Item.ID, "")
	assert.Equal(t, 200, w.Code)
	items, err := st.ListAlertBindings("AL-A")
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestAlertBindingHandler_Run_CrossAlertRejected(t *testing.T) {
	r, st, adminToken := alertBindingTestSetup(t)
	now := time.Now().Unix()
	require.NoError(t, st.CreateAlertBinding(owlmonitor.AlertBinding{ID: "AB-A", AlertID: "AL-A",
		Kind: "script", Name: "a", Content: "echo a", Seq: 1, CreatedAt: now}))
	require.NoError(t, st.CreateAlertBinding(owlmonitor.AlertBinding{ID: "AB-B", AlertID: "AL-B",
		Kind: "script", Name: "b", Content: "echo b", Seq: 1, CreatedAt: now}))

	// 携带 AL-B 的绑定到 AL-A 执行 → 400
	w := doReq(r, adminToken, "POST", "/api/v1/alerts/AL-A/bindings/run",
		`{"binding_ids":["AB-A","AB-B"],"mode":"sequential"}`)
	assert.Equal(t, 400, w.Code)
	assert.Contains(t, w.Body.String(), "不属于该告警")

	// 只跑自己的 → 202
	w = doReq(r, adminToken, "POST", "/api/v1/alerts/AL-A/bindings/run",
		`{"binding_ids":["AB-A"],"mode":"sequential"}`)
	assert.Equal(t, 202, w.Code)
}

func TestAlertBindingHandler_Run_MissingBinding(t *testing.T) {
	r, _, adminToken := alertBindingTestSetup(t)
	w := doReq(r, adminToken, "POST", "/api/v1/alerts/AL-A/bindings/run",
		`{"binding_ids":["AB-NOPE"],"mode":"sequential"}`)
	assert.Equal(t, 400, w.Code)
}
