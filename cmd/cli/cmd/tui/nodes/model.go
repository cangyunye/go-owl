package nodes

import (
	"sort"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

type Mode int

const (
	ModeNormal Mode = iota
	ModeInsert
)

type Location int

const (
	LocList Location = iota
	LocNew
	LocEdit
	LocDelete
	LocColumns
)

type pane int

const (
	paneList pane = iota
	paneDetail
)

type Model struct {
	store common.NodeStore

	stack  []Location
	mode   Mode
	focus  pane
	cursor int
	width  int

	nodes       []*common.NodeInfo
	filter      FilterQuery
	filterInput textinput.Model
	filterOpen  bool
	filterText  string

	columns      []string
	form         *FormModel
	confirm      *ConfirmModel
	columnsModel *ColumnsModel

	status string
}

func NewModel(store common.NodeStore) Model {
	m := Model{
		store:       store,
		stack:       []Location{LocList},
		columns:     append([]string(nil), defaultColumnKeys...),
		filterInput: newInput("/ 过滤 (g:组 l:标签)", 40),
		width:       120,
	}
	m.reload()
	return m
}

func newInput(placeholder string, width int) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Width = width
	ti.CharLimit = 256
	ti.Blur()
	return ti
}

func (m Model) Init() tea.Cmd { return nil }

// View 最小桩实现:保证 Model 满足 tea.Model 接口(Task 9 替换为完整渲染)
func (m Model) View() string { return "" }

func (m Model) current() Location { return m.stack[len(m.stack)-1] }

func (m *Model) push(l Location) { m.stack = append(m.stack, l) }

func (m *Model) pop() {
	if len(m.stack) > 1 {
		m.stack = m.stack[:len(m.stack)-1]
	}
}

func (m *Model) reload() {
	m.nodes, _ = m.store.List()
	sort.Slice(m.nodes, func(i, j int) bool { return m.nodes[i].ID < m.nodes[j].ID })
	m.clampCursor()
}

func (m *Model) clampCursor() {
	v := m.visible()
	if len(v) == 0 {
		m.cursor = 0
		return
	}
	if m.cursor >= len(v) {
		m.cursor = len(v) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *Model) moveCursor(d int) {
	v := m.visible()
	if len(v) == 0 {
		return
	}
	m.cursor += d
	m.clampCursor()
}

func (m Model) visible() []*common.NodeInfo {
	return applyFilter(m.nodes, m.filter)
}

func (m Model) selectedNode() *common.NodeInfo {
	v := m.visible()
	if m.cursor < 0 || m.cursor >= len(v) {
		return nil
	}
	return v[m.cursor]
}

func (m Model) selectedID() string {
	if n := m.selectedNode(); n != nil {
		return n.ID
	}
	return ""
}

func (m Model) Mode() Mode { return m.mode }

func (m Model) Path() []string {
	id := m.selectedID()
	switch m.current() {
	case LocNew:
		return []string{"nodes", "new"}
	case LocEdit:
		return []string{"nodes", id, "edit"}
	case LocDelete:
		return []string{"nodes", id, "delete"}
	case LocColumns:
		return []string{"nodes", "columns"}
	default:
		return []string{"nodes"}
	}
}

func (m Model) IsDirty() bool {
	if len(m.stack) > 1 {
		return true
	}
	return m.form != nil && m.form.IsDirty()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.current() {
	case LocColumns:
		return m.updateColumns(msg)
	default:
		return m.updateList(msg)
	}
}

func (m Model) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.filterOpen {
		return m.updateFilter(msg)
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "up":
		m.moveCursor(-1)
	case "down":
		m.moveCursor(1)
	case "left":
		m.focus = paneList
	case "right":
		m.focus = paneDetail
	case "g":
		m.cursor = 0
	case "G":
		m.cursor = len(m.visible()) - 1
	case "/":
		m.filterOpen = true
		m.mode = ModeInsert
		m.filterInput.Focus()
		// 每次打开从空输入开始:committed filter 只存于 filterText,live 编辑只改 filter
		m.filterInput.Reset()
	case "c":
		m.push(LocColumns)
		m.openColumns()
	}
	m.clampCursor()
	return m, nil
}

func (m Model) updateFilter(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok && km.String() == "esc" {
		m.filterOpen = false
		m.mode = ModeNormal
		m.filterInput.Blur()
		// 取消本次编辑:恢复到上一次已提交(Enter)的查询串与过滤
		m.filterInput.SetValue(m.filterText)
		m.filter = ParseFilterQuery(m.filterText)
		return m, nil
	}
	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	if km, ok := msg.(tea.KeyMsg); ok && km.String() == "enter" {
		// 提交:filterText 只在此刻更新为已应用查询串
		m.filterText = m.filterInput.Value()
		m.filter = ParseFilterQuery(m.filterText)
		m.filterOpen = false
		m.mode = ModeNormal
		m.filterInput.Blur()
		return m, cmd
	}
	// live 过滤:只改 filter,不改 filterText(否则 Esc 无法真正取消)
	m.filter = ParseFilterQuery(m.filterInput.Value())
	m.clampCursor()
	return m, cmd
}

// Task 6/8 将分别替换 FormModel / ConfirmModel 为完整实现。
// 占位:仅保证 model.go 能编译,含一个非导出方法避免 empty-struct lint 误伤。
type FormModel struct{ _ struct{} }

func (f *FormModel) IsDirty() bool { return false }

type ConfirmModel struct{ _ struct{} }

func (m *Model) openColumns() {
	m.columnsModel = NewColumnsModel(m.columns)
}

func (m Model) updateColumns(msg tea.Msg) (tea.Model, tea.Cmd) {
	cm := m.columnsModel
	if cm == nil {
		return m, nil
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "up":
		cm.cursor = (cm.cursor - 1 + len(cm.order)) % len(cm.order)
	case "down":
		cm.cursor = (cm.cursor + 1) % len(cm.order)
	case " ":
		cm.toggle(cm.cursor)
		m.columns = cm.selected()
	case "a":
		cm.selectAll()
		m.columns = cm.selected()
	case "r":
		cm.reset()
		m.columns = cm.selected()
	case "enter":
		m.pop()
		m.columnsModel = nil
	case "esc":
		cm.restoreSnapshot()
		m.columns = cm.selected()
		m.pop()
		m.columnsModel = nil
	}
	return m, nil
}
