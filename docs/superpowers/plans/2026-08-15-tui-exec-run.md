# TUI Exec Run 面板实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 owl TUI 中新增独立的 Exec 面板（菜单切换进入），实现 `exec run` 的完整交互：命令/节点/分组/标签/格式一等公民参数 + 高级选项模态表单 + 流式结果视图 + 黑名单确认。

**Architecture:** App 层引入 `Panel` 接口（Update/View/InsertMode/Path/IsDirty），持有 Nodes 与 Exec 两个面板，Tab/数字/x 切换。Exec 面板用 Location 栈管理 LocRun / LocAdvanced / LocResult / LocDanger 四个子视图，执行走 `command.Executor.RunStreaming` 流式经 bubbletea 消息泵渲染，执行器与黑名单检查均可注入以支持测试。

**Tech Stack:** Go, charmbracelet/bubbletea + bubbles/textinput + lipgloss, cobra（已有）

## Global Constraints

- 项目目录：`F:\pantheon\trae_projects\git\go-owl`；禁止越出项目根目录
- TDD：每个任务先写失败测试再实现；测试命令 `go test ./cmd/cli/cmd/tui/...`
- 每个任务结束提交一个 atomic commit；全部完成后跑 E2E 冒烟再提交
- 面板文案沿用项目风格（中文硬编码，与 nodes 包一致，不引入 i18n）
- 键位约束：新键不得与 nodes 面板现有键冲突（a/e/d/c/p/k/i/o/l/g/G//?/q/↑↓←→/Space）
- 执行路径复用 `internal/control/command`，不新建 SSH 执行逻辑
- 注入模式沿用 ping/check 的 `var` 全局注入（`pingDial`/`sshCheck` 同款）

---

### Task 1: Panel 接口 + App 菜单切换框架

**Files:**
- Modify: `cmd/cli/cmd/tui/app.go`（全量重写）
- Modify: `cmd/cli/cmd/tui/nodes/model.go:151`（追加 `Visible`/`InsertMode` 方法）
- Create: `cmd/cli/cmd/tui/exec/exec.go`（最小可编译骨架）
- Create: `cmd/cli/cmd/tui/app_test.go`

**Interfaces:**
- Consumes: `common.NodeStore`（已有）、`nodes.NewModel(store)`（已有）、`common.NewInMemoryNodeStoreAt`（已有）
- Produces: `Panel` 接口、`App`（含 `panel` 字段）、`exec.ExecModel`（含 `CaptureTargets([]*common.NodeInfo)`、`Targets() []*common.NodeInfo`）、`exec.LeavePanelMsg`、`nodes.Model.Visible() []*common.NodeInfo`、`nodes.Model.InsertMode() bool`

- [ ] **Step 1: 写失败测试** `cmd/cli/cmd/tui/app_test.go`

```go
package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/tui/exec"
)

func key(t tea.KeyType) tea.Msg { return tea.KeyMsg{Type: t} }
func runeKey(r rune) tea.Msg    { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func newApp(t *testing.T) *App {
	t.Helper()
	store := common.NewInMemoryNodeStoreAt(filepath.Join(t.TempDir(), "nodes.json"))
	for _, n := range []*common.NodeInfo{
		{ID: "n1", Name: "web-1", Address: "10.0.0.1", Port: 22, User: "root", Groups: []string{"web"}, Labels: map[string]string{"env": "prod"}},
		{ID: "n2", Name: "db-1", Address: "10.0.0.2", Port: 22, User: "admin", Groups: []string{"db"}, Labels: map[string]string{"env": "dev"}},
	} {
		if err := store.Add(n); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return NewApp(store)
}

func TestApp_DefaultPanelIsNodes(t *testing.T) {
	m := newApp(t)
	if m.panel != 0 {
		t.Fatalf("expected panel 0, got %d", m.panel)
	}
	if got := m.View(); !strings.Contains(got, "[Nodes]") {
		t.Fatalf("menu bar missing [Nodes]: %s", got)
	}
}

func TestApp_TabSwitchesToExecAndBack(t *testing.T) {
	m := newApp(t)
	nm, _ := m.Update(key(tea.KeyTab))
	m = nm.(*App)
	if m.panel != 1 {
		t.Fatalf("expected panel 1 after tab, got %d", m.panel)
	}
	if got := m.View(); !strings.Contains(got, "[Exec]") {
		t.Fatalf("menu bar missing [Exec]: %s", got)
	}
	nm, _ = m.Update(key(tea.KeyTab))
	m = nm.(*App)
	if m.panel != 0 {
		t.Fatalf("expected panel 0 after second tab, got %d", m.panel)
	}
}

func TestApp_DigitKeysJumpToPanel(t *testing.T) {
	m := newApp(t)
	nm, _ := m.Update(runeKey('2'))
	m = nm.(*App)
	if m.panel != 1 {
		t.Fatalf("expected panel 1 on '2', got %d", m.panel)
	}
	nm, _ = m.Update(runeKey('1'))
	m = nm.(*App)
	if m.panel != 0 {
		t.Fatalf("expected panel 0 on '1', got %d", m.panel)
	}
}

func TestApp_XJumpsToExec(t *testing.T) {
	m := newApp(t)
	nm, _ := m.Update(runeKey('x'))
	m = nm.(*App)
	if m.panel != 1 {
		t.Fatalf("expected panel 1 on 'x', got %d", m.panel)
	}
}

func TestApp_ExecCapturesVisibleSnapshot(t *testing.T) {
	m := newApp(t)
	// 过滤到 web 组后切到 exec, 快照应只有 n1
	nm, _ := m.Update(runeKey('/'))
	nm, _ = nm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g:web")})
	nm, _ = nm.Update(key(tea.KeyEnter))
	m = nm.(*App)
	nm, _ = m.Update(runeKey('2'))
	m = nm.(*App)
	if len(m.exec.Targets()) != 1 || m.exec.Targets()[0].ID != "n1" {
		t.Fatalf("expected snapshot [n1], got %v", m.exec.Targets())
	}
}

func TestApp_LeavePanelMsgReturnsToNodes(t *testing.T) {
	m := newApp(t)
	nm, _ := m.Update(runeKey('2'))
	m = nm.(*App)
	nm, _ = m.Update(exec.LeavePanelMsg{})
	m = nm.(*App)
	if m.panel != 0 {
		t.Fatalf("expected panel 0 after LeavePanelMsg, got %d", m.panel)
	}
}

func TestApp_QuitOnNodesStillWorks(t *testing.T) {
	m := newApp(t)
	nm, cmd := m.Update(runeKey('q'))
	m = nm.(*App)
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected QuitMsg, got %T", cmd())
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./cmd/cli/cmd/tui/ -run TestApp_`
Expected: 编译失败（App 无 panel 字段、exec 包不存在）

- [ ] **Step 3: 创建 exec 包最小骨架** `cmd/cli/cmd/tui/exec/exec.go`

```go
package exec

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

// LeavePanelMsg 请求 App 切回 Nodes 面板
type LeavePanelMsg struct{}

type ExecModel struct {
	targets []*common.NodeInfo
}

func NewModel(store common.NodeStore) ExecModel { return ExecModel{} }

func (m ExecModel) Targets() []*common.NodeInfo { return m.targets }

func (m *ExecModel) CaptureTargets(nodes []*common.NodeInfo) {
	m.targets = append([]*common.NodeInfo(nil), nodes...)
}

func (m ExecModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (m ExecModel) View() string                            { return "exec" }
func (m ExecModel) InsertMode() bool                        { return false }
func (m ExecModel) Path() []string                          { return []string{"exec"} }
func (m ExecModel) IsDirty() bool                           { return false }
```

