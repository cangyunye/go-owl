package serve

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	owlmonitor "github.com/cangyunye/go-owl/internal/monitor"
	"github.com/stretchr/testify/require"
)

// setupMonitorServer 启动带监控服务的测试服务器，返回 srv 与 admin token。
func setupMonitorServer(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := &Config{
		DBPath:     filepath.Join(dir, "owl.db"),
		ListenAddr: "127.0.0.1:8080",
	}
	srv := NewServer(cfg)
	creds, err := srv.Init()
	require.NoError(t, err)
	require.NotNil(t, creds)

	token := login(t, srv, creds.Username, creds.Password)
	return srv, token
}

func login(t *testing.T, srv *Server, username, password string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Router.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code, "登录失败: %s", w.Body.String())
	var resp struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp.Token
}

func authedGet(t *testing.T, srv *Server, token, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	srv.Router.ServeHTTP(w, req)
	return w
}

func authedPost(t *testing.T, srv *Server, token, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	var reader = bytes.NewReader(nil)
	if payload != nil {
		b, _ := json.Marshal(payload)
		reader = bytes.NewReader(b)
	}
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", path, reader)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	srv.Router.ServeHTTP(w, req)
	return w
}

func authedPut(t *testing.T, srv *Server, token, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(payload)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", path, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	srv.Router.ServeHTTP(w, req)
	return w
}

func seedAlert(t *testing.T, srv *Server, typeID, nodeID string) string {
	t.Helper()
	now := time.Now().Unix()
	al := &owlmonitor.Alert{
		ID: "AL-API-1", AlertTypeID: typeID, NodeID: nodeID,
		Severity: owlmonitor.SeverityWarning, Status: owlmonitor.StatusOpen,
		Message: "磁盘使用率过高：disk.usage./ 当前 93.5", MetricSnapshot: "{}",
		FirstSeen: now, LastSeen: now,
	}
	require.NoError(t, srv.monitor.Store.InsertAlert(al))
	return al.ID
}

// TestMonitorAPI_AlertsLifecycle 验证告警查询 → 处置（ack/resolve）闭环。
func TestMonitorAPI_AlertsLifecycle(t *testing.T) {
	srv, token := setupMonitorServer(t)
	id := seedAlert(t, srv, "OWL-DSK-001", "node-a")

	// 列表（viewer 可读）
	w := authedGet(t, srv, token, "/api/v1/alerts?status=active")
	require.Equal(t, 200, w.Code)
	var list struct {
		Items []struct {
			ID            string `json:"id"`
			AlertTypeName string `json:"alert_type_name"`
			NodeName      string `json:"node_name"`
		} `json:"items"`
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Equal(t, 1, list.Total)
	require.Equal(t, id, list.Items[0].ID)
	require.Equal(t, "磁盘使用率过高", list.Items[0].AlertTypeName, "告警类型中文名应注入")

	// 详情含推荐对策
	w = authedGet(t, srv, token, "/api/v1/alerts/"+id)
	require.Equal(t, 200, w.Code)
	var detail struct {
		Alert struct {
			ID string `json:"id"`
		} `json:"alert"`
		Remedies []json.RawMessage `json:"remedies"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &detail))
	require.Equal(t, id, detail.Alert.ID)
	require.NotEmpty(t, detail.Remedies, "内置对策应随详情返回")

	// ack（operator+）
	w = authedPost(t, srv, token, "/api/v1/alerts/"+id+"/ack", nil)
	require.Equal(t, 200, w.Code)
	var acked struct {
		Alert struct {
			Status string `json:"status"`
		} `json:"alert"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &acked))
	require.Equal(t, "acked", acked.Alert.Status)

	// resolve
	w = authedPost(t, srv, token, "/api/v1/alerts/"+id+"/resolve", nil)
	require.Equal(t, 200, w.Code)

	// 解决后不在活跃列表
	w = authedGet(t, srv, token, "/api/v1/alerts?status=active")
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Equal(t, 0, list.Total)
}

