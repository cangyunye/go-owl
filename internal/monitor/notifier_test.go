package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/smtp"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func sampleEvent() AlertEvent {
	return AlertEvent{
		Type: EventOpened,
		Alert: &Alert{
			ID: "AL-1", AlertTypeID: "OWL-DSK-001", NodeID: "node-a",
			Severity: SeverityWarning, Status: StatusOpen,
			Message:        "磁盘使用率过高：disk.usage./ 当前 93.50（规则 > 90）",
			MetricSnapshot: `{"disk.usage./":93.5}`, FirstSeen: 1750000000, LastSeen: 1750000000,
		},
	}
}

func dskAlertType() AlertType {
	at, _ := FindAlertType("OWL-DSK-001")
	return at
}

// TestWebhookNotifier_Send 验证 webhook POST JSON 载荷与自定义请求头。
func TestWebhookNotifier_Send(t *testing.T) {
	var mu sync.Mutex
	var gotPayload NotifyPayload
	var gotHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotHeaders = r.Header.Clone()
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotPayload))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := webhookChannel("CH-1", SeverityWarning)
	ch.Config.Webhook.URL = srv.URL
	ch.Config.Webhook.Headers = map[string]string{"X-Owl-Token": "secret"}

	n := NewWebhookNotifier()
	require.NoError(t, n.Send(context.Background(), ch, sampleEvent(), dskAlertType(), "web-01", "http://owl/alerts/AL-1"))

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, "AL-1", gotPayload.AlertID)
	require.Equal(t, "OWL-DSK-001", gotPayload.AlertType)
	require.Equal(t, "磁盘使用率过高", gotPayload.AlertTypeName)
	require.Equal(t, "web-01", gotPayload.NodeName)
	require.Equal(t, SeverityWarning, gotPayload.Severity)
	require.Equal(t, "http://owl/alerts/AL-1", gotPayload.WebURL)
	require.Equal(t, "secret", gotHeaders.Get("X-Owl-Token"))
	require.Equal(t, "application/json", gotHeaders.Get("Content-Type"))
}

// TestDispatcher_Retry 验证发送失败重试：2 次失败后成功。
func TestDispatcher_Retry(t *testing.T) {
	var mu sync.Mutex
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts++
		n := attempts
		mu.Unlock()
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := newTestStore(t)
	ch := webhookChannel("CH-1", SeverityInfo)
	ch.Config.Webhook.URL = srv.URL
	require.NoError(t, s.UpsertNotifyChannel(ch))

	d := NewDispatcher(s)
	d.retries = 3
	d.delay = 0
	errs := d.Notify(context.Background(), sampleEvent(), dskAlertType(), "web-01", "")
	require.Empty(t, errs, "重试后成功不应报错")

	mu.Lock()
	require.Equal(t, 3, attempts, "应重试至成功")
	mu.Unlock()
}

// TestDispatcher_RetryFail 验证重试耗尽返回错误。
func TestDispatcher_RetryFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := newTestStore(t)
	ch := webhookChannel("CH-1", SeverityInfo)
	ch.Config.Webhook.URL = srv.URL
	require.NoError(t, s.UpsertNotifyChannel(ch))

	d := NewDispatcher(s)
	d.retries = 2
	d.delay = 0
	errs := d.Notify(context.Background(), sampleEvent(), dskAlertType(), "web-01", "")
	require.Len(t, errs, 1, "重试耗尽应返回错误")
}

// TestDispatcher_Filter 验证分发按级别/类型过滤渠道。
func TestDispatcher_Filter(t *testing.T) {
	var received int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := newTestStore(t)
	// 级别门槛 warn：info 告警不通知
	ch := webhookChannel("CH-1", SeverityWarning)
	ch.Config.Webhook.URL = srv.URL
	require.NoError(t, s.UpsertNotifyChannel(ch))
	// 禁用渠道不通知
	ch2 := webhookChannel("CH-2", SeverityInfo)
	ch2.Config.Webhook.URL = srv.URL
	ch2.Enabled = false
	require.NoError(t, s.UpsertNotifyChannel(ch2))

	d := NewDispatcher(s)
	d.delay = 0

	errs := d.Notify(context.Background(), sampleEvent(), dskAlertType(), "web-01", "")
	require.Empty(t, errs)
	require.Equal(t, 1, received, "仅 warn 门槛渠道收到")

	// info 告警：无渠道匹配
	ev := sampleEvent()
	ev.Alert.Severity = SeverityInfo
	errs = d.Notify(context.Background(), ev, dskAlertType(), "web-01", "")
	require.Empty(t, errs)
	require.Equal(t, 1, received, "info 告警不触发 warn 门槛渠道")
}

// TestEmailNotifier_Message 验证邮件内容组装（标题/收件人/正文含告警信息）。
func TestEmailNotifier_Message(t *testing.T) {
	var mu sync.Mutex
	var gotFrom string
	var gotTo []string
	var gotBody string

	fake := &fakeEmailSender{
		onSend: func(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
			mu.Lock()
			defer mu.Unlock()
			gotFrom = from
			gotTo = to
			gotBody = string(msg)
			return nil
		},
	}

	ch := emailChannel("CH-E", SeverityWarning)
	cfg := ch.Config.Email
	cfg.SMTPHost = "smtp.example.com"
	cfg.SMTPPort = 587

	n := &EmailNotifier{sender: fake}
	require.NoError(t, n.Send(context.Background(), ch, sampleEvent(), dskAlertType(), "web-01", "http://owl/alerts/AL-1"))

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, "owl@example.com", gotFrom)
	require.Equal(t, []string{"ops@example.com"}, gotTo)
	require.Contains(t, gotBody, "Subject: [owl] 告警 warn: 磁盘使用率过高")
	require.Contains(t, gotBody, "告警 ID: AL-1")
	require.Contains(t, gotBody, "告警类型: OWL-DSK-001")
	require.Contains(t, gotBody, "节点: web-01 (node-a)")
	require.Contains(t, gotBody, "http://owl/alerts/AL-1")
}

// TestDispatcher_SendTest 验证测试发送可校验渠道连通性。
func TestDispatcher_SendTest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := newTestStore(t)
	ch := webhookChannel("CH-1", SeverityInfo)
	ch.Config.Webhook.URL = srv.URL
	require.NoError(t, s.UpsertNotifyChannel(ch))

	d := NewDispatcher(s)
	d.retries = 1
	require.NoError(t, d.SendTest(context.Background(), ch, "http://owl/alerts/test"))

	bad := webhookChannel("CH-BAD", SeverityInfo)
	bad.Config.Webhook.URL = "http://127.0.0.1:1/nope"
	err := d.SendTest(context.Background(), bad, "")
	require.Error(t, err, "不可达地址应报错")
	require.True(t, strings.Contains(err.Error(), "测试"), "错误应含测试语义")
}

type fakeEmailSender struct {
	onSend func(addr string, auth smtp.Auth, from string, to []string, msg []byte) error
}

func (f *fakeEmailSender) SendMail(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	if f.onSend == nil {
		return errors.New("no handler")
	}
	return f.onSend(addr, auth, from, to, msg)
}