- [ ] **Step 4: nodes.Model 补充导出方法** `cmd/cli/cmd/tui/nodes/model.go`（在 `func (m Model) Mode() Mode` 之后追加）

```go
func (m Model) Visible() []*common.NodeInfo { return m.visible() }

func (m Model) InsertMode() bool { return m.mode != ModeNormal }
```

- [ ] **Step 5: 重写 App** `cmd/cli/cmd/tui/app.go`

```go
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/tui/exec"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/tui/nodes"
)

// Panel 顶层面板: 节点管理 / 命令执行
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
	panel int // 0=Nodes 1=Exec

	Help        bool
	QuitConfirm bool
}

var panelNames = []string{"Nodes", "Exec"}

func NewApp(store common.NodeStore) *App {
	m := &App{nodes: nodes.NewModel(store)}
	m.exec = exec.NewModel(store)
	m.exec.CaptureTargets(m.nodes.Visible())
	return m
}

func (m *App) Init() tea.Cmd { return nil }

func (m *App) currentPanel() Panel {
	if m.panel == 1 {
		return &m.exec
	}
	return &m.nodes
}

func (m *App) switchPanel(i int) {
	if i < 0 || i >= len(panelNames) || i == m.panel {
		return
	}
	m.panel = i
	if m.panel == 1 {
		m.exec.CaptureTargets(m.nodes.Visible())
	}
}

func (m *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(exec.LeavePanelMsg); ok {
		m.switchPanel(0)
		return m, nil
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
			m.switchPanel((m.panel + 1) % 2)
			return m, nil
		case "1":
			m.switchPanel(0)
			return m, nil
		case "2":
			m.switchPanel(1)
			return m, nil
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
	if m.panel == 1 {
		m.exec = pm.(exec.ExecModel)
	} else {
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
	return strings.Join(parts, " ") + dim.Render("  Tab 切换  1/2 直达")
}

func helpView() string {
	return strings.Join([]string{
		"┌─ 帮助 ─────────────────────────────",
		"  菜单:  Tab 切换  1/2 直达  x 快捷执行",
		"  列表:  ↑↓ 选择  ←→ 切栏  g/G 首尾",
		"        a 添加  e 编辑  d 删除  c 列配置",
		"        p ping  k SSH检查  i 导入导出  o 分组  l 标签",
		"        / 过滤(g:组 l:标签 或搜索)  ? 帮助  q 退出",
		"  表单:  ↑↓ 移动字段(首尾回卷)  Enter 编辑",
		"        s 保存  Esc 返回/退出输入  ? 帮助",
		"  模式:  Normal=命令   Insert=输入(Esc 退出)",
		"└────────────────────────────────────",
	}, "\n")
}
```

- [ ] **Step 6: 运行测试确认通过**

Run: `go test ./cmd/cli/cmd/tui/...`
Expected: 全部 PASS（含既有 tui_test.go / nodes 包测试）

- [ ] **Step 7: 提交**

```bash
git add cmd/cli/cmd/tui/app.go cmd/cli/cmd/tui/app_test.go cmd/cli/cmd/tui/exec/exec.go cmd/cli/cmd/tui/nodes/model.go
git commit -m "feat(tui): App 面板化框架(Nodes/Exec Tab 切换 + 目标快照捕获)"
```

---

### Task 2: Exec 面板主视图（LocRun 表单 + 目标解析）

**Files:**
- Modify: `cmd/cli/cmd/tui/exec/exec.go`（重写为完整表单模型）
- Create: `cmd/cli/cmd/tui/exec/view.go`
- Create: `cmd/cli/cmd/tui/exec/exec_test.go`

**Interfaces:**
- Consumes: Task 1 的 `exec.ExecModel` 骨架、`common.NodeStore`
- Produces: `ExecModel`（含 `stack []Loc`、`cursor int`、`mode Mode`、四个 `textinput.Model`、`format string`）、`Loc` 常量（LocRun/LocAdvanced/LocResult/LocDanger）、`Mode` 常量（ModeNormal/ModeInsert）、`ExecModel.resolveTargets() ([]*common.NodeInfo, error)`、`splitTrim/parseLabels/groupsIntersect/labelsMatch/dedupeSorted` 工具函数

- [ ] **Step 1: 写失败测试** `cmd/cli/cmd/tui/exec/exec_test.go`

```go
package exec

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

func key(t tea.KeyType) tea.Msg { return tea.KeyMsg{Type: t} }
func runeKey(r rune) tea.Msg    { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func newTestModel(t *testing.T) ExecModel {
	t.Helper()
	store := common.NewInMemoryNodeStoreAt(filepath.Join(t.TempDir(), "nodes.json"))
	for _, n := range []*common.NodeInfo{
		{ID: "n1", Name: "web-1", Groups: []string{"web"}, Labels: map[string]string{"env": "prod"}},
		{ID: "n2", Name: "db-1", Groups: []string{"db"}, Labels: map[string]string{"env": "dev"}},
		{ID: "n3", Name: "cache-1", Groups: []string{"web", "cache"}, Labels: map[string]string{"env": "prod", "role": "cache"}},
	} {
		if err := store.Add(n); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	m := NewModel(store)
	nodes, _ := store.List()
	m.CaptureTargets(nodes)
	return m
}

func TestNewModel_DefaultState(t *testing.T) {
	m := newTestModel(t)
	if m.format != "simple" {
		t.Fatalf("expected format simple, got %s", m.format)
	}
	if m.current() != LocRun {
		t.Fatalf("expected stack top LocRun, got %v", m.current())
	}
	if got := m.Path(); len(got) != 2 || got[0] != "exec" || got[1] != "run" {
		t.Fatalf("unexpected path: %v", got)
	}
	if m.Mode() != ModeNormal || m.InsertMode() {
		t.Fatal("expected ModeNormal")
	}
	if m.IsDirty() {
		t.Fatal("exec panel never dirty")
	}
}

func TestFormatCycle_FToJsonToSimple(t *testing.T) {
	m := newTestModel(t)
	nm, _ := m.Update(runeKey('f'))
	m = nm.(ExecModel)
	if m.format != "detail" {
		t.Fatalf("expected detail after first f, got %s", m.format)
	}
	nm, _ = m.Update(runeKey('f'))
	m = nm.(ExecModel)
	if m.format != "json" {
		t.Fatalf("expected json after second f, got %s", m.format)
	}
	nm, _ = m.Update(runeKey('f'))
	m = nm.(ExecModel)
	if m.format != "simple" {
		t.Fatalf("expected simple after third f, got %s", m.format)
	}
}

func TestResolve_ExplicitNodes(t *testing.T) {
	m := newTestModel(t)
	m.nodesInput.SetValue("n1,n3")
	nodes, err := m.resolveTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || nodes[0].ID != "n1" || nodes[1].ID != "n3" {
		t.Fatalf("expected [n1 n3], got %v", nodeIDs(nodes))
	}
}

func TestResolve_Groups(t *testing.T) {
	m := newTestModel(t)
	m.groupsInput.SetValue("web")
	nodes, err := m.resolveTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || nodes[0].ID != "n1" || nodes[1].ID != "n3" {
		t.Fatalf("expected [n1 n3], got %v", nodeIDs(nodes))
	}
}

func TestResolve_Labels(t *testing.T) {
	m := newTestModel(t)
	m.labelsInput.SetValue("env=prod")
	nodes, err := m.resolveTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || nodes[0].ID != "n1" || nodes[1].ID != "n3" {
		t.Fatalf("expected [n1 n3], got %v", nodeIDs(nodes))
	}
}

func TestResolve_EmptyFallsBackToSnapshot(t *testing.T) {
	m := newTestModel(t)
	nodes, err := m.resolveTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 {
		t.Fatalf("expected 3 from snapshot, got %d", len(nodes))
	}
}

func TestResolve_PriorityNodesOverGroups(t *testing.T) {
	m := newTestModel(t)
	m.nodesInput.SetValue("n2")
	m.groupsInput.SetValue("web")
	nodes, err := m.resolveTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].ID != "n2" {
		t.Fatalf("expected [n2] (nodes wins), got %v", nodeIDs(nodes))
	}
}

func TestResolve_DedupeAndSort(t *testing.T) {
	m := newTestModel(t)
	m.nodesInput.SetValue("n3,n1,n3")
	nodes, err := m.resolveTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || nodes[0].ID != "n1" || nodes[1].ID != "n3" {
		t.Fatalf("expected deduped sorted [n1 n3], got %v", nodeIDs(nodes))
	}
}

func TestRunView_ShowsFourFieldsAndFormat(t *testing.T) {
	m := newTestModel(t)
	got := m.View()
	for _, want := range []string{"命令", "节点", "分组", "标签", "simple", "目标", "3 台"} {
		if !contains(got, want) {
			t.Fatalf("view missing %q:\n%s", want, got)
		}
	}
}

func TestEscAtRootEmitsLeavePanel(t *testing.T) {
	m := newTestModel(t)
	nm, cmd := m.Update(key(tea.KeyEsc))
	m = nm.(ExecModel)
	if cmd == nil {
		t.Fatal("expected cmd")
	}
	msg := cmd()
	if _, ok := msg.(LeavePanelMsg); !ok {
		t.Fatalf("expected LeavePanelMsg, got %T", msg)
	}
}

func TestEnterEditsFieldAndEscRestores(t *testing.T) {
	m := newTestModel(t)
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(ExecModel)
	if m.mode != ModeInsert {
		t.Fatal("expected ModeInsert after enter")
	}
	if !m.cmdInput.Focused() {
		t.Fatal("expected cmd input focused")
	}
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(ExecModel)
	if m.mode != ModeNormal {
		t.Fatal("expected ModeNormal after esc")
	}
	if m.cmdInput.Focused() {
		t.Fatal("expected cmd input blurred")
	}
}

func nodeIDs(nodes []*common.NodeInfo) []string {
	ids := make([]string, 0, len(nodes))
	for _, n := range nodes {
		ids = append(ids, n.ID)
	}
	return ids
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./cmd/cli/cmd/tui/exec/ -run TestNewModel_DefaultState`
Expected: 编译失败（Loc/cursor/format 等不存在）

