package nodes

import (
	"sort"
	"time"

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
	LocPing
	LocCheck
	LocImportExport
	LocGroups
	LocLabels
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

	marked map[string]bool

	columns      []string
	form         *FormModel
	confirm      *ConfirmModel
	columnsModel *ColumnsModel
	ping         *PingModel
	check        *CheckModel
	importExport *ImportExportModel
	groups       *GroupsModel
	labels       *LabelsModel

	status string
}

func NewModel(store common.NodeStore) Model {
	m := Model{
		store:       store,
		stack:       []Location{LocList},
		columns:     append([]string(nil), defaultColumnKeys...),
		filterInput: newInput("/ 过滤 (g:组 l:标签)", 40),
		marked:      map[string]bool{},
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

// AtList 是否位于节点列表主视图(非对话框/表单)
func (m Model) AtList() bool { return m.current() == LocList }

func (m Model) Visible() []*common.NodeInfo { return m.visible() }

func (m Model) InsertMode() bool { return m.mode != ModeNormal }

func (m Model) MarkedIDs() []string {
	ids := make([]string, 0, len(m.marked))
	for id := range m.marked {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (m Model) MarkedCount() int { return len(m.marked) }

func (m Model) IsMarked(id string) bool { return m.marked[id] }

func (m Model) Filter() FilterQuery { return m.filter }

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
	case LocPing:
		return []string{"nodes", "ping"}
	case LocCheck:
		return []string{"nodes", "check"}
	case LocImportExport:
		return []string{"nodes", "import"}
	case LocGroups:
		return []string{"nodes", m.selectedID(), "groups"}
	case LocLabels:
		return []string{"nodes", m.selectedID(), "labels"}
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
	case LocPing:
		return m.updatePing(msg)
	case LocCheck:
		return m.updateCheck(msg)
	case LocImportExport:
		return m.updateImportExport(msg)
	case LocGroups:
		return m.updateGroups(msg)
	case LocLabels:
		return m.updateLabels(msg)
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
	case " ":
		if n := m.selectedNode(); n != nil {
			if m.marked[n.ID] {
				delete(m.marked, n.ID)
			} else {
				m.marked[n.ID] = true
			}
		}
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
	case "p":
		m.push(LocPing)
		m.ping = NewPingModel(m.visible())
		return m, m.ping.Start()
	case "k":
		m.push(LocCheck)
		m.check = NewCheckModel(m.visible())
		return m, m.check.Start()
	case "i":
		m.push(LocImportExport)
		m.importExport = NewImportExportModel()
		m.mode = ModeInsert
		return m, textinput.Blink
	case "o":
		if n := m.selectedNode(); n != nil {
			m.push(LocGroups)
			m.groups = NewGroupsModel(m.store, n.ID)
			return m, nil
		}
	case "l":
		if n := m.selectedNode(); n != nil {
			m.push(LocLabels)
			m.labels = NewLabelsModel(m.store, n.ID)
			return m, nil
		}
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

func (m Model) updatePing(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case PingDoneMsg:
		if m.ping != nil {
			m.ping.results = msg.Results
			m.ping.loading = false
		}
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "esc" || msg.String() == "enter" {
			m.pop()
			m.ping = nil
			return m, nil
		}
	}
	return m, nil
}

func (m Model) updateCheck(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case CheckDoneMsg:
		if m.check != nil {
			m.check.results = msg.Results
			m.check.loading = false
		}
		// 回写 status/last_check 到 store
		now := time.Now().Format("2006-01-02 15:04:05")
		for _, r := range msg.Results {
			node, err := m.store.Get(r.Node.ID)
			if err != nil {
				continue
			}
			if r.Success {
				node.Status = "online"
			} else {
				node.Status = "offline"
			}
			node.LastCheckAt = now
			node.UpdatedAt = now
			_ = m.store.Update(node)
		}
		_ = m.store.Save()
		m.pop()
		m.check = nil
		m.reload()
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "esc" {
			m.pop()
			m.check = nil
			return m, nil
		}
	}
	return m, nil
}

func (m Model) updateImportExport(msg tea.Msg) (tea.Model, tea.Cmd) {
	ie := m.importExport
	if ie == nil {
		return m, nil
	}
	if m.mode == ModeInsert {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "esc":
				m.mode = ModeNormal
				ie.path.Blur()
				return m, nil
			case "enter":
				var err error
				if ie.op == "export" {
					err = m.doExport(ie.path.Value(), ie.format)
				} else {
					err = m.doImport(ie.path.Value(), ie.overwrite)
				}
				m.mode = ModeNormal
				if err != nil {
					ie.error = err.Error()
					ie.path.Blur()
					return m, nil
				}
				m.pop()
				m.importExport = nil
				m.status = "导入导出完成"
				return m, nil
			}
		}
		var cmd tea.Cmd
		ie.path, cmd = ie.path.Update(msg)
		return m, cmd
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "left", "right":
		if ie.op == "export" {
			ie.op = "import"
		} else {
			ie.op = "export"
		}
	case "f":
		if ie.format == "yaml" {
			ie.format = "json"
		} else {
			ie.format = "yaml"
		}
	case "o":
		ie.overwrite = !ie.overwrite
	case "e":
		m.mode = ModeInsert
		ie.path.Focus()
	case "esc":
		m.pop()
		m.importExport = nil
	case "enter":
		var err error
		if ie.op == "export" {
			err = m.doExport(ie.path.Value(), ie.format)
		} else {
			err = m.doImport(ie.path.Value(), ie.overwrite)
		}
		if err != nil {
			ie.error = err.Error()
			return m, nil
		}
		m.pop()
		m.importExport = nil
		m.status = "导入导出完成"
		return m, nil
	}
	return m, nil
}

