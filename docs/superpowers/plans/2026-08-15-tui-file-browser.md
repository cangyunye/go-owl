# TUI File 本地文件浏览器实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** File 面板「本地文件」字段（upload/transfer 操作）按 Enter 时弹出本地文件浏览器，起点目录为 owl tui 启动时的工作目录（`os.Getwd()`），支持列目录/导航/路径输入跳转/选中回填；download 操作（远程路径）不弹浏览器。本期只做文件选择，不做目录选择器。

**Architecture:** 新增 `LocBrowser` 位置。浏览器组件 `FileBrowser`（`browser.go`：目录列表+排序+导航+路径跳转，纯数据/导航，不依赖 FileModel）；按键处理与视图挂在 FileModel 上（`updateBrowser`/`browserView`，与 updateFile/updateAdvanced 同构）。起点目录每次打开时 `os.Getwd()` 捕获（进程 cwd 恒定，等于启动目录）。选中文件回填 `fileInput` 并返回字段 Insert 编辑态；Esc 同样返回字段编辑态。

**Tech Stack:** Go (os.ReadDir / filepath), charmbracelet/bubbletea + bubbles/textinput + lipgloss（已有）

## Global Constraints

- 项目目录：`F:\pantheon\trae_projects\git\go-owl`；禁止越出项目根目录
- TDD：每个任务先写失败测试再实现；测试命令 `go test ./cmd/cli/cmd/tui/...`
- 每个任务结束提交一个 atomic commit；全部完成后跑 E2E 冒烟再提交
- 面板文案沿用项目风格（中文硬编码，不引入 i18n）
- File 面板内部键位自由（面板级 keymap），不得影响 App 全局键（tab/1/2/3/f/x/q/?/ctrl+c）
- 浏览器内按键沿用 nodes filter 交互范式（`/` 打开输入、Esc 先退输入再退视图）
- 不引入第三方文件选择库；纯 os/filepath 实现
- 只改 `cmd/cli/cmd/tui/file/` 与 `cmd/cli/cmd/tui/` 测试

---

### Task 1: FileBrowser 组件（列目录/排序/导航/路径跳转/隐藏切换）

**Files:**
- Create: `cmd/cli/cmd/tui/file/browser.go`
- Create: `cmd/cli/cmd/tui/file/browser_test.go`

**Interfaces:**
- Consumes: `os.ReadDir`、`filepath`（均已入标准库）
- Produces: `BrowserEntry{Name string; IsDir bool}`、`FileBrowser{dir/entries/cursor/input/inputOpen/showHidden/err}`、`NewFileBrowser(startDir string) *FileBrowser`、`(*FileBrowser).Reload() error`、`Enter() (string, bool, error)`（目录→推进，文件→返回绝对路径）、`Up/Down/Parent() bool`、`Jump(path string) error`（目录/文件均可）、`ToggleHidden()`、`currentPath() string`

- [ ] **Step 1: 写失败测试** `cmd/cli/cmd/tui/file/browser_test.go`

