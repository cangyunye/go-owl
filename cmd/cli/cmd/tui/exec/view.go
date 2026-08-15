package exec

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

func (m ExecModel) View() string {
	switch m.current() {
	case LocAdvanced:
		return m.advancedView()
	case LocResult:
		return m.resultView()
	case LocDanger:
		return m.dangerView()
	default:
		return m.runView()
	}
}

func (m ExecModel) runView() string {
	var b strings.Builder
	b.WriteString("┌─ Exec Run ───────────────────────────\n")
	labels := []string{"命令", "节点", "分组", "标签"}
	for i := 0; i < 4; i++ {
		marker := " "
		if i == m.cursor && m.mode == ModeNormal {
			marker = ">"
		}
		b.WriteString(fmt.Sprintf("%s %s%-4s %s\n", marker, " ", labels[i], m.fieldAt(i).View()))
	}
	b.WriteString("  格式  " + styleSelected.Render(m.format) + styleDim.Render("  f 切换") + "\n")
	if nodes, err := m.resolveTargets(); err == nil {
		b.WriteString(styleDim.Render(fmt.Sprintf("  目标  %d 台", len(nodes))) + "\n")
	}
	if m.error != "" {
		b.WriteString(styleError.Render("  "+m.error) + "\n")
	}
	b.WriteString(styleDim.Render("  ↑↓移动 Enter编辑 f格式 a高级 r执行 Esc返回") + "\n")
	b.WriteString("└─")
	return b.String()
}

func (m ExecModel) advancedView() string { return "" }
func (m ExecModel) resultView() string   { return "" }
func (m ExecModel) dangerView() string   { return "" }
