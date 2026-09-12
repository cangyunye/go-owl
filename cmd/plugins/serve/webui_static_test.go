package serve

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 彩虹标签 tag-rN 的背景/文字原为按深色主题硬编码的固定 oklch 浅色，
// 在浅色主题（light-sky / dark-warm）下文字对比度不足（约 2:1）。
// 必须改为 color-mix 混入 var(--surface) 的主题自适应写法，
// 文字亮度走主题级变量 --tag-fg-l（深色 75%、浅色 40%）。
func TestWebUITagColors_ThemeAdaptive(t *testing.T) {
	css := readWebFile(t, "web/css/app.css")

	assert.Contains(t, css, "--tag-fg-l",
		"tag text lightness must come from a theme-scoped variable")
	assert.Contains(t, css,
		"color-mix(in oklch, oklch(62% var(--tag-c) var(--tag-h)) 25%, var(--surface))",
		"tag background must mix into var(--surface) instead of a translucent fixed oklch")
	assert.Contains(t, css,
		"oklch(var(--tag-fg-l) var(--tag-c) var(--tag-h))",
		"tag text color must derive from --tag-fg-l / --tag-h / --tag-c")
	assert.NotContains(t, css, "background: oklch(65% 0.18 25 / 0.25)",
		"dark-theme-hardcoded tag backgrounds must be replaced (tag-r0)")
	assert.NotContains(t, css, "background: oklch(62% 0.18 290 / 0.25)",
		"dark-theme-hardcoded tag backgrounds must be replaced (tag-r6)")
}