```go
package file

import (
	"os"
	"path/filepath"
	"testing"
)

func seedDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// 文件
	for _, n := range []string{"a.txt", "c.log", ".hidden"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// 目录
	for _, n := range []string{"b_dir", "d_sub"} {
		if err := os.Mkdir(filepath.Join(dir, n), 0755); err != nil {
			t.Fatal(err)
		}
	}
	// 隐藏目录
	if err := os.Mkdir(filepath.Join(dir, ".secret"), 0755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestBrowser_NewListsSortedDirsFirst(t *testing.T) {
	b := NewFileBrowser(seedDir(t))
	if b.err != "" {
		t.Fatalf("unexpected err: %s", b.err)
	}
	want := []string{"b_dir", "d_sub", "a.txt", "c.log"}
	got := make([]string, len(b.entries))
	for i, e := range b.entries {
		got[i] = e.Name
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("entry %d: want %s got %s (all: %v)", i, want[i], got[i], got)
		}
	}
	if !b.entries[0].IsDir {
		t.Fatal("expected first entry to be a dir")
	}
}

func TestBrowser_HiddenFilteredAndToggle(t *testing.T) {
	b := NewFileBrowser(seedDir(t))
	for _, e := range b.entries {
		if e.Name == ".hidden" || e.Name == ".secret" {
			t.Fatalf("hidden entries must be filtered: %s", e.Name)
		}
	}
	b.ToggleHidden()
	found := false
	for _, e := range b.entries {
		if e.Name == ".hidden" {
			found = true
		}
	}
	if !found {
		t.Fatal("hidden entries must appear after toggle")
	}
}

func TestBrowser_EnterDirMovesCursorAndReloads(t *testing.T) {
	root := seedDir(t)
	b := NewFileBrowser(root)
	// cursor 0 = b_dir
	path, isFile, err := b.Enter()
	if err != nil {
		t.Fatal(err)
	}
	if isFile {
		t.Fatal("expected dir")
	}
	if path != filepath.Join(root, "b_dir") {
		t.Fatalf("expected %s, got %s", filepath.Join(root, "b_dir"), path)
	}
	if b.dir != filepath.Join(root, "b_dir") {
		t.Fatalf("expected dir switch, got %s", b.dir)
	}
}

func TestBrowser_ParentReturns(t *testing.T) {
	root := seedDir(t)
	b := NewFileBrowser(root)
	b.Jump(filepath.Join(root, "b_dir"))
	if !b.Parent() {
		t.Fatal("expected parent success")
	}
	if b.dir != root {
		t.Fatalf("expected back to %s, got %s", root, b.dir)
	}
	if b.Parent() {
		t.Fatal("expected parent failure at root")
	}
}

func TestBrowser_EnterFileReturnsAbsPath(t *testing.T) {
	root := seedDir(t)
	b := NewFileBrowser(root)
	// 移到 a.txt (index 2)
	b.cursor = 2
	path, isFile, err := b.Enter()
	if err != nil {
		t.Fatal(err)
	}
	if !isFile {
		t.Fatal("expected file")
	}
	if path != filepath.Join(root, "a.txt") {
		t.Fatalf("expected %s, got %s", filepath.Join(root, "a.txt"), path)
	}
}

func TestBrowser_JumpToDirAndFile(t *testing.T) {
	root := seedDir(t)
	b := NewFileBrowser(root)
	sub := filepath.Join(root, "b_dir")
	if err := b.Jump(sub); err != nil {
		t.Fatal(err)
	}
	if b.dir != sub {
		t.Fatalf("expected dir %s, got %s", sub, b.dir)
	}
	f := filepath.Join(root, "a.txt")
	if err := b.Jump(f); err != nil {
		t.Fatal(err)
	}
	if got := b.currentPath(); got != f {
		t.Fatalf("expected file path %s, got %s", f, got)
	}
}

func TestBrowser_JumpInvalidKeepsDir(t *testing.T) {
	root := seedDir(t)
	b := NewFileBrowser(root)
	if err := b.Jump(filepath.Join(root, "no-such")); err == nil {
		t.Fatal("expected error for missing path")
	}
	if b.dir != root {
		t.Fatalf("dir must stay %s, got %s", root, b.dir)
	}
}

func TestBrowser_CursorClamped(t *testing.T) {
	b := NewFileBrowser(seedDir(t))
	for i := 0; i < 10; i++ {
		b.Down()
	}
	if b.cursor >= len(b.entries) {
		t.Fatalf("cursor out of range: %d >= %d", b.cursor, len(b.entries))
	}
	for i := 0; i < 10; i++ {
		b.Up()
	}
	if b.cursor < 0 {
		t.Fatalf("cursor negative: %d", b.cursor)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./cmd/cli/cmd/tui/file/ -run TestBrowser`
Expected: 编译失败（FileBrowser 不存在）

- [ ] **Step 3: 创建 browser.go** `cmd/cli/cmd/tui/file/browser.go`

