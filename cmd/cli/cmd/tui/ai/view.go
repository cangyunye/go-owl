package ai

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/tui/theme"
)

var (
	styleUser  = lipgloss.NewStyle().Foreground(theme.Fg(theme.CUser))
	styleAI    = lipgloss.NewStyle().Foreground(theme.Fg(theme.CAI))
	styleDim   = lipgloss.NewStyle().Foreground(theme.Fg(theme.CDim))
	styleError = lipgloss.NewStyle().Foreground(theme.Fg(theme.CError))
)

func (m Model) View() string {
	var b strings.Builder
	b.WriteString("┌─ AI Chat ──────────────────────────\n")
	if m.modelLabel != "" {
		b.WriteString(styleDim.Render("  模型  "+m.modelLabel) + "\n")
	}
	if m.busy {
		b.WriteString("  " + styleAI.Render("● AI 处理中…") + "\n")
	} else if m.status != "" {
		b.WriteString(styleDim.Render("  "+m.status) + "\n")
	}
	b.WriteString(m.view.View())
	b.WriteString("\n─ 输入 ─────────────────────────────\n")
	if m.mode == ModeInsert {
		b.WriteString("  " + m.input.View() + "\n")
	} else {
		b.WriteString(styleDim.Render("  " + m.input.Placeholder) + "\n")
	}
	b.WriteString(styleDim.Render("  Enter 输入/发送  n 新会话  Esc 返回 Nodes") + "\n")
	b.WriteString("└─")
	return b.String()
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

func renderMsg(msg ChatMsg, width int) string {
	label := "你"
	style := styleUser
	if msg.Role == "assistant" {
		label = "AI"
		style = styleAI
	}
	return style.Render(label+": ") + wrapText(msg.Content, width-4)
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
