// Package common 提供跨模块共享的基础工具。
package common

import (
	"unicode/utf8"
)

// Truncate 按字节上限截断 s，回退到 UTF-8 rune 边界，不产生非法多字节序列；
// 发生截断且 max >= 3 时以 "..." 结尾。结果总长不超过 max 字节。
func Truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	suffix := ""
	cut := max
	if max >= 3 {
		suffix = "..."
		cut = max - 3
	}
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + suffix
}
