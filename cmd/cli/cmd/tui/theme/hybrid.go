package theme

import "github.com/charmbracelet/lipgloss"

// HybridColor 内嵌 lipgloss.CompleteAdaptiveColor,复用其 color(*Renderer)
// 实现来满足 lipgloss.TerminalColor 接口。
//
// 注意:lipgloss.TerminalColor 的 color 方法是非导出的,Go 语言规定带非导出
// 方法的接口只能由声明它的包(lipgloss)内的类型实现,因此在 theme 包内无法
// 手写 color 方法。嵌入 CompleteAdaptiveColor 后其提升的 color 方法会使用
// 传入 Renderer 的 r.ColorProfile()/r.HasDarkBackground(),与 v1.1.0 的
// ssh 示例行为一致。
type HybridColor struct {
	lipgloss.CompleteAdaptiveColor
}

// hybridColor 由语义槽构造 HybridColor。ANSI256 缺省时以 TrueColor(hex)
// 兜底:termenv 会在 ANSI256 终端把 hex 自动降级为最近的 256 色索引。
func hybridColor(s Slot) HybridColor {
	return HybridColor{lipgloss.CompleteAdaptiveColor{
		Light: toLipglossColor(s.Light),
		Dark:  toLipglossColor(s.Dark),
	}}
}

// toLipglossColor 将 theme.CompleteColor 转换为 lipgloss.CompleteColor。
// ANSI256 为空时填入 TrueColor hex,使 256 色终端也能正确降级渲染。
func toLipglossColor(c CompleteColor) lipgloss.CompleteColor {
	ansi256 := c.ANSI256
	if ansi256 == "" {
		ansi256 = c.TrueColor
	}
	return lipgloss.CompleteColor{TrueColor: c.TrueColor, ANSI256: ansi256, ANSI: c.ANSI}
}

// RGBA 返回 Light 侧 TrueColor 的 RGBA 值。
func (c HybridColor) RGBA() (r, g, b, a uint32) {
	return lipgloss.Color(c.Light.TrueColor).RGBA()
}
