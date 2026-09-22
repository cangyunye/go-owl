package ai

import (
	"fmt"
	"testing"
)

// TestSessionWindowedMessages：多轮上下文只携带最近窗口，
// 更早轮次由 buildMemory 摘要兜底（消除全文+摘要双重注入）。
func TestSessionWindowedMessages(t *testing.T) {
	s := &Session{}
	for i := 0; i < 30; i++ {
		s.messages = append(s.messages,
			Message{Role: "user", Content: fmt.Sprintf("问题%d", i)},
			Message{Role: "assistant", Content: fmt.Sprintf("回复%d", i)},
		)
	}
	windowed, omitted := s.windowedMessages()
	if omitted != 60-msgWindow {
		t.Fatalf("expected %d omitted, got %d", 60-msgWindow, omitted)
	}
	if len(windowed) != msgWindow+1 {
		t.Fatalf("expected %d (marker+window), got %d", msgWindow+1, len(windowed))
	}
	if windowed[0].Role != "user" || windowed[0].Content == "" {
		t.Fatal("expected leading omission marker")
	}
	last := windowed[len(windowed)-1]
	if last.Content != "回复29" {
		t.Fatalf("expected newest message kept, got %q", last.Content)
	}

	// 小于窗口：原样返回
	s2 := &Session{messages: []Message{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "yo"}}}
	w2, o2 := s2.windowedMessages()
	if o2 != 0 || len(w2) != 2 || w2[0].Content != "hi" {
		t.Fatalf("small history must pass through, got o=%d len=%d", o2, len(w2))
	}
}

// TestCapHistoryLines：会话摘要行数封顶。
func TestCapHistoryLines(t *testing.T) {
	var h []string
	for i := 0; i < 45; i++ {
		h = append(h, fmt.Sprintf("line-%d", i))
		h = capHistoryLines(h, 40)
	}
	if len(h) != 40 {
		t.Fatalf("expected 40, got %d", len(h))
	}
	if h[0] != "line-5" || h[len(h)-1] != "line-44" {
		t.Fatalf("expected oldest dropped, got %q..%q", h[0], h[len(h)-1])
	}
}
