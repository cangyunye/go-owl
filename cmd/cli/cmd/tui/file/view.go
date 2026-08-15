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
	fileLabel := "本地文件"
	if m.op == OpDownload {
		fileLabel = "远程文件"
	}
	destLabel := "目标目录"
	if m.op == OpDownload {
		destLabel = "本地目录"
	}
	labels := []string{fileLabel, "节点", "分组", "标签", destLabel}
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

func (m FileModel) advancedView() string {
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
			line = fmt.Sprintf("%s %s %-14s Space 切换\n", marker, box, fd.label)
		} else {
			line = fmt.Sprintf("%s %-14s %s\n", marker, fd.label, fd.input.View())
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

func (m FileModel) resultView() string {
	var b strings.Builder
	title := "上传结果"
	if m.op == OpDownload {
		title = "下载结果"
	}
	b.WriteString("┌─ " + title + " ─────────────────────\n")
	if m.loading {
		if m.op == OpDownload {
			if m.lastDownload != nil {
				b.WriteString("  " + styleDim.Render("正在下载 "+m.lastDownload.remoteFile+" …") + "\n")
			}
		} else if m.lastUpload != nil {
			b.WriteString("  " + styleDim.Render("正在上传 "+m.lastUpload.localFile+" …") + "\n")
		}
	} else {
		success := 0
		for _, r := range m.results {
			mark := "✗"
			if r.Error == nil {
				mark = "✓"
				success++
			}
			method := r.Method
			if method == "" {
				method = "scp"
			}
			line := fmt.Sprintf("  %s %-24s %-5s %s\n", mark, r.NodeID, method, r.Path)
			if r.Error == nil {
				line = styleSelected.Render(line)
			}
			b.WriteString(line)
			if r.Error != nil {
				b.WriteString(styleError.Render("      "+r.Error.Error()) + "\n")
			} else if r.Speed != "" && r.Speed != "N/A" {
				b.WriteString("      " + styleDim.Render(r.Speed) + "\n")
			}
		}
		b.WriteString(styleDim.Render(fmt.Sprintf("  成功 %d/%d", success, len(m.results))) + "\n")
	}
	b.WriteString(styleDim.Render("  r 重跑  Esc 返回") + "\n")
	b.WriteString("└─")
	return b.String()
}
