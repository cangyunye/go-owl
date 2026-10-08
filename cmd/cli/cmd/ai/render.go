package ai

import (
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
	xterm "golang.org/x/term"
)

// maxRenderWidth 终端渲染折行上限：过宽的终端下表格/段落拉伸不可读。
const maxRenderWidth = 120

// renderReply 把 AI 回复渲染为终端 markdown（glamour）。
// 规则：
//   - 非 TTY（管道/重定向/tee）一律原样返回，输出干净无 ANSI；
//   - TTY 且内容带 markdown 特征 → glamour 渲染（深浅色终端自适应）；
//   - TTY 但无 markdown 特征（工具原始 ASCII 表格、普通句子）→ 原样返回，
//     避免段落重排毁掉等宽对齐。
func renderReply(s string, tty bool) string {
	if s == "" || !tty {
		return s
	}
	if hasPlainTableDivider(s) || !looksLikeMarkdown(s) {
		return s
	}
	rendered, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(terminalWidth()),
	)
	if err != nil {
		return s
	}
	out, err := rendered.Render(s)
	if err != nil {
		return s
	}
	return out
}

// hasPlainTableDivider 检测工具原始 ASCII 表格的分隔线（8+ 个连字符、无竖线）。
// markdown 表格分隔线是 |---| 形态不受影响；markdown hr（---）短于阈值也不误伤。
// 命中说明回复里贴了等宽表格（LLM 常把工具输出原样塞进总结），
// 整体进 glamour 会按段落重排拆毁表格对齐，调用方应直通。
func hasPlainTableDivider(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if len(t) >= 8 && strings.Trim(t, "-") == "" {
			return true
		}
	}
	return false
}

// looksLikeMarkdown 判断文本是否带 markdown 结构特征。
// 启发式：代码块/表格分隔行/标题/粗体/列表。任一命中即走渲染。
func looksLikeMarkdown(s string) bool {
	if strings.Contains(s, "```") || strings.Contains(s, "|---") ||
		strings.Contains(s, "**") {
		return true
	}
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "#") ||
			strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") ||
			strings.HasPrefix(t, "> ") {
			return true
		}
		// "1. " 有序列表
		if len(t) > 2 && t[0] >= '0' && t[0] <= '9' && t[1] == '.' && t[2] == ' ' {
			return true
		}
	}
	return false
}

// terminalWidth 返回 stdout 终端宽度；取不到时回退 80，封顶 maxRenderWidth。
func terminalWidth() int {
	w := 80
	if fd := int(os.Stdout.Fd()); fd >= 0 {
		if width, _, err := xterm.GetSize(fd); err == nil && width > 0 {
			w = width
		}
	}
	if w > maxRenderWidth {
		w = maxRenderWidth
	}
	return w
}
