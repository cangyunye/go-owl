package monitor

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func emailChannel(id string, severityMin Severity) NotifyChannel {
	return NotifyChannel{
		ID: id, Kind: "email", Name: "告警邮件",
		Config: NotifyConfig{Email: &EmailConfig{
			SMTPHost: "smtp.example.com", SMTPPort: 465,
			Username: "u", Password: "p", From: "owl@example.com",
			To: []string{"ops@example.com"},
		}},
		SeverityMin: severityMin, Enabled: true, CreatedAt: 1000,
	}
}

func webhookChannel(id string, severityMin Severity) NotifyChannel {
	return NotifyChannel{
		ID: id, Kind: "webhook", Name: "自定义 Webhook",
		Config: NotifyConfig{Webhook: &WebhookConfig{
			URL:     "https://example.com/hook",
			Headers: map[string]string{"Authorization": "Bearer x"},
		}},
		SeverityMin: severityMin, Enabled: true, CreatedAt: 1000,
	}
}

// TestNotifyChannel_CRUD 验证通知渠道增删改查与 JSON 配置往返。
func TestNotifyChannel_CRUD(t *testing.T) {
	s := newTestStore(t)

	ch := webhookChannel("CH-1", SeverityWarning)
	ch.AlertTypes = "OWL-DSK-001,OWL-MEM-001"
	require.NoError(t, s.UpsertNotifyChannel(ch))

	got, exists, err := s.GetNotifyChannel("CH-1")
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, "webhook", got.Kind)
	require.Equal(t, "https://example.com/hook", got.Config.Webhook.URL)
	require.Equal(t, "Bearer x", got.Config.Webhook.Headers["Authorization"])

	// 更新
	ch.Enabled = false
	require.NoError(t, s.UpsertNotifyChannel(ch))
	got, _, _ = s.GetNotifyChannel("CH-1")
	require.False(t, got.Enabled)

	require.NoError(t, s.DeleteNotifyChannel("CH-1"))
	_, exists, _ = s.GetNotifyChannel("CH-1")
	require.False(t, exists)
}

// TestChannelMatches 验证渠道匹配：启用、级别门槛、告警类型白名单。
func TestChannelMatches(t *testing.T) {
	ch := emailChannel("CH-1", SeverityWarning)

	// 级别：warn 门槛拒绝 info，接受 warn/critical
	require.False(t, ChannelMatches(ch, SeverityInfo, "OWL-DSK-001"))
	require.True(t, ChannelMatches(ch, SeverityWarning, "OWL-DSK-001"))
	require.True(t, ChannelMatches(ch, SeverityCritical, "OWL-DSK-001"))

	// 类型白名单：空=全部；非空时仅匹配列出的 ID
	ch.AlertTypes = "OWL-DSK-001,OWL-MEM-001"
	require.True(t, ChannelMatches(ch, SeverityCritical, "OWL-DSK-001"))
	require.False(t, ChannelMatches(ch, SeverityCritical, "OWL-ERR-002"))

	// 禁用渠道不匹配
	ch.Enabled = false
	require.False(t, ChannelMatches(ch, SeverityCritical, "OWL-DSK-001"))
}

// TestNotifyPayload_JSON 验证 webhook 载荷固定 JSON 结构。
func TestNotifyPayload_JSON(t *testing.T) {
	p := NotifyPayload{
		AlertID: "AL-1", AlertType: "OWL-DSK-001", AlertTypeName: "磁盘使用率过高",
		NodeID: "node-a", NodeName: "web-01", Severity: SeverityWarning,
		Status: StatusOpen, Message: "磁盘使用率 93.5%", MetricSnapshot: `{"disk.usage./":93.5}`,
		FirstSeen: 1750000000, WebURL: "http://owl-host/alerts/AL-1",
	}
	data, err := json.Marshal(p)
	require.NoError(t, err)

	var back NotifyPayload
	require.NoError(t, json.Unmarshal(data, &back))
	require.Equal(t, p, back)
	// 字段名对齐设计文档 8.3
	require.Contains(t, string(data), `"alert_type":"OWL-DSK-001"`)
	require.Contains(t, string(data), `"severity":"warn"`)
}
