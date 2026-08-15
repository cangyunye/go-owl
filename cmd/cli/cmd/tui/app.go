package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/tui/exec"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/tui/file"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/tui/nodes"
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
	panel int // 0=Nodes 1=Exec 2=File

	Help        bool
	QuitConfirm bool
}

var panelNames = []string{"Nodes", "Exec", "File"}

func NewApp(store common.NodeStore) *App {
	m := &App{nodes: nodes.NewModel(store)}
	m.exec = exec.NewModel(store)
	m.exec.CaptureTargets(m.nodes.Visible())
	m.file = file.NewModel(store)
	m.file.CaptureTargets(m.nodes.Visible())
	return m
}

func (m *App) Init() tea.Cmd { return nil }

func (m *App) currentPanel() Panel {
	switch m.panel {
	case 1:
		return &m.exec
	case 2:
		return &m.file
	default:
		return &m.nodes
	}
}

func (m *App) switchPanel(i int) {
	if i < 0 || i >= len(panelNames) || i == m.panel {
		return
	}
	if m.panel == 1 {
		m.exec.CancelRun()
	}
	m.panel = i
	if m.panel == 1 {
		m.exec.CaptureTargets(m.nodes.Visible())
	}
	if m.panel == 2 {
		m.file.CaptureTargets(m.nodes.Visible())
	}
}

func (m *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(exec.LeavePanelMsg); ok {
		m.switchPanel(0)
		return m, nil
	}
	if _, ok := msg.(file.LeavePanelMsg); ok {
		m.switchPanel(0)
		return m, nil
	}
	switch msg.(type) {
	case file.UploadDoneMsg, file.DownloadDoneMsg:
		// 传输完成消息直接路由到 File 面板,即使当前处于其他面板,避免结果丢失导致 loading 卡死
		fm, cmd := m.file.Update(msg)
		m.file = fm.(file.FileModel)
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
			m.switchPanel((m.panel + 1) % 3)
			return m, nil
		case "1":
			m.switchPanel(0)
			return m, nil
		case "2":
			m.switchPanel(1)
			return m, nil
		case "3":
			m.switchPanel(2)
			return m, nil
		case "f":
			if m.panel == 0 && m.nodes.AtList() {
				m.switchPanel(2)
				return m, nil
			}
		case "x":
			if m.panel == 0 {
				m.switchPanel(1)
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
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	var parts []string
	for i, name := range panelNames {
		if i == active {
			parts = append(parts, activeStyle.Render("["+name+"]"))
		} else {
			parts = append(parts, "["+name+"]")
		}
	}
	return strings.Join(parts, " ") + dim.Render("  Tab 切换  1/2/3 直达")
}

func helpView() string {
	return strings.Join([]string{
		"┌─ 帮助 ─────────────────────────────",
		"  菜单:  Tab 切换  1/2/3 直达  x 快捷执行  f 快捷文件",
		"  列表:  ↑↓ 选择  ←→ 切栏  g/G 首尾",
		"        a 添加  e 编辑  d 删除  c 列配置",
		"        p ping  k SSH检查  i 导入导出  o 分组  l 标签",
		"        / 过滤: 关键词 | g:组 l:k=v s:状态(空格或&&=AND)",
		"        例: g:web && l:env=prod  → 组含web且env=prod的节点",
		"        例: s:online  → 状态为在线的节点  ? 帮助  q 退出",
		"  表单:  ↑↓ 移动字段(首尾回卷)  Enter 编辑",
		"        s 保存  Esc 返回/退出输入  ? 帮助",
		"  执行:  命令必填  r 执行  a 高级选项  f 格式",
		"        ↑↓ 移动字段  Enter 编辑  Esc 返回 Nodes",
		"  文件:  ↑↓ 移动字段  Enter 编辑  ←→ 操作(upload/download)",
		"        a 高级选项  r 执行  Esc 返回 Nodes",
		"  模式:  Normal=命令   Insert=输入(Esc 退出)",
		"└────────────────────────────────────",
	}, "\n")
}
