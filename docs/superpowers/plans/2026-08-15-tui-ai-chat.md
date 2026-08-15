# TUI AI 聊天面板实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 owl TUI 中新增第三个 AI 面板（菜单 `3` 直达），复用 `owl ai` 的 Agent/Session/确认门能力，实现聊天式 AI 对话展示：输入指令 → 异步发送 → 消息气泡滚动渲染 → 确认门问题/结果闭环。

**Architecture:** `cmd/ai` 包提取导出函数 `SetupSession` 统一装配（节点桥/管理器/CLI 执行器/LLM 配置/Agent），TUI 面板与 `owl ai` 命令共用。`cmd/tui/ai` 新包实现 `Model`：持有 `*ai.Session`（通过 `Sender` 接口可注入 fake 测试），`Session.Send` 阻塞调用封装为 `tea.Cmd`（bubbletea 自动在 goroutine 运行），`ChatDoneMsg` 回传结果并追加消息列表；`bubbles/viewport` 滚动渲染消息、`bubbles/textinput` 输入区。写操作确认门由 Session 内部拦截并返回问题文本，TUI 原样渲染，用户回复「是/否」自动完成重放闭环。

**Tech Stack:** Go, charmbracelet/bubbletea + bubbles/viewport + bubbles/textinput + lipgloss, mattn/go-runewidth（已有 indirect 依赖，转 direct）

## Global Constraints

- 项目目录：`F:\pantheon\trae_projects\git\go-owl`；禁止越出项目根目录
- TDD：每个任务先写失败测试再实现；测试命令 `go test ./cmd/cli/cmd/tui/...` 与 `go test ./cmd/cli/cmd/ai/...`
- 每个任务结束提交一个 atomic commit；全部完成后跑 E2E 冒烟再提交
- 面板文案沿用项目风格（中文硬编码，与 nodes/exec 包一致，不引入 i18n）
- 键位约束：App 层新增 `4` 直达 AI 面板（面板序 0=Nodes 1=Exec 2=File 3=AI，Tab 循环扩为四面板）；不得破坏 Nodes/Exec/File 现有键位（a/e/d/c/p/k/i/o/l/g/G//?/q/↑↓←→/Space、`3`=File、`f`=快捷文件、`x`=快捷执行）
- AI 面板内键位：Normal 模式 `Enter`/`i` 进入输入、`n` 新会话、`↑↓/PgUp/PgDn` 滚动、`Esc` 返回 Nodes；Insert 模式 `Enter` 发送、`Esc` 退出输入
- 复用 `internal/ai` 的 Agent/Session/确认门，不新建 LLM 逻辑；装配入口统一走 `cmd/ai.SetupSession`
- 注入模式沿用 ping/check 的 `var` 全局注入（`newSessionFn` 同款）
- 会话并发约束：busy 门保证一次只允许一个进行中请求，`Session.Send` 不并发调用
- AI 面板不支持流式（LLM 客户端为非流式 `Generate`），v1 用「处理中」状态 + 整包结果渲染

---

### Task 1: 提取 SetupSession 统一 AI 会话装配

**Files:**
- Create: `cmd/cli/cmd/ai/setup.go`
- Modify: `cmd/cli/cmd/ai/ai.go:205-270`（runAI 装配段替换为 SetupSession 调用）
- Test: `cmd/cli/cmd/ai/setup_test.go`

**Interfaces:**
- Consumes: `common.NodeStore`（已有）、`ai.LoadConfig(path)`（internal/ai，已有）、`ai.SetLogVerbose`/`ai.SetLLMLogVerbose`（internal/ai，已有）、`ai.NewNodeStoreBridge`/`ai.InitNodeManager`/`ai.NewCLIExecutor`/`ai.NewAgent`（internal/ai，已有）、`playbook.NewParser`（已有）、`createBridgeAdapter(store)`（cmd/ai/ai.go 已有）
- Produces: `SetupSession(store common.NodeStore, cfg *ai.Config, verbose bool) (*ai.Agent, *ai.Config, error)` —— Task 2~5 的 TUI 面板与 runAI 均依赖此签名；cfg 为 nil 时自动从 `~/.owl/config.yaml` 加载（含环境变量回退）

- [ ] **Step 1: 写失败测试** `cmd/cli/cmd/ai/setup_test.go`

