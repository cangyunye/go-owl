package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/tui/nodes"
)

type App struct {
	nodes       nodes.Model
	help        bool
	quitConfirm bool
}

func NewApp(store common.NodeStore) *App {
	return &App{nodes: nodes.NewModel(store)}
}

func (m *App) Init() tea.Cmd { return nil }

func (m *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.quitConfirm {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "y", "enter":
				return m, tea.Quit
			case "n", "esc":
				m.quitConfirm = false
			}
		}
		return m, nil
	}
	if m.help {
		if km, ok := msg.(tea.KeyMsg); ok {
			if km.String() == "esc" || km.String() == "?" {
				m.help = false
			}
		}
		return m, nil
	}
	if m.nodes.Mode() != nodes.ModeNormal {
		// Insert 态隔离:键仅转发给 nodes 更新状态,不冒泡子模型 cmd(如 textinput 光标 blink)
		_, _ = m.forward(msg)
		return m, nil
	}
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "q":
			if m.nodes.IsDirty() {
				m.quitConfirm = true
				return m, nil
			}
			return m, tea.Quit
		case "ctrl+c":
			return m, tea.Quit
		case "?":
			m.help = true
			return m, nil
		}
	}
	return m.forward(msg)
}

func (m *App) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	nm, cmd := m.nodes.Update(msg)
	m.nodes = nm.(nodes.Model)
	return m, cmd
}

func (m *App) View() string {
	var b strings.Builder
	path := "/" + strings.Join(m.nodes.Path(), "/")
	mode := "Normal"
	if m.nodes.Mode() == nodes.ModeInsert {
		mode = "Insert"
	}
	b.WriteString(fmt.Sprintf("%s   Mode:%s\n", path, mode))
	b.WriteString(strings.Repeat("─", 60) + "\n")
	b.WriteString(m.nodes.View())
	if m.help {
		b.WriteString("\n\n" + helpView())
	}
	if m.quitConfirm {
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