- [ ] **Step 3: 重写 exec.go** `cmd/cli/cmd/tui/exec/exec.go`

```go
package exec

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

type Loc int

const (
	LocRun Loc = iota
	LocAdvanced
	LocResult
	LocDanger
)

type Mode int

const (
	ModeNormal Mode = iota
	ModeInsert
)

type LeavePanelMsg struct{}

var formats = []string{"simple", "detail", "json"}

type ExecModel struct {
	store common.NodeStore

	stack []Loc
	mode  Mode
	cursor int

	cmdInput    textinput.Model
	nodesInput  textinput.Model
	groupsInput textinput.Model
	labelsInput textinput.Model
	format      string
	formatIdx   int

	targets []*common.NodeInfo
	error   string
}

func NewModel(store common.NodeStore) ExecModel {
	return ExecModel{
		store:       store,
		stack:       []Loc{LocRun},
		cmdInput:    newInput("输入要执行的命令 (必填)", 60),
		nodesInput:  newInput("节点 ID,逗号分隔 (留空=当前过滤可见)", 40),
		groupsInput: newInput("分组,逗号分隔", 40),
		labelsInput: newInput("标签 k=v,逗号分隔", 40),
		format:      "simple",
	}
}

func newInput(placeholder string, width int) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Width = width
	ti.CharLimit = 256
	ti.Blur()
	return ti
}

func (m ExecModel) Targets() []*common.NodeInfo { return m.targets }

func (m *ExecModel) CaptureTargets(nodes []*common.NodeInfo) {
	m.targets = append([]*common.NodeInfo(nil), nodes...)
}

func (m ExecModel) current() Loc { return m.stack[len(m.stack)-1] }

func (m *ExecModel) push(l Loc) { m.stack = append(m.stack, l) }

func (m *ExecModel) pop() {
	if len(m.stack) > 1 {
		m.stack = m.stack[:len(m.stack)-1]
	}
}

func (m ExecModel) Mode() Mode { return m.mode }

func (m ExecModel) InsertMode() bool { return m.mode != ModeNormal }

func (m ExecModel) IsDirty() bool { return false }

func (m ExecModel) Path() []string {
	switch m.current() {
	case LocAdvanced:
		return []string{"exec", "run", "advanced"}
	case LocResult:
		return []string{"exec", "run", "result"}
	case LocDanger:
		return []string{"exec", "run", "danger"}
	default:
		return []string{"exec", "run"}
	}
}

func (m *ExecModel) fieldAt(i int) *textinput.Model {
	switch i {
	case 0:
		return &m.cmdInput
	case 1:
		return &m.nodesInput
	case 2:
		return &m.groupsInput
	default:
		return &m.labelsInput
	}
}

func (m ExecModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.current() {
	case LocAdvanced:
		return m.updateAdvanced(msg)
	case LocResult:
		return m.updateResult(msg)
	case LocDanger:
		return m.updateDanger(msg)
	default:
		return m.updateRun(msg)
	}
}

func (m ExecModel) updateRun(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.mode == ModeInsert {
		if km, ok := msg.(tea.KeyMsg); ok && km.String() == "esc" {
			m.mode = ModeNormal
			m.fieldAt(m.cursor).Blur()
			return m, nil
		}
		f := m.fieldAt(m.cursor)
		var cmd tea.Cmd
		*f, cmd = f.Update(msg)
		return m, cmd
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "up":
		m.cursor = (m.cursor - 1 + 4) % 4
	case "down":
		m.cursor = (m.cursor + 1) % 4
	case "enter":
		m.mode = ModeInsert
		m.fieldAt(m.cursor).Focus()
	case "f":
		m.formatIdx = (m.formatIdx + 1) % len(formats)
		m.format = formats[m.formatIdx]
	case "esc":
		return m, func() tea.Msg { return LeavePanelMsg{} }
	}
	return m, nil
}

func (m *ExecModel) resolveTargets() ([]*common.NodeInfo, error) {
	all, err := m.store.List()
	if err != nil {
		return nil, err
	}
	var nodes []*common.NodeInfo
	switch {
	case m.nodesInput.Value() != "":
		want := map[string]bool{}
		for _, id := range splitTrim(m.nodesInput.Value(), ",") {
			want[id] = true
		}
		for _, n := range all {
			if want[n.ID] {
				nodes = append(nodes, n)
			}
		}
	case m.groupsInput.Value() != "":
		groups := splitTrim(m.groupsInput.Value(), ",")
		for _, n := range all {
			if groupsIntersect(n.Groups, groups) {
				nodes = append(nodes, n)
			}
		}
	case m.labelsInput.Value() != "":
		labels := parseLabels(m.labelsInput.Value())
		for _, n := range all {
			if labelsMatch(n.Labels, labels) {
				nodes = append(nodes, n)
			}
		}
	default:
		nodes = m.targets
	}
	return dedupeSorted(nodes), nil
}

func splitTrim(s, sep string) []string {
	var out []string
	for _, p := range strings.Split(s, sep) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func groupsIntersect(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

func parseLabels(s string) map[string]string {
	out := map[string]string{}
	for _, pair := range splitTrim(s, ",") {
		if k, v, ok := strings.Cut(pair, "="); ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		} else {
			out[strings.TrimSpace(pair)] = ""
		}
	}
	return out
}

func labelsMatch(labels map[string]string, want map[string]string) bool {
	if labels == nil {
		return false
	}
	for k, v := range want {
		if labels[k] != v {
			return false
		}
	}
	return true
}

func dedupeSorted(nodes []*common.NodeInfo) []*common.NodeInfo {
	seen := map[string]bool{}
	out := make([]*common.NodeInfo, 0, len(nodes))
	for _, n := range nodes {
		if n == nil || seen[n.ID] {
			continue
		}
		seen[n.ID] = true
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
```

