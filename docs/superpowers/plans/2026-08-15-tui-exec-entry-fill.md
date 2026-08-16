# TUI Exec 入口填充 + Nodes 多选 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** x 进入 Exec 面板时自动带入 Nodes 的选择状态：有勾选则填显式节点列表，无勾选则把纯组/标签过滤条件填充到表单字段；Nodes 列表新增 Space 勾选多选。

**Architecture:** App 层 switchPanel 增加 Entry 参数区分"中性切换"与"按选择进入"；x 触发 EntryBySelection 时 App 组装勾选 ID 或过滤条件调用 exec 面板的 FillNodes/FillConditions；exec.resolveTargets 改为 nodes > (groups AND labels) > 快照。Nodes 面板加 marked map 与 Space 切换、行首 [x] 槽渲染。

**Tech Stack:** Go, charmbracelet/bubbletea + bubbles/textinput + lipgloss（已有）

## Global Constraints

- 项目目录：`F:\pantheon\trae_projects\git\go-owl`；禁止越出项目根目录
- TDD：每个任务先写失败测试再实现；测试命令 `go test ./cmd/cli/cmd/tui/...`
- 当前为 4 面板结构（Nodes/Exec/File/AI，panel 0/1/2/3），`switchPanel` 签名变更须同步全部调用点（tab mod 4、1/2/3/4、f、x、三个 LeavePanelMsg）
- 工作区存在无关未提交改动（nodes/filter.go 全角归一化 WIP、AGENTS.md、.reasonix/*、cli.exe、.github/）——**不得触碰、不得 stage**
- 每次提交只 stage 本任务文件；提交信息沿用项目风格
- 面板文案中文硬编码；新键不得与现有键冲突（nodes 列表已用 a/e/d/c/p/k/i/o/l/g/G//?/q/↑↓←→/Space 的 Space 仅在 LocColumns 内使用，列表层空闲）
- CLI exec run 的互斥选择语义不变，仅 TUI 内 resolveTargets 改为 AND 语义

---

### Task 1: resolveTargets 组+标签 AND 语义

**Files:**
- Modify: `cmd/cli/cmd/tui/exec/exec.go:203-217`（resolveTargets 的 groups/labels 分支）
- Test: `cmd/cli/cmd/tui/exec/exec_test.go`（追加）

**Interfaces:**
- Consumes: 现有 `resolveTargets`、`groupsIntersect`、`labelsMatch`、`splitTrim`、`parseLabels`、`dedupeSorted`
- Produces: 无新签名；行为变化——groupsInput 与 labelsInput 同时非空时取交集（此前 groups 优先互斥）

- [ ] **Step 1: 写失败测试**（追加到 `cmd/cli/cmd/tui/exec/exec_test.go`）

```go
func TestResolve_GroupsAndLabels_Intersect(t *testing.T) {
	m := newTestModel(t)
	m.groupsInput.SetValue("web")
	m.labelsInput.SetValue("role=cache")
	nodes, err := m.resolveTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].ID != "n3" {
		t.Fatalf("expected [n3] (web AND role=cache), got %v", nodeIDs(nodes))
	}
}

func TestResolve_GroupsAndLabels_Mismatch(t *testing.T) {
	m := newTestModel(t)
	m.groupsInput.SetValue("db")
	m.labelsInput.SetValue("env=prod")
	nodes, err := m.resolveTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 0 {
		t.Fatalf("expected no match (db AND env=prod), got %v", nodeIDs(nodes))
	}
}

func TestResolve_GroupsOnly_NoLabels(t *testing.T) {
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

func TestResolve_LabelsOnly_NoGroups(t *testing.T) {
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
```

注：`newTestModel` seed 数据（n1 web/prod、n2 db/dev、n3 web+cache/prod+role=cache）中，`web ∩ role=cache` = n3、`db ∩ env=prod` = 空。

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./cmd/cli/cmd/tui/exec/ -run TestResolve_GroupsAndLabels -v`
Expected: `TestResolve_GroupsAndLabels_Intersect` FAIL（现行为 groups 优先，结果为 [n1 n3] 而非 [n3]）

- [ ] **Step 3: 实现 AND 语义**（`cmd/cli/cmd/tui/exec/exec.go` 的 resolveTargets）

把现在的两个 case 分支（groups 分支 + labels 分支）替换为一个合并分支：

```go
	case m.groupsInput.Value() != "" || m.labelsInput.Value() != "":
		groups := splitTrim(m.groupsInput.Value(), ",")
		labels := parseLabels(m.labelsInput.Value())
		for _, n := range all {
			if len(groups) > 0 && !groupsIntersect(n.Groups, groups) {
				continue
			}
			if len(labels) > 0 && !labelsMatch(n.Labels, labels) {
				continue
			}
			nodes = append(nodes, n)
		}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./cmd/cli/cmd/tui/exec/`
Expected: 全部 PASS（既有 TestResolve_PriorityNodesOverGroups / TestResolve_ExplicitNodes / TestResolve_EmptyFallsBackToSnapshot 不受影响）

- [ ] **Step 5: 提交**

```bash
git add cmd/cli/cmd/tui/exec/exec.go cmd/cli/cmd/tui/exec/exec_test.go
git commit -m "feat(tui): Exec 目标解析组+标签 AND 交集(替代互斥)"
```

---

### Task 2: Nodes 多选勾选

**Files:**
- Modify: `cmd/cli/cmd/tui/nodes/model.go`（marked 字段 + Space 处理 + 导出方法）
- Modify: `cmd/cli/cmd/tui/nodes/view.go`（listPane 勾选槽 + statusBar 已选数）
- Test: `cmd/cli/cmd/tui/nodes/model_test.go`、`cmd/cli/cmd/tui/nodes/view_test.go`（追加）

**Interfaces:**
- Consumes: `Model`、`newTestStore`/`seedNodes`（已有测试 helper）、`sortedLabels` 等
- Produces: `Model.MarkedIDs() []string`（按 ID 排序）、`Model.MarkedCount() int`、`Model.IsMarked(id string) bool`、`Model.Filter() FilterQuery`（Task 3 消费）

- [ ] **Step 1: 写失败测试**（追加到 `cmd/cli/cmd/tui/nodes/model_test.go`）

```go
func TestMark_ToggleWithSpace(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	// 光标在 n1, Space 勾选
	nm, _ := m.Update(runeKey(' '))
	m = nm.(Model)
	if !m.IsMarked("n1") || m.MarkedCount() != 1 {
		t.Fatalf("expected n1 marked, got %d", m.MarkedCount())
	}
	// 再 Space 取消
	nm, _ = m.Update(runeKey(' '))
	m = nm.(Model)
	if m.IsMarked("n1") || m.MarkedCount() != 0 {
		t.Fatal("expected n1 unmarked after second space")
	}
}

func TestMarkedIDs_Sorted(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	m.marked["n3"] = true
	m.marked["n1"] = true
	if got := m.MarkedIDs(); len(got) != 2 || got[0] != "n1" || got[1] != "n3" {
		t.Fatalf("expected sorted [n1 n3], got %v", got)
	}
}

func TestMarked_MovesWithCursorAndSurvivesFilter(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	// 勾选 n1, 下移到 n2 勾选
	nm, _ := m.Update(runeKey(' '))
	m = nm.(Model)
	nm, _ = m.Update(key(tea.KeyDown))
	m = nm.(Model)
	nm, _ = m.Update(runeKey(' '))
	m = nm.(Model)
	if m.MarkedCount() != 2 {
		t.Fatalf("expected 2 marked, got %d", m.MarkedCount())
	}
	// 过滤到 db 组(n2), 勾选保留
	m.filter = ParseFilterQuery("g:db")
	m.reload()
	if !m.IsMarked("n1") || !m.IsMarked("n2") {
		t.Fatal("marks must survive filter change")
	}
	if len(m.visible()) != 1 || m.visible()[0].ID != "n2" {
		t.Fatalf("filter should show only n2, got %v", nodeIDs(m.visible()))
	}
}

func TestMark_DoesNotAffectIsDirty(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey(' '))
	m = nm.(Model)
	if m.IsDirty() {
		t.Fatal("marks are session state, must not dirty the model")
	}
}

func TestFilter_Exported(t *testing.T) {
	m := NewModel(newTestStore(t))
	m.filter = ParseFilterQuery("g:web l:env=prod")
	fq := m.Filter()
	if len(fq.Groups) != 1 || fq.Groups[0] != "web" || fq.Labels["env"] != "prod" {
		t.Fatalf("unexpected filter export: %#v", fq)
	}
}
```

注：`nodeIDs` helper 在 nodes 包测试中可能不存在——用局部遍历替代或检查既有 helper。若 `model_test.go` 无 `nodeIDs`，在测试内联：

```go
func idsOf(nodes []*common.NodeInfo) []string {
	ids := make([]string, 0, len(nodes))
	for _, n := range nodes {
		ids = append(ids, n.ID)
	}
	return ids
}
```

（把 `TestMarked_MovesWithCursorAndSurvivesFilter` 里的 `nodeIDs(m.visible())` 换成 `idsOf(...)` 并内联 helper。）

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -run TestMark -v`
Expected: 编译失败（marked 字段 / MarkedIDs 不存在）

- [ ] **Step 3: model.go 实现**

在 `Model` struct 的 `filterInput textinput.Model` 之后加字段：

```go
	marked map[string]bool
```

`NewModel` 初始化（`filterInput: newInput(...)` 行后）：

```go
		marked:      map[string]bool{},
```

`updateList` 的 `switch km.String()` 中 `case "/":` 之前加：

```go
	case " ":
		if n := m.selectedNode(); n != nil {
			if m.marked[n.ID] {
				delete(m.marked, n.ID)
			} else {
				m.marked[n.ID] = true
			}
		}
```

`InsertMode` 方法之后追加导出方法：

```go
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
```

（`sort` 已 import。）

- [ ] **Step 4: view.go 渲染**

`listPane()` 列头行前加 5 字符前缀对齐（box 3 + 空格 1 + marker 1）：

```go
	b.WriteString("     ")
	for i, c := range cols {
		b.WriteString(styleSelected.Render(truncateCell(c.Label, widths[i])))
		b.WriteString(" ")
	}
	b.WriteString("\n")
	b.WriteString("     " + strings.Repeat("─", sum(widths)+len(cols)) + "\n")
```

行渲染循环改为：

```go
	for i, n := range v {
		box := "[ ]"
		if m.marked[n.ID] {
			box = "[x]"
		}
		marker := " "
		if i == m.cursor {
			marker = ">"
		}
		b.WriteString(box + " " + marker)
```

列宽计算 `computeColumnWidths(cols, avail)` 改为 `computeColumnWidths(cols, avail-5)`（让出前缀宽度）。

`statusBar()` 中 filter chips 之后加：

```go
	if m.MarkedCount() > 0 {
		b.WriteString(styleSelected.Render(fmt.Sprintf("[已选 %d]", m.MarkedCount())))
		b.WriteString("  ")
	}
```

- [ ] **Step 5: 视图测试**（追加到 `cmd/cli/cmd/tui/nodes/view_test.go`）

```go
func TestListView_ShowsMarkBoxes(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey(' '))
	m = nm.(Model)
	got := m.View()
	if !strings.Contains(got, "[x]") || !strings.Contains(got, "[ ]") {
		t.Fatalf("expected mark boxes in list view:\n%s", got)
	}
}

func TestStatusBar_ShowsMarkCount(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	m.marked["n1"] = true
	m.marked["n2"] = true
	got := m.View()
	if !strings.Contains(got, "已选 2") {
		t.Fatalf("expected mark count in status bar:\n%s", got)
	}
}
```

（view_test.go 需 import "strings"——检查既有 import，缺失则加。）

- [ ] **Step 6: 运行测试确认通过**

Run: `go test ./cmd/cli/cmd/tui/nodes/`
Expected: 全部 PASS（既有过滤/列/表单测试不受影响）

- [ ] **Step 7: 提交**

```bash
git add cmd/cli/cmd/tui/nodes/model.go cmd/cli/cmd/tui/nodes/view.go cmd/cli/cmd/tui/nodes/model_test.go cmd/cli/cmd/tui/nodes/view_test.go
git commit -m "feat(tui): Nodes 列表 Space 勾选多选(会话态, 状态栏已选数)"
```

---

### Task 3: x 入口分派（勾选/过滤条件填充）

**Files:**
- Modify: `cmd/cli/cmd/tui/app.go`（switchPanel 签名 + Entry 类型 + applySelectionEntry + 调用点）
- Modify: `cmd/cli/cmd/tui/exec/exec.go`（FillNodes / FillConditions）
- Test: `cmd/cli/cmd/tui/app_test.go`、`cmd/cli/cmd/tui/exec/exec_test.go`（追加）

**Interfaces:**
- Consumes: Task 2 的 `MarkedIDs() []string`、`Filter() FilterQuery`；现有 `CaptureTargets`
- Produces: `type Entry int`（EntryNeutral=0 / EntryBySelection=1）、`switchPanel(i int, entry Entry)`、`applySelectionEntry()`、`ExecModel.FillNodes(ids []string)`、`ExecModel.FillConditions(groups []string, labels map[string]string)`

- [ ] **Step 1: 写失败测试**

exec 级（追加到 `cmd/cli/cmd/tui/exec/exec_test.go`）：

```go
func TestFillNodes_SetsAndClearsConditions(t *testing.T) {
	m := newTestModel(t)
	m.groupsInput.SetValue("web")
	m.labelsInput.SetValue("env=prod")
	m.FillNodes([]string{"n1", "n3"})
	if got := m.nodesInput.Value(); got != "n1,n3" {
		t.Fatalf("expected nodes n1,n3, got %q", got)
	}
	if m.groupsInput.Value() != "" || m.labelsInput.Value() != "" {
		t.Fatal("expected conditions cleared")
	}
}

func TestFillConditions_FillsSortedPairs(t *testing.T) {
	m := newTestModel(t)
	m.nodesInput.SetValue("n1")
	m.FillConditions([]string{"web", "db"}, map[string]string{"env": "prod", "zone": ""})
	if m.nodesInput.Value() != "" {
		t.Fatal("expected nodes cleared")
	}
	if got := m.groupsInput.Value(); got != "web,db" {
		t.Fatalf("expected groups web,db, got %q", got)
	}
	if got := m.labelsInput.Value(); got != "env=prod,zone" {
		t.Fatalf("expected labels env=prod,zone, got %q", got)
	}
}

func TestFillConditions_ClearsWhenNil(t *testing.T) {
	m := newTestModel(t)
	m.nodesInput.SetValue("n1")
	m.groupsInput.SetValue("web")
	m.labelsInput.SetValue("env=prod")
	m.FillConditions(nil, nil)
	if m.nodesInput.Value() != "" || m.groupsInput.Value() != "" || m.labelsInput.Value() != "" {
		t.Fatal("expected all fields cleared")
	}
}
```

App 级（追加到 `cmd/cli/cmd/tui/app_test.go`）：

```go
func TestApp_XWithMarksFillsNodes(t *testing.T) {
	m := newApp(t)
	// 勾选 n1, n2 (seed: n1/n2)
	nm, _ := m.Update(runeKey(' ')) // 光标 n1
	m = nm.(*App)
	nm, _ = m.Update(key(tea.KeyDown))
	m = nm.(*App)
	nm, _ = m.Update(runeKey(' ')) // n2
	m = nm.(*App)
	nm, _ = m.Update(runeKey('x'))
	m = nm.(*App)
	if m.panel != 1 {
		t.Fatalf("expected panel 1, got %d", m.panel)
	}
	if got := m.exec.NodesValue(); got != "n1,n2" {
		t.Fatalf("expected nodes n1,n2, got %q", got)
	}
}

func TestApp_XNoMarksFillsFilterConditions(t *testing.T) {
	m := newApp(t)
	// 过滤 g:web
	nm, _ := m.Update(runeKey('/'))
	nm, _ = nm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g:web")})
	nm, _ = nm.Update(key(tea.KeyEnter))
	m = nm.(*App)
	nm, _ = m.Update(runeKey('x'))
	m = nm.(*App)
	if m.panel != 1 {
		t.Fatalf("expected panel 1, got %d", m.panel)
	}
	if got := m.exec.GroupsValue(); got != "web" {
		t.Fatalf("expected groups web, got %q", got)
	}
	if got := m.exec.NodesValue(); got != "" {
		t.Fatalf("expected nodes cleared, got %q", got)
	}
}

func TestApp_XSearchFilterFallsBackToSnapshot(t *testing.T) {
	m := newApp(t)
	// 过滤搜索词 foo (seed 无匹配)
	nm, _ := m.Update(runeKey('/'))
	nm, _ = nm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("foo")})
	nm, _ = nm.Update(key(tea.KeyEnter))
	m = nm.(*App)
	nm, _ = m.Update(runeKey('x'))
	m = nm.(*App)
	if m.panel != 1 {
		t.Fatalf("expected panel 1, got %d", m.panel)
	}
	if got := m.exec.GroupsValue(); got != "" {
		t.Fatalf("search filter must not fill groups, got %q", got)
	}
	if got := m.exec.NodesValue(); got != "" {
		t.Fatalf("expected nodes cleared, got %q", got)
	}
	// 快照兜底: 目标仍为全部可见节点
	nodes, err := m.exec.ResolveForTest()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 0 {
		t.Fatalf("expected empty snapshot fallback (search foo matches nothing, visible=0), got %d", len(nodes))
	}
}

func TestApp_TabNeutralKeepsFormState(t *testing.T) {
	m := newApp(t)
	// 进 Exec 手填 nodes
	nm, _ := m.Update(runeKey('2'))
	m = nm.(*App)
	m.exec.FillNodes([]string{"n1"})
	// Tab 绕一圈(4 面板)再回 Exec: 字段保留
	for i := 0; i < 4; i++ {
		nm, _ = m.Update(key(tea.KeyTab))
		m = nm.(*App)
	}
	if m.panel != 1 {
		t.Fatalf("expected panel 1 after 4 tabs, got %d", m.panel)
	}
	if got := m.exec.NodesValue(); got != "n1" {
		t.Fatalf("tab must be neutral, expected nodes n1, got %q", got)
	}
}
```

注：exec 测试需要导出访问器。App 测试访问 `m.exec.NodesValue()`/`GroupsValue()`/`ResolveForTest()`。

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./cmd/cli/cmd/tui/exec/ -run TestFill -v; go test ./cmd/cli/cmd/tui/ -run TestApp_X -v`
Expected: 编译失败（FillNodes/FillConditions/NodesValue 等不存在）

- [ ] **Step 3: exec 侧实现**（`cmd/cli/cmd/tui/exec/exec.go`）

在 `CaptureTargets` 之后追加：

```go
// FillNodes 填显式节点列表并清空组/标签条件
func (m *ExecModel) FillNodes(ids []string) {
	m.nodesInput.SetValue(strings.Join(ids, ","))
	m.groupsInput.SetValue("")
	m.labelsInput.SetValue("")
}

// FillConditions 填分组/标签条件并清空显式节点列表(组/标签为空即清空)
func (m *ExecModel) FillConditions(groups []string, labels map[string]string) {
	m.nodesInput.SetValue("")
	m.groupsInput.SetValue(strings.Join(groups, ","))
	var pairs []string
	for k, v := range labels {
		if v != "" {
			pairs = append(pairs, k+"="+v)
		} else {
			pairs = append(pairs, k)
		}
	}
	sort.Strings(pairs)
	m.labelsInput.SetValue(strings.Join(pairs, ","))
}
```

（`sort` 已 import；`strings` 已 import。若 `strings.Join` 未用过的包检查 import。）

追加测试访问器（exec.go 末尾或 CaptureTargets 旁）：

```go
func (m ExecModel) NodesValue() string  { return m.nodesInput.Value() }
func (m ExecModel) GroupsValue() string { return m.groupsInput.Value() }

// ResolveForTest 导出目标解析(测试断言快照回退)
func (m ExecModel) ResolveForTest() ([]*common.NodeInfo, error) { return m.resolveTargets() }
```

- [ ] **Step 4: App 侧实现**（`cmd/cli/cmd/tui/app.go`）

```go
// Entry 进入面板的方式
type Entry int

const (
	EntryNeutral Entry = iota // Tab/数字/f: 不动表单, 仅刷新快照
	EntryBySelection          // x: 勾选优先, 否则纯组/标签过滤条件
)
```

`switchPanel` 签名与实现：

```go
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
	}
}

// applySelectionEntry 按 x 语义填充 Exec 表单: 勾选优先, 否则纯组/标签过滤; 含搜索/状态回退快照
func (m *App) applySelectionEntry() {
	if ids := m.nodes.MarkedIDs(); len(ids) > 0 {
		m.exec.FillNodes(ids)
		return
	}
	fq := m.nodes.Filter()
	if fq.Search == "" && fq.Status == "" {
		m.exec.FillConditions(fq.Groups, fq.Labels)
		return
	}
	m.exec.FillConditions(nil, nil)
}
```

调用点全部更新（`m.switchPanel(...)` → 带 Entry；当前为 4 面板 Nodes/Exec/File/AI）：
- `Update` 内 `exec.LeavePanelMsg` / `file.LeavePanelMsg` / `tuiai.LeavePanelMsg`：`m.switchPanel(0, EntryNeutral)`
- `case "tab":` → `m.switchPanel((m.panel+1)%4, EntryNeutral)`
- `case "1":` → `m.switchPanel(0, EntryNeutral)`；`"2"` → `(1, EntryNeutral)`；`"3"` → `(2, EntryNeutral)`；`"4"` → `(3, EntryNeutral)`
- `case "f":` → `m.switchPanel(2, EntryNeutral)`
- `case "x":` → `m.switchPanel(1, EntryBySelection)`

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./cmd/cli/cmd/tui/...`
Expected: 全部 PASS（含既有 TestApp_ExecCapturesVisibleSnapshot——x 进入后 groupsInput 填了 web，但该测试断言快照 Targets() 而非表单值，不受影响）

- [ ] **Step 6: 提交**

```bash
git add cmd/cli/cmd/tui/app.go cmd/cli/cmd/tui/app_test.go cmd/cli/cmd/tui/exec/exec.go cmd/cli/cmd/tui/exec/exec_test.go
git commit -m "feat(tui): x 进入 Exec 按勾选/过滤条件填充表单(EntryBySelection)"
```

---

### Task 4: 集成文案 + E2E

**Files:**
- Modify: `cmd/cli/cmd/tui/app.go`（helpView）
- Modify: `cmd/cli/cmd/tui/nodes/view.go`（statusBar 快捷键串）

- [ ] **Step 1: 文案更新**

`cmd/cli/cmd/tui/app.go` helpView 列表行追加 Space 说明：

```go
		"        / 过滤: 关键词 | g:组 l:k=v s:状态(空格或&&=AND)",
		"        Space 勾选多选(x 带入 Exec)  ? 帮助  q 退出",
```

`cmd/cli/cmd/tui/nodes/view.go` statusBar 快捷键串：

```go
		b.WriteString(styleDim.Render("↑↓选择 ←→切栏 g/G首尾 a添加 e编辑 d删除 c列 p ping k 检查 i导入导出 o分组 l标签 Space勾选 x执行 /过滤 ?帮助 q退出"))
```

- [ ] **Step 2: 全量测试**

Run: `go test ./cmd/cli/cmd/tui/...`
Expected: 全部 PASS

```bash
git add cmd/cli/cmd/tui/app.go cmd/cli/cmd/tui/nodes/view.go
git commit -m "docs(tui): 帮助/状态栏补充 Space 勾选与 x 带入说明"
```

- [ ] **Step 3: 构建**

Run: `go build -o build/owl.exe ./cmd/cli`
Expected: 构建成功

- [ ] **Step 4: E2E 冒烟（controller 与用户执行）**

```bash
./build/owl.exe tui
```
冒烟清单：
1. Nodes 列表行首显示 `[ ]` 槽；Space 勾选 n1 → `[x]`，状态栏出现"已选 1"
2. 过滤 `g:web` 后勾选保留；按 x 进入 Exec → 节点字段填勾选 ID、组/标签为空、目标数为勾选数
3. 清除勾选（再 Space 取消全部）后按 x → 组字段填 `web`（过滤条件带入）
4. 过滤 `foo`（搜索词，无匹配）按 x → 组/标签为空，目标为 0（快照=当前可见集）；过滤 `s:online` 按 x → 目标为在线节点数（快照回退，不填充表单）
5. Tab 切走切回 Exec → 手填字段保留（中性切换）
6. 回归：Exec 面板原有 r 执行 / a 高级 / 危险确认 / Esc 返回 Nodes 均正常

验证通过后由 controller 收尾提交。

---

## Self-Review

**Spec 覆盖：**
- 需求 1（过滤联动）：Task 1（AND 语义）+ Task 3（x 填充条件）✓
- 需求 2（多选勾选）：Task 2（Space + 槽 + 状态栏）✓；用户澄清"也是 x 进入"→ Task 3 勾选优先填充 nodes ✓
- 勾选列始终显示、p/k 不联动：Task 2 方案已定（仅列表槽 + 状态栏计数，p/k 不变）✓
- 3 面板兼容：Task 3 调用点全量更新（tab mod 3、1/2/3、f、LeavePanelMsg）✓
- 含搜索/状态回退快照：Task 3 applySelectionEntry ✓
- 工作区 WIP 隔离：每任务提交只 stage 本任务文件 ✓

**类型一致性：**
- `switchPanel(i int, entry Entry)` 在 Task 3 定义，Task 4 不调用 ✓
- `FillNodes([]string)` / `FillConditions([]string, map[string]string)` 在 Task 3 定义，App 同任务消费 ✓
- `MarkedIDs()/MarkedCount()/IsMarked()/Filter()` 在 Task 2 定义，Task 3 消费 ✓
- `NodesValue()/GroupsValue()/ResolveForTest()` 测试访问器与 App 测试断言一致 ✓
- 既有 `TestApp_ExecCapturesVisibleSnapshot` 依赖 x 旧行为（快照）——新行为下 x 还会 FillConditions("web")，但测试断言 Targets() 快照不受影响 ✓
