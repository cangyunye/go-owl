package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	tuiai "github.com/cangyunye/go-owl/cmd/cli/cmd/tui/ai"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/tui/exec"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/tui/file"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/tui/nodes"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/tui/theme"
)

// Panel 顶层面板: 节点管理 / 命令执行 / 文件传输
type Panel interface {
	Update(msg tea.Msg) (tea.Model, tea.Cmd)
	View() string
	InsertMode() bool
	Path() []string
	IsDirty() bool
}

type App struct {
	nodes nodes.Model
	exec  exec.ExecModel
	file  file.FileModel
	ai    tuiai.Model
	panel int // 0=Nodes 1=Exec 2=File 3=AI

	Help        bool
	QuitConfirm bool
}

var panelNames = []string{"Nodes", "Exec", "File", "AI"}

func NewApp(store common.NodeStore) *App {
	m := &App{nodes: nodes.NewModel(store)}
	m.exec = exec.NewModel(store)
	m.exec.CaptureTargets(m.nodes.Visible())
	m.file = file.NewModel(store)
	m.file.CaptureTargets(m.nodes.Visible())
	m.ai = tuiai.NewModel(store)
	return m
}

func (m *App) Init() tea.Cmd { return nil }

func (m *App) currentPanel() Panel {
	switch m.panel {
	case 1:
		return &m.exec
	case 2:
		return &m.file
	case 3:
		return &m.ai
	default:
		return &m.nodes
	}
}

// Entry 进入面板的方式
type Entry int

const (
	EntryNeutral     Entry = iota // Tab/数字: 不动表单, 仅刷新快照
	EntryBySelection              // x/f: 勾选优先, 否则纯组/标签过滤条件
)

func (m *App) switchPanel(i int, entry Entry) {
	if i < 0 || i >= len(panelNames) || i == m.panel {
		return
	}
	if m.panel == 1 {
		m.exec.CancelRun()
	}
	m.panel = i
	if m.panel == 1 {
		m.exec.CaptureTargets(m.nodes.Visible())
		if entry == EntryBySelection {
			m.applySelectionEntry()
		}
	}
	if m.panel == 2 {
		m.file.CaptureTargets(m.nodes.Visible())
		if entry == EntryBySelection {
			m.applyFileSelectionEntry()
		}
	}
}

// applySelectionEntry 按 x 语义填充 Exec 表单: 勾选优先, 否则纯组/标签过滤; 含搜索/状态回退快照(当前可见集)
func (m *App) applySelectionEntry() {
	m.selectionFill(m.exec.FillNodes, m.exec.FillConditions)
}

// selectionFill 按 x/f 语义填充目标面板表单: 勾选优先, 否则纯组/标签过滤; 含搜索/状态回退快照(当前可见集)
func (m *App) selectionFill(fillNodes func([]string), fillConditions func([]string, map[string]string)) {
	if ids := m.nodes.MarkedIDs(); len(ids) > 0 {
		fillNodes(ids)
		return
	}
	fq := m.nodes.Filter()
	if fq.Search == "" && fq.Status == "" {
		fillConditions(fq.Groups, fq.Labels)
		return
	}
	fillConditions(nil, nil)
}

// applyFileSelectionEntry 按 f 语义填充 File 表单: 与 x 同款(勾选优先/纯过滤/快照兜底)
func (m *App) applyFileSelectionEntry() {
	m.selectionFill(m.file.FillNodes, m.file.FillConditions)
}

