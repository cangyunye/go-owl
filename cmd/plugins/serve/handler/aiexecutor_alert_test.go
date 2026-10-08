package handler

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ai2 "github.com/cangyunye/go-owl/internal/ai"
	owlmonitor "github.com/cangyunye/go-owl/internal/monitor"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// WebExecutor 告警方法单测：临时 owl.db（monitor 表 + nodes 表同库），
// 复用 monitor.OpenStore 造种子数据，验证告警码归一/节点名解析/分组过滤。

func webAlertExecutorSetup(t *testing.T) (*WebExecutor, context.Context) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "owl.db")
	st, err := owlmonitor.OpenStore(file)
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	db, err := sql.Open("sqlite", file)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY, name TEXT, address TEXT, port INTEGER DEFAULT 22,
		user TEXT, password TEXT, ssh_key TEXT, status TEXT DEFAULT 'unknown',
		groups TEXT DEFAULT '[]', labels TEXT DEFAULT '{}',
		proxy_jump TEXT DEFAULT '', created_at TIMESTAMP, updated_at TIMESTAMP)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO nodes (id, name, address, status, groups) VALUES
		('n1', 'web-01', '10.0.0.1', 'online', '["web"]'),
		('n2', 'db-01', '10.0.0.2', 'online', '["db"]')`)
	require.NoError(t, err)

	now := time.Now().Unix()
	for _, a := range []*owlmonitor.Alert{
		{ID: "AL-1", AlertTypeID: "OWL-DSK-001", NodeID: "n1", Severity: owlmonitor.SeverityWarning,
			Status: owlmonitor.StatusOpen, Message: "磁盘使用率过高: / 95.2%", FirstSeen: now, LastSeen: now},
		{ID: "AL-2", AlertTypeID: "OWL-OSS-001", NodeID: "n2", Severity: owlmonitor.SeverityCritical,
			Status: owlmonitor.StatusOpen, Message: "节点失联", FirstSeen: now, LastSeen: now},
		{ID: "AL-3", AlertTypeID: "OWL-DSK-001", NodeID: "n1", Severity: owlmonitor.SeverityWarning,
			Status: owlmonitor.StatusResolved, Message: "磁盘使用率过高", FirstSeen: now - 7200, LastSeen: now - 3600},
	} {
		require.NoError(t, st.InsertAlert(a))
	}

	e := NewWebExecutor(db, nil, nil, nil, nil, nil, nil, nil, false)
	e.SetMonitorStore(st)
	ctx := WithIdentity(context.Background(), ExecIdentity{Username: "tester", Role: "viewer"})
	return e, ctx
}

func TestWebExecutor_ListAlerts_All(t *testing.T) {
	e, ctx := webAlertExecutorSetup(t)
	res, err := e.ListAlerts(ctx, ai2.AlertListParams{})
	require.NoError(t, err)
	// 默认 status=active：2 条 open，已恢复的 AL-3 不出现
	for _, want := range []string{"OWL-DSK-001", "OWL-OSS-001", "web-01", "db-01", "2 条"} {
		if !strings.Contains(res.Text, want) {
			t.Errorf("output missing %q:\n%s", want, res.Text)
		}
	}
	if strings.Contains(res.Text, "AL-3") {
		t.Errorf("resolved alert should be excluded:\n%s", res.Text)
	}
}

func TestWebExecutor_ListAlerts_ByCodeNormalized(t *testing.T) {
	e, ctx := webAlertExecutorSetup(t)
	res, err := e.ListAlerts(ctx, ai2.AlertListParams{AlertTypeID: "owl_dsk_001"})
	require.NoError(t, err)
	if !strings.Contains(res.Text, "web-01") || strings.Contains(res.Text, "db-01") {
		t.Errorf("code filter should match only web-01:\n%s", res.Text)
	}
}

func TestWebExecutor_ListAlerts_UnknownCode(t *testing.T) {
	e, ctx := webAlertExecutorSetup(t)
	_, err := e.ListAlerts(ctx, ai2.AlertListParams{AlertTypeID: "OWL-XXX-999"})
	require.Error(t, err)
	if !strings.Contains(err.Error(), "OWL-DSK-001") {
		t.Errorf("error should list available codes, got %v", err)
	}
}

func TestWebExecutor_ListAlerts_ByNodeName(t *testing.T) {
	e, ctx := webAlertExecutorSetup(t)
	res, err := e.ListAlerts(ctx, ai2.AlertListParams{Node: "web-01"})
	require.NoError(t, err)
	if !strings.Contains(res.Text, "web-01") || strings.Contains(res.Text, "db-01") {
		t.Errorf("node filter should match only web-01:\n%s", res.Text)
	}
}

func TestWebExecutor_ListAlerts_ByGroup(t *testing.T) {
	e, ctx := webAlertExecutorSetup(t)
	res, err := e.ListAlerts(ctx, ai2.AlertListParams{Group: "web"})
	require.NoError(t, err)
	if !strings.Contains(res.Text, "web-01") || strings.Contains(res.Text, "db-01") {
		t.Errorf("group filter should match only web-01:\n%s", res.Text)
	}
}

func TestWebExecutor_ListAlerts_BySeverity(t *testing.T) {
	e, ctx := webAlertExecutorSetup(t)
	res, err := e.ListAlerts(ctx, ai2.AlertListParams{Severity: "critical"})
	require.NoError(t, err)
	if !strings.Contains(res.Text, "db-01") || strings.Contains(res.Text, "web-01") {
		t.Errorf("critical filter should match only db-01:\n%s", res.Text)
	}
}

func TestWebExecutor_ListAlertTypes(t *testing.T) {
	e, ctx := webAlertExecutorSetup(t)
	res, err := e.ListAlertTypes(ctx)
	require.NoError(t, err)
	// 内置告警码种子
	for _, want := range []string{"OWL-DSK-001", "OWL-MEM-001", "OWL-OSS-001", "OWL-SVC-001"} {
		if !strings.Contains(res.Text, want) {
			t.Errorf("alert types missing %q:\n%s", want, res.Text)
		}
	}
}

func TestWebExecutor_ListAlertTypes_SourceColumn(t *testing.T) {
	e, ctx := webAlertExecutorSetup(t)
	// 注入一条自定义告警类型，验证来源列区分内置/自定义
	require.NoError(t, e.monitorStore.UpsertAlertType(owlmonitor.AlertType{
		ID: "OWL-CUS-001", Category: "custom", Name: "自定义队列堆积",
		DefaultSeverity: "warn", Enabled: true,
	}))
	res, err := e.ListAlertTypes(ctx)
	require.NoError(t, err)
	if !strings.Contains(res.Text, "来源") {
		t.Errorf("alert types output should have source column:\n%s", res.Text)
	}
	if !strings.Contains(res.Text, "内置") {
		t.Errorf("builtin types should be marked 内置:\n%s", res.Text)
	}
	if !strings.Contains(res.Text, "自定义") {
		t.Errorf("custom type should be marked 自定义:\n%s", res.Text)
	}
}

func TestWebExecutor_GetAlertRemedies(t *testing.T) {
	e, ctx := webAlertExecutorSetup(t)
	res, err := e.GetAlertRemedies(ctx, ai2.AlertRemedyParams{AlertTypeID: "OWL-DSK-001"})
	require.NoError(t, err)
	for _, want := range []string{"OWL-DSK-001", "磁盘使用率过高", "du -h", "SOP"} {
		if !strings.Contains(res.Text, want) {
			t.Errorf("remedies missing %q:\n%s", want, res.Text)
		}
	}
}

func TestWebExecutor_ListAlerts_NoMonitorStore(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	e := NewWebExecutor(db, nil, nil, nil, nil, nil, nil, nil, false)
	ctx := WithIdentity(context.Background(), ExecIdentity{Username: "tester", Role: "viewer"})
	_, err = e.ListAlerts(ctx, ai2.AlertListParams{})
	require.Error(t, err)
}

// AI 告警工具的角色校验与告警页面 REST RBAC 一致：
// viewer/editor/operator/admin 放行；空角色/未知角色触发权限拦截。
func TestWebExecutor_AlertToolsRoleCheck(t *testing.T) {
	t.Run("合法角色放行", func(t *testing.T) {
		for _, role := range []string{"viewer", "editor", "operator", "admin"} {
			e, _ := webAlertExecutorSetup(t)
			ctx := WithIdentity(context.Background(), ExecIdentity{Username: "tester", Role: role})
			_, err := e.ListAlerts(ctx, ai2.AlertListParams{})
			require.NoError(t, err, "role %s should be allowed to list alerts", role)
			_, err = e.ListAlertTypes(ctx)
			require.NoError(t, err, "role %s should be allowed to list alert types", role)
			_, err = e.GetAlertRemedies(ctx, ai2.AlertRemedyParams{AlertTypeID: "OWL-DSK-001"})
			require.NoError(t, err, "role %s should be allowed to get remedies", role)
		}
	})
	t.Run("空角色与未知角色拦截", func(t *testing.T) {
		for _, role := range []string{"", "hacker"} {
			e, _ := webAlertExecutorSetup(t)
			ctx := WithIdentity(context.Background(), ExecIdentity{Username: "tester", Role: role})
			_, err := e.ListAlerts(ctx, ai2.AlertListParams{})
			require.Error(t, err, "role %q should be intercepted", role)
			require.Contains(t, err.Error(), "权限不足")
			_, err = e.ListAlertTypes(ctx)
			require.Error(t, err, "role %q should be intercepted", role)
			require.Contains(t, err.Error(), "权限不足")
			_, err = e.GetAlertRemedies(ctx, ai2.AlertRemedyParams{AlertTypeID: "OWL-DSK-001"})
			require.Error(t, err, "role %q should be intercepted", role)
			require.Contains(t, err.Error(), "权限不足")
		}
	})
}
