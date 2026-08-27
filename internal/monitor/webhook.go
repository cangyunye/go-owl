package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// WebhookNotifier 自定义 Webhook 通知：POST 固定 JSON 载荷。
type WebhookNotifier struct{}

// NewWebhookNotifier 创建 Webhook 通知器。
func NewWebhookNotifier() *WebhookNotifier {
	return &WebhookNotifier{}
}

// Send 向渠道配置的 URL POST 告警载荷（含自定义请求头）。
func (n *WebhookNotifier) Send(ctx context.Context, ch NotifyChannel, event AlertEvent, at AlertType, nodeName, webURL string) error {
	if ch.Config.Webhook == nil || ch.Config.Webhook.URL == "" {
		return fmt.Errorf("webhook 配置缺失（URL 为空）")
	}
	payload := BuildPayload(event, at, nodeName, webURL)
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化告警载荷失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ch.Config.Webhook.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range ch.Config.Webhook.Headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求 webhook 失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("webhook 返回非 2xx 状态: %d", resp.StatusCode)
	}
	return nil
}