```go
package ai

import (
	"path/filepath"
	"testing"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/ai"
)

func testStore(t *testing.T) common.NodeStore {
	t.Helper()
	store := common.NewInMemoryNodeStoreAt(filepath.Join(t.TempDir(), "nodes.json"))
	return store
}

func TestSetupSession_WithExplicitConfig(t *testing.T) {
	cfg := &ai.Config{AI: ai.AIConfig{
		Provider: "openai", Model: "gpt-4o",
		APIKey: "test-key", BaseURL: "http://localhost:1/v1", Timeout: 5,
	}}
	agent, gotCfg, err := SetupSession(testStore(t), cfg, false)
	if err != nil {
		t.Fatalf("SetupSession: %v", err)
	}
	if agent == nil {
		t.Fatal("agent is nil")
	}
	if gotCfg != cfg {
		t.Fatal("cfg should be passed through unchanged")
	}
}

func TestSetupSession_NilConfigLoadsFileOrDefault(t *testing.T) {
	agent, cfg, err := SetupSession(testStore(t), nil, false)
	if err != nil {
		t.Fatalf("SetupSession: %v", err)
	}
	if agent == nil {
		t.Fatal("agent is nil")
	}
	if cfg == nil || cfg.AI.Provider == "" {
		t.Fatal("cfg should be loaded from file or default")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./cmd/cli/cmd/ai/ -run TestSetupSession -v`
Expected: FAIL，编译报错 `undefined: SetupSession`

- [ ] **Step 3: 实现** `cmd/cli/cmd/ai/setup.go`

```go
package ai

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/ai"
	"github.com/cangyunye/go-owl/internal/control/playbook"
)

// SetupSession 装配 owl ai 的完整依赖链: 节点桥/管理器/CLI 执行器/LLM 配置/Agent。
// owl ai 命令与 TUI AI 面板共用此入口,避免装配逻辑漂移。
// cfg 为 nil 时自动从 ~/.owl/config.yaml 加载(含环境变量回退)。
func SetupSession(store common.NodeStore, cfg *ai.Config, verbose bool) (*ai.Agent, *ai.Config, error) {
	ai.SetLogVerbose(verbose)
	ai.SetLLMLogVerbose(verbose)

	nodeStoreAdapter := createBridgeAdapter(store)
	bridge := ai.NewNodeStoreBridge()
	if err := bridge.SyncFromStore(nodeStoreAdapter); err != nil {
		return nil, nil, fmt.Errorf("同步节点数据失败: %w", err)
	}

	nodeMgr := ai.InitNodeManager(bridge)
	if nodeMgr == nil {
		return nil, nil, fmt.Errorf("节点管理器初始化失败")
	}

	if cfg == nil {
		home, _ := os.UserHomeDir()
		fileConfig, err := ai.LoadConfig(filepath.Join(home, ".owl", "config.yaml"))
		if err != nil {
			fileConfig = ai.DefaultConfig()
		}
		cfg = fileConfig
	}

	executor := ai.NewCLIExecutor(nodeMgr, nodeStoreAdapter)
	playbookParser := playbook.NewParser()
	agent, err := ai.NewAgent(executor, cfg, nodeMgr, nodeStoreAdapter, playbookParser, verbose)
	if err != nil {
		return nil, nil, err
	}
	return agent, cfg, nil
}
```

- [ ] **Step 4: 改造 runAI 复用 SetupSession** `cmd/cli/cmd/ai/ai.go`

把第 209-210 行（`ai.SetLogVerbose`/`ai.SetLLMLogVerbose` 调用）与第 212-222 行（store/bridge/nodeMgr/playbookParser 装配段）删除，第 266-270 行替换为：

```go
	agent, _, err := SetupSession(store, config, aiVerbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to initialize Eino LLM: %v, using fallback mode\n", err)
	}
```

