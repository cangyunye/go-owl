package file

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	styleSelected = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	styleDim      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleError    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)

func (m FileModel) View() string {
	switch m.current() {
	case LocAdvanced:
		return m.advancedView()
	case LocResult:
		return m.resultView()
	default:
		return m.fileView()
	}
}

func (m FileModel) fileView() string {
	var b strings.Builder
	b.WriteString("┌─ " + opLabels[m.op] + " ─────────────────────────\n")
	b.WriteString("  操作: " + styleSelected.Render(opLabels[m.op]) + styleDim.Render("  ←→ 切换") + "\n")
	labels := []string{"本地文件", "节点", "分组", "标签", "目标目录"}
	for i := 0; i < 5; i++ {
		marker := " "
		if i == m.cursor && m.mode == ModeNormal {
			marker = ">"
		}
		b.WriteString(fmt.Sprintf("%s %s%-6s %s\n", marker, " ", labels[i], m.fieldAt(i).View()))
	}
	if nodes, err := m.resolveTargets(); err == nil {
		b.WriteString(styleDim.Render(fmt.Sprintf("  目标 %d 台", len(nodes))) + "\n")
	}
	if m.advanced != nil {
		b.WriteString(styleDim.Render("  高级  "+advancedSummary(m.advanced)) + "\n")
	}
	if m.error != "" {
		b.WriteString(styleError.Render("  "+m.error) + "\n")
	}
	b.WriteString(styleDim.Render("  ↑↓移动 Enter编辑 ←→操作 a高级 r执行 Esc返回") + "\n")
	b.WriteString("└─")
	return b.String()
}