- [ ] **Step 4: 创建视图** `cmd/cli/cmd/tui/exec/view.go`

```go
package exec

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	styleSelected = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	styleDim      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleError    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)

func (m ExecModel) View() string {
	switch m.current() {
	case LocAdvanced:
		return m.advancedView()
	case LocResult:
		return m.resultView()
	case LocDanger:
		return m.dangerView()
	default:
		return m.runView()
	}
}

func (m ExecModel) runView() string {
	var b strings.Builder
	b.WriteString("┌─ Exec Run ───────────────────────────\n")
	labels := []string{"命令", "节点", "分组", "标签"}
	for i := 0; i < 4; i++ {
		marker := " "
		if i == m.cursor && m.mode == ModeNormal {
			marker = ">"
		}
		b.WriteString(fmt.Sprintf("%s %s%-4s %s\n", marker, " ", labels[i], m.fieldAt(i).View()))
	}
	b.WriteString("  格式  " + styleSelected.Render(m.format) + styleDim.Render("  f 切换") + "\n")
	if nodes, err := m.resolveTargets(); err == nil {
		b.WriteString(styleDim.Render(fmt.Sprintf("  目标  %d 台", len(nodes))) + "\n")
	}
	if m.error != "" {
		b.WriteString(styleError.Render("  "+m.error) + "\n")
	}
	b.WriteString(styleDim.Render("  ↑↓移动 Enter编辑 f格式 a高级 r执行 Esc返回") + "\n")
	b.WriteString("└─")
	return b.String()
}
```

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./cmd/cli/cmd/tui/exec/ ./cmd/cli/cmd/tui/`
Expected: 全部 PASS（app_test 仍通过——Task 1 测试仅用 Targets/CaptureTargets）

- [ ] **Step 6: 提交**

```bash
git add cmd/cli/cmd/tui/exec/
git commit -m "feat(tui): Exec 面板主视图(命令/节点/分组/标签/格式 + 目标解析快照)"
```

---

### Task 3: 高级选项模态表单（LocAdvanced）

**Files:**
- Create: `cmd/cli/cmd/tui/exec/advanced.go`
- Modify: `cmd/cli/cmd/tui/exec/exec.go`（updateRun 加 `a` 分支、Update 分发已有）
- Modify: `cmd/cli/cmd/tui/exec/view.go`（加 advancedView）
- Modify: `cmd/cli/cmd/tui/exec/exec_test.go`（追加测试）

**Interfaces:**
- Consumes: `internal/control/command.ExecuteOptions`、`internal/control/command.RetryConfig`、`internal/ssh.TimeoutConfig`（均已有）
- Produces: `AdvancedForm`（`fields []*AdvancedField`、`cursor`）、`FieldKind`（KindText/KindBool）、`newAdvancedForm() *AdvancedForm`、`(*AdvancedForm).buildOpts() (*command.ExecuteOptions, error)`、`advancedSummary(*AdvancedForm) string`、`ExecModel.updateAdvanced(msg) (tea.Model, tea.Cmd)`

- [ ] **Step 1: 写失败测试**（追加到 `cmd/cli/cmd/tui/exec/exec_test.go`）

```go
func TestAdvanced_Defaults(t *testing.T) {
	f := newAdvancedForm()
	if len(f.fields) != 20 {
		t.Fatalf("expected 20 fields, got %d", len(f.fields))
	}
	if got := f.value("timeout"); got != "60s" {
		t.Fatalf("expected timeout 60s, got %q", got)
	}
	if !f.isOn("parallel") || f.isOn("serial") {
		t.Fatal("expected parallel on, serial off")
	}
}

func TestAdvanced_ToggleBoolWithSpace(t *testing.T) {
	m := newTestModel(t)
	m.advanced = newAdvancedForm()
	m.push(LocAdvanced)
	// 下移到 parallel 行 (索引 3)
	for i := 0; i < 3; i++ {
		nm, _ := m.Update(key(tea.KeyDown))
		m = nm.(ExecModel)
	}
	nm, _ := m.Update(runeKey(' '))
	m = nm.(ExecModel)
	if m.advanced.isOn("parallel") {
		t.Fatal("space should toggle parallel off")
	}
	if m.advanced.isOn("serial") {
		t.Fatal("serial should stay off")
	}
}

func TestAdvanced_SpaceIgnoredOnTextField(t *testing.T) {
	m := newTestModel(t)
	m.advanced = newAdvancedForm()
	m.push(LocAdvanced)
	// 光标在 timeout (索引 0, KindText), 空格应被忽略
	nm, _ := m.Update(runeKey(' '))
	m = nm.(ExecModel)
	if !m.advanced.isOn("parallel") {
		t.Fatal("space on text field should be ignored")
	}
}

func TestAdvanced_EditTextField(t *testing.T) {
	m := newTestModel(t)
	m.advanced = newAdvancedForm()
	m.push(LocAdvanced)
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(ExecModel)
	if m.mode != ModeInsert {
		t.Fatal("expected ModeInsert on enter")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("120s")})
	m = nm.(ExecModel)
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(ExecModel)
	if got := m.advanced.value("timeout"); got != "120s" {
		t.Fatalf("expected timeout 120s, got %q", got)
	}
}

func TestAdvanced_SaveReturnsToRun(t *testing.T) {
	m := newTestModel(t)
	m.advanced = newAdvancedForm()
	m.push(LocAdvanced)
	nm, _ := m.Update(runeKey('s'))
	m = nm.(ExecModel)
	if m.current() != LocRun {
		t.Fatalf("expected LocRun after save, got %v", m.current())
	}
	if m.advanced != nil {
		t.Fatal("expected advanced cleared")
	}
}

func TestAdvanced_BuildOpts_Defaults(t *testing.T) {
	f := newAdvancedForm()
	opts, err := f.buildOpts()
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Parallel {
		t.Fatal("expected parallel")
	}
	if opts.TimeoutConfig == nil || opts.TimeoutConfig.ConnectTimeout != 10*time.Second || opts.TimeoutConfig.CommandTimeout != 30*time.Second {
		t.Fatalf("unexpected timeout config: %+v", opts.TimeoutConfig)
	}
	if opts.RetryConfig == nil || opts.RetryConfig.MaxRetries != 3 {
		t.Fatalf("unexpected retry config: %+v", opts.RetryConfig)
	}
}

func TestAdvanced_BuildOpts_SerialOverrides(t *testing.T) {
	f := newAdvancedForm()
	f.toggle(4) // serial 行
	opts, err := f.buildOpts()
	if err != nil {
		t.Fatal(err)
	}
	if opts.Parallel {
		t.Fatal("expected serial mode")
	}
}

func TestAdvanced_BuildOpts_NoRetryDisables(t *testing.T) {
	f := newAdvancedForm()
	f.toggle(8) // no-retry 行
	opts, err := f.buildOpts()
	if err != nil {
		t.Fatal(err)
	}
	if opts.RetryConfig != nil {
		t.Fatalf("expected no retry config, got %+v", opts.RetryConfig)
	}
}

