package ai

import (
	"context"
	"fmt"
	"strings"
	"time"

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

// ChatDeltaMsg AI 回复的流式文本增量(内核 OnProgress "delta" 桥接)。
type ChatDeltaMsg struct {
	Text string
}

// ToolProgressMsg 工具调用阶段进度;Phase 为内核 OnProgress 的 "generate"/"execute",Name 为工具名。
type ToolProgressMsg struct {
	Phase string
	Name  string
}

// FlushTickMsg 流式期间节流刷新视口的节拍。
type FlushTickMsg struct{}

// streamFlushInterval 流式视口刷新节流间隔。
const streamFlushInterval = 90 * time.Millisecond

// streamCursor 流式进行中追加在缓冲尾部的光标。
const streamCursor = "▍"

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

// msgSink 跨 goroutine 投递 tea.Msg 的出口;*tea.Program 天然满足,测试注入 channel 桩。
type msgSink interface {
	Send(tea.Msg)
}

type Model struct {
	store common.NodeStore

	mode       Mode
	messages   []ChatMsg
	status     string
	modelLabel string
	busy       bool

	streaming    bool   // 流式进行中(发送即置位,done 结算)
	streamBuf    string // 进行中回复的累计增量
	toolPhase    string // 工具阶段状态栏文案
	tickArmed    bool   // flush tick 是否已挂起
	pendingFlush bool   // 有未刷入视口的增量
	prog         msgSink

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
	m.registerProgress()
}

// SetProgram 在 tea.NewProgram 之后回填 program,并为当前会话注册进度桥接。
func (m *Model) SetProgram(p *tea.Program) {
	if p == nil {
		return
	}
	m.prog = p
	m.registerProgress()
}

// registerProgress 把内核 OnProgress 事件桥接为 tea.Msg 投递到 program;
// OnProgress 是会话级字段,n 键重建会话后需重新注册。
func (m *Model) registerProgress() {
	if m.session == nil || m.prog == nil {
		return
	}
	m.session.OnProgress = m.bridgeProgress
}

// bridgeProgress 内核 OnProgress 回调入口,在 Send 的 goroutine 中触发;
// Program.Send 并发安全,负责跨 goroutine 送达主循环。
func (m *Model) bridgeProgress(step, detail string) {
	if m.prog == nil {
		return
	}
	if msg := progressMsg(step, detail); msg != nil {
		m.prog.Send(msg)
	}
}

// progressMsg 把内核进度事件映射为 tea.Msg;返回 nil 表示忽略。
func progressMsg(step, detail string) tea.Msg {
	switch step {
	case "delta":
		return ChatDeltaMsg{Text: detail}
	case "generate", "execute":
		return ToolProgressMsg{Phase: step, Name: detail}
	}
	return nil
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
	case ChatDeltaMsg:
		m.streaming = true
		m.streamBuf += msg.Text
		m.pendingFlush = true
		if m.tickArmed {
			return m, nil
		}
		m.tickArmed = true
		return m, flushTickCmd()
	case FlushTickMsg:
		m.tickArmed = false
		if !m.streaming {
			return m, nil
		}
		if m.pendingFlush {
			m.pendingFlush = false
			m.refreshViewport()
		}
		m.tickArmed = true
		return m, flushTickCmd()
	case ToolProgressMsg:
		switch msg.Phase {
		case "generate":
			m.toolPhase = "🤖 正在生成 " + msg.Name + "…"
		case "execute":
			m.toolPhase = "🔧 正在执行 " + msg.Name + "…"
		}
		return m, nil
	case ChatDoneMsg:
		// 结算: 服务端最终文本替换流式缓冲(两者内容一致,替换幂等)
		m.busy = false
		m.streaming = false
		m.streamBuf = ""
		m.toolPhase = ""
		m.pendingFlush = false
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
				m.busy = true
				m.streaming = true
				m.streamBuf = ""
				m.toolPhase = ""
				m.pendingFlush = false
				m.status = "AI 处理中…"
				m.refreshViewportFollow()
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
		m.streaming = false
		m.streamBuf = ""
		m.toolPhase = ""
		m.pendingFlush = false
		m.resetSession()
		m.refreshViewportFollow()
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

// flushTickCmd 流式节流节拍;streaming 期间由 FlushTickMsg 自续订。
func flushTickCmd() tea.Cmd {
	return tea.Tick(streamFlushInterval, func(time.Time) tea.Msg { return FlushTickMsg{} })
}

// refreshViewport 重设视口内容;仅当用户仍在底部时跟随滚动,上翻浏览不被打断。
func (m *Model) refreshViewport() {
	follow := m.view.AtBottom()
	m.setViewportContent()
	if follow {
		m.view.GotoBottom()
	}
}

// refreshViewportFollow 重设视口内容并强制滚到底(用户主动发送/重置后)。
func (m *Model) refreshViewportFollow() {
	m.setViewportContent()
	m.view.GotoBottom()
}

func (m *Model) setViewportContent() {
	content := renderMessages(m.messages, m.width)
	if m.streaming {
		if content != "" {
			content += "\n"
		}
		content += renderMsg(ChatMsg{Role: "assistant", Content: m.streamBuf + streamCursor}, m.width)
	}
	m.view.SetContent(content)
}

