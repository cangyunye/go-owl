package ai

import (
	"context"
	"errors"
	"testing"
)

type scriptedFailModel struct {
	calls  int
	errGen func() error
}

func (m *scriptedFailModel) Generate(ctx context.Context, messages []Message) (string, error) {
	m.calls++
	return "", m.errGen()
}

// TestGenerateWithRetry_NoRetryOn4xx：4xx（key 无效/参数错）必须立即失败。
func TestGenerateWithRetry_NoRetryOn4xx(t *testing.T) {
	m := &scriptedFailModel{errGen: func() error {
		return &APIStatusError{StatusCode: 401, Message: "invalid key"}
	}}
	_, err := generateWithRetry(context.Background(), m, nil, "测试")
	if err == nil {
		t.Fatal("expected error")
	}
	if m.calls != 1 {
		t.Fatalf("expected exactly 1 call for 4xx, got %d", m.calls)
	}
	var se *APIStatusError
	if !errors.As(err, &se) || se.StatusCode != 401 {
		t.Fatalf("expected APIStatusError passthrough, got %v", err)
	}
}

// TestGenerateWithRetry_RetriesOn5xx：5xx 保持既有 3 次重试。
func TestGenerateWithRetry_RetriesOn5xx(t *testing.T) {
	if testing.Short() {
		t.Skip("retry delay")
	}
	m := &scriptedFailModel{errGen: func() error {
		return &APIStatusError{StatusCode: 503, Body: "overloaded"}
	}}
	_, _ = generateWithRetry(context.Background(), m, nil, "测试")
	if m.calls != maxRetries {
		t.Fatalf("expected %d calls for 5xx, got %d", maxRetries, m.calls)
	}
}