func TestAdvanced_BuildOpts_InvalidDuration(t *testing.T) {
	f := newAdvancedForm()
	f.fields[0].input.SetValue("abc")
	if _, err := f.buildOpts(); err == nil {
		t.Fatal("expected error for invalid duration")
	}
}

func TestAdvanced_ViewShowsCheckboxes(t *testing.T) {
	m := newTestModel(t)
	m.advanced = newAdvancedForm()
	m.push(LocAdvanced)
	got := m.View()
	for _, want := range []string{"高级选项", "parallel", "[x]", "[ ]"} {
		if !contains(got, want) {
			t.Fatalf("view missing %q:\n%s", want, got)
		}
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./cmd/cli/cmd/tui/exec/ -run TestAdvanced`
Expected: 编译失败（newAdvancedForm 不存在）

- [ ] **Step 3: 创建 advanced.go** `cmd/cli/cmd/tui/exec/advanced.go`

```go
package exec

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/internal/control/command"
	"github.com/cangyunye/go-owl/internal/ssh"
)

type FieldKind int

const (
	KindText FieldKind = iota
	KindBool
)

type AdvancedField struct {
	key     string
	label   string
	kind    FieldKind
	input   textinput.Model
	checked bool
}

type AdvancedForm struct {
	fields []*AdvancedField
	cursor int
	error  string
}

func newAdvancedForm() *AdvancedForm {
	f := &AdvancedForm{}
	specs := []struct {
		key, label, def string
		kind            FieldKind
		checked         bool
	}{
		{"timeout", "timeout", "60s", KindText, false},
		{"connect-timeout", "connect-timeout", "10s", KindText, false},
		{"command-timeout", "command-timeout", "30s", KindText, false},
		{"parallel", "parallel", "", KindBool, true},
		{"serial", "serial", "", KindBool, false},
		{"retry", "retry", "3", KindText, false},
		{"retry-interval", "retry-interval", "1s", KindText, false},
		{"retry-max-interval", "retry-max-interval", "30s", KindText, false},
		{"no-retry", "no-retry", "", KindBool, false},
		{"async", "async", "", KindBool, false},
		{"async-timeout", "async-timeout", "1h", KindText, false},
		{"async-poll-interval", "async-poll-interval", "10s", KindText, false},
		{"async-max-poll-count", "async-max-poll-count", "3600", KindText, false},
		{"async-remote-dir", "async-remote-dir", "/tmp/owl", KindText, false},
		{"status", "status", "", KindText, false},
		{"no-color", "no-color", "", KindBool, false},
		{"debug", "debug", "", KindBool, false},
		{"force", "force", "", KindBool, false},
		{"sync-nodes", "sync-nodes", "", KindBool, false},
		{"silent", "silent", "", KindBool, false},
	}
	for _, s := range specs {
		ti := textinput.New()
		ti.SetValue(s.def)
		ti.Width = 20
		ti.CharLimit = 64
		ti.Blur()
		f.fields = append(f.fields, &AdvancedField{key: s.key, label: s.label, kind: s.kind, input: ti, checked: s.checked})
	}
	return f
}

func (f *AdvancedForm) value(key string) string {
	for _, fd := range f.fields {
		if fd.key == key {
			return strings.TrimSpace(fd.input.Value())
		}
	}
	return ""
}

func (f *AdvancedForm) isOn(key string) bool {
	for _, fd := range f.fields {
		if fd.key == key {
			return fd.checked
		}
	}
	return false
}

func (f *AdvancedForm) move(d int) {
	f.cursor = (f.cursor + d + len(f.fields)) % len(f.fields)
}

func (f *AdvancedForm) toggle(i int) {
	f.fields[i].checked = !f.fields[i].checked
}

func (f *AdvancedForm) buildOpts() (*command.ExecuteOptions, error) {
	for _, key := range []string{"timeout", "connect-timeout", "command-timeout", "retry-interval", "retry-max-interval"} {
		if v := f.value(key); v != "" {
			if _, err := time.ParseDuration(v); err != nil {
				return nil, fmt.Errorf("%s 无效: %s", key, v)
			}
		}
	}
	opts := &command.ExecuteOptions{Parallel: f.isOn("parallel") && !f.isOn("serial")}
	if v := f.value("timeout"); v != "" {
		d, _ := time.ParseDuration(v)
		opts.Timeout = d
	}
	opts.TimeoutConfig = &ssh.TimeoutConfig{
		ConnectTimeout: mustDuration(f.value("connect-timeout")),
		CommandTimeout: mustDuration(f.value("command-timeout")),
	}
	if retry := f.value("retry"); retry != "" && !f.isOn("no-retry") {
		n, err := strconv.Atoi(retry)
		if err != nil {
			return nil, fmt.Errorf("retry 必须是整数: %s", retry)
		}
		opts.RetryConfig = &command.RetryConfig{
			MaxRetries:      n,
			InitialInterval: mustDuration(f.value("retry-interval")),
			MaxInterval:     mustDuration(f.value("retry-max-interval")),
		}
	}
	return opts, nil
}

func mustDuration(s string) time.Duration {
	d, _ := time.ParseDuration(s)
	return d
}

func advancedSummary(f *AdvancedForm) string {
	parts := []string{}
	if v := f.value("timeout"); v != "" {
		parts = append(parts, "timeout="+v)
	}
	if f.isOn("serial") {
		parts = append(parts, "串行")
	} else {
		parts = append(parts, "并行")
	}
	if v := f.value("retry"); v != "" && !f.isOn("no-retry") {
		parts = append(parts, "retry="+v)
	}
	if f.isOn("async") {
		parts = append(parts, "async")
	}
	if f.isOn("force") {
		parts = append(parts, "force")
	}
	if f.isOn("debug") {
		parts = append(parts, "debug")
	}
	if f.isOn("silent") {
		parts = append(parts, "silent")
	}
	return strings.Join(parts, " ")
}
```

- [ ] **Step 4: updateRun 加 a 键**（`cmd/cli/cmd/tui/exec/exec.go`，在 `case "f":` 之后插入）

```go
	case "a":
		m.advanced = newAdvancedForm()
		m.push(LocAdvanced)
```

并追加 updateAdvanced 方法（放在 updateRun 之后）：

```go
func (m ExecModel) updateAdvanced(msg tea.Msg) (tea.Model, tea.Cmd) {
	f := m.advanced
	if f == nil {
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
		if f.fields[f.cursor].kind == KindText {
			m.mode = ModeInsert
			f.fields[f.cursor].input.Focus()
		}
	case " ":
		if f.fields[f.cursor].kind == KindBool {
			f.toggle(f.cursor)
		}
	case "s", "esc":
		m.pop()
		m.advanced = nil
	}
	return m, nil
}
```

- [ ] **Step 5: 添加 advancedView + 高级摘要行**（`cmd/cli/cmd/tui/exec/view.go`）

在 runView 的格式行之后插入：

```go
	if m.advanced != nil {
		b.WriteString(styleDim.Render("  高级  "+advancedSummary(m.advanced)) + "\n")
	}
```

文件末尾追加：

```go
func (m ExecModel) advancedView() string {
	f := m.advanced
	if f == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("┌─ 高级选项 ─────────────────────────\n")
	for i, fd := range f.fields {
		marker := " "
		if i == f.cursor && m.mode == ModeNormal {
			marker = ">"
		}
		line := ""
		if fd.kind == KindBool {
			box := "[ ]"
			if fd.checked {
				box = "[x]"
			}
			line = fmt.Sprintf("%s %s %-18s Space 切换\n", marker, box, fd.label)
		} else {
			line = fmt.Sprintf("%s %-18s %s\n", marker, fd.label, fd.input.View())
		}
		if i == f.cursor && m.mode == ModeNormal {
			line = styleSelected.Render(strings.TrimRight(line, "\n")) + "\n"
		}
		b.WriteString("  " + line)
	}
	if f.error != "" {
		b.WriteString(styleError.Render("  "+f.error) + "\n")
	}
	b.WriteString(styleDim.Render("  ↑↓移动 Enter编辑 Space切换bool s保存 Esc返回") + "\n")
	b.WriteString("└─")
	return b.String()
}
```

- [ ] **Step 6: 运行测试确认通过**

Run: `go test ./cmd/cli/cmd/tui/exec/`
Expected: 全部 PASS

- [ ] **Step 7: 提交**

```bash
git add cmd/cli/cmd/tui/exec/
git commit -m "feat(tui): Exec 高级选项模态表单(bool 空格切换 + ExecuteOptions 映射)"
```

---

### Task 4: 执行流（RunStreaming 流式泵 + 结果视图）

**Files:**
- Create: `cmd/cli/cmd/tui/exec/run.go`
- Modify: `cmd/cli/cmd/tui/exec/exec.go`（加字段 `runCh`/`lastCmd`/`lastIDs`/`lastOpts`/`loading`/`results`、updateRun 加 `r` 分支、加 updateResult）
- Modify: `cmd/cli/cmd/tui/exec/view.go`（加 resultView）
- Modify: `cmd/cli/cmd/tui/exec/exec_test.go`（追加测试）

**Interfaces:**
- Consumes: `internal/control/command`（Executor/RunStreaming/CommandResult/ExecuteOptions）、`internal/node.NewNodeResolver`、Task 3 的 `buildOpts`
- Produces: `ExecStreamMsg{ch chan command.CommandResult}`、`ExecResultMsg{Result command.CommandResult}`、`ExecDoneMsg{}`、`var runStream`（可注入）、`ExecModel.startRun() (tea.Cmd, error)`、`ExecModel.launchRun(ids, cmd, opts) tea.Cmd`、`pumpResults(ch) tea.Cmd`

- [ ] **Step 1: 写失败测试**（追加到 `cmd/cli/cmd/tui/exec/exec_test.go`）

```go
import (
	"context"
	"errors"
	"time"

	"github.com/cangyunye/go-owl/internal/control/command"
)

func fakeStream(results []command.CommandResult) {
	runStream = func(ctx context.Context, ids []string, cmd string, opts *command.ExecuteOptions) (<-chan command.CommandResult, func()) {
		ch := make(chan command.CommandResult, len(results))
		for _, r := range results {
			ch <- r
		}
		close(ch)
		return ch, func() {}
	}
}

func TestStartRun_EmptyCommand(t *testing.T) {
	m := newTestModel(t)
	if _, err := m.startRun(); err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestStartRun_NoTargets(t *testing.T) {
	m := newTestModel(t)
	m.CaptureTargets(nil)
	m.cmdInput.SetValue("echo hi")
	if _, err := m.startRun(); err == nil {
		t.Fatal("expected error for no targets")
	}
}

func TestStartRun_InvalidAdvancedOption(t *testing.T) {
	m := newTestModel(t)
	m.cmdInput.SetValue("echo hi")
	m.advanced = newAdvancedForm()
	m.advanced.fields[0].input.SetValue("abc")
	if _, err := m.startRun(); err == nil {
		t.Fatal("expected error for invalid timeout")
	}
}

func TestRun_StreamsResultsAndRenders(t *testing.T) {
	fakeStream([]command.CommandResult{
		{NodeID: "n1", Success: true, ExitCode: 0, Duration: time.Second, Output: "ok"},
		{NodeID: "n2", Success: false, ExitCode: 127, Error: errors.New("boom")},
	})
	m := newTestModel(t)
	m.cmdInput.SetValue("echo hi")
	nm, cmd := m.Update(runeKey('r'))
	m = nm.(ExecModel)
	if m.current() != LocResult {
		t.Fatalf("expected LocResult after r, got %v", m.current())
	}
	if cmd == nil {
		t.Fatal("expected start cmd")
	}
	msg := cmd()
	sm, ok := msg.(ExecStreamMsg)
	if !ok {
		t.Fatalf("expected ExecStreamMsg, got %T", msg)
	}
	// 泵出第一条
	nm, cmd = m.Update(sm)
	m = nm.(ExecModel)
	rmsg := cmd().(ExecResultMsg)
	nm, cmd = m.Update(rmsg)
	m = nm.(ExecModel)
	if len(m.results) != 1 || m.results[0].NodeID != "n1" {
		t.Fatalf("expected 1 result n1, got %v", m.results)
	}
	// 第二条
	nm, cmd = m.Update((cmd().(ExecResultMsg)))
	m = nm.(ExecModel)
	if len(m.results) != 2 || m.results[1].NodeID != "n2" {
		t.Fatalf("expected 2 results, got %v", m.results)
	}
	// 泵结束: ch 已关闭, 下一泵消息是 ExecDoneMsg
	nm, cmd = m.Update(cmd())
	m = nm.(ExecModel)
	if cmd != nil {
		t.Fatal("expected nil cmd after ExecDoneMsg")
	}
	got := m.View()
	for _, want := range []string{"Exec 结果", "n1", "n2", "成功 1/2"} {
		if !contains(got, want) {
			t.Fatalf("view missing %q:\n%s", want, got)
		}
	}
}

func TestResult_EscReturnsToRun(t *testing.T) {
	m := newTestModel(t)
	m.push(LocResult)
	nm, _ := m.Update(key(tea.KeyEsc))
	m = nm.(ExecModel)
	if m.current() != LocRun {
		t.Fatalf("expected LocRun after esc, got %v", m.current())
	}
}

func TestResult_Rerun(t *testing.T) {
	fakeStream(nil)
	m := newTestModel(t)
	m.cmdInput.SetValue("echo hi")
	m.push(LocResult)
	nm, cmd := m.Update(runeKey('r'))
	m = nm.(ExecModel)
	if cmd == nil {
		t.Fatal("expected rerun cmd")
	}
	if _, ok := cmd().(ExecStreamMsg); !ok {
		t.Fatalf("expected ExecStreamMsg, got %T", cmd())
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./cmd/cli/cmd/tui/exec/ -run TestStartRun`
Expected: 编译失败（startRun/runStream 不存在）

- [ ] **Step 3: 创建 run.go** `cmd/cli/cmd/tui/exec/run.go`

```go
package exec

import (
	"context"
	"errors"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/internal/control/command"
	"github.com/cangyunye/go-owl/internal/node"
)

type ExecStreamMsg struct {
	ch chan command.CommandResult
}

type ExecResultMsg struct {
	Result command.CommandResult
}

type ExecDoneMsg struct{}

// runStream 可注入的命令执行器(测试替换);默认实现走 command.Executor + RunStreaming
var runStream = func(ctx context.Context, ids []string, cmd string, opts *command.ExecuteOptions) (<-chan command.CommandResult, func()) {
	ex := command.NewExecutor(node.NewNodeResolver())
	return ex.RunStreaming(ctx, ids, cmd, opts), ex.Close
}

func (m *ExecModel) startRun() (tea.Cmd, error) {
	cmd := strings.TrimSpace(m.cmdInput.Value())
	if cmd == "" {
		return nil, errors.New("命令不能为空")
	}
	nodes, err := m.resolveTargets()
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, errors.New("没有目标节点")
	}
	f := m.advanced
	if f == nil {
		f = newAdvancedForm()
	}
	opts, err := f.buildOpts()
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(nodes))
	for _, n := range nodes {
		ids = append(ids, n.ID)
	}
	return m.launchRun(ids, cmd, opts), nil
}

func (m *ExecModel) launchRun(ids []string, cmd string, opts *command.ExecuteOptions) tea.Cmd {
	m.lastCmd = cmd
	m.lastIDs = ids
	m.lastOpts = opts
	m.results = nil
	m.loading = true
	m.push(LocResult)
	return func() tea.Msg {
		ch, close := runStream(context.Background(), ids, cmd, opts)
		out := make(chan command.CommandResult, len(ids))
		go func() {
			defer close()
			defer close(out)
			for r := range ch {
				out <- r
			}
		}()
		return ExecStreamMsg{ch: out}
	}
}

func pumpResults(ch chan command.CommandResult) tea.Cmd {
	return func() tea.Msg {
		r, ok := <-ch
		if !ok {
			return ExecDoneMsg{}
		}
		return ExecResultMsg{Result: r}
	}
}
```

- [ ] **Step 4: exec.go 补字段与方法**

字段追加（在 `targets []*common.NodeInfo` 之后）：

```go
	runCh     chan command.CommandResult
	lastCmd   string
	lastIDs   []string
	lastOpts  *command.ExecuteOptions
	loading   bool
	results   []command.CommandResult
```

import 块追加 `"github.com/cangyunye/go-owl/internal/control/command"`。

updateRun 的 `case "f":` 之后插入：

```go
	case "r":
		cmd, err := m.startRun()
		if err != nil {
			m.error = err.Error()
			return m, nil
		}
		return m, cmd
```

updateResult 方法（放在 updateRun 之后）：

```go
func (m ExecModel) updateResult(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ExecStreamMsg:
		m.loading = false
		m.results = nil
		m.runCh = msg.ch
		return m, pumpResults(msg.ch)
	case ExecResultMsg:
		m.results = append(m.results, msg.Result)
		return m, pumpResults(m.runCh)
	case ExecDoneMsg:
		m.loading = false
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.pop()
			return m, nil
		case "r":
			cmd, err := m.startRun()
			if err != nil {
				m.error = err.Error()
				return m, nil
			}
			return m, cmd
		}
	}
	return m, nil
}
```

- [ ] **Step 5: 加 resultView**（`cmd/cli/cmd/tui/exec/view.go` 末尾追加）

```go
func (m ExecModel) resultView() string {
	var b strings.Builder
	b.WriteString("┌─ Exec 结果 ─────────────────────────\n")
	if m.loading {
		b.WriteString("  " + styleDim.Render("正在执行 "+m.lastCmd+" …") + "\n")
	} else {
		success := 0
		for _, r := range m.results {
			mark := "✗"
			if r.Success {
				mark = "✓"
				success++
			}
			line := fmt.Sprintf("  %s %-24s exit %-3d %s\n", mark, r.NodeID, r.ExitCode, r.Duration.Round(time.Millisecond))
			if r.Success {
				line = styleSelected.Render(line)
			}
			b.WriteString(line)
			if !r.Success && r.ErrorDetail != "" {
				b.WriteString(styleError.Render("      "+r.ErrorDetail) + "\n")
			} else if !r.Success && r.Error != nil {
				b.WriteString(styleError.Render("      "+r.Error.Error()) + "\n")
			}
			if r.Output != "" {
				out := r.Output
				if len([]rune(out)) > 500 {
					out = string([]rune(out)[:497]) + "..."
				}
				for _, l := range strings.Split(out, "\n") {
					b.WriteString("      " + l + "\n")
				}
			}
		}
		b.WriteString(styleDim.Render(fmt.Sprintf("  成功 %d/%d", success, len(m.results))) + "\n")
	}
	b.WriteString(styleDim.Render("  r 重跑  Esc 返回") + "\n")
	b.WriteString("└─")
	return b.String()
}
```

view.go import 块追加 `"time"`。

- [ ] **Step 6: 运行测试确认通过**

Run: `go test ./cmd/cli/cmd/tui/exec/`
Expected: 全部 PASS

- [ ] **Step 7: 提交**

```bash
git add cmd/cli/cmd/tui/exec/
git commit -m "feat(tui): Exec 执行流(RunStreaming 流式泵 + 结果视图 + 重跑)"
```

---

### Task 5: 黑名单确认 + 集成接线 + E2E

**Files:**
- Create: `cmd/cli/cmd/tui/exec/danger.go`
- Modify: `cmd/cli/cmd/tui/exec/run.go`（startRun 加黑名单检查）
- Modify: `cmd/cli/cmd/tui/exec/exec.go`（加 pending 字段 + updateDanger）
- Modify: `cmd/cli/cmd/tui/exec/view.go`（加 dangerView）
- Modify: `cmd/cli/cmd/tui/exec/exec_test.go`（追加测试）
- Modify: `cmd/cli/cmd/tui/nodes/view.go:137`（状态栏加 x 提示）
- Modify: `cmd/cli/cmd/tui/app.go`（helpView 加 Exec 面板键位）

**Interfaces:**
- Consumes: `internal/control/blacklist`（LoadConfig/NewChecker/CheckResult/MatchItem）
- Produces: `BlockedInfo{NodeID, User, Matches}`、`var checkBlacklist`（可注入）、`ExecModel.pendingBlocked/pendingCmd/pendingIDs/pendingOpts`、`updateDanger`

- [ ] **Step 1: 写失败测试**（追加到 `cmd/cli/cmd/tui/exec/exec_test.go`）

```go
func fakeBlacklist(blocked []BlockedInfo) {
	checkBlacklist = func(nodeUsers map[string]string, cmd string) []BlockedInfo {
		return blocked
	}
}

