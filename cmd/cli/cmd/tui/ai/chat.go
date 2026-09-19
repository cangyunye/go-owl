package ai

import (
	"context"
	"fmt"
	"strings"
	"time"

	bubbleskey "github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	aisetup "github.com/cangyunye/go-owl/cmd/cli/cmd/ai"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/ai/input"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/i18n"
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

// spinnerFrames 状态行 spinner(braille 字符),随 FlushTickMsg 推进。
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// panelChromeRows 面板自身固定占用行数: 状态行 1 + 输入框 4 + 提示行 1。
const panelChromeRows = 6

// appChromeRows App 层固定占用行数: 菜单栏 + 路径/模式 + 分隔线。
const appChromeRows = 3

// maxMenuRows 斜杠菜单最多渲染行数。
const maxMenuRows = 4

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
	spinnerIdx   int    // 状态行 spinner 帧
	prog         msgSink
	menu         *input.SlashMenu // 斜杠命令补全(纯逻辑状态机,复用 CLI 实现)
	menuOpen     bool

	session *owlavi.Session
	sender  Sender
	ta      textarea.Model
	view    viewport.Model

	width  int
	height int
}

func NewModel(store common.NodeStore) Model {
	m := Model{
		store:  store,
		ta:     newTextarea(),
		view:   viewport.New(78, 9),
		width:  78,
		height: 9,
		menu:   input.NewSlashMenu(tuiSlashCommands()),
	}
	m.resetSession()
	m.sender = m.session
	return m
}

// tuiSlashCommands TUI 侧斜杠命令目录: 共享 task 模板 + TUI 语义的 action。
// 不用 Action 闭包(confirm 按 Name 分发),避免 Model 值拷贝导致的闭包别名。
func tuiSlashCommands() []input.SlashCommand {
	return append(aisetup.TaskSlashCommands(),
		input.SlashCommand{Name: "help", Category: "action", Icon: "ℹ️", Label: i18n.T("ai.slash.help_label"), Desc: i18n.T("ai.slash.help_desc")},
		input.SlashCommand{Name: "new", Category: "action", Icon: "➕", Label: i18n.T("ai.slash.new_label"), Desc: i18n.T("ai.slash.new_desc")},
		input.SlashCommand{Name: "clear", Category: "action", Icon: "🗑️", Label: i18n.T("ai.slash.clear_label"), Desc: i18n.T("ai.slash.clear_desc")},
	)
}

func newTextarea() textarea.Model {
	ta := textarea.New()
	ta.Placeholder = "输入指令, / 唤起命令…"
	ta.ShowLineNumbers = false
	ta.SetHeight(2)
	ta.CharLimit = 4096
	// Enter 留给发送,换行走 Ctrl+J
	ta.KeyMap.InsertNewline = bubbleskey.NewBinding(bubbleskey.WithKeys("ctrl+j"))
	ta.Blur()
	return ta
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

// FocusInput 面板被激活时由 App 调用:自动进入输入模式(对话式,免先按 Enter)。
func (m *Model) FocusInput() {
	m.mode = ModeInsert
	m.ta.Focus()
}

// BlurInput 面板失活时由 App 调用:回到 Normal 模式。
func (m *Model) BlurInput() {
	m.mode = ModeNormal
	m.ta.Blur()
}

func (m Model) IsDirty() bool { return false }

func (m Model) Path() []string { return []string{"ai"} }

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// 宽度预算: 左右各留 2 列边距,内容不贴窗口边缘
		if msg.Width > 4 {
			m.width = msg.Width - 4
		}
		// 高度精确预算: App chrome 3 + 状态行 1 + 视口 + 输入框 4 + 提示行 1
		if msg.Height > appChromeRows+panelChromeRows {
			m.height = msg.Height - appChromeRows - panelChromeRows
		}
		m.view.Width = m.width
		m.view.Height = m.height - m.menuRows()
		m.ta.SetWidth(m.width - 6)
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
		// 节拍双职: 推进状态行 spinner + 节流刷新视口
		m.spinnerIdx = (m.spinnerIdx + 1) % len(spinnerFrames)
		m.tickArmed = false
		if m.streaming && m.pendingFlush {
			m.pendingFlush = false
			m.refreshViewport()
		}
		// 续订只跟 busy 走(spinner 生命周期);闲时的孤儿节拍直接熄火
		if m.busy {
			m.tickArmed = true
			return m, flushTickCmd()
		}
		return m, nil
	case ToolProgressMsg:
		switch msg.Phase {
		case "generate":
			m.toolPhase = "🤖 正在生成 " + msg.Name + "…"
		case "execute":
			m.toolPhase = "🔧 正在执行 " + msg.Name + "…"
			// 工具活动进对话流(仅渲染,不进 LLM 上下文)
			m.messages = append(m.messages, ChatMsg{Role: "tool", Content: msg.Name})
			m.refreshViewport()
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
			// 菜单打开时优先消费导航/确认键,↑↓ 不进输入框
			if m.menuOpen {
				switch km.String() {
				case "up":
					m.menu.MoveUp()
					return m, nil
				case "down":
					m.menu.MoveDown()
					return m, nil
				case "tab", "enter":
					cmd := m.confirmSlash()
					return m, cmd
				case "esc":
					m.menuOpen = false
					m.syncViewportHeight()
					return m, nil
				}
			}
			switch km.String() {
			case "esc":
				m.mode = ModeNormal
				m.ta.Blur()
				return m, nil
			case "enter":
				text := strings.TrimSpace(m.ta.Value())
				m.ta.Reset()
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
				m.tickArmed = true // spinner 从发送即开始转动
				m.status = "AI 处理中…"
				m.refreshViewportFollow()
				return m, tea.Batch(m.sendCmd(text), flushTickCmd())
			}
			var cmd tea.Cmd
			m.ta, cmd = m.ta.Update(msg)
			m.syncMenu()
			return m, cmd
		}
		var cmd tea.Cmd
		m.ta, cmd = m.ta.Update(msg)
		return m, cmd
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "enter", "i":
		m.mode = ModeInsert
		m.ta.Focus()
	case "n":
		if m.busy {
			m.status = "处理中,请等待…"
			return m, nil
		}
		m.resetChatState()
	case "up", "down", "pgup", "pgdown", "home", "end":
		var cmd tea.Cmd
		m.view, cmd = m.view.Update(msg)
		return m, cmd
	case "esc":
		return m, func() tea.Msg { return LeavePanelMsg{} }
	}
	return m, nil
}

