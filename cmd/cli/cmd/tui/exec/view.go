package exec

import (
	"fmt"
	"strings"
	"time"

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
	if m.advanced != nil {
		b.WriteString(styleDim.Render("  高级  "+advancedSummary(m.advanced)) + "\n")
	} else {
		b.WriteString(styleDim.Render("  高级  默认(未设置)  a 打开") + "\n")
	}
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

func (m ExecModel) advancedView() string {
	f := m.advanced
	if f == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("┌─ 高级选项 ─────────────────────────\n")
	for i, fd := range f.fields {
		marker := " "
		if i == f.cursor && m.mode == ModeNormal {
			marker = ">"
		}
		line := ""
		if fd.kind == KindBool {
			box := "[ ]"
			if fd.checked {
				box = "[x]"
			}
			line = fmt.Sprintf("%s %s %-18s Space 切换\n", marker, box, fd.label)
		} else {
			line = fmt.Sprintf("%s %-18s %s\n", marker, fd.label, fd.input.View())
		}
		if i == f.cursor && m.mode == ModeNormal {
			line = styleSelected.Render(strings.TrimRight(line, "\n")) + "\n"
		}
		b.WriteString("  " + line)
	}
	if f.error != "" {
		b.WriteString(styleError.Render("  "+f.error) + "\n")
	}
	b.WriteString(styleDim.Render("  ↑↓移动 Enter编辑 Space切换bool s保存 Esc返回") + "\n")
	b.WriteString("└─")
	return b.String()
}
func (m ExecModel) resultView() string {
	var b strings.Builder
	b.WriteString("┌─ Exec 结果 ─────────────────────────\n")
	if m.loading {
		b.WriteString("  " + styleDim.Render("正在执行 "+m.lastCmd+" …") + "\n")
	} else {
		success := 0
		for _, r := range m.results {
			mark := "✗"
			if r.Success {
				mark = "✓"
				success++
			}
			line := fmt.Sprintf("  %s %-24s exit %-3d %s\n", mark, r.NodeID, r.ExitCode, r.Duration.Round(time.Millisecond))
			if r.Success {
				line = styleSelected.Render(line)
			}
			b.WriteString(line)
			if !r.Success && r.ErrorDetail != "" {
				b.WriteString(styleError.Render("      "+r.ErrorDetail) + "\n")
			} else if !r.Success && r.Error != nil {
				b.WriteString(styleError.Render("      "+r.Error.Error()) + "\n")
			}
			if r.Output != "" {
				out := r.Output
				if len([]rune(out)) > 500 {
					out = string([]rune(out)[:497]) + "..."
				}
				for _, l := range strings.Split(out, "\n") {
					b.WriteString("      " + l + "\n")
				}
			}
		}
		b.WriteString(styleDim.Render(fmt.Sprintf("  成功 %d/%d", success, len(m.results))) + "\n")
	}
	b.WriteString(styleDim.Render("  r 重跑  Esc 返回") + "\n")
	b.WriteString("└─")
	return b.String()
}
func (m ExecModel) dangerView() string {
	var b strings.Builder
	b.WriteString("┌─ 危险命令确认 ─────────────────────\n")
	b.WriteString("  命令: " + styleSelected.Render(m.pendingCmd) + "\n")
	for _, bl := range m.pendingBlocked {
		b.WriteString(fmt.Sprintf("  ✗ %s (user=%s)\n", bl.NodeID, bl.User))
		for _, mt := range bl.Matches {
			b.WriteString(styleError.Render("     匹配 "+mt.Pattern+": "+mt.Line) + "\n")
		}
	}
	b.WriteString(styleDim.Render("  继续执行? y 执行  n 取消") + "\n")
	b.WriteString("└─")
	return b.String()
}
