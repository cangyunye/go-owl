package ai

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	aisetup "github.com/cangyunye/go-owl/cmd/cli/cmd/ai"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	owlavi "github.com/cangyunye/go-owl/internal/ai"
)

// ChatDoneMsg 会话 Send 完成回传。
type ChatDoneMsg struct {
	Text string
	Err  error
}

// Sender 会话发送接口; *owlavi.Session 天然满足,测试注入 fake。
type Sender interface {
	Send(ctx context.Context, input string) (string, error)
}

type Mode int

const (
	ModeNormal Mode = iota
	ModeInsert
)

// LeavePanelMsg 请求 App 离开 AI 面板返回 Nodes。
type LeavePanelMsg struct{}

// ChatMsg 渲染用消息条目。
type ChatMsg struct {
	Role    string // "user" | "assistant"
	Content string
}

// newSessionFn 装配真实会话(测试可注入)。
var newSessionFn = func(store common.NodeStore) (*owlavi.Session, *owlavi.Config, error) {
	agent, cfg, err := aisetup.SetupSession(store, nil, false)
	if err != nil {
		return nil, nil, err
	}
	s := owlavi.NewSession(agent)
	s.SetDefaultConfirmGate()
	return s, cfg, nil
}

type Model struct {
	store common.NodeStore

	mode       Mode
	messages   []ChatMsg
	status     string
	modelLabel string
	busy       bool

	session *owlavi.Session
	sender  Sender
	input   textinput.Model
	view    viewport.Model

	width  int
	height int
}

func NewModel(store common.NodeStore) Model {
	m := Model{
		store:  store,
		input:  newInput(),
		view:   viewport.New(78, 18),
		width:  78,
		height: 18,
	}
	m.resetSession()
	m.sender = m.session
	return m
}

func newInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "输入指令… (Enter 发送, Esc 退出输入)"
	ti.Width = 40
	ti.CharLimit = 512
	ti.Blur()
	return ti
}

func (m *Model) resetSession() {
	session, cfg, err := newSessionFn(m.store)
	if err != nil {
		m.session = nil
		m.modelLabel = ""
		m.status = "AI 会话装配失败: " + err.Error()
		m.sender = m.session
		return
	}
	m.session = session
	m.status = ""
	if cfg != nil {
		m.modelLabel = cfg.AI.Provider + "/" + cfg.AI.Model
	}
	m.sender = m.session
}

func (m Model) InsertMode() bool { return m.mode != ModeNormal }

func (m Model) IsDirty() bool { return false }

func (m Model) Path() []string { return []string{"ai"} }

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width - 2
		}
		if msg.Height > 0 {
			m.height = msg.Height - 8
		}
		m.view.Width = m.width
		m.view.Height = m.height
		m.input.Width = m.width - 10
		return m, nil
	case ChatDoneMsg:
		m.busy = false
		if msg.Err != nil {
			m.messages = append(m.messages, ChatMsg{Role: "assistant", Content: "错误: " + msg.Err.Error()})
			m.status = "出错"
		} else {
			m.messages = append(m.messages, ChatMsg{Role: "assistant", Content: msg.Text})
			m.status = "完成"
		}
		m.refreshViewport()
		return m, nil
	}
	if m.mode == ModeInsert {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "esc":
				m.mode = ModeNormal
				m.input.Blur()
				return m, nil
			case "enter":
				text := strings.TrimSpace(m.input.Value())
				m.input.SetValue("")
				if text == "" {
					return m, nil
				}
				if m.busy {
					m.status = "处理中,请等待…"
					return m, nil
				}
				m.messages = append(m.messages, ChatMsg{Role: "user", Content: text})
				m.refreshViewport()
				m.busy = true
				m.status = "AI 处理中…"
				return m, m.sendCmd(text)
			}
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "enter", "i":
		m.mode = ModeInsert
		m.input.Focus()
	case "n":
		if m.busy {
			m.status = "处理中,请等待…"
			return m, nil
		}
		m.messages = nil
		m.status = ""
		m.resetSession()
		m.refreshViewport()
	case "up", "down", "pgup", "pgdown", "home", "end":
		var cmd tea.Cmd
		m.view, cmd = m.view.Update(msg)
		return m, cmd
	case "esc":
		return m, func() tea.Msg { return LeavePanelMsg{} }
	}
	return m, nil
}

func (m *Model) sendCmd(input string) tea.Cmd {
	sender := m.sender
	if sender == nil {
		return func() tea.Msg {
			return ChatDoneMsg{Err: fmt.Errorf("会话不可用")}
		}
	}
	return func() tea.Msg {
		ctx := context.Background()
		text, err := sender.Send(ctx, input)
		return ChatDoneMsg{Text: text, Err: err}
	}
}

func (m *Model) refreshViewport() {
	m.view.SetContent(renderMessages(m.messages, m.width))
	m.view.GotoBottom()
}

func (m Model) View() string {
	var b strings.Builder
	b.WriteString("┌─ AI Chat ──────────────────────────\n")
	if m.modelLabel != "" {
		b.WriteString("  模型  " + m.modelLabel + "\n")
	}
	if m.status != "" {
		b.WriteString("  " + m.status + "\n")
	}
	b.WriteString(m.view.View())
	b.WriteString("\n  " + m.input.View() + "\n")
	b.WriteString("  Enter 输入/发送  n 新会话  Esc 返回 Nodes\n")
	b.WriteString("└─")
	return b.String()
}