func (m Model) updateGroups(msg tea.Msg) (tea.Model, tea.Cmd) {
	g := m.groups
	if g == nil {
		return m, nil
	}
	if g.adding {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "esc":
				g.adding = false
				g.input.Blur()
				m.mode = ModeNormal
				return m, nil
			case "enter":
				if err := g.addGroup(g.input.Value()); err != nil {
					g.error = err.Error()
				} else {
					g.error = ""
				}
				g.adding = false
				g.input.Blur()
				m.mode = ModeNormal
				return m, nil
			}
		}
		var cmd tea.Cmd
		g.input, cmd = g.input.Update(msg)
		return m, cmd
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "esc":
		m.pop()
		m.groups = nil
	case "up":
		if g.cursor > 0 {
			g.cursor--
		}
	case "down":
		if g.cursor < len(g.groups)-1 {
			g.cursor++
		}
	case "a":
		g.adding = true
		g.input.Focus()
		m.mode = ModeInsert
	case "d":
		if g.cursor >= 0 && g.cursor < len(g.groups) {
			if err := g.removeGroup(g.groups[g.cursor]); err != nil {
				g.error = err.Error()
			} else {
				g.error = ""
			}
		}
	}
	return m, nil
}

func (m Model) updateLabels(msg tea.Msg) (tea.Model, tea.Cmd) {
	l := m.labels
	if l == nil {
		return m, nil
	}
	if l.adding {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "esc":
				l.adding = false
				l.input.Blur()
				m.mode = ModeNormal
				return m, nil
			case "enter":
				if err := l.setLabel(l.input.Value()); err != nil {
					l.error = err.Error()
				} else {
					l.error = ""
				}
				l.adding = false
				l.input.Blur()
				m.mode = ModeNormal
				return m, nil
			}
		}
		var cmd tea.Cmd
		l.input, cmd = l.input.Update(msg)
		return m, cmd
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "esc":
		m.pop()
		m.labels = nil
	case "up":
		if l.cursor > 0 {
			l.cursor--
		}
	case "down":
		if l.cursor < len(l.keys)-1 {
			l.cursor++
		}
	case "a":
		l.adding = true
		l.input.Focus()
		m.mode = ModeInsert
	case "d":
		if l.cursor >= 0 && l.cursor < len(l.keys) {
			if err := l.removeLabel(l.keys[l.cursor]); err != nil {
				l.error = err.Error()
			} else {
				l.error = ""
			}
		}
	}
	return m, nil
}