func TestDanger_BlockedStopsAndConfirms(t *testing.T) {
	fakeBlacklist([]BlockedInfo{
		{NodeID: "n1", User: "root", Matches: []blacklist.MatchItem{{Pattern: "rm", Line: "rm -rf /"}}},
	})
	m := newTestModel(t)
	m.cmdInput.SetValue("rm -rf /")
	nm, cmd := m.startRun()
	m = nm.(ExecModel)
	if cmd != nil {
		t.Fatal("expected nil cmd while blocked")
	}
	if m.current() != LocDanger {
		t.Fatalf("expected LocDanger, got %v", m.current())
	}
	got := m.View()
	for _, want := range []string{"危险命令确认", "rm -rf /", "rm"} {
		if !contains(got, want) {
			t.Fatalf("view missing %q:\n%s", want, got)
		}
	}
}

func TestDanger_YProceedsToResult(t *testing.T) {
	fakeBlacklist([]BlockedInfo{{NodeID: "n1", User: "root"}})
	fakeStream(nil)
	m := newTestModel(t)
	m.cmdInput.SetValue("rm -rf /")
	nm, _ := m.startRun()
	m = nm.(ExecModel)
	nm, cmd := m.Update(runeKey('y'))
	m = nm.(ExecModel)
	if cmd == nil {
		t.Fatal("expected exec cmd after y")
	}
	if _, ok := cmd().(ExecStreamMsg); !ok {
		t.Fatalf("expected ExecStreamMsg, got %T", cmd())
	}
	if m.current() != LocResult {
		t.Fatalf("expected LocResult, got %v", m.current())
	}
}

