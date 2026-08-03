package i18n

import (
	"testing"

	"golang.org/x/text/language"
)

func TestTChinese(t *testing.T) {
	Init(language.Chinese)
	if got := T("common.no_nodes"); got != "未发现节点" {
		t.Fatalf("zh lookup: got %q", got)
	}
	if got := T("common.total", F(5)); got != "总计: 5 个节点" {
		t.Fatalf("zh plural arg: got %q", got)
	}
}

func TestTEnglish(t *testing.T) {
	Init(language.English)
	if got := T("common.no_nodes"); got != "No nodes found." {
		t.Fatalf("en lookup: got %q", got)
	}
	if got := T("common.total", F(5)); got != "Total: 5 nodes" {
		t.Fatalf("en plural arg: got %q", got)
	}
}

func TestFPreventsDigitGrouping(t *testing.T) {
	Init(language.Chinese)
	if got := T("common.total", F(5000)); got != "总计: 5000 个节点" {
		t.Fatalf("digit grouping leaked: got %q", got)
	}
	SetLang(language.English)
	if got := T("common.total", F(5000)); got != "Total: 5000 nodes" {
		t.Fatalf("digit grouping leaked: got %q", got)
	}
	SetLang(language.Chinese)
}

func TestMissingKeyFallsBackToSource(t *testing.T) {
	Init(language.English)
	// en 缺失的 key 应回退到 zh-CN（源语言）
	if got := T("zh.only_key"); got == "" {
		t.Fatal("expected non-empty fallback")
	}
}

func TestMissingKeyReturnsKey(t *testing.T) {
	Init(language.Chinese)
	// 源语言也缺失的 key 原样返回 key 本身
	if got := T("no.such.key"); got != "no.such.key" {
		t.Fatalf("expected raw key, got %q", got)
	}
}

func TestSetLangSwitch(t *testing.T) {
	Init(language.Chinese)
	SetLang(language.English)
	if got := T("common.no_nodes"); got != "No nodes found." {
		t.Fatalf("after switch: got %q", got)
	}
	if ActiveLang() != language.English {
		t.Fatalf("ActiveLang: got %v", ActiveLang())
	}
}

func TestRawKeepsPlaceholder(t *testing.T) {
	Init(language.Chinese)
	if got := Raw("node.check.auth_key_failed"); got != "密钥认证失败: %w" {
		t.Fatalf("Raw zh: got %q", got)
	}
	SetLang(language.English)
	if got := Raw("node.check.auth_key_failed"); got != "key authentication failed: %w" {
		t.Fatalf("Raw en: got %q", got)
	}
}

func TestRawFallback(t *testing.T) {
	Init(language.English)
	// en 缺失的 key 回退到 zh 源语言
	if got := Raw("node.check.no_auth_configured"); got == "" || got == "node.check.no_auth_configured" {
		t.Fatalf("Raw fallback: got %q", got)
	}
}
