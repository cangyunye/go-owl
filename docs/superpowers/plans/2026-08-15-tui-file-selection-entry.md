# TUI f 键升级：带选择跳转 File 面板

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** nodes 面板按 `f` 跳转 File 时，像 `x` 跳转 Exec 一样带选择带入：勾选节点（Space）优先填入节点字段；否则纯 g:/l: 筛选填入分组/标签字段；含搜索/状态时清空条件走快照兜底。Tab/`3` 进入 File 保持中性（不填充）。

**Architecture:** 复用 App 已有 `Entry` 机制（app.go:64-70 EntryNeutral/EntryBySelection）。file.FileModel 补齐 `FillNodes`/`FillConditions`（exec.go:99-119 同款语义）+ 导出 getter（App 层测试断言）。App 把 `applySelectionEntry`/`applyFileSelectionEntry` 的公共逻辑抽成共享 helper `selectionFill(fillNodes, fillConditions)`，`case "f":` 改为 `switchPanel(2, EntryBySelection)`，保留 `m.nodes.AtList()` 限定（防劫持 i 对话框格式切换键）。

**Tech Stack:** Go, charmbracelet/bubbletea（已有）

## Global Constraints

- 项目目录：`F:\pantheon\trae_projects\git\go-owl`；禁止越出项目根目录
- TDD：每个任务先写失败测试再实现；测试命令 `go test ./cmd/cli/cmd/tui/...`
- 每个任务结束提交一个 atomic commit；全部完成后跑 E2E 冒烟再提交
- 面板文案沿用项目风格（中文硬编码）
- `f` 键升级后语义与 `x` 完全对称；Tab/`3`/`1`/`2`/`4` 数字键保持 EntryNeutral（中性跳转不覆盖表单）
- `f` 键保留 `m.nodes.AtList()` 限定（i 对话框内 `f` 仍是格式切换）
- 只改 `cmd/cli/cmd/tui/file/`、`cmd/cli/cmd/tui/app.go`、`cmd/cli/cmd/tui/app_test.go`、`cmd/cli/cmd/tui/nodes/view.go`（若需状态栏文案）
- **注意**：工作区存在无关改动（nodes/filter.go、nodes/filter_test.go、AGENTS.md、.reasonix/*）与无关提交（7df381b refactor(ai)），不得触碰/纳入提交

---

### Task 1: file 面板补齐 FillNodes/FillConditions + 导出 getter

**Files:**
- Modify: `cmd/cli/cmd/tui/file/file.go`
- Modify: `cmd/cli/cmd/tui/file/file_test.go`

**Interfaces:**
- Consumes: 无新依赖
- Produces: `(*FileModel).FillNodes(ids []string)`、`(*FileModel).FillConditions(groups []string, labels map[string]string)`、`(FileModel).NodesValue() string`、`(FileModel).GroupsValue() string`

- [ ] **Step 1: 写失败测试**（追加到 `cmd/cli/cmd/tui/file/file_test.go`）

```go
func TestFillNodes_FillsAndClears(t *testing.T) {
	m := newTestModel(t)
	m.groupsInput.SetValue("web")
	m.labelsInput.SetValue("env=prod")
	m.FillNodes([]string{"n1", "n3"})
	if got := m.NodesValue(); got != "n1,n3" {
		t.Fatalf("expected nodes n1,n3, got %q", got)
	}
	if got := m.GroupsValue(); got != "" {
		t.Fatalf("expected groups cleared, got %q", got)
	}
	if got := m.labelsInput.Value(); got != "" {
		t.Fatalf("expected labels cleared, got %q", got)
	}
}

func TestFillConditions_FillsAndClears(t *testing.T) {
	m := newTestModel(t)
	m.nodesInput.SetValue("n1")
	m.FillConditions([]string{"web", "cache"}, map[string]string{"env": "prod", "role": "cache"})
	if got := m.NodesValue(); got != "" {
		t.Fatalf("expected nodes cleared, got %q", got)
	}
	if got := m.GroupsValue(); got != "web,cache" {
		t.Fatalf("expected groups web,cache, got %q", got)
	}
	if got := m.labelsInput.Value(); got != "env=prod,role=cache" {
		t.Fatalf("expected sorted labels env=prod,role=cache, got %q", got)
	}
}

func TestFillConditions_EmptyClearsAll(t *testing.T) {
	m := newTestModel(t)
	m.nodesInput.SetValue("n1")
	m.groupsInput.SetValue("web")
	m.labelsInput.SetValue("env=prod")
	m.FillConditions(nil, nil)
	if got := m.NodesValue(); got != "" {
		t.Fatalf("expected nodes cleared, got %q", got)
	}
	if got := m.GroupsValue(); got != "" {
		t.Fatalf("expected groups cleared, got %q", got)
	}
	if got := m.labelsInput.Value(); got != "" {
		t.Fatalf("expected labels cleared, got %q", got)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./cmd/cli/cmd/tui/file/ -run TestFill`
Expected: 编译失败（FillNodes/FillConditions/NodesValue/GroupsValue 不存在）

- [ ] **Step 3: 实现**（`cmd/cli/cmd/tui/file/file.go`，放在 `CaptureTargets` 之后；import 块追加 `"sort"`）

```go
// FillNodes 填显式节点列表并清空组/标签条件
func (m *FileModel) FillNodes(ids []string) {
	m.nodesInput.SetValue(strings.Join(ids, ","))
	m.groupsInput.SetValue("")
	m.labelsInput.SetValue("")
}

// FillConditions 填分组/标签条件并清空显式节点列表(组/标签为空即清空)
func (m *FileModel) FillConditions(groups []string, labels map[string]string) {
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

func (m FileModel) NodesValue() string { return m.nodesInput.Value() }

func (m FileModel) GroupsValue() string { return m.groupsInput.Value() }
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./cmd/cli/cmd/tui/... -count=1`
Expected: 全部 PASS

- [ ] **Step 5: 提交**

```bash
git add cmd/cli/cmd/tui/file/
git commit -m "feat(tui): File 面板 FillNodes/FillConditions 填充方法(与 Exec 对称)"
```

---

### Task 2: App 层 `f` 键升级 EntryBySelection + 共享 helper + 测试

**Files:**
- Modify: `cmd/cli/cmd/tui/app.go`（selectionFill helper、applySelectionEntry 重构、applyFileSelectionEntry、switchPanel、case "f"、helpView）
- Modify: `cmd/cli/cmd/tui/app_test.go`（追加 4 个测试 + 可能调整）
- Modify: `cmd/cli/cmd/tui/nodes/view.go`（状态栏 `f文件` → `f文件(带入)` 文案，可选但建议）

**Interfaces:**
- Consumes: Task 1 的 `file.FillNodes`/`file.FillConditions`/`file.NodesValue`/`file.GroupsValue`、既有 `nodes.MarkedIDs`/`nodes.Filter`/`nodes.AtList`、`Entry` 机制
- Produces: `App.selectionFill(fillNodes func([]string), fillConditions func([]string, map[string]string))`

- [ ] **Step 1: 写失败测试**（追加到 `cmd/cli/cmd/tui/app_test.go`，镜像既有 `TestApp_X*` 系列）

```go
func TestApp_FWithMarksFillsNodes(t *testing.T) {
	m := newApp(t)
	nm, _ := m.Update(runeKey(' ')) // 勾选 n1
	m = nm.(*App)
	nm, _ = m.Update(key(tea.KeyDown))
	m = nm.(*App)
	nm, _ = m.Update(runeKey(' ')) // 勾选 n2
	m = nm.(*App)
	nm, _ = m.Update(runeKey('f'))
	m = nm.(*App)
	if m.panel != 2 {
		t.Fatalf("expected panel 2, got %d", m.panel)
	}
	if got := m.file.NodesValue(); got != "n1,n2" {
		t.Fatalf("expected nodes n1,n2, got %q", got)
	}
}

func TestApp_FNoMarksFillsFilterConditions(t *testing.T) {
	m := newApp(t)
	nm, _ := m.Update(runeKey('/'))
	nm, _ = nm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g:web")})
	nm, _ = nm.Update(key(tea.KeyEnter))
	m = nm.(*App)
	nm, _ = m.Update(runeKey('f'))
	m = nm.(*App)
	if m.panel != 2 {
		t.Fatalf("expected panel 2, got %d", m.panel)
	}
	if got := m.file.GroupsValue(); got != "web" {
		t.Fatalf("expected groups web, got %q", got)
	}
	if got := m.file.NodesValue(); got != "" {
		t.Fatalf("expected nodes cleared, got %q", got)
	}
}

func TestApp_FSearchFilterFallsBackToSnapshot(t *testing.T) {
	m := newApp(t)
	nm, _ := m.Update(runeKey('/'))
	nm, _ = nm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("foo")})
	nm, _ = nm.Update(key(tea.KeyEnter))
	m = nm.(*App)
	nm, _ = m.Update(runeKey('f'))
	m = nm.(*App)
	if m.panel != 2 {
		t.Fatalf("expected panel 2, got %d", m.panel)
	}
	if got := m.file.GroupsValue(); got != "" {
		t.Fatalf("search filter must not fill groups, got %q", got)
	}
	if got := m.file.NodesValue(); got != "" {
		t.Fatalf("expected nodes cleared, got %q", got)
	}
}

func TestApp_FTabNeutralKeepsFormState(t *testing.T) {
	m := newApp(t)
	// 进 File 手填节点
	nm, _ := m.Update(runeKey('3'))
	m = nm.(*App)
	m.file.FillNodes([]string{"n1"})
	// Tab 绕一圈(4 面板)再回 File: 字段保留
	for i := 0; i < 4; i++ {
		nm, _ = m.Update(key(tea.KeyTab))
		m = nm.(*App)
	}
	if m.panel != 2 {
		t.Fatalf("expected panel 2 after 4 tabs, got %d", m.panel)
	}
	if got := m.file.NodesValue(); got != "n1" {
		t.Fatalf("tab must be neutral, expected nodes n1, got %q", got)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./cmd/cli/cmd/tui/ -run TestApp_F`
Expected: 前 3 个失败（f 仍 EntryNeutral，字段未填）、第 4 个通过（中性已成立）

- [ ] **Step 3: app.go 改造**

a) `applySelectionEntry` 重构为共享 helper + 两个入口函数：

```go
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

func (m *App) applySelectionEntry() {
	m.selectionFill(m.exec.FillNodes, m.exec.FillConditions)
}

// applyFileSelectionEntry 按 f 语义填充 File 表单: 与 x 同款(勾选优先/纯过滤/快照兜底)
func (m *App) applyFileSelectionEntry() {
	m.selectionFill(m.file.FillNodes, m.file.FillConditions)
}
```

b) `switchPanel` 的 panel==2 分支追加：

```go
	if m.panel == 2 {
		m.file.CaptureTargets(m.nodes.Visible())
		if entry == EntryBySelection {
			m.applyFileSelectionEntry()
		}
	}
```

c) `case "f":` 改为（保留 AtList 限定）：

```go
		case "f":
			if m.panel == 0 && m.nodes.AtList() {
				m.switchPanel(2, EntryBySelection)
				return m, nil
			}
```

d) helpView 文件行更新（`cmd/cli/cmd/tui/app.go`）：

```go
		"  文件:  ↑↓ 移动字段  Enter 编辑  ←→ 操作(upload/download)",
		"        a 高级选项  r 执行  Esc 返回 Nodes  f 勾选/筛选带入",
```

e) nodes 状态栏文案（`cmd/cli/cmd/tui/nodes/view.go` 状态栏串）：`f文件` → `f文件(带入)`

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./cmd/cli/cmd/tui/... -count=1`
Expected: 全部 PASS（含既有 TestApp_FJumpsToFileFromNodes / FIgnoredInExecPanel / FNotInterceptedInsideImportExportDialog / 全部 x 系列）

- [ ] **Step 5: 提交**

```bash
git add cmd/cli/cmd/tui/app.go cmd/cli/cmd/tui/app_test.go cmd/cli/cmd/tui/nodes/view.go
git commit -m "feat(tui): f 键升级带选择跳转 File(勾选优先/纯过滤填充, 与 x 对称)"
```

---

### Task 3: E2E 冒烟

- [ ] **Step 1: 全量测试 + 构建**

```bash
go build -o build/owl.exe ./cmd/cli
go test ./cmd/cli/cmd/tui/... -count=1
```

- [ ] **Step 2: 手动冒烟清单**（交互终端）

1. `owl tui` → nodes 面板 `Space` 勾选 2 个节点 → `f` → File 面板节点字段已填 "n1,n2"，目标行 2 台
2. Esc 回 nodes → `/` 过滤 `g:web` → `f` → 分组字段填 web、节点字段空、目标=web 组数
3. `/` 过滤 `s:online` → `f` → 分组/节点字段空、目标=可见集数
4. Tab 绕圈回 File → 手填的字段不被覆盖
5. nodes 按 `i` 进导入导出对话框 → `f` 仍切换格式（不跳 File）
6. 表单字段填好后 `r` 上传/下载正常

验证通过后：

```bash
git add -A && git commit -m "docs(tui): f 键带选择跳转 File E2E 冒烟验证"
```

---

## Known Limitations（后续工作，不在本计划内）

- **光标选中节点**：无勾选且无过滤时 f 不清空也不带入光标节点（与 x 一致：清空条件走快照）。如需"光标选中单节点带入"，另立需求
- **Transfer 扩散模式**：带入的字段同样作用于 transfer op（复用同一表单），语义一致

## Self-Review

**Spec 覆盖检查：**
- f 与 x 对称：Task 2 共享 selectionFill helper，两入口仅目标面板不同 ✓
- 勾选优先：MarkedIDs → FillNodes ✓
- 纯 g:/l: 过滤 → FillConditions(groups, labels) ✓（与 exec 完全一致）
- 搜索/状态 → FillConditions(nil, nil) + 快照兜底（CaptureTargets 已在 switchPanel）✓
- Tab/数字键中性：switchPanel 默认 EntryNeutral，仅 f/x 传 EntryBySelection ✓
- AtList 限定保留：i 对话框 f 格式切换不被劫持 ✓
- 帮助/状态栏文案更新 ✓

**类型一致性检查：**
- `selectionFill` 闭包签名 `func([]string)` / `func([]string, map[string]string)` 与 exec/file 的 Fill 方法签名完全一致 ✓
- `FillNodes`/`FillConditions` 在 Task 1 定义（file 包），Task 2 消费 ✓
- `NodesValue`/`GroupsValue` 导出供 app_test 断言（镜像 exec）✓
- 既有 x 系列测试不回归：applySelectionEntry 重构为闭包调用，行为等价 ✓
