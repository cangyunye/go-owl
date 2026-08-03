package encoding

import (
	"os"
	"strings"
)

const envCharset = "OWL_IO_ENCODING"

// DetectCharset 解析字符串形式的编码值。返回 ok=false 表示无法识别。
func ParseCharset(s string) (Charset, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "utf8", "utf-8", "65001":
		return UTF8, true
	case "gbk", "gb2312", "cp936", "936":
		return GBK, true
	case "big5", "cp950", "950":
		return Big5, true
	default:
		return UTF8, false
	}
}

// ResolveCharset 按优先级解析当前字符集：
// OWL_IO_ENCODING > LC_*/LANG 后缀 > 平台默认。
// avoidLC 用于测试时屏蔽环境探测。
func ResolveCharset() Charset {
	if s := os.Getenv(envCharset); s != "" {
		if cs, ok := ParseCharset(s); ok {
			return cs
		}
	}
	if cs, ok := fromLocaleSuffix(); ok {
		return cs
	}
	return platformDefaultCharset()
}

func fromLocaleSuffix() (Charset, bool) {
	for _, env := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		v := os.Getenv(env)
		if v == "" {
			continue
		}
		if i := strings.LastIndex(v, "."); i >= 0 {
			if cs, ok := ParseCharset(v[i+1:]); ok {
				return cs, true
			}
		}
	}
	return UTF8, false
}