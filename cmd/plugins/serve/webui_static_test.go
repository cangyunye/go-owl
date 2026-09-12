package serve

import (
	"fmt"
	"strings"
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

// 深色主题 --muted 亮度 55% 对 12% 背景对比度约 4.2:1，低于 WCAG AA(4.5:1)，
// 且 muted 大量用于 11-12px 小字；提亮到 63%（约 5.8:1）。
// light-sky 主题 accent 色相 150 与 success(145) 几乎同色相，
// 主按钮/链接与成功徽章无法区分；accent 系列整体移到 250 天蓝，
// 背景的薄荷绿色调（低饱和 140/150）保留。
func TestWebUIContrast_MutedAAAndLightSkyAccent(t *testing.T) {
	css := readWebFile(t, "web/css/app.css")

	assert.Contains(t, css, "--muted:       oklch(63% 0.015 250);",
		"dark-theme muted must be lightened to ~5.8:1 contrast")
	assert.NotContains(t, css, "oklch(55% 0.015 250)",
		"old below-AA muted value must go")

	assert.Contains(t, css, "--accent:      oklch(52% 0.18 250);",
		"light-sky accent must move off the success hue to sky blue 250")
	assert.NotContains(t, css, "oklch(58% 0.22 150)",
		"old light-sky accent / chat-user / sidebar-hover values must be replaced")
	assert.Contains(t, css, "--success:     oklch(56% 0.18 145);",
		"success green must stay for status semantics")
}

// 字号收敛为 token：下限提到 12px（消灭 10/11px 小字），
// 层级 xs12/sm13/md14/lg16/xl18/2xl24，便于全局调节与密度切换；
// 表格单元格启用 tabular-nums 对齐数字列。
func TestWebUIFontTokens(t *testing.T) {
	css := readWebFile(t, "web/css/app.css")

	for _, tok := range []string{"--fs-xs:", "--fs-sm:", "--fs-md:", "--fs-lg:", "--fs-xl:", "--fs-2xl:"} {
		assert.Contains(t, css, tok, "font scale token %s must be defined", tok)
	}
	assert.Contains(t, css, "font-size: var(--fs-",
		"font-size declarations must use the scale tokens")

	for _, n := range []string{"10", "11", "12", "13", "14", "15", "16", "17", "18", "20", "22", "24"} {
		assert.NotContains(t, css, "font-size: "+n+"px",
			"hardcoded %spx font-size must use a token", n)
		assert.NotContains(t, css, "font-size:"+n+"px",
			"hardcoded %spx font-size must use a token", n)
	}
	assert.GreaterOrEqual(t, strings.Count(css, "tabular-nums"), 2,
		"tabular-nums must cover stat values and table cells")
}

// 7 个色相桶对 11+ 个分组必然哈希碰撞，且 r1/r2 同为黄系难以区分，
// 扩到 12 桶（每 30° 一档）；标签 chip 用 tag-out 描边变体与分组实心
// chip 形成形状区分，不再单纯依赖颜色。
func TestWebUITagBuckets_TwelveAndLabelOutline(t *testing.T) {
	css := readWebFile(t, "web/css/app.css")
	for i := 0; i < 12; i++ {
		assert.Contains(t, css, fmt.Sprintf(".tag-r%d { --tag-h:", i),
			"tag hue bucket r%d must be defined", i)
	}
	assert.Contains(t, css, ".tag-out {",
		"outline variant for label chips must exist")

	for _, name := range []string{"nodes.js", "exec.js", "files.js"} {
		src := readWebFile(t, "web/js/pages/"+name)
		assert.Contains(t, src, "% 12", "%s tagColor must hash into 12 hue buckets", name)
		assert.NotContains(t, src, "% 7)", "%s must not keep the 7-bucket hash", name)
	}

	nodes := readWebFile(t, "web/js/pages/nodes.js")
	assert.Contains(t, nodes, `class="tag tag-out ${tagColor(`,
		"label chips must render with the tag-out outline variant")
}
