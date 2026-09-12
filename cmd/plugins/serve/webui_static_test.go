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

// readWebLF 读取 web 资源并把 CRLF 归一为 LF，
// 使断言在 Windows autocrlf 检出环境下同样成立。
func readWebLF(t *testing.T, name string) string {
	return strings.ReplaceAll(readWebFile(t, name), "\r\n", "\n")
}

func TestWebUITagColors_ThemeAdaptive(t *testing.T) {
	css := readWebLF(t, "web/css/app.css")

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
	css := readWebLF(t, "web/css/app.css")

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
	css := readWebLF(t, "web/css/app.css")

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
	css := readWebLF(t, "web/css/app.css")
	for i := 0; i < 12; i++ {
		assert.Contains(t, css, fmt.Sprintf(".tag-r%d { --tag-h:", i),
			"tag hue bucket r%d must be defined", i)
	}
	assert.Contains(t, css, ".tag-out {",
		"outline variant for label chips must exist")

	for _, name := range []string{"nodes.js", "exec.js", "files.js"} {
		src := readWebLF(t, "web/js/pages/"+name)
		assert.Contains(t, src, "% 12", "%s tagColor must hash into 12 hue buckets", name)
		assert.NotContains(t, src, "% 7)", "%s must not keep the 7-bucket hash", name)
	}

	nodes := readWebLF(t, "web/js/pages/nodes.js")
	assert.Contains(t, nodes, `class="tag tag-out ${tagColor(`,
		"label chips must render with the tag-out outline variant")
}

// 分组筛选升级为彩色胶囊：色点常驻该组彩虹色相（与列表徽章同源 tagColor），
// 选中态整颗组色填充；去掉原生 checkbox；chip 上带节点计数；
// exec 页分组筛选同样换用 .group-chip 与节点管理视觉统一。
func TestWebUIGroupFilterChips(t *testing.T) {
	css := readWebLF(t, "web/css/app.css")
	assert.Contains(t, css, ".group-chips {", "chip flow container must exist")
	assert.Contains(t, css, ".group-chip {", "group chip style must exist")
	assert.Contains(t, css, ".group-chip.selected {",
		"selected state must be styled")
	assert.Contains(t, css, "background: color-mix(in oklch, oklch(62% var(--tag-c) var(--tag-h))",
		"selected chip must fill with the group's own hue via color-mix")
	assert.Contains(t, css, ".group-chip .count {",
		"per-group node count badge must be styled")

	nodes := readWebLF(t, "web/js/pages/nodes.js")
	assert.NotContains(t, nodes, "group-check",
		"native checkbox group filter must be removed")
	assert.Contains(t, nodes, `class="group-chip`,
		"group filter must render as colored chips")
	assert.Contains(t, nodes, "${tagColor(g)}",
		"chip must carry the group's hue bucket class")
	assert.Contains(t, nodes, "aria-pressed",
		"chip must expose pressed state for a11y")
	assert.Contains(t, nodes, "loadGroupCounts",
		"per-group node counts must be loaded")
	assert.Contains(t, nodes, "groupCounts",
		"counts must be kept in state and rendered")

	exec := readWebLF(t, "web/js/pages/exec.js")
	assert.Contains(t, exec, `class="group-chip`,
		"exec group filter must use the same group-chip style")
	assert.NotContains(t, exec, `<span class="node-chip ${active ? 'selected' : ''}" data-group=`,
		"exec group filter must stop borrowing node-chip styling")
}

// 节点选择 chip（命令执行/文件传输侧栏）与 group-chip 视觉语言对齐：
// 胶囊造型、状态点改类驱动、选中态 accent 填充；files 页原本借用
// node-chip 的分组筛选同步换 group-chip。
func TestWebUINodeChipsPill(t *testing.T) {
	css := readWebLF(t, "web/css/app.css")
	assert.Contains(t, css, "border-radius: 999px;\n  background: var(--surface);\n  border: 1px solid var(--border);\n  color: var(--fg);",
		"node-chip must share the group-chip pill geometry")
	assert.Contains(t, css, ".node-chip.selected {\n  border-color: color-mix(in oklch, var(--accent) 55%, transparent);",
		"selected node chip must be an accent-tinted fill")
	assert.Contains(t, css, ".node-chip .dot.st-online { background: var(--success); }",
		"status dot colors must be class-driven, not inline styles")
	assert.Contains(t, css, ".node-chip.selected .dot { background: currentColor; }",
		"selected chip dot must adopt the fill color")

	for _, name := range []string{"exec.js", "files.js"} {
		src := readWebLF(t, "web/js/pages/" + name)
		assert.Contains(t, src, `<button type="button" class="node-chip ${`,
			"%s node chips must be buttons with aria-pressed", name)
		assert.Contains(t, src, "aria-pressed=",
			"%s node chips must expose selection state", name)
	}

	files := readWebLF(t, "web/js/pages/files.js")
	assert.Contains(t, files, `class="group-chip ${tagColor(g)}`,
		"files group filter must use group-chip with hue buckets")
	assert.NotContains(t, files, `class="node-chip ${active ? 'selected' : ''}" data-group=`,
		"files group filter must stop borrowing node-chip styling")
}
