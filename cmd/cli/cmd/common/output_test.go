package common

import "testing"

func TestStripANSI(t *testing.T) {
	if got := StripANSI("\033[31mred\033[0m text"); got != "red text" {
		t.Fatalf("got %q", got)
	}
	if got := StripANSI("plain"); got != "plain" {
		t.Fatalf("got %q", got)
	}
	// 未闭合序列整段丢弃
	if got := StripANSI("ok \033[31m"); got != "ok " {
		t.Fatalf("got %q", got)
	}
	// DisplayWidth 与 StripANSI 行为兼容：宽度按 ANSI 忽略计算
	if w := DisplayWidth("\033[31m中文\033[0m"); w != 4 {
		t.Fatalf("DisplayWidth got %d", w)
	}
}
