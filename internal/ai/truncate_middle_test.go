package ai

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestTruncateMiddle：超长工具结果按预算截断并保留头尾，rune 安全。
func TestTruncateMiddle(t *testing.T) {
	long := strings.Repeat("A", 5000) + strings.Repeat("中", 2000) + strings.Repeat("B", 2000)
	got := truncateMiddle(long, 4096)
	if len(got) > 4096+32 { // 预算 + 省略标记的容差
		t.Fatalf("result too long: %d", len(got))
	}
	if !utf8.ValidString(got) {
		t.Fatal("not valid UTF-8")
	}
	if !strings.HasPrefix(got, "AAAA") || !strings.HasSuffix(got, "BBBB") {
		t.Fatal("head/tail not preserved")
	}
	if !strings.Contains(got, "省略") {
		t.Fatal("missing omission marker")
	}
	if short := truncateMiddle("short result", 4096); short != "short result" {
		t.Fatalf("short input must be untouched, got %q", short)
	}
}