// resetChatState 清空对话并重建会话(n 键/斜杠命令共用)。
func (m *Model) resetChatState() {
	m.messages = nil
	m.status = ""
	m.streaming = false
	m.streamBuf = ""
	m.toolPhase = ""
	m.pendingFlush = false
	m.resetSession()
	m.refreshViewportFollow()
}

// syncMenu 按当前输入更新斜杠菜单开关与过滤词,并同步视口高度。
func (m *Model) syncMenu() {
	v := m.ta.Value()
	if strings.HasPrefix(v, "/") {
		m.menuOpen = true
		m.menu.SetQuery(strings.TrimPrefix(v, "/"))
	} else {
		m.menuOpen = false
	}
	m.syncViewportHeight()
}

// menuRows 菜单渲染占用行数(0=关闭);无匹配时渲染 1 行占位提示。
func (m *Model) menuRows() int {
	if !m.menuOpen {
		return 0
	}
	n := m.menu.Len()
	if n == 0 {
		return 1
	}
	if n > maxMenuRows {
		n = maxMenuRows
	}
	return n
}

// syncViewportHeight 菜单打开时收缩视口,保证面板总行数不超窗口。
func (m *Model) syncViewportHeight() {
	m.view.Height = m.height - m.menuRows()
}

// confirmSlash 菜单确认: action 按 Name 分发,task 展开模板并定位占位符。
func (m *Model) confirmSlash() tea.Cmd {
	cmd, ok := m.menu.Selected()
	m.menuOpen = false
	m.ta.Reset()
	m.syncViewportHeight()
	if !ok {
		return nil
	}
	if cmd.Category == "action" {
		switch cmd.Name {
		case "help":
			m.messages = append(m.messages, ChatMsg{Role: "assistant", Content: m.helpText()})
			m.refreshViewportFollow()
		case "new", "clear":
			m.resetChatState()
		}
		return nil
	}
	// task 类: 填入提示词模板,光标定位到首个 {arg}
	m.ta.SetValue(cmd.Template)
	if start, _, ok := input.PlaceholderRange(cmd.Template); ok {
		m.ta.SetCursor(start)
	}
	return nil
}

// helpText 斜杠帮助: 命令目录 + 快捷键(仅作为对话流消息渲染,不进 LLM)。
func (m Model) helpText() string {
	var b strings.Builder
	b.WriteString("斜杠命令:\n")
	for _, c := range tuiSlashCommands() {
		b.WriteString(fmt.Sprintf("  /%s %s %s — %s\n", c.Name, c.Icon, c.Label, c.Desc))
	}
	b.WriteString("快捷键: Enter 发送 · Ctrl+J 换行 · Esc 失焦/返回 · Tab 切面板 · 失焦后 n 新会话\n")
	b.WriteString("退出 owl tui 请用 q 或 Ctrl+C")
	return b.String()
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