```go
package file

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type BrowserEntry struct {
	Name  string
	IsDir bool
}

// FileBrowser 本地文件浏览器: 目录列表/排序/导航/路径跳转。
// 纯数据与导航, 按键处理与视图由 FileModel 负责。
type FileBrowser struct {
	dir        string
	entries    []BrowserEntry
	cursor     int
	showHidden bool
	err        string
}

// NewFileBrowser 以 startDir 为起点创建浏览器; startDir 为空时回退当前工作目录
func NewFileBrowser(startDir string) *FileBrowser {
	if startDir == "" {
		if wd, err := os.Getwd(); err == nil {
			startDir = wd
		} else {
			startDir = "."
		}
	}
	b := &FileBrowser{dir: startDir}
	b.reload()
	return b
}

func (b *FileBrowser) reload() {
	entries, err := os.ReadDir(b.dir)
	if err != nil {
		b.err = err.Error()
		b.entries = nil
		b.cursor = 0
		return
	}
	b.err = ""
	b.entries = b.entries[:0]
	for _, e := range entries {
		if !b.showHidden && strings.HasPrefix(e.Name(), ".") {
			continue
		}
		b.entries = append(b.entries, BrowserEntry{Name: e.Name(), IsDir: e.IsDir()})
	}
	// 目录在前, 各自按字母序
	sort.Slice(b.entries, func(i, j int) bool {
		if b.entries[i].IsDir != b.entries[j].IsDir {
			return b.entries[i].IsDir
		}
		return b.entries[i].Name < b.entries[j].Name
	})
	b.clamp()
}

func (b *FileBrowser) clamp() {
	if b.cursor >= len(b.entries) {
		b.cursor = len(b.entries) - 1
	}
	if b.cursor < 0 {
		b.cursor = 0
	}
}

func (b *FileBrowser) Up() { b.cursor--; b.clamp() }

func (b *FileBrowser) Down() { b.cursor++; b.clamp() }

// Enter 光标项: 目录→进入并返回 (path,false,nil); 文件→返回绝对路径 (path,true,nil)
func (b *FileBrowser) Enter() (string, bool, error) {
	if len(b.entries) == 0 {
		return "", false, nil
	}
	e := b.entries[b.cursor]
	path := filepath.Join(b.dir, e.Name)
	if e.IsDir {
		b.dir = path
		b.cursor = 0
		b.reload()
		return path, false, nil
	}
	return path, true, nil
}

// Parent 返回上级目录; 已在根目录时返回 false
func (b *FileBrowser) Parent() bool {
	parent := filepath.Dir(b.dir)
	if parent == b.dir {
		return false
	}
	b.dir = parent
	b.cursor = 0
	b.reload()
	return true
}

// Jump 跳转到指定路径: 目录→进入; 文件→作为当前选中项; 无效路径返回 error 且保持原目录
func (b *FileBrowser) Jump(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		b.dir = path
		b.cursor = 0
		b.reload()
		return nil
	}
	// 文件: 进入其所在目录并把光标指向该文件
	dir := filepath.Dir(path)
	name := filepath.Base(path)
	b.dir = dir
	b.reload()
	for i, e := range b.entries {
		if e.Name == name {
			b.cursor = i
			break
		}
	}
	return nil
}

func (b *FileBrowser) ToggleHidden() {
	b.showHidden = !b.showHidden
	b.reload()
}

func (b *FileBrowser) currentPath() string {
	if len(b.entries) == 0 {
		return b.dir
	}
	return filepath.Join(b.dir, b.entries[b.cursor].Name)
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./cmd/cli/cmd/tui/file/ -run TestBrowser`
Expected: 全部 PASS

- [ ] **Step 5: 提交**

```bash
git add cmd/cli/cmd/tui/file/browser.go cmd/cli/cmd/tui/file/browser_test.go
git commit -m "feat(tui): 本地文件浏览器组件(列目录/排序/导航/路径跳转/隐藏切换)"
```

---

### Task 2: File 面板集成（Enter 弹出浏览器 + 回填 + Esc 返回）

**Files:**
- Modify: `cmd/cli/cmd/tui/file/file.go`（LocBrowser、browser 字段、updateFile enter 分支、updateBrowser、Update/Path 分发）
- Modify: `cmd/cli/cmd/tui/file/view.go`（browserView）
- Modify: `cmd/cli/cmd/tui/file/file_test.go`（追加集成测试）

**Interfaces:**
- Consumes: Task 1 的 `FileBrowser`/`NewFileBrowser`
- Produces: `LocBrowser` 常量、`FileModel.browser *FileBrowser`、`FileModel.updateBrowser(msg)`、`browserView()`

- [ ] **Step 1: 写失败测试**（追加到 `cmd/cli/cmd/tui/file/file_test.go`）