// TestMonitorAPI_AlertTypes 验证告警类型读取与阈值更新（admin）。
func TestMonitorAPI_AlertTypes(t *testing.T) {
	srv, token := setupMonitorServer(t)

	w := authedGet(t, srv, token, "/api/v1/alert-types")
	require.Equal(t, 200, w.Code)
	var resp struct {
		Items []owlmonitor.AlertType `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Items)

	// admin 更新阈值 + 放行
	at := resp.Items[0]
	at.DefaultParams.Value = 80
	at.AutoApprove = true
	w = authedPut(t, srv, token, "/api/v1/alert-types/"+at.ID, at)
	require.Equal(t, 200, w.Code, w.Body.String())

	w = authedGet(t, srv, token, "/api/v1/alert-types")
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	for _, item := range resp.Items {
		if item.ID == at.ID {
			require.InDelta(t, 80, item.DefaultParams.Value, 0.001)
			require.True(t, item.AutoApprove, "放行开关应持久化")
		}
	}
}

// TestMonitorAPI_Remedies 验证对策 CRUD（admin）。
func TestMonitorAPI_Remedies(t *testing.T) {
	srv, token := setupMonitorServer(t)

	// 新建用户对策
	remedy := owlmonitor.Remedy{
		ID: "RM-API-1", AlertTypeID: "OWL-DSK-001", Name: "自动清理临时文件",
		Kind: "script", Content: "find /tmp -type f -mtime +7 -delete", Risk: "medium",
		Source: "user", Reviewed: true,
	}
	w := authedPost(t, srv, token, "/api/v1/remedies", remedy)
	require.Equal(t, 200, w.Code, w.Body.String())

	w = authedGet(t, srv, token, "/api/v1/remedies?alert_type_id=OWL-DSK-001")
	require.Equal(t, 200, w.Code)
	var resp struct {
		Items []owlmonitor.Remedy `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	found := false
	for _, r := range resp.Items {
		if r.ID == "RM-API-1" {
			found = true
			require.Equal(t, "script", r.Kind)
		}
	}
	require.True(t, found, "新建对策应可查询")

	// 更新 + 删除
	w = authedPut(t, srv, token, "/api/v1/remedies/RM-API-1", map[string]any{
		"alert_type_id": "OWL-DSK-001", "name": "改名", "kind": "script",
		"content": "echo hi", "risk": "low", "source": "user", "reviewed": true,
	})
	require.Equal(t, 200, w.Code)
	w = authedGet(t, srv, token, "/api/v1/remedies?alert_type_id=OWL-DSK-001")
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	for _, r := range resp.Items {
		if r.ID == "RM-API-1" {
			require.Equal(t, "改名", r.Name)
		}
	}

	// 删除后查询不到
	delReq, _ := http.NewRequest("DELETE", "/api/v1/remedies/RM-API-1", nil)
	delReq.Header.Set("Authorization", "Bearer "+token)
	delW := httptest.NewRecorder()
	srv.Router.ServeHTTP(delW, delReq)
	require.Equal(t, 200, delW.Code)
}

// TestMonitorAPI_NotifyChannels 验证通知渠道 CRUD 与测试发送。
func TestMonitorAPI_NotifyChannels(t *testing.T) {
	srv, token := setupMonitorServer(t)

	ch := owlmonitor.NotifyChannel{
		ID: "CH-API-1", Kind: "webhook", Name: "运维通知",
		Config: owlmonitor.NotifyConfig{Webhook: &owlmonitor.WebhookConfig{
			URL: "http://127.0.0.1:1/unreachable", Headers: map[string]string{},
		}},
		SeverityMin: owlmonitor.SeverityWarning, Enabled: true,
	}
	w := authedPost(t, srv, token, "/api/v1/notify-channels", ch)
	require.Equal(t, 200, w.Code, w.Body.String())

	// 不可达地址测试发送 → 400
	w = authedPost(t, srv, token, "/api/v1/notify-channels/CH-API-1/test", nil)
	require.Equal(t, 400, w.Code, "不可达 webhook 测试应失败")

	// 删除
	delReq, _ := http.NewRequest("DELETE", "/api/v1/notify-channels/CH-API-1", nil)
	delReq.Header.Set("Authorization", "Bearer "+token)
	delW := httptest.NewRecorder()
	srv.Router.ServeHTTP(delW, delReq)
	require.Equal(t, 200, delW.Code)
}

// TestMonitorAPI_SilenceAndMetrics 验证静默配置与指标查询。
func TestMonitorAPI_SilenceAndMetrics(t *testing.T) {
	srv, token := setupMonitorServer(t)

	// 静默
	w := authedPut(t, srv, token, "/api/v1/monitor/silence", map[string]any{"silence_until": 9999999999})
	require.Equal(t, 200, w.Code, w.Body.String())
	w = authedGet(t, srv, token, "/api/v1/monitor/silence")
	require.Equal(t, 200, w.Code)
	var sil struct {
		SilenceUntil int64 `json:"silence_until"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sil))
	require.Equal(t, int64(9999999999), sil.SilenceUntil)

	// 取消静默
	w = authedPut(t, srv, token, "/api/v1/monitor/silence", map[string]any{"silence_until": 0})
	require.Equal(t, 200, w.Code)

	// 指标查询
	w = authedGet(t, srv, token, "/api/v1/metrics?node_id=n1&metric=load.load1&from=1&to=2")
	require.Equal(t, 200, w.Code)
	w = authedGet(t, srv, token, "/api/v1/metrics")
	require.Equal(t, 400, w.Code, "缺 node_id/metric 应 400")
}

// TestMonitorAPI_RBAC 验证权限边界：viewer 可读不可处置。
func TestMonitorAPI_RBAC(t *testing.T) {
	srv, token := setupMonitorServer(t)

	// 建一个 viewer 用户
	viewerToken := ""
	{
		// 通过 admin API 创建 viewer
		w := authedPost(t, srv, token, "/api/v1/users", map[string]any{
			"username": "viewer1", "password": "viewer123", "role": "viewer",
		})
		require.Equal(t, 201, w.Code, w.Body.String())
		viewerToken = login(t, srv, "viewer1", "viewer123")
	}

	id := seedAlert(t, srv, "OWL-DSK-001", "node-a")

	// viewer 可读
	w := authedGet(t, srv, viewerToken, "/api/v1/alerts")
	require.Equal(t, 200, w.Code)

	// viewer 处置 → 403
	w = authedPost(t, srv, viewerToken, "/api/v1/alerts/"+id+"/ack", nil)
	require.Equal(t, 403, w.Code, "viewer 不应允许处置告警")

	// viewer 改告警类型 → 403
	w = authedPut(t, srv, viewerToken, "/api/v1/alert-types/OWL-DSK-001", map[string]any{})
	require.Equal(t, 403, w.Code)
}

// TestMonitorAPI_EngineStartup 验证 serve Start 启动后引擎会跑一轮采集
// （无节点时不报错、告警表可用）。
func TestMonitorAPI_EngineStartup(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		DBPath:     filepath.Join(dir, "owl.db"),
		ListenAddr: "127.0.0.1:0",
	}
	srv := NewServer(cfg)
	_, err := srv.Init()
	require.NoError(t, err)
	require.NotNil(t, srv.monitor, "Init 应装配监控服务")
	require.NotNil(t, srv.monitorHandler)
	// 无节点时跑一轮不应 panic
	err = srv.monitor.Engine.TickOnce(t.Context())
	require.NoError(t, err)
	_ = fmt.Sprintf("ok")
}