保留原有的 config 解析段（fileConfig 加载 + flag 合并，第 225-262 行）与 `store := common.GetNodeStore()`。

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./cmd/cli/cmd/ai/...`
Expected: 全部 PASS（含原 runAI 相关测试无回归）

- [ ] **Step 6: 提交**

```bash
git add cmd/cli/cmd/ai/setup.go cmd/cli/cmd/ai/setup_test.go cmd/cli/cmd/ai/ai.go
git commit -m "refactor(ai): 提取 SetupSession 统一 AI 会话装配(供 TUI 复用)"
```

---

### Task 2: AI 面板骨架 + App 第四面板接入（4-panel 适配）

**Files:**
- Create: `cmd/cli/cmd/tui/ai/chat.go`
- Create: `cmd/cli/cmd/tui/ai/view.go`
- Modify: `cmd/cli/cmd/tui/app.go`
- Test: `cmd/cli/cmd/tui/app_test.go`（追加用例）
- Test: `cmd/cli/cmd/tui/ai/chat_test.go`

**Interfaces:**
- Consumes: `SetupSession(store common.NodeStore, cfg *ai.Config, verbose bool) (*ai.Agent, *ai.Config, error)`（Task 1）、`ai.NewSession(agent)`/`ai.Session.SetDefaultConfirmGate()`（internal/ai，已有）、`common.NodeStore`（已有）
- Produces: `ai.Model`（含 `NewModel(store common.NodeStore) Model`、`InsertMode() bool`、`Path() []string`、`IsDirty() bool`、`Update`/`View`，实现 `Panel` 接口）、`ai.LeavePanelMsg`、`ai.ChatMsg{Role, Content}`、包级可注入 `var newSessionFn func(store common.NodeStore) (*ai.Session, *ai.Config, error)`（Task 3 测试依赖）、`App.panel` 取值 0/1/2/3（3=AI）

- [ ] **Step 1: 写失败测试** `cmd/cli/cmd/tui/app_test.go` 追加（注意：现有 app.go 已有 File 面板（panel 2、`3` 键），本任务将 AI 追加为 panel 3，键为 `4`；`newApp`/`key`/`runeKey` helper 已存在，直接复用）：

```go
func TestApp_Digit4JumpsToAI(t *testing.T) {
	m := newApp(t)
	nm, _ := m.Update(runeKey('4'))
	m = nm.(*App)
	if m.panel != 3 {
		t.Fatalf("expected panel 3 on '4', got %d", m.panel)
	}
	if got := m.View(); !strings.Contains(got, "[AI]") {
		t.Fatalf("menu bar missing [AI]: %s", got)
	}
}

func TestApp_TabCyclesFourPanels(t *testing.T) {
	m := newApp(t)
	for _, want := range []int{1, 2, 3, 0} {
		nm, _ := m.Update(key(tea.KeyTab))
		m = nm.(*App)
		if m.panel != want {
			t.Fatalf("expected panel %d after tab, got %d", want, m.panel)
		}
	}
}

func TestApp_AIEscReturnsToNodes(t *testing.T) {
	m := newApp(t)
	nm, _ := m.Update(runeKey('4'))
	m = nm.(*App)
	nm, _ = m.Update(tuiai.LeavePanelMsg{})
	m = nm.(*App)
	if m.panel != 0 {
		t.Fatalf("expected panel 0 after AI esc, got %d", m.panel)
	}
}
```

顶部 import 增加 `tuiai "github.com/cangyunye/go-owl/cmd/cli/cmd/tui/ai"`。

另建 `cmd/cli/cmd/tui/ai/chat_test.go` 骨架测试：

```go
package ai

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

func runeKey(r rune) tea.Msg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func TestChat_DefaultState(t *testing.T) {
	store := common.NewInMemoryNodeStoreAt("")
	m := NewModel(store)
	if m.InsertMode() {
		t.Fatal("should start in normal mode")
	}
	if p := m.Path(); len(p) != 1 || p[0] != "ai" {
		t.Fatalf("unexpected path: %v", p)
	}
	if m.IsDirty() {
		t.Fatal("AI panel never dirty")
	}
	if got := m.View(); got == "" {
		t.Fatal("view should not be empty")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./cmd/cli/cmd/tui/... -v`
Expected: FAIL，编译错误 `undefined: tuiai` / `cannot find package cmd/cli/cmd/tui/ai`

- [ ] **Step 3: 实现** `cmd/cli/cmd/tui/ai/chat.go`

```go
package ai

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	aisetup "github.com/cangyunye/go-owl/cmd/cli/cmd/ai"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	owlavi "github.com/cangyunye/go-owl/internal/ai"
)

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

	session *owlavi.Session
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
		m.status = "AI 会话装配失败: " + err.Error()
		return
	}
	m.session = session
	m.status = ""
	if cfg != nil {
		m.modelLabel = cfg.AI.Provider + "/" + cfg.AI.Model
	}
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
	}
	if km, ok := msg.(tea.KeyMsg); ok && km.String() == "esc" {
		return m, func() tea.Msg { return LeavePanelMsg{} }
	}
	return m, nil
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
	b.WriteString("  Enter 输入  n 新会话  Esc 返回 Nodes\n")
	b.WriteString("└─")
	return b.String()
}
```

`cmd/cli/cmd/tui/ai/view.go`（Task 4 将替换为样式化版本）：

```go
package ai

