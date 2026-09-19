package ai

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	ai "github.com/cangyunye/go-owl/internal/ai"
	owlmonitor "github.com/cangyunye/go-owl/internal/monitor"
	"github.com/stretchr/testify/require"
)

// owlAlertData 单测：临时 owl.db 造告警数据，验证告警码归一/节点名解析/状态过滤。

func owlAlertDataSetup(t *testing.T) *owlAlertData {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "owl.db")
	st, err := owlmonitor.OpenStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	now := time.Now().Unix()
	for _, a := range []*owlmonitor.Alert{
		{ID: "AL-1", AlertTypeID: "OWL-DSK-001", NodeID: "n1", Severity: owlmonitor.SeverityWarning,
			Status: owlmonitor.StatusOpen, Message: "磁盘使用率过高: / 95.2%", FirstSeen: now, LastSeen: now},
		{ID: "AL-2", AlertTypeID: "OWL-OSS-001", NodeID: "n2", Severity: owlmonitor.SeverityCritical,
			Status: owlmonitor.StatusOpen, Message: "节点失联", FirstSeen: now, LastSeen: now},
		{ID: "AL-3", AlertTypeID: "OWL-DSK-001", NodeID: "n2", Severity: owlmonitor.SeverityWarning,
			Status: owlmonitor.StatusResolved, Message: "已恢复", FirstSeen: now - 7200, LastSeen: now - 3600},
	} {
		require.NoError(t, st.InsertAlert(a))
	}

	nodeStore := common.NewInMemoryNodeStoreAt(filepath.Join(t.TempDir(), "nodes.json"))
	for _, n := range []*common.NodeInfo{
		{ID: "n1", Name: "web-01"},
		{ID: "n2", Name: "db-01"},
	} {
		require.NoError(t, nodeStore.Add(n))
	}

	d := newOwlAlertData(nodeStore, dbPath)
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestOwlAlertData_ListAlerts(t *testing.T) {
	d := owlAlertDataSetup(t)

	t.Run("默认活跃告警", func(t *testing.T) {
		rows, total, err := d.ListAlerts(ai.AlertListParams{})
		require.NoError(t, err)
		require.Equal(t, 2, total)
		require.Len(t, rows, 2)
		// 节点名解析
		require.Equal(t, "web-01", rows[1].NodeName)
	})

	t.Run("下划线告警码归一化", func(t *testing.T) {
		rows, total, err := d.ListAlerts(ai.AlertListParams{AlertTypeID: "owl_dsk_001"})
		require.NoError(t, err)
		require.Equal(t, 1, total) // resolved 不算活跃
		require.Len(t, rows, 1)
		require.Equal(t, "OWL-DSK-001", rows[0].TypeID)
	})

	t.Run("未知告警码报错", func(t *testing.T) {
		_, _, err := d.ListAlerts(ai.AlertListParams{AlertTypeID: "OWL-XXX-999"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "OWL-DSK-001")
	})

	t.Run("节点名过滤", func(t *testing.T) {
		rows, total, err := d.ListAlerts(ai.AlertListParams{Node: "db-01"})
		require.NoError(t, err)
		require.Equal(t, 1, total)
		require.Equal(t, "OWL-OSS-001", rows[0].TypeID)
	})

	t.Run("resolved 状态过滤", func(t *testing.T) {
		rows, _, err := d.ListAlerts(ai.AlertListParams{Status: "resolved"})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, "n2", rows[0].NodeID)
	})

	t.Run("类别过滤", func(t *testing.T) {
		rows, total, err := d.ListAlerts(ai.AlertListParams{Category: "disk", Status: ""})
		require.NoError(t, err)
		require.Equal(t, 1, total)
		require.Equal(t, "OWL-DSK-001", rows[0].TypeID)
	})
}

func TestOwlAlertData_ListAlertTypes(t *testing.T) {
	d := owlAlertDataSetup(t)
	rows, err := d.ListAlertTypes()
	require.NoError(t, err)
	require.NotEmpty(t, rows)
	found := false
	for _, r := range rows {
		if r.ID == "OWL-OSS-001" {
			found = true
		}
	}
	require.True(t, found, "builtin types should be seeded")
}

func TestOwlAlertData_ListRemedies(t *testing.T) {
	d := owlAlertDataSetup(t)
	rows, err := d.ListRemedies("OWL-DSK-001")
	require.NoError(t, err)
	require.NotEmpty(t, rows)
	require.Contains(t, rows[0].Content, "du -h")
}

func TestOwlAlertData_InjectedIntoAgent(t *testing.T) {
	// SetupSession 装配链验证：注入后 CLIExecutor 告警方法可用
	d := owlAlertDataSetup(t)
	executor := ai.NewCLIExecutor(nil, nil)
	executor.SetAlertData(d)
	res, err := executor.ListAlerts(context.Background(), ai.AlertListParams{AlertTypeID: "OWL_DSK_001"})
	require.NoError(t, err)
	require.True(t, strings.Contains(res.Text, "web-01"), "output:\n%s", res.Text)
}