```go
func TestEnterOnLocalFileFieldOpensBrowser(t *testing.T) {
	m := newTestModel(t)
	m.fileInput.Reset()
	// 本地文件字段是 cursor 0, 直接 Enter
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(FileModel)
	if m.current() != LocBrowser {
		t.Fatalf("expected LocBrowser, got %v", m.current())
	}
	if m.browser == nil || m.browser.dir == "" {
		t.Fatal("expected browser opened")
	}
	got := m.View()
	for _, want := range []string{"文件浏览器", "本地文件"} {
		if !contains(got, want) {
			t.Fatalf("view missing %q:\n%s", want, got)
		}
	}
}

func TestBrowserSelectFileFillsInputAndEdits(t *testing.T) {
	m := newTestModel(t)
	m.fileInput.Reset()
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(FileModel)
	// 写入一个真实文件供选中
	dir := m.browser.dir
	f := filepath.Join(dir, "pick-me.txt")
	if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	// 重新加载使文件可见
	if err := m.browser.Jump(dir); err != nil {
		t.Fatal(err)
	}
	// 光标移到该文件(目录在前, 文件按字母序, pick-me.txt 应在最后)
	for i := 0; i < len(m.browser.entries); i++ {
		if m.browser.entries[i].Name == "pick-me.txt" {
			m.browser.cursor = i
			break
		}
	}
	nm, _ = m.Update(key(tea.KeyEnter))
	m = nm.(FileModel)
	if m.current() != LocFile {
		t.Fatalf("expected back to LocFile, got %v", m.current())
	}
	if m.fileInput.Value() != f {
		t.Fatalf("expected input %q, got %q", f, m.fileInput.Value())
	}
	if m.mode != ModeInsert || !m.fileInput.Focused() {
		t.Fatal("expected field editing mode after pick")
	}
}

func TestBrowserEscReturnsToFieldEdit(t *testing.T) {
	m := newTestModel(t)
	m.fileInput.Reset()
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(FileModel)
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(FileModel)
	if m.current() != LocFile {
		t.Fatalf("expected LocFile, got %v", m.current())
	}
	if m.mode != ModeInsert || !m.fileInput.Focused() {
		t.Fatal("expected field editing mode after esc")
	}
}

func TestBrowserEscClosesPathInputFirst(t *testing.T) {
	m := newTestModel(t)
	m.fileInput.Reset()
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(FileModel)
	nm, _ = m.Update(runeKey('/')) // 打开路径输入
	m = nm.(FileModel)
	nm, _ = m.Update(key(tea.KeyEsc)) // 第一次 Esc: 关输入
	m = nm.(FileModel)
	if m.current() != LocBrowser {
		t.Fatal("expected still in browser after first esc")
	}
	nm, _ = m.Update(key(tea.KeyEsc)) // 第二次 Esc: 退出浏览器
	m = nm.(FileModel)
	if m.current() != LocFile {
		t.Fatalf("expected LocFile after second esc, got %v", m.current())
	}
}

func TestBrowserNavigateDirAndBack(t *testing.T) {
	m := newTestModel(t)
	m.fileInput.Reset()
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(FileModel)
	root := m.browser.dir
	// 首个条目应是目录
	if len(m.browser.entries) == 0 || !m.browser.entries[0].IsDir {
		t.Fatal("expected a dir at cursor 0")
	}
	nm, _ = m.Update(key(tea.KeyEnter)) // 进入
	m = nm.(FileModel)
	if m.browser.dir == root {
		t.Fatal("expected dir changed")
	}
	nm, _ = m.Update(key(tea.KeyBackspace)) // 返回上级
	m = nm.(FileModel)
	if m.browser.dir != root {
		t.Fatalf("expected back to %s, got %s", root, m.browser.dir)
	}
}

func TestEnterOnDownloadNoBrowser(t *testing.T) {
	m := newTestModel(t)
	m.op = OpDownload
	m.fileInput.Reset()
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(FileModel)
	if m.current() != LocFile || m.mode != ModeInsert {
		t.Fatalf("download must stay on field edit, got loc=%v mode=%v", m.current(), m.mode)
	}
}

func TestEnterOnOtherFieldNoBrowser(t *testing.T) {
	m := newTestModel(t)
	m.cursor = 1 // 节点字段
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(FileModel)
	if m.current() != LocFile || m.mode != ModeInsert {
		t.Fatalf("other fields must stay on field edit, got loc=%v mode=%v", m.current(), m.mode)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./cmd/cli/cmd/tui/file/ -run TestEnterOnLocalFileFieldOpensBrowser`
Expected: 编译失败（LocBrowser 不存在）

