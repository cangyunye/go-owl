package ai

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/tui/theme"
)

var (
	styleUser  = theme.Style(theme.SlotUser)
	styleAI    = theme.Style(theme.SlotAI)
	styleDim   = theme.Style(theme.SlotDim)
	styleError = theme.Style(theme.SlotError)

	// inputBoxStyle 输入框圆角边框(SlotBorder 已有主题定义,此前未使用)。
	inputBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.Color(theme.SlotBorder)).
			Padding(0, 1)
)

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(m.statusLine() + "\n")
	b.WriteString(m.view.View())
	b.WriteString("\n")
	b.WriteString(m.inputBox())
	b.WriteString("\n")
	b.WriteString(styleDim.Render("  Enter 发送  Ctrl+J 换行  / 命令  Tab 切面板  Esc 失焦/返回"))
	return b.String()
}

// statusLine 常驻一行: busy 时 spinner+工具阶段,否则完成态/模型标签。
func (m Model) statusLine() string {
	if m.busy {
		icon := spinnerFrames[m.spinnerIdx%len(spinnerFrames)]
		text := m.toolPhase
		if text == "" {
			text = "AI 处理中…"
		}
		return "  " + styleAI.Render(icon+" "+text)
	}
	if m.status != "" {
		if m.status == "出错" {
			return "  " + styleError.Render(m.status)
		}
		return "  " + styleDim.Render(m.status)
	}
	if m.modelLabel != "" {
		return "  " + styleDim.Render("模型 " + m.modelLabel)
	}
	return "  " + styleDim.Render("模型 未配置")
}

// inputBox 圆角边框输入区(内容区 2 行,含边框 4 行)。
func (m Model) inputBox() string {
	return inputBoxStyle.Render(m.ta.View())
}

func renderMessages(msgs []ChatMsg, width int) string {
	var b strings.Builder
	for i, msg := range msgs {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(renderMsg(msg, width))
	}
	return b.String()
}

// renderMsg 对话式渲染: user=❯ 前缀,assistant=无前缀正文,tool=⏺ 活动行。
func renderMsg(msg ChatMsg, width int) string {
	switch msg.Role {
	case "user":
		return styleUser.Render("❯ ") + wrapText(msg.Content, width-3)
	case "tool":
		return styleDim.Render("  ⏺ " + msg.Content)
	default: // assistant
		return wrapText(msg.Content, width-2)
	}
}

// wrapText 按显示宽度(中文字符=2)硬换行,保持消息在固定宽度内。
func wrapText(s string, width int) string {
	if width < 4 {
		width = 4
	}
	var sb strings.Builder
	line := ""
	for _, ch := range s {
		if ch == '\n' {
			sb.WriteString(line + "\n")
			line = ""
			continue
		}
		w := runewidth.RuneWidth(ch)
		if runewidth.StringWidth(line)+w > width {
			sb.WriteString(line + "\n")
			line = ""
		}
		line += string(ch)
	}
	sb.WriteString(line)
	return sb.String()
}