import "strings"

func renderMessages(msgs []ChatMsg, width int) string {
	var b strings.Builder
	for i, msg := range msgs {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(msg.Role + ": " + msg.Content)
	}
	return b.String()
}
```

- [ ] **Step 4: 接入 App** `cmd/cli/cmd/tui/app.go`（保留现有 File 面板不动，AI 追加为 panel 3）

import 块追加：`tuiai "github.com/cangyunye/go-owl/cmd/cli/cmd/tui/ai"`（与 exec/file/nodes 同组）。

```go
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
```

`Update` 方法：LeavePanelMsg 分支追加 AI 面板处理，Tab/数字分支扩展：

```go
	if _, ok := msg.(exec.LeavePanelMsg); ok {
		m.switchPanel(0)
		return m, nil
	}
	if _, ok := msg.(file.LeavePanelMsg); ok {
		m.switchPanel(0)
		return m, nil
	}
	if _, ok := msg.(tuiai.LeavePanelMsg); ok {
		m.switchPanel(0)
		return m, nil
	}
```

```go
		case "tab":
			m.switchPanel((m.panel + 1) % 4)
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
		case "4":
			m.switchPanel(3)
			return m, nil
```

`forward` 方法：

```go
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
```

`menuBar` 提示文案改为 `dim.Render("  Tab 切换  1/2/3/4 直达")`。

- [ ] **Step 5: 运行测试确认通过**

Run: `go mod tidy`（bubbles/viewport 与 runewidth 由 indirect 转 direct），再 `go test ./cmd/cli/cmd/tui/...`
Expected: 全部 PASS

- [ ] **Step 6: 提交**

```bash
git add cmd/cli/cmd/tui/ai/ cmd/cli/cmd/tui/app.go cmd/cli/cmd/tui/app_test.go go.mod go.sum
git commit -m "feat(tui): AI 面板接入 App(第三面板 + 3/Tab 切换)"
```

---

### Task 3: 会话泵 + 消息流（Enter 发送 / busy 门 / 确认门闭环）

**Files:**
- Modify: `cmd/cli/cmd/tui/ai/chat.go`（追加 Sender 接口、ChatDoneMsg、sendCmd、发送/新会话/滚动键处理，替换 Update/View）
- Test: `cmd/cli/cmd/tui/ai/chat_test.go`（追加用例）

**Interfaces:**
- Consumes: `newSessionFn`（Task 2）、`textinput.Model`/`viewport.Model`（已有）
- Produces: `Sender` 接口（`Send(ctx context.Context, input string) (string, error)`，`*ai.Session` 天然满足）、`ChatDoneMsg{Text string; Err error}`、`Model.sender Sender` 字段、`Model.refreshViewport()`（内容 = `renderMessages(m.messages, m.width)` + `GotoBottom()`，Task 4 保留）
- 行为契约：Insert 模式 `Enter` 发送（空输入忽略、busy 时拒绝并提示）；发送时先追加 user 消息、置 busy、返回 sendCmd；`ChatDoneMsg` 到达时置 busy=false、追加 assistant 消息（err 时文案 `"错误: "+err`）；Normal 模式 `n` 新会话（busy 时拒绝）、`↑↓/PgUp/PgDn/Home/End` 转发 viewport、`Esc` 发 LeavePanelMsg

- [ ] **Step 1: 写失败测试** `cmd/cli/cmd/tui/ai/chat_test.go` 追加（import 块与 Task 2 已有的合并，追加 `context`/`fmt`/`path/filepath`/`strings`/`owlavi`；`runeKey` 已在 Task 2 定义，勿重复）：

```go
import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	owlavi "github.com/cangyunye/go-owl/internal/ai"
)

func key(t tea.KeyType) tea.Msg { return tea.KeyMsg{Type: t} }

type fakeSender struct {
	fn func(ctx context.Context, input string) (string, error)
}

func (f fakeSender) Send(ctx context.Context, input string) (string, error) {
	if f.fn == nil {
		return "", nil
	}
	return f.fn(ctx, input)
}

