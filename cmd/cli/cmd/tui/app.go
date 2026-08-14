package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/tui/nodes"
)

type App struct {
	Nodes       nodes.Model
	Help        bool
	QuitConfirm bool
}

func NewApp(store common.NodeStore) *App {
	return &App{Nodes: nodes.NewModel(store)}
}

func (m *App) Init() tea.Cmd { return nil }

func (m *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
	if m.Nodes.Mode() != nodes.ModeNormal {
		// Insert 态隔离:按键仅转发给 nodes(不过任何 keymap),但 cmd 必须冒泡
		// (textinput 的 blink tick 依赖它,否则光标不闪烁)
		return m.forward(msg)
	}
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "q":
			if m.Nodes.IsDirty() {
				m.QuitConfirm = true
				return m, nil
			}
			return m, tea.Quit
		case "ctrl+c":
			return m, tea.Quit
		case "?":
			m.Help = true
			return m, nil
		}
	}
	return m.forward(msg)
}

func (m *App) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	nm, cmd := m.Nodes.Update(msg)
	m.Nodes = nm.(nodes.Model)
	return m, cmd
}

func (m *App) View() string {
	var b strings.Builder
	path := "/" + strings.Join(m.Nodes.Path(), "/")
	mode := "Normal"
	if m.Nodes.Mode() == nodes.ModeInsert {
		mode = "Insert"
	}
	b.WriteString(fmt.Sprintf("%s   Mode:%s\n", path, mode))
	b.WriteString(strings.Repeat("─", 60) + "\n")
	b.WriteString(m.Nodes.View())
	if m.Help {
		b.WriteString("\n\n" + helpView())
	}
	if m.QuitConfirm {
		b.WriteString("\n\n退出并丢弃未保存修改? y/n")
	}
	return b.String()
}

func helpView() string {
	return strings.Join([]string{
		"┌─ 帮助 ─────────────────────────────",
		"  列表:  ↑↓ 选择  ←→ 切栏  g/G 首尾",
		"        a 添加  e 编辑  d 删除  c 列配置",
		"        / 过滤(g:组 l:标签 或搜索)  ? 帮助  q 退出",
		"  表单:  ↑↓ 移动字段(首尾回卷)  Enter 编辑",
		"        s 保存  Esc 返回/退出输入  ? 帮助",
		"  模式:  Normal=命令   Insert=输入(Esc 退出)",
		"└────────────────────────────────────",
	}, "\n")
}
