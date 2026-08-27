package monitor

import (
	"context"
	"fmt"
	"time"
)

// Notifier 通知器：向某渠道发送一条告警事件。
type Notifier interface {
	Send(ctx context.Context, ch NotifyChannel, event AlertEvent, at AlertType, nodeName, webURL string) error
}

// BuildPayload 构造固定结构通知载荷（设计文档 8.3）。
func BuildPayload(event AlertEvent, at AlertType, nodeName, webURL string) NotifyPayload {
	a := event.Alert
	return NotifyPayload{
		AlertID:        a.ID,
		AlertType:      a.AlertTypeID,
		AlertTypeName:  at.Name,
		NodeID:         a.NodeID,
		NodeName:       nodeName,
		Severity:       a.Severity,
		Status:         a.Status,
		Message:        a.Message,
		MetricSnapshot: a.MetricSnapshot,
		FirstSeen:      a.FirstSeen,
		WebURL:         webURL,
	}
}

// Dispatcher 通知分发器：按渠道配置过滤事件，异步重试发送。
type Dispatcher struct {
	store   *Store
	retries int
	delay   time.Duration
}

// NewDispatcher 创建分发器，默认重试 3 次、间隔 1s。
func NewDispatcher(store *Store) *Dispatcher {
	return &Dispatcher{store: store, retries: 3, delay: time.Second}
}

// Notify 向全部匹配渠道发送事件，返回各渠道错误（不中断）。
func (d *Dispatcher) Notify(ctx context.Context, event AlertEvent, at AlertType, nodeName, webURL string) []error {
	channels, err := d.store.ListNotifyChannels()
	if err != nil {
		return []error{fmt.Errorf("monitor: 读取通知渠道失败: %w", err)}
	}
	var errs []error
	for _, ch := range channels {
		if !ChannelMatches(ch, event.Alert.Severity, event.Alert.AlertTypeID) {
			continue
		}
		notifier, ok := notifierFor(ch)
		if !ok {
			continue
		}
		if err := d.sendWithRetry(ctx, notifier, ch, event, at, nodeName, webURL); err != nil {
			errs = append(errs, fmt.Errorf("渠道 %s(%s): %w", ch.ID, ch.Kind, err))
		}
	}
	return errs
}

// SendTest 发送测试通知以校验渠道连通性。
func (d *Dispatcher) SendTest(ctx context.Context, ch NotifyChannel, webURL string) error {
	testEvent := AlertEvent{
		Type: EventOpened,
		Alert: &Alert{
			ID: "test", AlertTypeID: "OWL-TEST-000", NodeID: "test-node",
			Severity: SeverityWarning, Status: StatusOpen,
			Message: "这是一条测试通知", MetricSnapshot: "{}", FirstSeen: time.Now().Unix(),
		},
	}
	testType := AlertType{ID: "OWL-TEST-000", Name: "测试告警"}
	notifier, ok := notifierFor(ch)
	if !ok {
		return fmt.Errorf("未知渠道类型 %q", ch.Kind)
	}
	if err := d.sendWithRetry(ctx, notifier, ch, testEvent, testType, "测试节点", webURL); err != nil {
		return fmt.Errorf("测试发送失败: %w", err)
	}
	return nil
}

func notifierFor(ch NotifyChannel) (Notifier, bool) {
	switch ch.Kind {
	case "webhook":
		return NewWebhookNotifier(), true
	case "email":
		return NewEmailNotifier(), true
	default:
		return nil, false
	}
}

func (d *Dispatcher) sendWithRetry(ctx context.Context, n Notifier, ch NotifyChannel, event AlertEvent, at AlertType, nodeName, webURL string) error {
	var err error
	for i := 0; i < d.retries; i++ {
		if err = n.Send(ctx, ch, event, at, nodeName, webURL); err == nil {
			return nil
		}
		if d.delay > 0 && i < d.retries-1 {
			select {
			case <-time.After(d.delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return err
}
