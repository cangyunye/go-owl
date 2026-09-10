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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMonitorAPI_ValidationErrorIsEnglish 校验监控 API 的校验类错误文案统一为英文。
func TestMonitorAPI_ValidationErrorIsEnglish(t *testing.T) {
	srv, token := setupMonitorServer(t)

	w := authedGet(t, srv, token, "/api/v1/metrics")
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "missing node_id/metric")
}

// TestMonitorAPI_500DoesNotLeakInternalError 校验 5xx 不把内部错误细节回传客户端：
// 底层报错（如 sql: database is closed）只应进服务端日志。
func TestMonitorAPI_500DoesNotLeakInternalError(t *testing.T) {
	srv, token := setupMonitorServer(t)
	require.NoError(t, srv.monitor.Store.Close())

	w := authedGet(t, srv, token, "/api/v1/alerts")
	require.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "query alerts failed")
	body := w.Body.String()
	assert.NotContains(t, body, "database is closed")
	assert.NotContains(t, body, "sql:")
}

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

func authedDelete(t *testing.T, srv *Server, token, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
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

// TestMonitorAPI_RemedyPlan 验证处置计划：创建 → 真实本地执行 → 查询进度 → 反馈。
// 节点指向 127.0.0.1（本地执行器），脚本经 base64 管道真实运行。
func TestMonitorAPI_RemedyPlan(t *testing.T) {
	srv, token := setupMonitorServer(t)

	// 注册本机节点（本地执行器路径，无需真实 SSH）
	nodeReq, _ := json.Marshal(map[string]any{
		"id": "local-test", "name": "本机", "address": "127.0.0.1",
		"port": 22, "user": "local", "groups": []string{"web"},
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/nodes", bytes.NewReader(nodeReq))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	srv.Router.ServeHTTP(w, req)
	require.Equal(t, 201, w.Code, w.Body.String())

	// 建一个可执行脚本对策（属于 OWL-DSK-001）
	remedy := owlmonitor.Remedy{
		ID: "RM-EXEC", AlertTypeID: "OWL-DSK-001", Name: "测试脚本",
		Kind: "script", Content: "echo remedy-ok && hostname", Risk: "low",
		Source: "user", Reviewed: true,
	}
	w = authedPost(t, srv, token, "/api/v1/remedies", remedy)
	require.Equal(t, 200, w.Code, w.Body.String())

	// 种子告警（local-test 节点，OWL-DSK-001）
	id := seedAlert(t, srv, "OWL-DSK-001", "local-test")

	// 创建处置计划
	w = authedPost(t, srv, token, "/api/v1/alerts/"+id+"/plans", map[string]any{
		"remedy_ids": []string{"RM-EXEC"}, "stop_on_error": true,
	})
	require.Equal(t, 202, w.Code, w.Body.String())
	var created struct {
		Run struct {
			ID    string `json:"id"`
			Steps []struct {
				Order  int    `json:"order"`
				Kind   string `json:"kind"`
				Status string `json:"status"`
				NodeID string `json:"node_id"`
			} `json:"steps"`
		} `json:"run"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.Equal(t, "local-test", created.Run.Steps[0].NodeID, "步骤应绑定告警节点")
	runID := created.Run.ID

	// 轮询直到执行完成（本地执行很快）
	deadline := time.Now().Add(5 * time.Second)
	var run owlmonitor.RemedyRun
	for time.Now().Before(deadline) {
		w = authedGet(t, srv, token, "/api/v1/plans/"+runID)
		require.Equal(t, 200, w.Code)
		var resp struct {
			Run owlmonitor.RemedyRun `json:"run"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		run = resp.Run
		if run.IsTerminal() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Equal(t, owlmonitor.RunDone, run.Status, "本地脚本应执行成功")
	require.Len(t, run.Steps, 1)
	require.Equal(t, owlmonitor.StepSuccess, run.Steps[0].Status)
	require.Contains(t, run.Steps[0].Output, "remedy-ok", "应捕获脚本输出")

	// 对策反馈计数累加
	rm, _, _ := srv.monitor.Store.GetRemedy("RM-EXEC")
	require.Equal(t, 1, rm.ExecCount)
	require.Equal(t, 1, rm.SuccessCount)

	// 处置记录可按告警查询
	w = authedGet(t, srv, token, "/api/v1/alerts/"+id+"/plans")
	require.Equal(t, 200, w.Code)
	var list struct {
		Items []owlmonitor.RemedyRun `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list.Items, 1)
}

// TestMonitorAPI_RemedyPlan_Validation 验证非法创建被拒绝。
func TestMonitorAPI_RemedyPlan_Validation(t *testing.T) {
	srv, token := setupMonitorServer(t)
	id := seedAlert(t, srv, "OWL-DSK-001", "node-a")

	// 空对策列表 → 400
	w := authedPost(t, srv, token, "/api/v1/alerts/"+id+"/plans", map[string]any{"remedy_ids": []string{}})
	require.Equal(t, 400, w.Code)

	// 不存在的对策 → 400
	w = authedPost(t, srv, token, "/api/v1/alerts/"+id+"/plans", map[string]any{"remedy_ids": []string{"RM-NOPE"}})
	require.Equal(t, 400, w.Code)

	// 类型不匹配的对策 → 400（建一条 MEM 类型对策用于 DSK 告警）
	remedy := owlmonitor.Remedy{
		ID: "RM-WRONG", AlertTypeID: "OWL-MEM-001", Name: "内存对策",
		Kind: "script", Content: "echo x", Risk: "low", Source: "user", Reviewed: true,
	}
	w = authedPost(t, srv, token, "/api/v1/remedies", remedy)
	require.Equal(t, 200, w.Code)
	w = authedPost(t, srv, token, "/api/v1/alerts/"+id+"/plans", map[string]any{"remedy_ids": []string{"RM-WRONG"}})
	require.Equal(t, 400, w.Code, "类型不匹配应拒绝")

	// viewer 不能创建
	srv2, token2 := setupMonitorServer(t)
	w = authedPost(t, srv2, token2, "/api/v1/users", map[string]any{
		"username": "viewer1", "password": "viewer123", "role": "viewer",
	})
	require.Equal(t, 201, w.Code)
	vt := login(t, srv2, "viewer1", "viewer123")
	w = authedPost(t, srv2, vt, "/api/v1/alerts/"+id+"/plans", map[string]any{"remedy_ids": []string{"RM-X"}})
	require.Equal(t, 403, w.Code, "viewer 不应允许创建处置计划")
}

// TestMonitorAPI_RemedyPlan_ApprovalFlow 验证待审批计划：批准 → 恢复执行 → 完成。
func TestMonitorAPI_RemedyPlan_ApprovalFlow(t *testing.T) {
	srv, token := setupMonitorServer(t)

	// 注册本机节点 + 种子告警
	nodeReq, _ := json.Marshal(map[string]any{
		"id": "local-test", "name": "本机", "address": "127.0.0.1",
		"port": 22, "user": "local", "groups": []string{"web"},
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/nodes", bytes.NewReader(nodeReq))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	srv.Router.ServeHTTP(w, req)
	require.Equal(t, 201, w.Code, w.Body.String())
	id := seedAlert(t, srv, "OWL-DSK-001", "local-test")

	// 直接构造一个含待审批步骤的计划（模拟自愈管线产生）
	now := time.Now().Unix()
	run := &owlmonitor.RemedyRun{
		ID: "RUN-APPR", AlertID: id, NodeID: "local-test",
		Status: owlmonitor.RunWaitingApproval, StopOnError: true,
		CreatedBy: "auto-heal", CreatedAt: now, UpdatedAt: now,
		Steps: []owlmonitor.RemedyStep{
			{Order: 0, RemedyID: "RM-APPR", Name: "待审批脚本", Kind: "script",
				Content: "echo approved-ran", Status: owlmonitor.StepPendingApproval, NodeID: "local-test"},
		},
	}
	require.NoError(t, srv.monitor.Store.CreateRemedyRun(run))

	// 状态确认
	w = authedGet(t, srv, token, "/api/v1/plans/RUN-APPR")
	require.Equal(t, 200, w.Code)
	var resp struct {
		Run owlmonitor.RemedyRun `json:"run"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, owlmonitor.RunWaitingApproval, resp.Run.Status)

	// 批准
	w = authedPost(t, srv, token, "/api/v1/plans/RUN-APPR/approve", nil)
	require.Equal(t, 200, w.Code, w.Body.String())

	// 轮询至完成
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		w = authedGet(t, srv, token, "/api/v1/plans/RUN-APPR")
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		if resp.Run.IsTerminal() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Equal(t, owlmonitor.RunDone, resp.Run.Status)
	require.Equal(t, owlmonitor.StepSuccess, resp.Run.Steps[0].Status)
	require.Contains(t, resp.Run.Steps[0].Output, "approved-ran", "批准后应真实执行")
}

// TestMonitorAPI_RemedyPlan_RejectFlow 验证拒绝后步骤跳过。
func TestMonitorAPI_RemedyPlan_RejectFlow(t *testing.T) {
	srv, token := setupMonitorServer(t)
	id := seedAlert(t, srv, "OWL-DSK-001", "node-a")

	now := time.Now().Unix()
	run := &owlmonitor.RemedyRun{
		ID: "RUN-REJ", AlertID: id, NodeID: "node-a",
		Status: owlmonitor.RunWaitingApproval, StopOnError: true,
		CreatedBy: "auto-heal", CreatedAt: now, UpdatedAt: now,
		Steps: []owlmonitor.RemedyStep{
			{Order: 0, RemedyID: "RM-X", Name: "待审批", Kind: "script",
				Content: "echo x", Status: owlmonitor.StepPendingApproval},
		},
	}
	require.NoError(t, srv.monitor.Store.CreateRemedyRun(run))

	w := authedPost(t, srv, token, "/api/v1/plans/RUN-REJ/reject", nil)
	require.Equal(t, 200, w.Code, w.Body.String())

	deadline := time.Now().Add(5 * time.Second)
	var resp struct {
		Run owlmonitor.RemedyRun `json:"run"`
	}
	for time.Now().Before(deadline) {
		w = authedGet(t, srv, token, "/api/v1/plans/RUN-REJ")
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		if resp.Run.IsTerminal() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.Equal(t, owlmonitor.RunDone, resp.Run.Status)
	require.Equal(t, owlmonitor.StepSkipped, resp.Run.Steps[0].Status)
	require.Contains(t, resp.Run.Steps[0].Output, "拒绝")
}
