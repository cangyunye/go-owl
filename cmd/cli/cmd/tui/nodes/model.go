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
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		if ws.Width > 0 {
			m.width = ws.Width
		}
		return m, nil
	}
	switch m.current() {
	case LocNew, LocEdit:
		return m.updateForm(msg)
	case LocDelete:
		return m.updateConfirm(msg)
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
	case "e":
		if n := m.selectedNode(); n != nil {
			m.push(LocEdit)
			m.openForm(FormEdit, n)
		}
	case "d":
		if n := m.selectedNode(); n != nil {
			m.push(LocDelete)
			m.openConfirm(n)
		}
	case "a":
		m.push(LocNew)
		m.openForm(FormAdd, nil)
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

func (m *Model) openForm(mode FormMode, node *common.NodeInfo) {
	m.form = NewFormModel(mode, node)
	m.mode = ModeNormal
}

func (m Model) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	f := m.form
	if f == nil {
		return m, nil
	}
	if f.confirmDiscard {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "y", "enter":
				m.pop()
				m.form = nil
				m.reload()
			case "n", "esc":
				f.confirmDiscard = false
			}
		}
		return m, nil
	}
	if m.mode == ModeInsert {
		if km, ok := msg.(tea.KeyMsg); ok && km.String() == "esc" {
			m.mode = ModeNormal
			f.fields[f.cursor].input.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		f.fields[f.cursor].input, cmd = f.fields[f.cursor].input.Update(msg)
		return m, cmd
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "up":
		f.move(-1)
	case "down":
		f.move(1)
	case "enter":
		m.mode = ModeInsert
		f.fields[f.cursor].input.Focus()
	case "s":
		return m.saveForm()
	case "esc":
		if f.IsDirty() {
			f.confirmDiscard = true
		} else {
			m.pop()
			m.form = nil
			m.reload()
		}
	}
	return m, nil
}

func (m Model) saveForm() (tea.Model, tea.Cmd) {
	f := m.form
	if err := f.validate(); err != "" {
		f.error = err
		f.focusFirstInvalid()
		return m, nil
	}
	f.error = ""
	node := f.toNode()
	var err error
	if f.mode == FormAdd {
		err = m.store.Add(node)
	} else {
		err = m.store.Update(node)
	}
	if err != nil {
		f.error = "保存失败: " + err.Error()
		return m, nil
	}
	if err := m.store.Save(); err != nil {
		f.error = "保存失败: " + err.Error()
		return m, nil
	}
	m.pop()
	m.form = nil
	m.reload()
	m.status = "已保存节点 " + node.ID
	return m, nil
}

func (m *Model) openConfirm(n *common.NodeInfo) {
	m.confirm = NewConfirmModel(n)
}

func (m Model) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	c := m.confirm
	if c == nil {
		return m, nil
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "left":
		c.cursor = 0
	case "right":
		c.cursor = 1
	case "esc":
		m.pop()
		m.confirm = nil
	case "enter":
		if c.cursor == 0 {
			if err := m.store.Remove(c.node.ID); err != nil {
				c.error = "删除失败: " + err.Error()
				return m, nil
			}
			if err := m.store.Save(); err != nil {
				c.error = "删除失败: " + err.Error()
				return m, nil
			}
			m.pop()
			m.confirm = nil
			m.reload()
			m.status = "已删除节点 " + c.node.ID
		} else {
			m.pop()
			m.confirm = nil
		}
	}
	return m, nil
}

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
	case "a", "A":
		cm.selectAll()
		m.columns = cm.selected()
	case "r", "R":
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