func newChat(t *testing.T) *Model {
	t.Helper()
	old := newSessionFn
	t.Cleanup(func() { newSessionFn = old })
	newSessionFn = func(store common.NodeStore) (*owlavi.Session, *owlavi.Config, error) {
		return nil, &owlavi.Config{AI: owlavi.AIConfig{Provider: "openai", Model: "gpt-4o"}}, nil
	}
	store := common.NewInMemoryNodeStoreAt(filepath.Join(t.TempDir(), "nodes.json"))
	m := NewModel(store)
	return &m
}

func TestChat_EnterToInsertAndSend(t *testing.T) {
	m := newChat(t)
	m.sender = fakeSender{fn: func(ctx context.Context, input string) (string, error) {
		return "回答: " + input, nil
	}}

	nm, _ := m.Update(runeKey('i'))
	m = nm.(*Model)
	if !m.InsertMode() {
		t.Fatal("expected insert mode after 'i'")
	}

	nm, _ = m.Update(runeKey('a'))
	m = nm.(*Model)
	nm, _ = m.Update(runeKey('b'))
	m = nm.(*Model)

	nm, cmd := m.Update(key(tea.KeyEnter))
	m = nm.(*Model)
	if !m.busy {
		t.Fatal("expected busy after send")
	}
	if len(m.messages) != 1 || m.messages[0].Role != "user" || m.messages[0].Content != "ab" {
		t.Fatalf("user message missing: %+v", m.messages)
	}

	msg := cmd()
	done, ok := msg.(ChatDoneMsg)
	if !ok {
		t.Fatalf("expected ChatDoneMsg, got %T", msg)
	}
	if done.Text != "回答: ab" {
		t.Fatalf("unexpected done text: %q", done.Text)
	}

	nm, _ = m.Update(done)
	m = nm.(*Model)
	if m.busy {
		t.Fatal("expected not busy after done")
	}
	if len(m.messages) != 2 || m.messages[1].Role != "assistant" || m.messages[1].Content != "回答: ab" {
		t.Fatalf("assistant message missing: %+v", m.messages)
	}
}

func TestChat_EmptyInputIgnored(t *testing.T) {
	m := newChat(t)
	m.sender = fakeSender{}
	nm, _ := m.Update(runeKey('i'))
	m = nm.(*Model)
	nm, cmd := m.Update(key(tea.KeyEnter))
	m = nm.(*Model)
	if cmd != nil {
		t.Fatal("empty input should not send")
	}
	if len(m.messages) != 0 {
		t.Fatalf("no messages expected, got %+v", m.messages)
	}
}

func TestChat_BusyBlocksSecondSend(t *testing.T) {
	m := newChat(t)
	m.sender = fakeSender{}
	m.busy = true
	nm, _ := m.Update(runeKey('i'))
	m = nm.(*Model)
	nm, _ = m.Update(runeKey('a'))
	m = nm.(*Model)
	nm, cmd := m.Update(key(tea.KeyEnter))
	_ = nm
	if cmd != nil {
		t.Fatal("busy should block send")
	}
}

func TestChat_SendErrorShownAsAssistant(t *testing.T) {
	m := newChat(t)
	m.sender = fakeSender{fn: func(ctx context.Context, input string) (string, error) {
		return "", fmt.Errorf("网络错误")
	}}
	nm, _ := m.Update(runeKey('i'))
	m = nm.(*Model)
	nm, _ = m.Update(runeKey('x'))
	m = nm.(*Model)
	nm, cmd := m.Update(key(tea.KeyEnter))
	m = nm.(*Model)
	nm, _ = m.Update(cmd())
	m = nm.(*Model)
	if len(m.messages) != 2 || !strings.Contains(m.messages[1].Content, "网络错误") {
		t.Fatalf("error not rendered: %+v", m.messages)
	}
}