func TestDanger_NCancels(t *testing.T) {
	fakeBlacklist([]BlockedInfo{{NodeID: "n1", User: "root"}})
	m := newTestModel(t)
	m.cmdInput.SetValue("rm -rf /")
	nm, _ := m.startRun()
	m = nm.(ExecModel)
	nm, cmd := m.Update(runeKey('n'))
	m = nm.(ExecModel)
	if cmd != nil {
		t.Fatal("expected nil cmd after n")
	}
	if m.current() != LocRun {
		t.Fatalf("expected LocRun, got %v", m.current())
	}
}

func TestForce_SkipsBlacklist(t *testing.T) {
	checkBlacklist = func(nodeUsers map[string]string, cmd string) []BlockedInfo {
		t.Fatal("checkBlacklist should not be called with force")
		return nil
	}
	fakeStream(nil)
	m := newTestModel(t)
	m.cmdInput.SetValue("rm -rf /")
	m.advanced = newAdvancedForm()
	m.advanced.fields[17].checked = true // force 行
	nm, cmd := m.startRun()
	m = nm.(ExecModel)
	if cmd == nil {
		t.Fatal("expected exec cmd with force")
	}
	if _, ok := cmd().(ExecStreamMsg); !ok {
		t.Fatalf("expected ExecStreamMsg, got %T", cmd())
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./cmd/cli/cmd/tui/exec/ -run TestDanger`
Expected: 编译失败（checkBlacklist 不存在）

- [ ] **Step 3: 创建 danger.go** `cmd/cli/cmd/tui/exec/danger.go`

```go
package exec

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/internal/control/blacklist"
)

type BlockedInfo struct {
	NodeID  string
	User    string
	Matches []blacklist.MatchItem
}

// checkBlacklist 可注入的黑名单检查(测试替换)
var checkBlacklist = func(nodeUsers map[string]string, cmd string) []BlockedInfo {
	cfg, err := blacklist.LoadConfig()
	if err != nil {
		return nil
	}
	checker := blacklist.NewChecker(cfg)
	var out []BlockedInfo
	for id, user := range nodeUsers {
		r := checker.Check(user, cmd)
		if r.Blocked {
			out = append(out, BlockedInfo{NodeID: id, User: user, Matches: r.Matches})
		}
	}
	return out
}

func (m ExecModel) updateDanger(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "y":
		m.pop()
		return m, m.launchRun(m.pendingIDs, m.pendingCmd, m.pendingOpts)
	case "n", "esc":
		m.pop()
		return m, nil
	}
	return m, nil
}
```

- [ ] **Step 4: startRun 加黑名单检查 + pending 字段**

`cmd/cli/cmd/tui/exec/run.go` 的 startRun 中，`ids` 组装之后、`return m.launchRun(...)` 之前插入：

```go
	if !f.isOn("force") {
		nodeUsers := map[string]string{}
		for _, n := range nodes {
			nodeUsers[n.ID] = n.User
		}
		if blocked := checkBlacklist(nodeUsers, cmd); len(blocked) > 0 {
			m.pendingBlocked = blocked
			m.pendingCmd = cmd
			m.pendingIDs = ids
			m.pendingOpts = opts
			m.push(LocDanger)
			return nil, nil
		}
	}
	return m.launchRun(ids, cmd, opts), nil
