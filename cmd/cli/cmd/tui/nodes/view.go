package nodes

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	styleListBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	styleDetail     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	styleError      = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	styleDim        = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleSelected   = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
)

func (m Model) selectedColumns() []Column {
	cols := make([]Column, 0, len(m.columns))
	for _, k := range m.columns {
		if c, ok := colByKey(k); ok {
			cols = append(cols, c)
		}
	}
	return cols
}

func (m Model) View() string {
	switch m.current() {
	case LocNew, LocEdit:
		return m.listPane() + "\n\n" + m.formView()
	case LocDelete:
		return m.listPane() + "\n\n" + m.confirmView()
	case LocColumns:
		return m.listPane() + "\n\n" + m.columnsView()
	default:
		return m.listPane() + m.statusBar()
	}
}

func (m Model) listPane() string {
	cols := m.selectedColumns()
	avail := m.width / 2
	widths := computeColumnWidths(cols, avail)
	var b strings.Builder
	for i, c := range cols {
		b.WriteString(styleSelected.Render(truncateCell(c.Label, widths[i])))
		b.WriteString(" ")
	}
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", sum(widths)+len(cols)) + "\n")
	v := m.visible()
	for i, n := range v {
		marker := " "
		if i == m.cursor {
			marker = ">"
		}
		b.WriteString(marker)
		for j, c := range cols {
			cell := truncateCell(cellValue(n, c.Key), widths[j])
			if i == m.cursor {
				cell = styleSelected.Render(cell)
			}
			b.WriteString(cell)
			b.WriteString(" ")
		}
		b.WriteString("\n")
	}
	if len(v) == 0 {
		b.WriteString(styleDim.Render("  (无匹配节点,按 / 修改过滤或 a 添加)"))
		b.WriteString("\n")
	}
	listBox := styleListBorder.Width(avail + 2).Render(b.String())
	detailBox := styleDetail.Width(avail + 2).Render(m.detailPane())
	if m.focus == paneDetail {
		listBox = styleDim.Render(b.String())
		listBox = styleListBorder.Width(avail + 2).Render(listBox)
		detailBox = styleSelected.Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("14")).Render(m.detailPane())
		detailBox = styleDetail.Width(avail + 2).Render(detailBox)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, listBox, "  ", detailBox)
}

func (m Model) detailPane() string {
	n := m.selectedNode()
	if n == nil {
		return "  " + styleDim.Render("(未选择节点)")
	}
	var b strings.Builder
	rows := [][2]string{
		{"ID", n.ID}, {"Name", n.Name}, {"Address", fmt.Sprintf("%s:%d", n.Address, n.Port)},
		{"User", n.User}, {"Status", n.Status}, {"Groups", strings.Join(n.Groups, ",")},
		{"Labels", sortedLabels(n.Labels)}, {"ProxyJump", n.ProxyJump},
		{"SSHKey", n.SSHKey}, {"LastCheck", n.LastCheckAt}, {"CreatedAt", n.CreatedAt},
	}
	for _, r := range rows {
		if r[1] == "" {
			r[1] = "—"
		}
		b.WriteString(fmt.Sprintf("%-12s %s\n", r[0], r[1]))
	}
	return b.String()
}

func (m Model) statusBar() string {
	var chips []string
	for _, g := range m.filter.Groups {
		chips = append(chips, "g:"+g)
	}
	for k, v := range m.filter.Labels {
		chips = append(chips, "l:"+k+"="+v)
	}
	if m.filter.Status != "" {
		chips = append(chips, "s:"+m.filter.Status)
	}
	var b strings.Builder
	if len(chips) > 0 {
		b.WriteString(styleSelected.Render("[" + strings.Join(chips, " ") + "]"))
		b.WriteString("  ")
	}
	if m.filterOpen {
		b.WriteString(m.filterInput.View())
		b.WriteString(styleDim.Render("  Enter 应用  Esc 取消"))
	} else {
		b.WriteString(styleDim.Render("↑↓选择 ←→切栏 g/G首尾 a添加 e编辑 d删除 c列 /过滤 ?帮助 q退出"))
	}
	if m.status != "" {
		b.WriteString("  " + styleDim.Render(m.status))
	}
	return "\n" + b.String()
}

func (m Model) formView() string {
	f := m.form
	if f == nil {
		return ""
	}
	title := "添加节点"
	if f.mode == FormEdit {
		title = "编辑节点 " + f.nodeID()
	}
	var b strings.Builder
	b.WriteString("┌─ " + title + " ───────────────\n")
	for i, fd := range f.fields {
		marker := " "
		if i == f.cursor && m.mode == ModeNormal {
			marker = ">"
		}
		req := ""
		if fd.required {
			req = "*"
		}
		b.WriteString(fmt.Sprintf("%s %s%-10s %s\n", marker, req, fd.label, fd.input.View()))
	}
	if f.confirmDiscard {
		b.WriteString(styleError.Render("  有未保存修改,确认丢弃? y/n"))
	} else if f.error != "" {
		b.WriteString(styleError.Render("  " + f.error))
	} else {
		b.WriteString(styleDim.Render("  ↑↓移动 Enter编辑 s保存 Esc返回 ?帮助"))
	}
	b.WriteString("\n└─")
	return b.String()
}

func (m Model) confirmView() string {
	c := m.confirm
	if c == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("┌─ 删除节点 ─────────────\n")
	b.WriteString(fmt.Sprintf("  确定删除节点 %s (%s)?\n", c.node.ID, c.node.Name))
	if c.cursor == 0 {
		b.WriteString(styleSelected.Render("  [Delete]") + "   [Cancel]\n")
	} else {
		b.WriteString("   [Delete]  " + styleSelected.Render("[Cancel]") + "\n")
	}
	if c.error != "" {
		b.WriteString(styleError.Render("  "+c.error) + "\n")
	}
	b.WriteString(styleDim.Render("  ←→选择 Enter执行 Esc返回"))
	b.WriteString("\n└─")
	return b.String()
}

func (m Model) columnsView() string {
	cm := m.columnsModel
	if cm == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("┌─ 表格列配置 ────────────\n")
	for i, k := range cm.order {
		marker := "[ ]"
		if cm.checked[i] {
			marker = "[x]"
		}
		line := fmt.Sprintf("  %s %s", marker, k)
		if i == cm.cursor {
			line = styleSelected.Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString(styleDim.Render("  ↑↓移动 Space切换 A全选 R重置 Enter应用 Esc取消"))
	b.WriteString("\n└─")
	return b.String()
}

func sum(ns []int) int {
	total := 0
	for _, n := range ns {
		total += n
	}
	return total
}