func TestChat_ConfirmQuestionFlowsAsMessage(t *testing.T) {
	m := newChat(t)
	calls := 0
	m.sender = fakeSender{fn: func(ctx context.Context, input string) (string, error) {
		calls++
		if calls == 1 {
			return "即将执行: execute_command(...)\n是否继续？（是/否）", nil
		}
		return "已执行: 完成", nil
	}}
	send := func(input string) {
		nm, _ := m.Update(runeKey('i'))
		m = nm.(*Model)
		for _, r := range input {
			nm, _ = m.Update(runeKey(r))
			m = nm.(*Model)
		}
		nm, cmd := m.Update(key(tea.KeyEnter))
		m = nm.(*Model)
		nm, _ = m.Update(cmd())
		m = nm.(*Model)
	}
	send("删除 web-1")
	if len(m.messages) != 2 || !strings.Contains(m.messages[1].Content, "是否继续") {
		t.Fatalf("confirm question not rendered: %+v", m.messages)
	}
	send("是")
	if len(m.messages) != 4 || !strings.Contains(m.messages[3].Content, "已执行") {
		t.Fatalf("replay result not rendered: %+v", m.messages)
	}
}

func TestChat_NewSessionClearsAndReassembles(t *testing.T) {
	m := newChat(t)
	m.messages = []ChatMsg{{Role: "user", Content: "x"}}
	calls := 0
	newSessionFn = func(store common.NodeStore) (*owlavi.Session, *owlavi.Config, error) {
		calls++
		return nil, &owlavi.Config{AI: owlavi.AIConfig{Provider: "openai", Model: "gpt-4o"}}, nil
	}
	nm, _ := m.Update(runeKey('n'))
	m = nm.(*Model)
	if calls != 1 {
		t.Fatalf("expected 1 reassembly, got %d", calls)
	}
	if len(m.messages) != 0 {
		t.Fatalf("messages not cleared: %+v", m.messages)
	}
}

func TestChat_EscExitsInsert(t *testing.T) {
	m := newChat(t)
	nm, _ := m.Update(runeKey('i'))
	m = nm.(*Model)
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(*Model)
	if m.InsertMode() {
		t.Fatal("expected normal mode after esc")
	}
}