- [ ] **Step 3: file.go 集成**

a) `Loc` 常量追加（`LocResult` 之后）：

```go
	LocBrowser
```

b) `FileModel` 结构体追加字段（`advanced` 之后）：

```go
	browser *FileBrowser
```

c) `Path()` 追加分支（`case LocResult:` 之后）：

```go
	case LocBrowser:
		return []string{"file", sub, "browser"}
```

d) `Update` 分发追加（`case LocResult:` 之前）：

```go
	case LocBrowser:
		return m.updateBrowser(msg)
```

e) `updateFile` 的 `case "enter":` 改为：

```go
	case "enter":
		if m.cursor == 0 && m.op != OpDownload {
			// 本地文件字段: 弹出文件浏览器(起点为启动工作目录)
			m.browser = NewFileBrowser("")
			m.push(LocBrowser)
			return m, nil
		}
		m.mode = ModeInsert
		m.fieldAt(m.cursor).Focus()
```

f) 追加 `updateBrowser`（放在 `updateFile` 之后）：

```go
// updateBrowser 文件浏览器按键处理; 路径输入用 / 打开(同 nodes filter 范式)
func (m FileModel) updateBrowser(msg tea.Msg) (tea.Model, tea.Cmd) {
	b := m.browser
	if b == nil {
		return m, nil
	}
	if b.inputOpen {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "esc":
				b.inputOpen = false
				b.input.Blur()
				return m, nil
			case "enter":
				if err := b.Jump(b.input.Value()); err != nil {
					b.err = err.Error()
				} else {
					b.err = ""
					b.inputOpen = false
					b.input.Blur()
				}
				return m, nil
			}
		}
		var cmd tea.Cmd
		b.input, cmd = b.input.Update(msg)
		return m, cmd
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "up":
		b.Up()
	case "down":
		b.Down()
	case "enter":
		path, isFile, _ := b.Enter()
		if !isFile {
			return m, nil // 已进入子目录
		}
		m.fileInput.SetValue(path)
		m.pop()
		m.browser = nil
		m.mode = ModeInsert
		m.fileInput.Focus()
		return m, textinput.Blink
	case "backspace", "left":
		b.Parent()
	case "/":
		b.inputOpen = true
		b.input.Focus()
		m.mode = ModeInsert
		return m, textinput.Blink
	case "h":
		b.ToggleHidden()
	case "esc":
		m.pop()
		m.browser = nil
		m.mode = ModeInsert
		m.fileInput.Focus()
		return m, textinput.Blink
	}
	return m, nil
}
```

`FileBrowser` 增加 `input textinput.Model` 与 `inputOpen bool` 字段（browser.go）：

```go
type FileBrowser struct {
	dir        string
	entries    []BrowserEntry
	cursor     int
	showHidden bool
	err        string
	input      textinput.Model
	inputOpen  bool
}
```

`NewFileBrowser` 初始化输入框（browser.go）：

```go
	ti := textinput.New()
	ti.Placeholder = "输入路径跳转"
	ti.Width = 50
	ti.CharLimit = 512
	ti.Blur()
	b := &FileBrowser{dir: startDir, input: ti}
```

注意：浏览器视图在 Insert 模式下依赖 App 的 InsertMode 隔离，`updateBrowser` 内 `inputOpen` 时按键直接转交 input；浏览列表态保持 `m.mode == ModeNormal`（`/` 时才临时 Insert，Esc 后回到 Normal）。

- [ ] **Step 4: 追加 browserView**（`cmd/cli/cmd/tui/file/view.go`）

```go
func (m FileModel) browserView() string {
	b := m.browser
	if b == nil {
		return ""
	}
	var b2 strings.Builder
	b2.WriteString("┌─ 文件浏览器 ─────────────────────\n")
	b2.WriteString("  目录: " + styleSelected.Render(b.dir) + "\n")
	if b.inputOpen {
		b2.WriteString("  路径: " + b.input.View() + styleDim.Render("  Enter 跳转  Esc 取消") + "\n")
	} else {
		b2.WriteString("  路径: " + b.input.View() + styleDim.Render("  / 输入") + "\n")
	}
	for i, e := range b.entries {
		marker := " "
		if i == b.cursor {
			marker = ">"
		}
		name := e.Name
		if e.IsDir {
			name += "/"
		}
		line := fmt.Sprintf("  %s %s\n", marker, name)
		if i == b.cursor {
			line = styleSelected.Render(line)
		}
		b2.WriteString(line)
	}
	if len(b.entries) == 0 {
		b2.WriteString(styleDim.Render("  (空目录)") + "\n")
	}
	if b.err != "" {
		b2.WriteString(styleError.Render("  "+b.err) + "\n")
	}
	b2.WriteString(styleDim.Render("  ↑↓选择 Enter进入/选中 ←上级 /输入路径 h隐藏 Esc返回") + "\n")
	b2.WriteString("└─")
	return b2.String()
}
```