```

`cmd/cli/cmd/tui/exec/exec.go` 字段追加：

```go
	pendingBlocked []BlockedInfo
	pendingCmd     string
	pendingIDs     []string
	pendingOpts    *command.ExecuteOptions
```

- [ ] **Step 5: 加 dangerView**（`cmd/cli/cmd/tui/exec/view.go` 末尾追加）

```go
func (m ExecModel) dangerView() string {
	var b strings.Builder
	b.WriteString("┌─ 危险命令确认 ─────────────────────\n")
	b.WriteString("  命令: " + styleSelected.Render(m.pendingCmd) + "\n")
	for _, bl := range m.pendingBlocked {
		b.WriteString(fmt.Sprintf("  ✗ %s (user=%s)\n", bl.NodeID, bl.User))
		for _, mt := range bl.Matches {
			b.WriteString(styleError.Render("     匹配 "+mt.Pattern+": "+mt.Line) + "\n")
		}
	}
	b.WriteString(styleDim.Render("  继续执行? y 执行  n 取消") + "\n")
	b.WriteString("└─")
	return b.String()
}
```

- [ ] **Step 6: 运行测试确认通过**

Run: `go test ./cmd/cli/cmd/tui/...`
Expected: 全部 PASS

- [ ] **Step 7: 集成提示文案**

`cmd/cli/cmd/tui/nodes/view.go:137` 状态栏快捷键串追加：

```go
		b.WriteString(styleDim.Render("↑↓选择 ←→切栏 g/G首尾 a添加 e编辑 d删除 c列 p ping k 检查 i导入导出 o分组 l标签 x执行 /过滤 ?帮助 q退出"))
```

`cmd/cli/cmd/tui/app.go` 的 helpView 追加 Exec 面板说明：

```go
		"  执行:  命令必填  r 执行  a 高级选项  f 格式",
		"        ↑↓ 移动字段  Enter 编辑  Esc 返回 Nodes",
```

- [ ] **Step 8: 全量测试 + 提交**

Run: `go test ./...`
Expected: 全部 PASS

```bash
git add cmd/cli/cmd/tui/
git commit -m "feat(tui): Exec 黑名单确认视图 + 帮助/状态栏集成"
```

- [ ] **Step 9: E2E 冒烟**

Run:
```bash
go build -o build/owl.exe ./cmd/cli
./build/owl.exe tui
```
手动冒烟清单：
1. 启动 TUI，顶栏显示 `[Nodes] [Exec]`，Nodes 高亮，Nodes 面板正常
2. 按 Tab → Exec 面板，显示命令/节点/分组/标签/格式表单；按 1 切回
3. Nodes 面板 `/` 过滤 `g:web` → 按 x 进入 Exec，目标显示过滤后节点数
4. 输入命令 `echo hi`，r 执行 → 结果视图流式出现 ✓ 节点
5. 命令填 `rm -rf /`，r 执行 → 危险确认视图，n 取消回表单，y 执行
6. a 打开高级选项，空格切换 bool、Enter 编辑文本、s 返回主视图
7. 结果视图 r 重跑、Esc 返回表单；表单 Esc 回 Nodes 面板；q 退出

验证通过后：
```bash
git add -A && git commit -m "docs(tui): Exec 面板 E2E 冒烟验证"
```

---

## Self-Review

**Spec 覆盖检查：**
- 菜单切换方案：Task 1（Tab/数字/x + 快照捕获 + LeavePanelMsg）✓
- --groups/--labels/--nodes 一等公民：Task 2（分组/标签/节点三个输入行 + 优先级解析）✓
- --format 一等公民：Task 2（f 循环 simple/detail/json）✓
- 其他 flags 模态表单：Task 3（20 字段高级表单）✓
- 目标默认当前过滤可见：Task 1 快照 + Task 2 回退逻辑 ✓
- 流式结果渲染：Task 4 ✓
- 黑名单安全确认：Task 5 ✓
- 可注入测试（TDD）：每任务测试均替换全局 var ✓

**类型一致性检查：**
- `CaptureTargets`/`Targets` 签名在 Task 1 定义，Task 2 重写时保留 ✓
- `buildOpts` 返回 `(*command.ExecuteOptions, error)`，Task 4/5 均按此消费 ✓
- `launchRun(ids []string, cmd string, opts *command.ExecuteOptions) tea.Cmd` 在 Task 4 定义，Task 5 复用 ✓
- `Loc` 常量顺序 LocRun/LocAdvanced/LocResult/LocDanger 一致 ✓
- advanced 字段索引：parallel=3, serial=4, no-retry=8, force=17（specs 顺序）— 测试中 `toggle(4)`/`toggle(8)`/`fields[17]` 与 specs 对齐 ✓
