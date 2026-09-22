package ai

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestTruncateStr_UTF8Safe：按字节截断不得切碎多字节字符，结果必须是合法 UTF-8。
func TestTruncateStr_UTF8Safe(t *testing.T) {
	s := strings.Repeat("运维节点", 100) // 全 3 字节字符
	got := truncateStr(s, 100)
	if len(got) > 100 {
		t.Fatalf("expected <= 100 bytes, got %d", len(got))
	}
	if !utf8.ValidString(got) {
		t.Fatalf("truncated string is not valid UTF-8: %q", got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected ellipsis suffix on truncation, got %q", got)
	}

	// 短串不截断
	if got := truncateStr("abc", 10); got != "abc" {
		t.Fatalf("short string should be untouched, got %q", got)
	}
	// 截断 ASCII 时保持旧契约：总长恰为 max
	if got := truncateStr("abcdefghij", 7); got != "abcd..." {
		t.Fatalf("ascii truncation contract changed, got %q", got)
	}
}