`View()` 分发追加（`case LocResult:` 之前），实际实现为：

```go
	switch m.current() {
	case LocAdvanced:
		return m.advancedView()
	case LocResult:
		return m.resultView()
	case LocBrowser:
		return m.browserView()
	default:
		return m.fileView()
	}
```

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./cmd/cli/cmd/tui/... -count=1`
Expected: 全部 PASS（含 Task 1 组件测试与既有 file/app 测试）

- [ ] **Step 6: 提交**

```bash
git add cmd/cli/cmd/tui/file/
git commit -m "feat(tui): 本地文件字段 Enter 弹出文件浏览器(起点启动目录, 选中回填/Esc 返回)"
```

---

### Task 3: E2E 冒烟

- [ ] **Step 1: 全量测试 + 构建**

```bash
go build -o build/owl.exe ./cmd/cli
go test ./cmd/cli/cmd/tui/... -count=1
```

- [ ] **Step 2: 手动冒烟清单**（交互终端）

1. `owl tui` → `3` 进入 File 面板
2. 光标在「本地文件」，按 `Enter` → 弹出文件浏览器，顶部显示启动目录（当前 cwd）
3. `↑↓` 选择目录 → `Enter` 进入，`←`/`Backspace` 返回上级
4. 按 `/` 输入绝对路径（如 `D:\xxx`）→ `Enter` 跳转，`Esc` 取消输入
5. 选中一个文件 `Enter` → 回到表单，「本地文件」字段已回填该路径并处于输入态
6. `Esc` 退出浏览器同样回到字段输入态；再 `Esc` 退出输入
7. `←→` 切到「文件下载」→ `Enter` 在「远程文件」字段**不**弹浏览器（正常编辑）
8. 填好路径后 `r` 执行上传，结果正常

验证通过后：

```bash
git add -A && git commit -m "docs(tui): 本地文件浏览器 E2E 冒烟验证"
```

---

## Known Limitations（后续工作，不在本计划内）

- **目录选择模式**：download 的「本地目录」字段仍手输；后续可在 FileBrowser 上加模式切换复用同一组件
- **历史/书签**：无最近目录记忆
- **大目录性能**：全量 ReadDir 渲染（数千条目时列表截断显示，不虚拟滚动）
- **非本地文件字段的浏览器**：dest 目标目录为远程路径（upload 场景），不适用本地浏览器

## Self-Review

**Spec 覆盖检查：**
- 起点为启动工作目录：Task 1 `NewFileBrowser("")` 内部 `os.Getwd()` 兜底 ✓
- 仅 upload/transfer 的本地文件字段弹浏览器：Task 2 enter 分支 `m.op != OpDownload` 条件 ✓
- 选中回填并进入字段编辑态：Task 2 `SetValue + pop + ModeInsert + Focus + textinput.Blink` ✓
- Esc 返回字段编辑态：Task 2 esc 分支同款 ✓
- 路径输入框保留（用户确认）：`/` 打开、Enter 跳转（目录/文件均可）、Esc 先关输入再退浏览器 ✓
- 目录选择器本期不做（用户确认）✓
- 键位不与 App 全局键冲突：浏览器内 /、h、←、Backspace、Esc、Enter、↑↓ 均非 App 拦截键 ✓

**类型一致性检查：**
- `FileBrowser.Enter() (string, bool, error)` 在 Task 1 定义，Task 2 按 (path, isFile, err) 消费 ✓
- `LocBrowser` 常量追加在 LocResult 之后，不影响既有常量索引（无 iota 依赖的测试）✓
- `input`/`inputOpen` 字段在 Task 1 组件与 Task 2 按键处理中保持一致 ✓
- 浏览器视图仅存在于 File 面板内（无 listPane 概念），View 分发直接返回 browserView ✓
