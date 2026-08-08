package locale

import (
	"os"

	"golang.org/x/text/language"

	"github.com/cangyunye/go-owl/internal/encoding"
)

// Locale 是一个纯配置聚合值：同时携带 Language 与 Charset。
// 不是业务实体——没有 ID、无生命周期、不存库。
type Locale struct {
	Lang    language.Tag
	Charset encoding.Charset
}

// Resolve 逐维独立解析 Language 与 Charset，各取各源。
// Language 取 OWL_LANG > LC_ALL/LC_CTYPE/LANG 前缀 > 默认 zh；
// Charset 交给 encoding.ResolveCharset（OWL_IO_ENCODING > LC 后缀 > 平台）。
// 返回的 Lang 已规范化为基础 tag（zh/en）。
func Resolve() Locale {
	return Locale{
		Lang:    CanonicalLang(resolveLang().String()),
		Charset: encoding.ResolveCharset(),
	}
}

// CanonicalLang 将语言名/标签规范化为基础 tag（zh-CN -> zh，en-US -> en）。
func CanonicalLang(name string) language.Tag {
	switch name {
	case "zh", "zh-CN", "zh-TW", "zh-Hant", "zh-Hans", "zh-Hans-CN", "zh-Hant-TW":
		return language.Chinese
	case "en", "en-US", "en-GB", "en-CA", "en-AU":
		return language.English
	default:
		if t, err := language.Parse(name); err == nil {
			return t
		}
		return language.Chinese
	}
}

const envLang = "OWL_LANG"

func resolveLang() language.Tag {
	if v := os.Getenv(envLang); v != "" {
		if t, ok := parseLocale(v); ok {
			return t
		}
	}
	for _, env := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := os.Getenv(env); v != "" {
			if t, ok := parseLocale(v); ok {
				return t
			}
		}
	}
	return language.Chinese
}

// parseLocale 解析 locale 字符串为 language.Tag，容忍 .字符集 后缀。
func parseLocale(v string) (language.Tag, bool) {
	if i := lastDot(v); i >= 0 {
		v = v[:i]
	}
	t, err := language.Parse(v)
	return t, err == nil
}

func lastDot(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' {
			return i
		}
	}
	return -1
}