func TestChat_EscLeavesPanelFromNormal(t *testing.T) {
	m := newChat(t)
	nm, cmd := m.Update(key(tea.KeyEsc))
	_ = nm
	if cmd == nil {
		t.Fatal("esc should return a cmd")
	}
	if _, ok := cmd().(LeavePanelMsg); !ok {
		t.Fatalf("expected LeavePanelMsg, got %T", cmd())
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./cmd/cli/cmd/tui/ai/ -v`
Expected: FAIL，编译错误 `undefined: ChatDoneMsg` / `undefined: busy` / `m.sender undefined`

- [ ] **Step 3: 实现** `cmd/cli/cmd/tui/ai/chat.go` 追加/替换：

追加类型与字段：

```go
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
```

`Model` 增加字段：

```go
	busy   bool
	sender Sender
```

`NewModel` 中 `resetSession()` 后追加：

```go
	m.sender = m.session
```

`resetSession` 末尾追加：

```go
	m.sender = m.session
```

替换 `Update` 为：

```go
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
```

追加 sendCmd 与 refreshViewport：

```go
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
```

替换 `View` 为（Task 4 将做样式化，此处保持可编译的最小版本）：

```go
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
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./cmd/cli/cmd/tui/...`
Expected: 全部 PASS

- [ ] **Step 5: 提交**

```bash
git add cmd/cli/cmd/tui/ai/chat.go cmd/cli/cmd/tui/ai/chat_test.go
git commit -m "feat(tui): AI 聊天面板模型(Enter 发送/会话泵/确认门闭环)"
```

---

### Task 4: 视图渲染（气泡样式 / 中文换行 / 输入区 / 滚动）

**Files:**
- Modify: `cmd/cli/cmd/tui/ai/view.go`（全量重写：样式、wrapText、renderMsg、View）
- Test: `cmd/cli/cmd/tui/ai/view_test.go`

**Interfaces:**
- Consumes: `Model` 字段 `messages/status/modelLabel/mode/busy/view/input`（Task 2/3）、`renderMessages(msgs []ChatMsg, width int) string`（Task 2 定义，签名保留）
- Produces: `wrapText(s string, width int) string`（runewidth 感知换行）、`renderMsg(msg ChatMsg, width int) string`、样式化 `View()`

- [ ] **Step 1: 写失败测试** `cmd/cli/cmd/tui/ai/view_test.go`

```go
package ai

import (
	"strings"
	"testing"
)

func TestWrapText(t *testing.T) {
	cases := []struct {
		in    string
		width int
		want  string
	}{
		{"abcdef", 3, "abc\ndef"},
		{"你好世界", 4, "你好\n世界"},
		{"a\nb", 5, "a\nb"},
		{"abc def", 4, "abc \ndef"},
		{"", 5, ""},
	}
	for _, c := range cases {
		if got := wrapText(c.in, c.width); got != c.want {
			t.Fatalf("wrapText(%q,%d) = %q, want %q", c.in, c.width, got, c.want)
		}
	}
}

func TestRenderMsg_RoleLabels(t *testing.T) {
	got := renderMsg(ChatMsg{Role: "user", Content: "查询 web 组"}, 60)
	if !strings.Contains(got, "你: ") {
		t.Fatalf("user label missing: %s", got)
	}
	got = renderMsg(ChatMsg{Role: "assistant", Content: "完成"}, 60)
	if !strings.Contains(got, "AI: ") {
		t.Fatalf("assistant label missing: %s", got)
	}
}

func TestView_ShowsMessagesInputAndKeys(t *testing.T) {
	m := newChat(t)
	m.messages = []ChatMsg{
		{Role: "user", Content: "查询 web"},
		{Role: "assistant", Content: "完成"},
	}
	m.status = "完成"
	m.refreshViewport()
	got := m.View()
	for _, want := range []string{"查询 web", "完成", "Enter 输入/发送", "n 新会话", "Esc 返回 Nodes"} {
		if !strings.Contains(got, want) {
			t.Fatalf("view missing %q: %s", want, got)
		}
	}
}

func TestView_NormalModeShowsPlaceholder(t *testing.T) {
	m := newChat(t)
	m.messages = []ChatMsg{}
	m.refreshViewport()
	got := m.View()
	if !strings.Contains(got, "输入指令…") {
		t.Fatalf("placeholder missing: %s", got)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./cmd/cli/cmd/tui/ai/ -run "TestWrapText|TestRenderMsg|TestView" -v`
Expected: FAIL，`wrapText` 未定义 / 断言不通过

- [ ] **Step 3: 实现** `cmd/cli/cmd/tui/ai/view.go` 全量替换为：

```go
package ai

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

var (
	styleUser  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	styleAI    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	styleDim   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleError = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)

func (m Model) View() string {
	var b strings.Builder
	b.WriteString("┌─ AI Chat ──────────────────────────\n")
	if m.modelLabel != "" {
		b.WriteString(styleDim.Render("  模型  "+m.modelLabel) + "\n")
	}
	if m.busy {
		b.WriteString("  " + styleAI.Render("● AI 处理中…") + "\n")
	} else if m.status != "" {
		b.WriteString(styleDim.Render("  "+m.status) + "\n")
	}
	b.WriteString(m.view.View())
	b.WriteString("\n─ 输入 ─────────────────────────────\n")
	if m.mode == ModeInsert {
		b.WriteString("  " + m.input.View() + "\n")
	} else {
		b.WriteString(styleDim.Render("  " + m.input.Placeholder) + "\n")
	}
	b.WriteString(styleDim.Render("  Enter 输入/发送  n 新会话  Esc 返回 Nodes") + "\n")
	b.WriteString("└─")
	return b.String()
}

func renderMessages(msgs []ChatMsg, width int) string {
	var b strings.Builder
	for i, msg := range msgs {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(renderMsg(msg, width))
	}
	return b.String()
}

func renderMsg(msg ChatMsg, width int) string {
	label := "你"
	style := styleUser
	if msg.Role == "assistant" {
		label = "AI"
		style = styleAI
	}
	return style.Render(label+": ") + wrapText(msg.Content, width-4)
}

// wrapText 按显示宽度(中文字符=2)硬换行,保持消息在固定宽度内。
func wrapText(s string, width int) string {
	if width < 4 {
		width = 4
	}
	var sb strings.Builder
	line := ""
	for _, ch := range s {
		if ch == '\n' {
			sb.WriteString(line + "\n")
			line = ""
			continue
		}
		w := runewidth.RuneWidth(ch)
		if runewidth.StringWidth(line)+w > width {
			sb.WriteString(line + "\n")
			line = ""
		}
		line += string(ch)
	}
	sb.WriteString(line)
	return sb.String()
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./cmd/cli/cmd/tui/...`
Expected: 全部 PASS

- [ ] **Step 5: 提交**

```bash
git add cmd/cli/cmd/tui/ai/view.go cmd/cli/cmd/tui/ai/view_test.go
git commit -m "feat(tui): AI 聊天面板视图(气泡样式/中文换行/输入区)"
```

---

### Task 5: 帮助集成 + 全量验证 + E2E 冒烟

**Files:**
- Modify: `cmd/cli/cmd/tui/app.go`（helpView 追加 AI 面板说明）

**Interfaces:**
- Consumes: Task 1~4 全部产物

- [ ] **Step 1: helpView 追加 AI 说明** `cmd/cli/cmd/tui/app.go`

菜单行改为 `"  菜单:  Tab 切换  1/2/3/4 直达  x 快捷执行  f 快捷文件"`，并追加：

```go
		"  AI:      Enter 输入  Enter 发送  n 新会话  Esc 返回",
```

- [ ] **Step 2: 全量测试**

Run: `go test ./...`
Expected: 全部 PASS

- [ ] **Step 3: 提交**

```bash
git add cmd/cli/cmd/tui/app.go
git commit -m "docs(tui): AI 面板帮助说明集成"
```

- [ ] **Step 4: E2E 冒烟**

Run:
```bash
go build -o build/owl.exe ./cmd/cli
./build/owl.exe tui
```

手动冒烟清单：
1. 启动 TUI，顶栏显示 `[Nodes] [Exec] [File] [AI]`，Nodes 高亮；按 `4` → AI 面板，状态栏显示模型 `provider/model`（如 `openai/gpt-4o`）
2. Enter 进入输入，输入「列出 web 组节点」Enter 发送 → 输入区清空，显示「● AI 处理中…」，结果以 `AI: ` 消息出现并自动滚到底部
3. 处理中状态按 Enter 再发送 → 提示「处理中,请等待…」，不发第二条
4. 写操作（如「删除节点 web-1」）→ 显示确认问题消息；回复「是」→ 执行结果出现；回复「否」→ 取消
5. `n` 清空会话重新开始；Tab 四面板循环；AI 面板 Esc 返回 Nodes；`q` 退出
6. 无 API Key 环境：仍可进入面板（本地意图降级），模型行显示默认配置
7. 窗口 resize 后消息区/输入区宽度自适应（视口不溢出）

验证通过后：
```bash
git add -A && git commit -m "docs(tui): AI 面板 E2E 冒烟验证"
```

---

## Self-Review

**Spec 覆盖检查：**
- 复用 owl ai 能力（Agent/Session/确认门/装配）：Task 1（SetupSession）+ Task 3（session.Send 泵 + 确认门问题原样渲染闭环）✓
- 第三面板菜单接入：Task 2（panelNames 三面板 + `3` 直达 + Tab 循环 + Esc 返回）✓
- AI 对话展示（滚动消息区）：Task 3（消息列表/视口刷新）+ Task 4（气泡渲染）✓
- 输入发送交互：Task 3（Enter/i 进入输入、Enter 发送、空输入忽略、busy 门）✓
- 新会话：Task 3（`n` 重建 session）✓
- 无 API Key 降级可用：Task 1（LoadConfig 回退 DefaultConfig + agent 内置本地 fallback）✓
- 键位不冲突：Task 2/3（AI 面板键位仅限本面板，App 层仅新增 `3`）✓
- E2E：Task 5 冒烟清单 ✓

**Placeholder 扫描：** 全部步骤含完整代码与测试，无 TBD/占位实现。

**类型一致性检查：**
- `SetupSession(store, cfg, verbose) (*ai.Agent, *ai.Config, error)`：Task 1 定义，Task 2（newSessionFn 消费）签名一致 ✓
- `newSessionFn func(store) (*owlavi.Session, *owlavi.Config, error)`：Task 2 定义，Task 3 测试复用并重写 ✓
- `Sender.Send(ctx, input) (string, error)`：Task 3 定义，fakeSender 与 `*owlavi.Session` 均满足 ✓
- `ChatDoneMsg{Text, Err}`：Task 3 Update 消费与 sendCmd 生产一致 ✓
- `renderMessages(msgs, width)`：Task 2 定义签名，Task 3 refreshViewport 调用，Task 4 重写实现但保留签名 ✓
- `Model` 字段（mode/messages/status/modelLabel/busy/sender/session/input/view/width/height）：Task 2/3/4 逐任务增量一致 ✓
- viewport/textinput 为值类型字段，`m.view.SetContent/GotoBottom` 通过 `(&m.view)` 指针方法调用（Go 自动取址），Task 3 已按此实现 ✓