func (m *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(exec.LeavePanelMsg); ok {
		m.switchPanel(0, EntryNeutral)
		return m, nil
	}
	if _, ok := msg.(file.LeavePanelMsg); ok {
		m.switchPanel(0, EntryNeutral)
		return m, nil
	}
	if _, ok := msg.(tuiai.LeavePanelMsg); ok {
		m.switchPanel(0, EntryNeutral)
		return m, nil
	}
	switch msg.(type) {
	case file.UploadDoneMsg, file.DownloadDoneMsg:
		// 传输完成消息直接路由到 File 面板,即使当前处于其他面板,避免结果丢失导致 loading 卡死
		fm, cmd := m.file.Update(msg)
		m.file = fm.(file.FileModel)
		return m, cmd
	case tuiai.ChatDoneMsg:
		// AI 回复直接路由到 AI 面板,即使当前处于其他面板,避免 busy 永久卡死
		am, cmd := m.ai.Update(msg)
		m.ai = am.(tuiai.Model)
		return m, cmd
	}
	if m.QuitConfirm {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "y", "enter":
				return m, tea.Quit
			case "n", "esc":
				m.QuitConfirm = false
			}
		}
		return m, nil
	}
	if m.Help {
		if km, ok := msg.(tea.KeyMsg); ok {
			if km.String() == "esc" || km.String() == "?" {
				m.Help = false
			}
		}
		return m, nil
	}
	if m.currentPanel().InsertMode() {
		return m.forward(msg)
	}
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "q":
			if m.panel == 0 && m.nodes.IsDirty() {
				m.QuitConfirm = true
				return m, nil
			}
			return m, tea.Quit
		case "ctrl+c":
			return m, tea.Quit
		case "?":
			m.Help = true
			return m, nil
		case "tab":
			m.switchPanel((m.panel+1)%4, EntryNeutral)
			return m, nil
		case "1":
			m.switchPanel(0, EntryNeutral)
			return m, nil
		case "2":
			m.switchPanel(1, EntryNeutral)
			return m, nil
		case "3":
			m.switchPanel(2, EntryNeutral)
			return m, nil
		case "4":
			m.switchPanel(3, EntryNeutral)
			return m, nil
		case "f":
			if m.panel == 0 && m.nodes.AtList() {
				m.switchPanel(2, EntryBySelection)
				return m, nil
			}
		case "x":
			if m.panel == 0 {
				m.switchPanel(1, EntryBySelection)
				return m, nil
			}
		}
	}
	return m.forward(msg)
}

func (m *App) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	pm, cmd := m.currentPanel().Update(msg)
	switch m.panel {
	case 1:
		m.exec = pm.(exec.ExecModel)
	case 2:
		m.file = pm.(file.FileModel)
	case 3:
		m.ai = pm.(tuiai.Model)
	default:
		m.nodes = pm.(nodes.Model)
	}
	return m, cmd
}

func (m *App) View() string {
	var b strings.Builder
	p := m.currentPanel()
	mode := "Normal"
	if p.InsertMode() {
		mode = "Insert"
	}
	b.WriteString(menuBar(m.panel) + "\n")
	b.WriteString(fmt.Sprintf("/%s   Mode:%s\n", strings.Join(p.Path(), "/"), mode))
	b.WriteString(strings.Repeat("─", 60) + "\n")
	b.WriteString(p.View())
	if m.Help {
		b.WriteString("\n\n" + helpView())
	}
	if m.QuitConfirm {
		b.WriteString("\n\n退出并丢弃未保存修改? y/n")
	}
	return b.String()
}

func menuBar(active int) string {
	activeStyle := theme.Style(theme.SlotSelected)
	dim := theme.Style(theme.SlotDim)
	var parts []string
	for i, name := range panelNames {
		if i == active {
			parts = append(parts, activeStyle.Render("["+name+"]"))
		} else {
			parts = append(parts, "["+name+"]")
		}
	}
	return strings.Join(parts, " ") + dim.Render("  Tab 切换  1/2/3/4 直达")
}

func helpView() string {
	return strings.Join([]string{
		"┌─ 帮助 ─────────────────────────────",
		"  菜单:  Tab 切换  1/2/3/4 直达  x 快捷执行  f 快捷文件",
		"  列表:  ↑↓ 选择  ←→ 切栏  g/G 首尾",
		"        PgUp/PgDn 或 Ctrl+u/d 整页翻页(列表超屏自动滚动)",
		"        a 添加  e 编辑  d 删除  c 列配置",
		"        p ping  k SSH检查  i 导入导出  o 分组  l 标签",
		"        / 过滤: 关键词 | g:组 l:k=v s:状态(空格或&&=AND)",
		"        Space 勾选多选(x 带入 Exec)  ? 帮助  q 退出",
		"        例: g:web && l:env=prod  → 组含web且env=prod的节点",
		"        例: s:online  → 状态为在线的节点  ? 帮助  q 退出",
		"  表单:  ↑↓ 移动字段(首尾回卷)  Enter 编辑",
		"        s 保存  Esc 返回/退出输入  ? 帮助",
		"  执行:  命令必填  r 执行  a 高级选项  f 格式",
		"        ↑↓ 移动字段  Enter 编辑  Esc 返回 Nodes",
		"  文件:  ↑↓ 移动字段  Enter 编辑  ←→ 操作(upload/download)",
		"        a 高级选项  r 执行  Esc 返回 Nodes  f 勾选/筛选带入",
		"  AI:      Enter 输入  Enter 发送  n 新会话  Esc 返回",
		"  模式:  Normal=命令   Insert=输入(Esc 退出)",
		"└────────────────────────────────────",
	}, "\n")
}
