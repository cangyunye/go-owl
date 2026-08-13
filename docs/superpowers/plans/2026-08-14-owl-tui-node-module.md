# owl tui 重构 Phase 1(Node 管理模块)Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 go-owl 主仓库内实现原生 `owl tui` 全屏应用,Phase 1 提供 Node 管理模块(列表双栏 / 增删改表单 / 删除确认 / 列选择器 / 行过滤),彻底替换转发外部 go-owl-tui 二进制的旧壳子,并用 vim 式模式隔离解决"输入时全局快捷键冲突"的痛点。

**Architecture:** 路径栈(Path Stack)决定层级与激活的 keymap 组,顶栏面包屑显示当前路径;Mode(Normal/Insert)在按键路由入口最先判定,Insert 下所有按键只进当前输入框。bubbletea Elm 架构,module 以值语义 Update/View,子结构(表单/确认/列选择器)为指针子 model。

**Tech Stack:** Go 1.26.0、charmbracelet/bubbletea v1.3.4、bubbles v0.21.0(textinput)、lipgloss v1.1.0、golang.org/x/term(已存在)。

## Global Constraints

- 模块 `github.com/cangyunye/go-owl`,Go 1.26.0;测试命令 `go test ./cmd/cli/cmd/...`
- 依赖版本必须与 go-owl-tui 一致:bubbletea `v1.3.4`、bubbles `v0.21.0`、lipgloss `v1.1.0`
- TUI 内部文案用简体中文(与现有 `tui.cmd.long` i18n 文案一致);不新增 i18n key
- Model 内**禁止 os.Exit**;错误一律渲染到视图状态栏,仅 App 通过 `tea.Quit` 退出
- 数据层只经 `common.NodeStore` 接口注入;测试用新增的 `common.NewInMemoryNodeStoreAt(tempPath)` 写临时文件,不得触碰 `~/.owl/nodes.json`
- 位置栈深度上限 3(list → form/confirm/columns),`Esc` 弹栈
- `owl tui` 保持无子命令、无 flag(现有 `tui_test.go` 断言不变)

---

### Task 1: 引入 TUI 依赖 + 可测试的 NodeStore 构造器

**Files:**
- Modify: `go.mod`
- Modify: `cmd/cli/cmd/common/node.go`(追加构造器)
- Create: `cmd/cli/cmd/common/node_store_file_test.go`
- Test: `cmd/cli/cmd/common/node_store_file_test.go`

**Interfaces:**
- Produces: `common.NewInMemoryNodeStoreAt(dataFile string) *InMemoryNodeStore` —— 后续所有 TUI 测试用它建临时文件 store

- [ ] **Step 1: 拉取依赖**

```bash
go get github.com/charmbracelet/bubbletea@v1.3.4
go get github.com/charmbracelet/bubbles@v0.21.0
go get github.com/charmbracelet/lipgloss@v1.1.0
go mod tidy
```

Expected: `go.mod` 新增三个 require + 各自 indirect 依赖;`go build ./...` 通过。

- [ ] **Step 2: 写失败测试**

创建 `cmd/cli/cmd/common/node_store_file_test.go`(`package common`,与现有 common 测试一致):

```go
package common

import (
	"path/filepath"
	"testing"
)

func TestInMemoryNodeStoreAt_RoundTrip(t *testing.T) {
	store := NewInMemoryNodeStoreAt(filepath.Join(t.TempDir(), "nodes.json"))
	if err := store.Add(&NodeInfo{ID: "n1", Name: "web", Address: "1.2.3.4", Port: 22, Status: "offline"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded := NewInMemoryNodeStoreAt(store.dataFile)
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}
	nodes, _ := reloaded.List()
	if len(nodes) != 1 || nodes[0].ID != "n1" {
		t.Fatalf("unexpected nodes: %+v", nodes)
	}
}
```

- [ ] **Step 3: 运行确认失败**

Run: `go test ./cmd/cli/cmd/common/ -run TestInMemoryNodeStoreAt_RoundTrip -v`
Expected: FAIL,`undefined: NewInMemoryNodeStoreAt`

- [ ] **Step 4: 实现构造器**

在 `cmd/cli/cmd/common/node.go` 的 `NewInMemoryNodeStore` 之后追加:

```go
// NewInMemoryNodeStoreAt 创建内存节点存储(数据文件路径自定义,测试用)
func NewInMemoryNodeStoreAt(dataFile string) *InMemoryNodeStore {
	store := &InMemoryNodeStore{
		nodes:    make(map[string]*NodeInfo),
		dataFile: dataFile,
	}
	return store
}
```

- [ ] **Step 5: 运行确认通过**

Run: `go test ./cmd/cli/cmd/common/ -run TestInMemoryNodeStoreAt_RoundTrip -v`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add go.mod go.sum cmd/cli/cmd/common/node.go cmd/cli/cmd/common/node_store_file_test.go
git commit -m "chore(tui): add bubbletea/bubbles/lipgloss deps + NewInMemoryNodeStoreAt test helper"
```

---

### Task 2: 行过滤查询解析与过滤逻辑(`filter.go`)

**Files:**
- Create: `cmd/cli/cmd/tui/nodes/filter.go`
- Create: `cmd/cli/cmd/tui/nodes/filter_test.go`
- Test: `cmd/cli/cmd/tui/nodes/filter_test.go`

**Interfaces:**
- Produces(供 Task 3+ 使用):
  - `type FilterQuery struct { Groups []string; Labels map[string]string; Search string }`
  - `func ParseFilterQuery(q string) FilterQuery`
  - `func (fq FilterQuery) Empty() bool`
  - `func applyFilter(nodes []*common.NodeInfo, fq FilterQuery) []*common.NodeInfo`
  - `func groupsIntersect(a, b []string) bool`

- [ ] **Step 1: 写失败测试**

创建 `cmd/cli/cmd/tui/nodes/filter_test.go`(`package nodes`):

```go
package nodes

import (
	"testing"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

func TestParseFilterQuery_Groups(t *testing.T) {
	fq := ParseFilterQuery("g:web,db")
	if len(fq.Groups) != 2 || fq.Groups[0] != "web" || fq.Groups[1] != "db" {
		t.Fatalf("unexpected groups: %#v", fq.Groups)
	}
	if fq.Empty() {
		t.Fatal("expected not empty")
	}
}

func TestParseFilterQuery_Labels(t *testing.T) {
	fq := ParseFilterQuery("l:env=prod,os=debian")
	if len(fq.Labels) != 2 || fq.Labels["env"] != "prod" || fq.Labels["os"] != "debian" {
		t.Fatalf("unexpected labels: %#v", fq.Labels)
	}
}

func TestParseFilterQuery_Search(t *testing.T) {
	fq := ParseFilterQuery("web-1")
	if fq.Search != "web-1" {
		t.Fatalf("unexpected search: %q", fq.Search)
	}
}

func TestParseFilterQuery_Mixed(t *testing.T) {
	fq := ParseFilterQuery("g:web l:env=prod cache")
	if len(fq.Groups) != 1 || fq.Groups[0] != "web" {
		t.Fatalf("groups: %#v", fq.Groups)
	}
	if fq.Labels["env"] != "prod" {
		t.Fatalf("labels: %#v", fq.Labels)
	}
	if fq.Search != "cache" {
		t.Fatalf("search: %q", fq.Search)
	}
}

func TestParseFilterQuery_Empty(t *testing.T) {
	if !ParseFilterQuery("").Empty() {
		t.Fatal("empty query should be Empty")
	}
}

func TestApplyFilter_Groups(t *testing.T) {
	nodes := []*common.NodeInfo{
		{ID: "n1", Groups: []string{"web"}},
		{ID: "n2", Groups: []string{"db"}},
		{ID: "n3", Groups: []string{"cache", "web"}},
	}
	fq := ParseFilterQuery("g:web")
	got := applyFilter(nodes, fq)
	if len(got) != 2 || got[0].ID != "n1" || got[1].ID != "n3" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestApplyFilter_Labels(t *testing.T) {
	nodes := []*common.NodeInfo{
		{ID: "n1", Labels: map[string]string{"env": "prod"}},
		{ID: "n2", Labels: map[string]string{"env": "dev"}},
		{ID: "n3", Labels: nil},
	}
	fq := ParseFilterQuery("l:env=prod")
	got := applyFilter(nodes, fq)
	if len(got) != 1 || got[0].ID != "n1" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestApplyFilter_Search(t *testing.T) {
	nodes := []*common.NodeInfo{
		{ID: "web-1", Name: "prod-web", Address: "10.0.0.1"},
		{ID: "db-1", Name: "prod-db", Address: "10.0.0.2"},
	}
	got := applyFilter(nodes, ParseFilterQuery("10.0.0.1"))
	if len(got) != 1 || got[0].ID != "web-1" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestApplyFilter_Empty_ReturnsAll(t *testing.T) {
	nodes := []*common.NodeInfo{{ID: "n1"}, {ID: "n2"}}
	if got := applyFilter(nodes, FilterQuery{}); len(got) != 2 {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestApplyFilter_Empty_ReturnsNothing(t *testing.T) {
	var nodes []*common.NodeInfo
	if got := applyFilter(nodes, ParseFilterQuery("g:web")); len(got) != 0 {
		t.Fatalf("expected empty, got %+v", got)
	}
}

func TestGroupsIntersect(t *testing.T) {
	if !groupsIntersect([]string{"a", "b"}, []string{"b", "c"}) {
		t.Fatal("expected true")
	}
	if groupsIntersect([]string{"a"}, []string{"b"}) {
		t.Fatal("expected false")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -v`
Expected: FAIL,`undefined: ParseFilterQuery` / `applyFilter` / `groupsIntersect`

- [ ] **Step 3: 实现 filter.go**

创建 `cmd/cli/cmd/tui/nodes/filter.go`:

```go
package nodes

import (
	"strings"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

type FilterQuery struct {
	Groups []string
	Labels map[string]string
	Search string
}

func ParseFilterQuery(q string) FilterQuery {
	fq := FilterQuery{Labels: map[string]string{}}
	var search []string
	for _, tok := range strings.Fields(q) {
		switch {
		case strings.HasPrefix(tok, "g:"):
			for _, g := range strings.Split(tok[2:], ",") {
				g = strings.TrimSpace(g)
				if g != "" {
					fq.Groups = append(fq.Groups, g)
				}
			}
		case strings.HasPrefix(tok, "l:"):
			for _, pair := range strings.Split(tok[2:], ",") {
				parts := strings.SplitN(pair, "=", 2)
				if len(parts) == 2 {
					k := strings.TrimSpace(parts[0])
					v := strings.TrimSpace(parts[1])
					if k != "" {
						fq.Labels[k] = v
					}
				}
			}
		default:
			if s := strings.TrimSpace(tok); s != "" {
				search = append(search, s)
			}
		}
	}
	fq.Search = strings.Join(search, " ")
	return fq
}

func (fq FilterQuery) Empty() bool {
	return len(fq.Groups) == 0 && len(fq.Labels) == 0 && fq.Search == ""
}

func matchFilter(n *common.NodeInfo, fq FilterQuery) bool {
	if fq.Empty() {
		return true
	}
	if len(fq.Groups) > 0 && !groupsIntersect(n.Groups, fq.Groups) {
		return false
	}
	for k, v := range fq.Labels {
		if n.Labels == nil || n.Labels[k] != v {
			return false
		}
	}
	if fq.Search != "" {
		hay := strings.ToLower(n.ID + " " + n.Name + " " + n.Address)
		if !strings.Contains(hay, strings.ToLower(fq.Search)) {
			return false
		}
	}
	return true
}

func applyFilter(nodes []*common.NodeInfo, fq FilterQuery) []*common.NodeInfo {
	out := make([]*common.NodeInfo, 0, len(nodes))
	for _, n := range nodes {
		if matchFilter(n, fq) {
			out = append(out, n)
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
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -v`
Expected: 全部 PASS

- [ ] **Step 5: 提交**

```bash
git add cmd/cli/cmd/tui/nodes/filter.go cmd/cli/cmd/tui/nodes/filter_test.go
git commit -m "feat(tui): node 行过滤查询解析(g:/l:/搜索, AND 组合)"
```

---

### Task 3: 列表数据层 + 列表导航(Model 骨架)

**Files:**
- Create: `cmd/cli/cmd/tui/nodes/model.go`(Mode/Location/pane 定义 + Model + list/filter 更新)
- Create: `cmd/cli/cmd/tui/nodes/list.go`(列定义、cell 取值、列宽、截断)
- Create: `cmd/cli/cmd/tui/nodes/model_test.go`
- Test: `cmd/cli/cmd/tui/nodes/model_test.go`

**Interfaces:**
- Consumes: `common.NodeStore`、`FilterQuery`、`applyFilter`(Task 2)
- Produces(供后续任务使用):
  - `type Mode int` 常量 `ModeNormal`/`ModeInsert`;`type Location int` 常量 `LocList/LocNew/LocEdit/LocDelete/LocColumns`
  - `func NewModel(store common.NodeStore) Model`
  - `func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd)`、`func (m Model) View() string`、`func (m Model) Init() tea.Cmd`
  - `func (m Model) Mode() Mode`、`func (m Model) Path() []string`、`func (m Model) IsDirty() bool`、`func (m Model) selectedNode() *common.NodeInfo`、`func (m Model) visible() []*common.NodeInfo`
  - 内部:`func (m *Model) push(l Location)` / `pop()` / `reload()` / `clampCursor()` / `moveCursor(d int)`
  - `func cellValue(n *common.NodeInfo, key string) string`、`func sortedLabels(map[string]string) string`、`func computeColumnWidths(cols []Column, avail int) []int`、`func truncateCell(s string, width int) string`
  - `var columnDefs []Column`、`var defaultColumnKeys []string`
  - 测试辅助:`func seedNodes(t *testing.T, store common.NodeStore)`(见本任务 Step 1,后续测试复用)

- [ ] **Step 1: 写失败测试**

创建 `cmd/cli/cmd/tui/nodes/model_test.go`(`package nodes`):

```go
package nodes

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

func key(t tea.KeyType) tea.Msg { return tea.KeyMsg{Type: t} }

func runeKey(r rune) tea.Msg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func seedNodes(t *testing.T, store common.NodeStore) {
	t.Helper()
	for _, n := range []*common.NodeInfo{
		{ID: "n2", Name: "db-1", Address: "10.0.0.2", Port: 22, User: "admin", Status: "offline", Groups: []string{"db"}, Labels: map[string]string{"env": "dev"}},
		{ID: "n3", Name: "cache-1", Address: "10.0.0.3", Port: 22, User: "root", Status: "online", Groups: []string{"cache", "web"}, Labels: map[string]string{"env": "prod", "role": "cache"}},
		{ID: "n1", Name: "web-1", Address: "10.0.0.1", Port: 22, User: "root", Status: "online", Groups: []string{"web"}, Labels: map[string]string{"env": "prod"}},
	} {
		if err := store.Add(n); err != nil {
			t.Fatalf("seed add: %v", err)
		}
	}
}

func newTestStore(t *testing.T) common.NodeStore {
	t.Helper()
	return common.NewInMemoryNodeStoreAt(filepath.Join(t.TempDir(), "nodes.json"))
}

func TestNewModel_LoadsAndSortsByID(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	if len(m.nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(m.nodes))
	}
	if m.nodes[0].ID != "n1" || m.nodes[1].ID != "n2" || m.nodes[2].ID != "n3" {
		t.Fatalf("expected sorted by id, got %s,%s,%s", m.nodes[0].ID, m.nodes[1].ID, m.nodes[2].ID)
	}
	if m.cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", m.cursor)
	}
}

func TestNewModel_ModeAndPath(t *testing.T) {
	m := NewModel(newTestStore(t))
	if m.Mode() != ModeNormal {
		t.Fatal("expected ModeNormal")
	}
	path := m.Path()
	if len(path) != 1 || path[0] != "nodes" {
		t.Fatalf("unexpected path: %v", path)
	}
}

func TestMoveCursor_DownAndUp(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(key(tea.KeyDown))
	m = nm.(Model)
	if m.cursor != 1 {
		t.Fatalf("expected cursor 1, got %d", m.cursor)
	}
	nm, _ = m.Update(key(tea.KeyUp))
	m = nm.(Model)
	if m.cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", m.cursor)
	}
}

func TestMoveCursor_Clamps(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	for i := 0; i < 5; i++ {
		nm, _ := m.Update(key(tea.KeyDown))
		m = nm.(Model)
	}
	if m.cursor != 2 {
		t.Fatalf("expected clamp at 2, got %d", m.cursor)
	}
	nm, _ := m.Update(key(tea.KeyUp))
	nm, _ = nm.(Model).Update(key(tea.KeyUp))
	nm, _ = nm.(Model).Update(key(tea.KeyUp))
	nm, _ = nm.(Model).Update(key(tea.KeyUp))
	m = nm.(Model)
	if m.cursor != 0 {
		t.Fatalf("expected clamp at 0, got %d", m.cursor)
	}
}

func TestFocusPane_LeftRight(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(key(tea.KeyRight))
	m = nm.(Model)
	if m.focus != paneDetail {
		t.Fatal("expected focus detail")
	}
	nm, _ = m.Update(key(tea.KeyLeft))
	m = nm.(Model)
	if m.focus != paneList {
		t.Fatal("expected focus list")
	}
}

func TestJumpTopBottom_G(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('G'))
	m = nm.(Model)
	if m.cursor != 2 {
		t.Fatalf("expected cursor 2, got %d", m.cursor)
	}
	nm, _ = m.Update(runeKey('g'))
	m = nm.(Model)
	if m.cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", m.cursor)
	}
}

func TestSelectedNode(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	n := m.selectedNode()
	if n == nil || n.ID != "n1" {
		t.Fatalf("unexpected selected node: %+v", n)
	}
}

func TestComputeColumnWidths_Fits(t *testing.T) {
	cols := []Column{{Key: "id", Pref: 10}, {Key: "name", Pref: 10}}
	got := computeColumnWidths(cols, 40)
	if got[0] != 10 || got[1] != 10 {
		t.Fatalf("unexpected widths: %v", got)
	}
}

func TestComputeColumnWidths_Scales(t *testing.T) {
	cols := []Column{{Key: "id", Pref: 20}, {Key: "name", Pref: 20}, {Key: "status", Pref: 10}}
	got := computeColumnWidths(cols, 30)
	total := 0
	for _, w := range got {
		if w < 6 {
			t.Fatalf("width below floor: %v", got)
		}
		total += w
	}
	if total > 30 {
		t.Fatalf("total %d exceeds avail 30: %v", total, got)
	}
}

func TestTruncateCell(t *testing.T) {
	if s := truncateCell("hello", 3); s != "he…" {
		t.Fatalf("unexpected: %q", s)
	}
	if s := truncateCell("hello", 10); s != "hello     " {
		t.Fatalf("unexpected pad: %q", s)
	}
}

func TestCellValue_VariousKeys(t *testing.T) {
	n := &common.NodeInfo{ID: "n1", Name: "web", Address: "1.2.3.4", Port: 22, User: "root", Status: "online", Groups: []string{"web"}, Labels: map[string]string{"b": "2", "a": "1"}, LastCheckAt: "x", ProxyJump: "jump"}
	cases := map[string]string{
		"id": "n1", "name": "web", "address": "1.2.3.4", "port": "22", "user": "root",
		"status": "online", "groups": "web", "labels": "a=1,b=2", "last_check": "x", "metadata": "jump",
	}
	for k, want := range cases {
		if got := cellValue(n, k); got != want {
			t.Fatalf("cellValue(%s) = %q, want %q", k, got, want)
		}
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -run TestNewModel -v`
Expected: FAIL,`undefined: Mode` / `NewModel` / `columnDefs` 等

- [ ] **Step 3: 实现 list.go**

创建 `cmd/cli/cmd/tui/nodes/list.go`:

```go
package nodes

import (
	"sort"
	"strconv"
	"strings"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

type Column struct {
	Key   string
	Label string
	Pref  int
}

var columnDefs = []Column{
	{"id", "ID", 20},
	{"name", "Name", 20},
	{"address", "Address", 24},
	{"port", "Port", 8},
	{"user", "User", 12},
	{"status", "Status", 10},
	{"groups", "Groups", 18},
	{"labels", "Labels", 24},
	{"last_check", "Last Check", 16},
	{"metadata", "Metadata", 20},
}

var defaultColumnKeys = []string{"id", "name", "address", "status"}

func colByKey(key string) (Column, bool) {
	for _, c := range columnDefs {
		if c.Key == key {
			return c, true
		}
	}
	return Column{}, false
}

func cellValue(n *common.NodeInfo, key string) string {
	switch key {
	case "id":
		return n.ID
	case "name":
		return n.Name
	case "address":
		return n.Address
	case "port":
		return strconv.Itoa(n.Port)
	case "user":
		return n.User
	case "status":
		return n.Status
	case "groups":
		return strings.Join(n.Groups, ",")
	case "labels":
		return sortedLabels(n.Labels)
	case "last_check":
		return n.LastCheckAt
	case "metadata":
		return n.ProxyJump
	}
	return ""
}

func sortedLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(labels[k])
	}
	return b.String()
}

func computeColumnWidths(cols []Column, avail int) []int {
	widths := make([]int, len(cols))
	total := 0
	for i, c := range cols {
		widths[i] = c.Pref
		total += c.Pref
	}
	for total > avail {
		maxI := 0
		for i := 1; i < len(widths); i++ {
			if widths[i] > widths[maxI] {
				maxI = i
			}
		}
		if widths[maxI] <= 6 {
			break
		}
		widths[maxI]--
		total--
	}
	return widths
}

func truncateCell(s string, width int) string {
	if width <= 0 {
		return ""
	}
	w := 0
	var out []rune
	for _, r := range s {
		rw := 1
		if r > 127 {
			rw = 2
		}
		if w+rw > width {
			if w < width {
				out = append(out, '…')
			}
			break
		}
		w += rw
		out = append(out, r)
	}
	res := string(out)
	for common.DisplayWidth(res) < width {
		res += " "
	}
	return res
}
```

- [ ] **Step 4: 实现 model.go(列表导航部分)**

创建 `cmd/cli/cmd/tui/nodes/model.go`:

```go
package nodes

import (
	"sort"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

type Mode int

const (
	ModeNormal Mode = iota
	ModeInsert
)

type Location int

const (
	LocList Location = iota
	LocNew
	LocEdit
	LocDelete
	LocColumns
)

type pane int

const (
	paneList pane = iota
	paneDetail
)

type Model struct {
	store common.NodeStore

	stack  []Location
	mode   Mode
	focus  pane
	cursor int
	width  int

	nodes       []*common.NodeInfo
	filter      FilterQuery
	filterInput textinput.Model
	filterOpen  bool
	filterText  string

	columns      []string
	form         *FormModel
	confirm      *ConfirmModel
	columnsModel *ColumnsModel

	status string
}

func NewModel(store common.NodeStore) Model {
	m := Model{
		store:       store,
		stack:       []Location{LocList},
		columns:     append([]string(nil), defaultColumnKeys...),
		filterInput: newInput("/ 过滤 (g:组 l:标签)", 40),
		width:       120,
	}
	m.reload()
	return m
}

func newInput(placeholder string, width int) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Width = width
	ti.CharLimit = 256
	ti.Blur()
	return ti
}

func (m Model) Init() tea.Cmd { return nil }

// View 最小桩实现:保证 Model 满足 tea.Model 接口(Task 9 替换为完整渲染)
func (m Model) View() string { return "" }

func (m Model) current() Location { return m.stack[len(m.stack)-1] }

func (m *Model) push(l Location) { m.stack = append(m.stack, l) }

func (m *Model) pop() {
	if len(m.stack) > 1 {
		m.stack = m.stack[:len(m.stack)-1]
	}
}

func (m *Model) reload() {
	m.nodes, _ = m.store.List()
	sort.Slice(m.nodes, func(i, j int) bool { return m.nodes[i].ID < m.nodes[j].ID })
	m.clampCursor()
}

func (m *Model) clampCursor() {
	v := m.visible()
	if len(v) == 0 {
		m.cursor = 0
		return
	}
	if m.cursor >= len(v) {
		m.cursor = len(v) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *Model) moveCursor(d int) {
	v := m.visible()
	if len(v) == 0 {
		return
	}
	m.cursor += d
	m.clampCursor()
}

func (m Model) visible() []*common.NodeInfo {
	return applyFilter(m.nodes, m.filter)
}

func (m Model) selectedNode() *common.NodeInfo {
	v := m.visible()
	if m.cursor < 0 || m.cursor >= len(v) {
		return nil
	}
	return v[m.cursor]
}

func (m Model) selectedID() string {
	if n := m.selectedNode(); n != nil {
		return n.ID
	}
	return ""
}

func (m Model) Mode() Mode { return m.mode }

func (m Model) Path() []string {
	id := m.selectedID()
	switch m.current() {
	case LocNew:
		return []string{"nodes", "new"}
	case LocEdit:
		return []string{"nodes", id, "edit"}
	case LocDelete:
		return []string{"nodes", id, "delete"}
	case LocColumns:
		return []string{"nodes", "columns"}
	default:
		return []string{"nodes"}
	}
}

func (m Model) IsDirty() bool {
	if len(m.stack) > 1 {
		return true
	}
	return m.form != nil && m.form.IsDirty()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// 本阶段只有 LocList 可达,直接走 updateList。
	// Task 5/6/8 扩展 switch 加入 LocColumns / LocNew+LocEdit / LocDelete 分支。
	return m.updateList(msg)
}

func (m Model) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.filterOpen {
		return m.updateFilter(msg)
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "up":
		m.moveCursor(-1)
	case "down":
		m.moveCursor(1)
	case "left":
		m.focus = paneList
	case "right":
		m.focus = paneDetail
	case "g":
		m.cursor = 0
	case "G":
		m.cursor = len(m.visible()) - 1
	case "/":
		m.filterOpen = true
		m.mode = ModeInsert
		m.filterInput.Focus()
		m.filterInput.SetValue(m.filterText)
	}
	m.clampCursor()
	return m, nil
}

func (m Model) updateFilter(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok && km.String() == "esc" {
		m.filterOpen = false
		m.mode = ModeNormal
		m.filterInput.Blur()
		// 恢复上一次已应用的查询串与过滤(取消本次编辑)
		m.filterInput.SetValue(m.filterText)
		m.filter = ParseFilterQuery(m.filterText)
		return m, nil
	}
	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	m.filterText = m.filterInput.Value()
	m.filter = ParseFilterQuery(m.filterText)
	if km, ok := msg.(tea.KeyMsg); ok && km.String() == "enter" {
		m.filterOpen = false
		m.mode = ModeNormal
		m.filterInput.Blur()
	}
	m.clampCursor()
	return m, cmd
}
```

注意:`model.go` 里 `FormModel`/`ConfirmModel`/`ColumnsModel` 类型此时未定义,Task 3 先让 `updateForm`/`updateConfirm`/`updateColumns` 分支不可达(这些位置不会进入),但引用类型必须存在 —— 因此在 `model.go` 末尾追加 **占位类型定义**(后续任务逐个替换为真实实现):

```go
// Task 6/8/5 将分别替换 FormModel / ConfirmModel / ColumnsModel 为完整实现。
// 占位:仅保证 model.go 能编译,含一个非导出方法避免 empty-struct lint 误伤。
type FormModel struct{ _ struct{} }
func (f *FormModel) IsDirty() bool { return false }

type ConfirmModel struct{ _ struct{} }

type ColumnsModel struct{ _ struct{} }
```

- [ ] **Step 5: 运行确认通过**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -v`
Expected: 全部 PASS(含 Task 2 过滤测试)

- [ ] **Step 6: 提交**

```bash
git add cmd/cli/cmd/tui/nodes/list.go cmd/cli/cmd/tui/nodes/model.go cmd/cli/cmd/tui/nodes/model_test.go
git commit -m "feat(tui): node 列表数据层与导航(排序/光标/焦点/过滤输入框)"
```

---

### Task 4: 行过滤交互(`/` 打开、输入即过滤、Enter/Esc)

**Files:**
- Modify: `cmd/cli/cmd/tui/nodes/model.go`(已含 updateFilter)
- Create: `cmd/cli/cmd/tui/nodes/filter_ui_test.go`
- Test: `cmd/cli/cmd/tui/nodes/filter_ui_test.go`

**Interfaces:**
- Consumes: Task 3 的 `Model`/`Mode`/`runeKey`/`newTestStore`/`seedNodes`
- Produces(供后续):`Model.filter` 状态、`updateFilter` 行为约定

- [ ] **Step 1: 写失败测试**

创建 `cmd/cli/cmd/tui/nodes/filter_ui_test.go`(`package nodes`):

```go
package nodes

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFilterOpen_EntersInsertMode(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('/'))
	m = nm.(Model)
	if m.Mode() != ModeInsert {
		t.Fatal("expected Insert mode after /")
	}
	if !m.filterOpen {
		t.Fatal("expected filterOpen")
	}
}

func TestFilterType_LiveFilters(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('/'))
	m = nm.(Model)
	for _, r := range []rune("g:web") {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(Model)
	}
	if len(m.visible()) != 2 {
		t.Fatalf("expected 2 visible (n1,n3 in group web), got %d", len(m.visible()))
	}
	if m.visible()[0].ID != "n1" || m.visible()[1].ID != "n3" {
		t.Fatalf("unexpected visible: %s, %s", m.visible()[0].ID, m.visible()[1].ID)
	}
}

func TestFilterEnter_AppliesAndReturnsNormal(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('/'))
	m = nm.(Model)
	for _, r := range []rune("l:env=prod") {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(Model)
	}
	nm, _ = m.Update(key(tea.KeyEnter))
	m = nm.(Model)
	if m.Mode() != ModeNormal {
		t.Fatal("expected Normal after Enter")
	}
	if m.filterOpen {
		t.Fatal("expected filter closed")
	}
	if len(m.visible()) != 2 {
		t.Fatalf("expected 2 visible, got %d", len(m.visible()))
	}
	if m.filterText != "l:env=prod" {
		t.Fatalf("unexpected filterText: %q", m.filterText)
	}
}

func TestFilterEsc_CancelsAndRestores(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('/'))
	m = nm.(Model)
	for _, r := range []rune("g:web") {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(Model)
	}
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(Model)
	if m.Mode() != ModeNormal || m.filterOpen {
		t.Fatal("expected filter closed and Normal after Esc")
	}
	if len(m.visible()) != 3 {
		t.Fatalf("expected all 3 visible after cancel, got %d", len(m.visible()))
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -run TestFilter -v`
Expected: FAIL —— `updateFilter` 尚未接通 `/` 分支前,`/` 无效果,断言失败

- [ ] **Step 3: 接通实现**

**核对实现(无需改动)**:Task 3 已在 `updateList` 加入 `/` 分支、且 `updateFilter` 已实现。Step 3 仅核对 `case "/"` 分支与 `updateFilter` 与 Task 3 一致即可,直接进入 Step 4:

- [ ] **Step 4: 运行确认通过**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -run TestFilter -v`
Expected: 全部 PASS

- [ ] **Step 5: 提交**

```bash
git add cmd/cli/cmd/tui/nodes/model.go cmd/cli/cmd/tui/nodes/filter_ui_test.go
git commit -m "feat(tui): / 打开行过滤输入,输入即过滤,Enter/Esc 提交/取消"
```

---

### Task 5: 列选择器 `/nodes/columns`

**Files:**
- Create: `cmd/cli/cmd/tui/nodes/columns.go`
- Create: `cmd/cli/cmd/tui/nodes/columns_test.go`
- Modify: `cmd/cli/cmd/tui/nodes/model.go`(`updateColumns` 真实现 + 列字段)
- Test: `cmd/cli/cmd/tui/nodes/columns_test.go`

**Interfaces:**
- Consumes: `columnDefs`/`defaultColumnKeys`(Task 3)
- Produces:
  - `func NewColumnsModel(selected []string) *ColumnsModel`
  - `func (cm *ColumnsModel) selected() []string`
  - `func (cm *ColumnsModel) toggle(i int)` / `selectAll()` / `reset()` / `restoreSnapshot()`
  - `func columnKeys() []string`
  - `Model.updateColumns(msg tea.Msg) (tea.Model, tea.Cmd)`(真实实现,替换占位)

- [ ] **Step 1: 写失败测试**

创建 `cmd/cli/cmd/tui/nodes/columns_test.go`(`package nodes`):

```go
package nodes

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewColumnsModel_Default(t *testing.T) {
	cm := NewColumnsModel(defaultColumnKeys)
	if len(cm.order) != len(columnDefs) {
		t.Fatalf("expected %d order, got %d", len(columnDefs), len(cm.order))
	}
	got := cm.selected()
	if len(got) != 4 || got[0] != "id" || got[1] != "name" || got[2] != "address" || got[3] != "status" {
		t.Fatalf("unexpected selected: %v", got)
	}
}

func TestColumns_ToggleAndApply(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('c'))
	m = nm.(Model)
	if m.current() != LocColumns {
		t.Fatal("expected in columns")
	}
	// cursor 在 id(第 0),按 Space 取消 id
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = nm.(Model)
	nm, _ = m.Update(key(tea.KeyEnter))
	m = nm.(Model)
	if m.current() != LocList {
		t.Fatal("expected back to list")
	}
	if len(m.columns) != 3 {
		t.Fatalf("expected 3 columns after unchecking id, got %v", m.columns)
	}
	for _, c := range m.columns {
		if c == "id" {
			t.Fatal("id should be removed")
		}
	}
}

func TestColumns_SelectAllAndReset(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('c'))
	m = nm.(Model)
	nm, _ = m.Update(runeKey('a'))
	m = nm.(Model)
	if len(m.columns) != len(columnDefs) {
		t.Fatalf("expected all columns, got %v", m.columns)
	}
	nm, _ = m.Update(runeKey('r'))
	m = nm.(Model)
	if len(m.columns) != 4 {
		t.Fatalf("expected default 4 columns after reset, got %v", m.columns)
	}
}

func TestColumns_EscRestoresSnapshot(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('c'))
	m = nm.(Model)
	nm, _ = m.Update(runeKey('a'))
	m = nm.(Model)
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(Model)
	if m.current() != LocList {
		t.Fatal("expected back to list")
	}
	if len(m.columns) != 4 {
		t.Fatalf("expected snapshot restored to 4, got %v", m.columns)
	}
}

func TestColumnsModel_Methods(t *testing.T) {
	cm := NewColumnsModel(defaultColumnKeys)
	cm.toggle(0)
	if cm.checked[0] {
		t.Fatal("expected id unchecked after toggle")
	}
	cm.selectAll()
	for i, c := range cm.checked {
		if !c {
			t.Fatalf("expected index %d checked", i)
		}
	}
	cm.reset()
	if len(cm.selected()) != 4 {
		t.Fatalf("expected 4 after reset, got %v", cm.selected())
	}
	cm.toggle(0)
	cm.restoreSnapshot()
	if len(cm.selected()) != 4 {
		t.Fatalf("expected snapshot restore, got %v", cm.selected())
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -run TestColumns -v`
Expected: FAIL,`undefined: NewColumnsModel` / `ColumnsModel has no field toggle`

- [ ] **Step 3: 实现 columns.go**

创建 `cmd/cli/cmd/tui/nodes/columns.go`:

```go
package nodes

type ColumnsModel struct {
	order    []string
	checked  []bool
	snapshot []bool
	cursor   int
}

func columnKeys() []string {
	keys := make([]string, len(columnDefs))
	for i, c := range columnDefs {
		keys[i] = c.Key
	}
	return keys
}

func NewColumnsModel(selected []string) *ColumnsModel {
	cm := &ColumnsModel{order: columnKeys()}
	cm.checked = make([]bool, len(cm.order))
	for i, k := range cm.order {
		for _, s := range selected {
			if k == s {
				cm.checked[i] = true
			}
		}
	}
	cm.snapshot = append([]bool(nil), cm.checked...)
	return cm
}

func (cm *ColumnsModel) selected() []string {
	out := []string{}
	for i, k := range cm.order {
		if cm.checked[i] {
			out = append(out, k)
		}
	}
	if len(out) == 0 {
		return append([]string(nil), defaultColumnKeys...)
	}
	return out
}

func (cm *ColumnsModel) toggle(i int) { cm.checked[i] = !cm.checked[i] }

func (cm *ColumnsModel) selectAll() {
	for i := range cm.checked {
		cm.checked[i] = true
	}
}

func (cm *ColumnsModel) reset() {
	cm.checked = make([]bool, len(cm.order))
	for i, k := range cm.order {
		for _, d := range defaultColumnKeys {
			if k == d {
				cm.checked[i] = true
			}
		}
	}
}

func (cm *ColumnsModel) restoreSnapshot() {
	cm.checked = append([]bool(nil), cm.snapshot...)
}
```

- [ ] **Step 4: 替换 model.go 中的占位 `ColumnsModel` 与 `updateColumns`**

将 Task 3 追加的占位块中 `ColumnsModel` 相关行删除(保留 FormModel/ConfirmModel 占位),并给 `model.go` 增加真实 `updateColumns` 与 `openColumns`:

```go
func (m *Model) openColumns() {
	m.columnsModel = NewColumnsModel(m.columns)
}

func (m Model) updateColumns(msg tea.Msg) (tea.Model, tea.Cmd) {
	cm := m.columnsModel
	if cm == nil {
		return m, nil
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "up":
		cm.cursor = (cm.cursor - 1 + len(cm.order)) % len(cm.order)
	case "down":
		cm.cursor = (cm.cursor + 1) % len(cm.order)
	case " ":
		cm.toggle(cm.cursor)
		m.columns = cm.selected()
	case "a":
		cm.selectAll()
		m.columns = cm.selected()
	case "r":
		cm.reset()
		m.columns = cm.selected()
	case "enter":
		m.pop()
		m.columnsModel = nil
	case "esc":
		cm.restoreSnapshot()
		m.columns = cm.selected()
		m.pop()
		m.columnsModel = nil
	}
	return m, nil
}
```

同时在 `updateList` 的 switch 中追加入口(在 `/` 分支后):

```go
	case "c":
		m.push(LocColumns)
		m.openColumns()
```

并确认 `openColumns` 在 push 后调用(m.form/confirm 不动)。**同时把顶层 `Update` 分发 switch 改为按位置分发**(否则在 LocColumns 里按键会落到 updateList):

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.current() {
	case LocColumns:
		return m.updateColumns(msg)
	default:
		return m.updateList(msg)
	}
}
```

Task 6/8 会继续向这个 switch 追加 `LocNew, LocEdit` 与 `LocDelete` 分支。

- [ ] **Step 5: 运行确认通过**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -v`
Expected: 全部 PASS

- [ ] **Step 6: 提交**

```bash
git add cmd/cli/cmd/tui/nodes/columns.go cmd/cli/cmd/tui/nodes/columns_test.go cmd/cli/cmd/tui/nodes/model.go
git commit -m "feat(tui): 列选择器 /nodes/columns(勾选/A 全选/R 重置/Esc 还原)"
```

---

### Task 6: 表单核心(导航/输入态隔离 + 回卷)与新增表单

**Files:**
- Create: `cmd/cli/cmd/tui/nodes/form.go`
- Create: `cmd/cli/cmd/tui/nodes/form_test.go`
- Modify: `cmd/cli/cmd/tui/nodes/model.go`(替换 FormModel 占位 + `openForm`/`updateForm`)
- Test: `cmd/cli/cmd/tui/nodes/form_test.go`

**Interfaces:**
- Consumes: `common.NodeInfo`、`common.NodeStore`、`Mode`、`textinput`
- Produces:
  - `type FormMode int` 常量 `FormAdd`/`FormEdit`
  - `func NewFormModel(mode FormMode, node *common.NodeInfo) *FormModel`
  - `func (f *FormModel) IsDirty() bool` / `move(d int)` / `validate() string` / `focusFirstInvalid()` / `toNode() *common.NodeInfo`
  - `Model.openForm(mode FormMode, node *common.NodeInfo)`、`Model.updateForm(msg) (tea.Model, tea.Cmd)`、`Model.saveForm() (tea.Model, tea.Cmd)`
  - `func parseLabels(s string) map[string]string`、`func splitTrim(s, sep string) []string`
  - 测试辅助:`func openAddForm(t *testing.T, store common.NodeStore) Model`、`func typeField(m Model, runes []rune) Model`

- [ ] **Step 1: 写失败测试(隔离与回卷是关键验收)**

创建 `cmd/cli/cmd/tui/nodes/form_test.go`(`package nodes`):

```go
package nodes

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func openAddForm(t *testing.T, store common.NodeStore) Model {
	t.Helper()
	m := NewModel(store)
	nm, _ := m.Update(runeKey('a'))
	return nm.(Model)
}

func typeField(m Model, runes string) Model {
	for _, r := range runes {
		nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(Model)
	}
	return m
}

func TestOpenAddForm_PathAndCursor(t *testing.T) {
	store := newTestStore(t)
	m := openAddForm(t, store)
	if m.current() != LocNew {
		t.Fatal("expected LocNew")
	}
	if m.form == nil {
		t.Fatal("expected form non-nil")
	}
	path := m.Path()
	if len(path) != 2 || path[1] != "new" {
		t.Fatalf("unexpected path: %v", path)
	}
	if m.form.cursor != 0 {
		t.Fatalf("expected cursor 0 (ID), got %d", m.form.cursor)
	}
	if m.Mode() != ModeNormal {
		t.Fatal("expected Normal navigate mode on open")
	}
}

func TestFormEnter_EntersInsertMode(t *testing.T) {
	m := openAddForm(t, newTestStore(t))
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(Model)
	if m.Mode() != ModeInsert {
		t.Fatal("expected Insert after Enter on field")
	}
}

func TestFormInsert_Isolation_NoGlobalKeys(t *testing.T) {
	m := openAddForm(t, newTestStore(t))
	nm, _ := m.Update(key(tea.KeyEnter)) // 进入 ID 输入
	m = nm.(Model)
	before := m.form.cursor
	// 在输入态下发 q / s / 方向键 / Esc 之外的所有键,都不应触发保存/导航/回列表
	for _, r := range []rune("qsa./") {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(Model)
		if m.Mode() != ModeInsert {
			t.Fatalf("mode broke on %q", r)
		}
		if m.current() != LocNew {
			t.Fatalf("location popped on %q", r)
		}
		if m.form.cursor != before {
			t.Fatalf("form cursor moved on %q", r)
		}
	}
	if got := m.form.fields[0].input.Value(); got != "qsa./" {
		t.Fatalf("expected value qsa./, got %q", got)
	}
}

func TestFormEsc_ExitsInsertMode(t *testing.T) {
	m := openAddForm(t, newTestStore(t))
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(Model)
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(Model)
	if m.Mode() != ModeNormal {
		t.Fatal("expected Normal after Esc in insert")
	}
	if m.current() != LocNew {
		t.Fatal("expected still in form")
	}
}

func TestFormWrapAround_UpFromTop(t *testing.T) {
	m := openAddForm(t, newTestStore(t))
	// 首个可编辑字段是 ID(0);按 ↑ 应回卷到最后一个可编辑字段(Labels, 9)
	nm, _ := m.Update(key(tea.KeyUp))
	m = nm.(Model)
	if m.form.cursor != len(m.form.fields)-1 {
		t.Fatalf("expected wrap to last field %d, got %d", len(m.form.fields)-1, m.form.cursor)
	}
}

func TestFormWrapAround_DownFromBottom(t *testing.T) {
	m := openAddForm(t, newTestStore(t))
	// 跳到最后一个可编辑字段,再按 ↓ 回卷到 ID
	m.form.cursor = len(m.form.fields) - 1
	nm, _ := m.Update(key(tea.KeyDown))
	m = nm.(Model)
	if m.form.cursor != 0 {
		t.Fatalf("expected wrap to first field 0, got %d", m.form.cursor)
	}
}

func TestFormMove_SkipsReadonlyInEdit(t *testing.T) {
	node := &common.NodeInfo{ID: "n1", Name: "web", Address: "1.2.3.4", Port: 22}
	f := NewFormModel(FormEdit, node)
	if f.cursor != 1 {
		t.Fatalf("edit form should start at first editable field (Name), got %d", f.cursor)
	}
	// 从 Name(1) 按 ↑ 回卷到 Labels(9),不会落到只读 ID(0)
	f.move(-1)
	if f.cursor != len(f.fields)-1 {
		t.Fatalf("expected wrap to last field, got %d", f.cursor)
	}
}
```

注意 `form_test.go` 使用 `common.NodeInfo`,需在文件头加 import `"github.com/cangyunye/go-owl/cmd/cli/cmd/common"`。

- [ ] **Step 2: 运行确认失败**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -run TestForm -v`
Expected: FAIL,`undefined: NewFormModel` / `FormModel has no field fields`

- [ ] **Step 3: 实现 form.go(导航/输入态/回卷/校验/数据转换)**

创建 `cmd/cli/cmd/tui/nodes/form.go`:

```go
package nodes

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

type FormMode int

const (
	FormAdd FormMode = iota
	FormEdit
)

type FormField struct {
	key      string
	label    string
	input    textinput.Model
	required bool
	editable bool
}

type FormModel struct {
	mode   FormMode
	base   *common.NodeInfo
	fields []*FormField
	cursor int
	error  string
	original map[string]string
	confirmDiscard bool
}

func NewFormModel(mode FormMode, node *common.NodeInfo) *FormModel {
	f := &FormModel{mode: mode, base: node, original: map[string]string{}}
	var base common.NodeInfo
	if node != nil {
		base = *node
	}
	specs := []struct {
		key, label string
		req        bool
		val        string
		editable   bool
	}{
		{"id", "ID", mode == FormAdd, base.ID, mode == FormAdd},
		{"name", "Name", true, base.Name, true},
		{"address", "Address", true, base.Address, true},
		{"port", "Port", false, strconv.Itoa(base.Port), true},
		{"user", "User", false, base.User, true},
		{"password", "Password", false, base.Password, true},
		{"ssh_key", "SSHKey", false, base.SSHKey, true},
		{"proxy_jump", "ProxyJump", false, base.ProxyJump, true},
		{"groups", "Groups", false, strings.Join(base.Groups, ","), true},
		{"labels", "Labels", false, sortedLabels(base.Labels), true},
	}
	for _, s := range specs {
		ti := textinput.New()
		ti.SetValue(s.val)
		ti.Placeholder = s.label
		ti.Width = 30
		ti.CharLimit = 256
		ti.Blur()
		f.fields = append(f.fields, &FormField{key: s.key, label: s.label, input: ti, required: s.req, editable: s.editable})
		f.original[s.key] = s.val
	}
	f.cursor = f.firstEditable()
	return f
}

func (f *FormModel) firstEditable() int {
	for i, fd := range f.fields {
		if fd.editable {
			return i
		}
	}
	return 0
}

func (f *FormModel) IsDirty() bool {
	for _, fd := range f.fields {
		if fd.input.Value() != f.original[fd.key] {
			return true
		}
	}
	return false
}

// move 在可编辑字段间移动并首尾回卷(跳过只读字段)
func (f *FormModel) move(d int) {
	if f.editableCount() == 0 {
		return
	}
	for i := 0; i < len(f.fields); i++ {
		f.cursor = (f.cursor + d + len(f.fields)) % len(f.fields)
		if f.fields[f.cursor].editable {
			return
		}
	}
}

func (f *FormModel) editableCount() int {
	n := 0
	for _, fd := range f.fields {
		if fd.editable {
			n++
		}
	}
	return n
}

func (f *FormModel) validate() string {
	for _, fd := range f.fields {
		if !fd.editable {
			continue
		}
		if fd.required && strings.TrimSpace(fd.input.Value()) == "" {
			return fd.label + " 不能为空"
		}
		if fd.key == "port" {
			v := strings.TrimSpace(fd.input.Value())
			if v != "" {
				p, err := strconv.Atoi(v)
				if err != nil || p < 1 || p > 65535 {
					return "Port 必须是 1-65535 的整数"
				}
			}
		}
	}
	return ""
}

func (f *FormModel) focusFirstInvalid() {
	for i, fd := range f.fields {
		if !fd.editable {
			continue
		}
		if fd.required && strings.TrimSpace(fd.input.Value()) == "" {
			f.cursor = i
			return
		}
		if fd.key == "port" {
			v := strings.TrimSpace(fd.input.Value())
			if v != "" {
				if _, err := strconv.Atoi(v); err != nil {
					f.cursor = i
					return
				}
			}
		}
	}
}

func (f *FormModel) value(key string) string {
	for _, fd := range f.fields {
		if fd.key == key {
			return strings.TrimSpace(fd.input.Value())
		}
	}
	return ""
}

func (f *FormModel) toNode() *common.NodeInfo {
	now := time.Now().Format(time.RFC3339)
	port := 22
	if v := f.value("port"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p >= 1 {
			port = p
		}
	}
	n := &common.NodeInfo{
		ID:        f.value("id"),
		Name:      f.value("name"),
		Address:   f.value("address"),
		Port:      port,
		User:      f.value("user"),
		Password:  f.value("password"),
		SSHKey:    f.value("ssh_key"),
		ProxyJump: f.value("proxy_jump"),
		Groups:    splitTrim(f.value("groups"), ","),
		Labels:    parseLabels(f.value("labels")),
		Status:    "offline",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if f.mode == FormEdit && f.base != nil {
		n.CreatedAt = f.base.CreatedAt
		n.Status = f.base.Status
		if port == 22 && f.base.Port != 22 && f.value("port") == "" {
			n.Port = f.base.Port
		}
	}
	return n
}

func parseLabels(s string) map[string]string {
	out := map[string]string{}
	for _, pair := range strings.Split(s, ",") {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			if k != "" {
				out[k] = v
			}
		}
	}
	return out
}

func splitTrim(s, sep string) []string {
	out := []string{}
	for _, p := range strings.Split(s, sep) {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
```

- [ ] **Step 4: 替换 model.go 占位 `FormModel` 并接 `updateForm`/`openForm`/`saveForm`**

删除 Task 3 占位块中的 `FormModel` 部分(保留 ConfirmModel/ColumnsModel 占位),追加:

```go
func (m *Model) openForm(mode FormMode, node *common.NodeInfo) {
	m.form = NewFormModel(mode, node)
	m.mode = ModeNormal
}

func (m Model) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	f := m.form
	if f == nil {
		return m, nil
	}
	if f.confirmDiscard {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "y", "enter":
				m.pop()
				m.form = nil
				m.reload()
			case "n", "esc":
				f.confirmDiscard = false
			}
		}
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
		m.mode = ModeInsert
		f.fields[f.cursor].input.Focus()
	case "s":
		return m.saveForm()
	case "esc":
		if f.IsDirty() {
			f.confirmDiscard = true
		} else {
			m.pop()
			m.form = nil
			m.reload()
		}
	}
	return m, nil
}

func (m Model) saveForm() (tea.Model, tea.Cmd) {
	f := m.form
	if err := f.validate(); err != "" {
		f.error = err
		f.focusFirstInvalid()
		return m, nil
	}
	f.error = ""
	node := f.toNode()
	var err error
	if f.mode == FormAdd {
		err = m.store.Add(node)
	} else {
		err = m.store.Update(node)
	}
	if err != nil {
		f.error = "保存失败: " + err.Error()
		return m, nil
	}
	if err := m.store.Save(); err != nil {
		f.error = "保存失败: " + err.Error()
		return m, nil
	}
	m.pop()
	m.form = nil
	m.reload()
	m.status = "已保存节点 " + node.ID
	return m, nil
}
```

同时在 `updateList` 的 switch 中追加入口(在 `c` 分支后):

```go
	case "a":
		m.push(LocNew)
		m.openForm(FormAdd, nil)
```

同时把顶层 `Update` 分发 switch 追加表单分支(Task 5 建立的 switch 基础上):

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.current() {
	case LocNew, LocEdit:
		return m.updateForm(msg)
	case LocColumns:
		return m.updateColumns(msg)
	default:
		return m.updateList(msg)
	}
}
```

Task 8 会再追加 `LocDelete` 分支。

- [ ] **Step 5: 运行确认通过**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -v`
Expected: 全部 PASS(新增隔离/回卷测试通过)

- [ ] **Step 6: 提交**

```bash
git add cmd/cli/cmd/tui/nodes/form.go cmd/cli/cmd/tui/nodes/form_test.go cmd/cli/cmd/tui/nodes/model.go
git commit -m "feat(tui): 表单导航/输入态硬隔离 + 首尾回卷 + 新增节点表单"
```

---

### Task 7: 新增表单保存/校验/错误回显数据流

**Files:**
- Modify: `cmd/cli/cmd/tui/nodes/form.go`(如无缺,不变)
- Create: `cmd/cli/cmd/tui/nodes/form_save_test.go`
- Test: `cmd/cli/cmd/tui/nodes/form_save_test.go`

**Interfaces:**
- Consumes: Task 6 的 `openAddForm`/`typeField`/`saveForm`
- Produces: 保存数据流的行为约定(校验失败不弹栈、重复 ID 回显、成功弹栈刷新)

- [ ] **Step 1: 写失败测试**

创建 `cmd/cli/cmd/tui/nodes/form_save_test.go`(`package nodes`):

```go
package nodes

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFormAdd_SavePersists(t *testing.T) {
	store := newTestStore(t)
	m := openAddForm(t, store)
	m = enterAndType(t, m, "new-node")
	m = moveToAndEdit(t, m, 1)
	m = typeField(m, "web-9")
	m = moveToAndEdit(t, m, 2)
	m = typeField(m, "10.9.9.9")
	m = save(t, m)
	if m.current() != LocList {
		t.Fatalf("expected back to list, got loc %v", m.current())
	}
	got, err := store.Get("new-node")
	if err != nil {
		t.Fatalf("expected node persisted: %v", err)
	}
	if got.Name != "web-9" || got.Address != "10.9.9.9" {
		t.Fatalf("unexpected node: %+v", got)
	}
	if got.Status != "offline" {
		t.Fatalf("expected offline status, got %q", got.Status)
	}
	if m.status == "" {
		t.Fatal("expected status message")
	}
}

func TestFormAdd_Validation_Required(t *testing.T) {
	store := newTestStore(t)
	m := openAddForm(t, store)
	// ID 必填未填,直接保存 → 错误行回显,不弹栈
	nm, _ := m.Update(runeKey('s'))
	m = nm.(Model)
	if m.current() != LocNew {
		t.Fatal("expected stay in form")
	}
	if m.form.error == "" {
		t.Fatal("expected validation error")
	}
}

func TestFormAdd_Validation_Port(t *testing.T) {
	store := newTestStore(t)
	m := openAddForm(t, store)
	m = enterAndType(t, m, "n1")
	m = moveToAndEdit(t, m, 1)
	m = typeField(m, "web")
	m = moveToAndEdit(t, m, 2)
	m = typeField(m, "1.2.3.4")
	m = moveToAndEdit(t, m, 3)
	m = typeField(m, "99999")
	m = save(t, m)
	if m.current() != LocNew {
		t.Fatal("expected stay in form on invalid port")
	}
	if m.form.error == "" || m.form.cursor != 3 {
		t.Fatalf("expected error and focus port, got error=%q cursor=%d", m.form.error, m.form.cursor)
	}
}

func TestFormAdd_DuplicateID(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := openAddForm(t, store)
	m = enterAndType(t, m, "n1")
	m = moveToAndEdit(t, m, 1)
	m = typeField(m, "web")
	m = moveToAndEdit(t, m, 2)
	m = typeField(m, "1.2.3.4")
	m = save(t, m)
	if m.current() != LocNew {
		t.Fatal("expected stay in form on duplicate")
	}
	if m.form.error == "" {
		t.Fatal("expected duplicate error")
	}
}

func TestFormAdd_EscDirty_ConfirmDiscard(t *testing.T) {
	store := newTestStore(t)
	m := openAddForm(t, store)
	m = enterAndType(t, m, "n1")
	// 第一次 Esc:退出输入态回到导航态(此时仍是表单,未触发丢弃确认)
	nm, _ := m.Update(key(tea.KeyEsc))
	m = nm.(Model)
	// 第二次 Esc:导航态 + 有改动 → 进入丢弃确认
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(Model)
	if !m.form.confirmDiscard {
		t.Fatal("expected confirmDiscard when dirty")
	}
	nm, _ = m.Update(runeKey('n'))
	m = nm.(Model)
	if m.current() != LocNew || m.form.confirmDiscard {
		t.Fatal("expected stay in form after n")
	}
	// 再次两次 Esc:先退输入?此时导航态,直接一次 Esc 即确认
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(Model)
	if !m.form.confirmDiscard {
		t.Fatal("expected confirmDiscard again")
	}
	nm, _ = m.Update(runeKey('y'))
	m = nm.(Model)
	if m.current() != LocList {
		t.Fatal("expected back to list after y")
	}
}

func enterAndType(t *testing.T, m Model, s string) Model {
	t.Helper()
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(Model)
	return typeField(m, s)
}

func moveToAndEdit(t *testing.T, m Model, target int) Model {
	t.Helper()
	if m.form == nil {
		t.Fatal("form nil")
	}
	// 若仍处输入态,先 Esc 退回导航态,否则 Down 移动的是文本光标而非字段
	if m.Mode() == ModeInsert {
		nm, _ := m.Update(key(tea.KeyEsc))
		m = nm.(Model)
	}
	for m.form.cursor != target {
		nm, _ := m.Update(key(tea.KeyDown))
		m = nm.(Model)
		if m.form.cursor == target {
			break
		}
	}
	nm, _ := m.Update(key(tea.KeyEnter))
	return nm.(Model)
}

// save 退出输入态后按 s 保存(输入态按 s 只会输入字符)
func save(t *testing.T, m Model) Model {
	t.Helper()
	if m.Mode() == ModeInsert {
		nm, _ := m.Update(key(tea.KeyEsc))
		m = nm.(Model)
	}
	nm, _ := m.Update(runeKey('s'))
	return nm.(Model)
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -run TestFormAdd -v`
Expected: FAIL(`saveForm`/`validate`/`toNode` 等在新表单下失败,或 helper 编译错误)

- [ ] **Step 3: 修复实现(若有)**

`form.go` 的 `validate`/`focusFirstInvalid`/`saveForm`(在 model.go)已在 Task 6 实现。若运行中出现具体失败,修复对应逻辑;重点核对:
- `saveForm` 里 `FormAdd` 走 `store.Add`,重复 ID 时 store 返回 `node already exists`,进入错误回显分支
- `moveToAndEdit`/`save` 在 `form_save_test.go` 中 `m.form` 可直接访问(测试在 package nodes 内);两者都会先 Esc 退出输入态,再移动字段/保存

- [ ] **Step 4: 运行确认通过**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -v`
Expected: 全部 PASS

- [ ] **Step 5: 提交**

```bash
git add cmd/cli/cmd/tui/nodes/form_save_test.go cmd/cli/cmd/tui/nodes/model.go cmd/cli/cmd/tui/nodes/form.go
git commit -m "feat(tui): 新增节点保存/校验/重复 ID 错误回显/脏表单丢弃确认"
```

---

### Task 8: 编辑表单 + 删除确认

**Files:**
- Modify: `cmd/cli/cmd/tui/nodes/model.go`(updateList 的 `e`/`d` 入口;替换 ConfirmModel 占位)
- Create: `cmd/cli/cmd/tui/nodes/confirm.go`
- Create: `cmd/cli/cmd/tui/nodes/edit_confirm_test.go`
- Test: `cmd/cli/cmd/tui/nodes/edit_confirm_test.go`

**Interfaces:**
- Consumes: Task 6/7 的 `FormModel`/`saveForm`;`confirm.go` 新增:
  - `func NewConfirmModel(n *common.NodeInfo) *ConfirmModel`
  - `Model.updateConfirm(msg) (tea.Model, tea.Cmd)`(真实实现,替换占位)

- [ ] **Step 1: 写失败测试**

创建 `cmd/cli/cmd/tui/nodes/edit_confirm_test.go`(`package nodes`):

```go
package nodes

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

func TestEditForm_PrefilledAndReadonlyID(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('e'))
	m = nm.(Model)
	if m.current() != LocEdit {
		t.Fatal("expected LocEdit")
	}
	if m.form == nil {
		t.Fatal("expected form")
	}
	// ID 只读且预填
	if m.form.fields[0].editable {
		t.Fatal("expected ID readonly in edit")
	}
	if m.form.fields[0].input.Value() != "n1" {
		t.Fatalf("expected prefilled id n1, got %q", m.form.fields[0].input.Value())
	}
	if m.form.cursor != 1 {
		t.Fatalf("expected cursor at Name(1), got %d", m.form.cursor)
	}
	path := m.Path()
	if len(path) != 3 || path[1] != "n1" || path[2] != "edit" {
		t.Fatalf("unexpected path: %v", path)
	}
}

func TestEditForm_SaveUpdates(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('e'))
	m = nm.(Model)
	// 光标在 Name(1)。Name 已预填 "web-1",直接改值(打字会拼接,故用 SetValue)
	m.form.fields[1].input.SetValue("web-9")
	// 编辑表单此刻为导航态,直接 s 保存
	nm, _ = m.Update(runeKey('s'))
	m = nm.(Model)
	if m.current() != LocList {
		t.Fatal("expected back to list")
	}
	got, _ := store.Get("n1")
	if got.Name != "web-9" {
		t.Fatalf("expected updated name, got %q", got.Name)
	}
	// 编辑不应清空原有字段(Address/Port 保留)
	if got.Address != "10.0.0.1" {
		t.Fatalf("expected address preserved, got %q", got.Address)
	}
}

func TestConfirm_OpenAndPath(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('d'))
	m = nm.(Model)
	if m.current() != LocDelete {
		t.Fatal("expected LocDelete")
	}
	if m.confirm == nil || m.confirm.node.ID != "n1" {
		t.Fatalf("unexpected confirm: %+v", m.confirm)
	}
	path := m.Path()
	if len(path) != 3 || path[1] != "n1" || path[2] != "delete" {
		t.Fatalf("unexpected path: %v", path)
	}
}

func TestConfirm_DeleteExecutes(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('d'))
	m = nm.(Model)
	// 默认光标在 Delete(0),Enter 执行
	nm, _ = m.Update(key(tea.KeyEnter))
	m = nm.(Model)
	if m.current() != LocList {
		t.Fatal("expected back to list")
	}
	if _, err := store.Get("n1"); err == nil {
		t.Fatal("expected node removed")
	}
	if m.status == "" {
		t.Fatal("expected status message")
	}
}

func TestConfirm_Cancel(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('d'))
	m = nm.(Model)
	nm, _ = m.Update(key(tea.KeyRight)) // 切到 Cancel
	m = nm.(Model)
	nm, _ = m.Update(key(tea.KeyEnter))
	m = nm.(Model)
	if m.current() != LocList {
		t.Fatal("expected back to list without delete")
	}
	if _, err := store.Get("n1"); err != nil {
		t.Fatal("expected node still present")
	}
}

func TestConfirm_EscCancels(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('d'))
	m = nm.(Model)
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(Model)
	if m.current() != LocList {
		t.Fatal("expected back to list")
	}
	if _, err := store.Get("n1"); err != nil {
		t.Fatal("expected node still present")
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -run "TestEditForm|TestConfirm" -v`
Expected: FAIL(`updateList` 无 `e`/`d` 分支,`updateConfirm` 占位)

- [ ] **Step 3: 实现 confirm.go**

创建 `cmd/cli/cmd/tui/nodes/confirm.go`:

```go
package nodes

import (
	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

type ConfirmModel struct {
	node   *common.NodeInfo
	cursor int // 0=Delete 1=Cancel
	error  string
}

func NewConfirmModel(n *common.NodeInfo) *ConfirmModel {
	return &ConfirmModel{node: n}
}
```

- [ ] **Step 4: 替换 model.go 占位 ConfirmModel 并接 `updateConfirm` + `e`/`d` 入口**

删除 Task 3 占位块中剩余部分,追加:

```go
func (m *Model) openConfirm(n *common.NodeInfo) {
	m.confirm = NewConfirmModel(n)
}

func (m Model) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	c := m.confirm
	if c == nil {
		return m, nil
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "left":
		c.cursor = 0
	case "right":
		c.cursor = 1
	case "esc":
		m.pop()
		m.confirm = nil
	case "enter":
		if c.cursor == 0 {
			if err := m.store.Remove(c.node.ID); err != nil {
				c.error = "删除失败: " + err.Error()
				return m, nil
			}
			m.store.Save()
			m.pop()
			m.confirm = nil
			m.reload()
			m.status = "已删除节点 " + c.node.ID
		} else {
			m.pop()
			m.confirm = nil
		}
	}
	return m, nil
}
```

在 `updateList` 的 switch 中追加(在 `c` 分支后):

```go
	case "e":
		if n := m.selectedNode(); n != nil {
			m.push(LocEdit)
			m.openForm(FormEdit, n)
		}
	case "d":
		if n := m.selectedNode(); n != nil {
			m.push(LocDelete)
			m.openConfirm(n)
		}
```

同时把顶层 `Update` 分发 switch 补全为最终形态(Task 6 建立的 switch 上追加):

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.current() {
	case LocNew, LocEdit:
		return m.updateForm(msg)
	case LocDelete:
		return m.updateConfirm(msg)
	case LocColumns:
		return m.updateColumns(msg)
	default:
		return m.updateList(msg)
	}
}
```

- [ ] **Step 5: 运行确认通过**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -v`
Expected: 全部 PASS

- [ ] **Step 6: 提交**

```bash
git add cmd/cli/cmd/tui/nodes/confirm.go cmd/cli/cmd/tui/nodes/model.go cmd/cli/cmd/tui/nodes/edit_confirm_test.go
git commit -m "feat(tui): 编辑表单(ID 只读预填/保存保留原字段) + 删除确认"
```

---

### Task 9: View 渲染(双栏/表单/确认/列选择器 + 面包屑 + 状态栏)

**Files:**
- Create: `cmd/cli/cmd/tui/nodes/view.go`
- Create: `cmd/cli/cmd/tui/nodes/view_test.go`
- Modify: `cmd/cli/cmd/tui/nodes/model.go`(加 `selectedColumns() []Column` 辅助)
- Test: `cmd/cli/cmd/tui/nodes/view_test.go`

**Interfaces:**
- Consumes: 全部前序;`Model.selectedColumns() []Column`
- Produces: `Model.View() string`(真实实现,替换空 View),渲染约定供 Task 10 断言

- [ ] **Step 1: 写失败测试**

创建 `cmd/cli/cmd/tui/nodes/view_test.go`(`package nodes`):

```go
package nodes

import (
	"strings"
	"testing"
)

func TestView_ListRendersNodesAndDetail(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	v := m.View()
	if !strings.Contains(v, "n1") || !strings.Contains(v, "web-1") {
		t.Fatalf("list missing node: %q", v)
	}
	if !strings.Contains(v, "db-1") {
		t.Fatalf("detail missing selected node name: %q", v)
	}
	if !strings.Contains(v, "env=prod") {
		t.Fatalf("detail missing labels: %q", v)
	}
	if !strings.Contains(v, "Groups") {
		t.Fatalf("detail missing Groups label: %q", v)
	}
}

func TestView_EmptyList(t *testing.T) {
	m := NewModel(newTestStore(t))
	if m.View() == "" {
		t.Fatal("expected non-empty empty-state view")
	}
}

func TestView_FormRendersFields(t *testing.T) {
	store := newTestStore(t)
	m := openAddForm(t, store)
	v := m.View()
	for _, label := range []string{"ID", "Name", "Address", "Port", "Groups", "Labels"} {
		if !strings.Contains(v, label) {
			t.Fatalf("form missing %s: %q", label, v)
		}
	}
}

func TestView_ConfirmRendersNode(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('d'))
	m = nm.(Model)
	v := m.View()
	if !strings.Contains(v, "n1") {
		t.Fatalf("confirm missing node: %q", v)
	}
}

func TestView_ColumnsRendersFields(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('c'))
	m = nm.(Model)
	v := m.View()
	for _, label := range []string{"id", "name", "status", "labels"} {
		if !strings.Contains(v, label) {
			t.Fatalf("columns missing %s: %q", label, v)
		}
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -run TestView -v`
Expected: FAIL(`Model.View()` 尚未实现,可能 panic 或空)

- [ ] **Step 3: 实现 view.go**

创建 `cmd/cli/cmd/tui/nodes/view.go`:

```go
package nodes

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	styleListBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	styleDetail     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	styleError      = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	styleDim        = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleSelected   = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
)

func (m Model) selectedColumns() []Column {
	cols := make([]Column, 0, len(m.columns))
	for _, k := range m.columns {
		if c, ok := colByKey(k); ok {
			cols = append(cols, c)
		}
	}
	return cols
}

func (m Model) View() string {
	switch m.current() {
	case LocNew, LocEdit:
		return m.listPane() + "\n\n" + m.formView()
	case LocDelete:
		return m.listPane() + "\n\n" + m.confirmView()
	case LocColumns:
		return m.listPane() + "\n\n" + m.columnsView()
	default:
		return m.listPane() + m.statusBar()
	}
}

func (m Model) listPane() string {
	cols := m.selectedColumns()
	avail := m.width / 2
	widths := computeColumnWidths(cols, avail)
	var b strings.Builder
	for i, c := range cols {
		b.WriteString(styleSelected.Render(truncateCell(c.Label, widths[i])))
		b.WriteString(" ")
	}
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", sum(widths)+len(cols)) + "\n")
	v := m.visible()
	for i, n := range v {
		marker := " "
		if i == m.cursor {
			marker = ">"
		}
		b.WriteString(marker)
		for j, c := range cols {
			cell := truncateCell(cellValue(n, c.Key), widths[j])
			if i == m.cursor {
				cell = styleSelected.Render(cell)
			}
			b.WriteString(cell)
			b.WriteString(" ")
		}
		b.WriteString("\n")
	}
	if len(v) == 0 {
		b.WriteString(styleDim.Render("  (无匹配节点,按 / 修改过滤或 a 添加)"))
		b.WriteString("\n")
	}
	listBox := styleListBorder.Width(avail + 2).Render(b.String())
	detailBox := styleDetail.Width(avail + 2).Render(m.detailPane())
	if m.focus == paneDetail {
		listBox = styleDim.Render(b.String())
		listBox = styleListBorder.Width(avail + 2).Render(listBox)
		detailBox = styleSelected.Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("14")).Render(m.detailPane())
		detailBox = styleDetail.Width(avail + 2).Render(detailBox)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, listBox, "  ", detailBox)
}

func (m Model) detailPane() string {
	n := m.selectedNode()
	if n == nil {
		return "  " + styleDim.Render("(未选择节点)")
	}
	var b strings.Builder
	rows := [][2]string{
		{"ID", n.ID}, {"Name", n.Name}, {"Address", fmt.Sprintf("%s:%d", n.Address, n.Port)},
		{"User", n.User}, {"Status", n.Status}, {"Groups", strings.Join(n.Groups, ",")},
		{"Labels", sortedLabels(n.Labels)}, {"ProxyJump", n.ProxyJump},
		{"SSHKey", n.SSHKey}, {"LastCheck", n.LastCheckAt}, {"CreatedAt", n.CreatedAt},
	}
	for _, r := range rows {
		if r[1] == "" {
			r[1] = "—"
		}
		b.WriteString(fmt.Sprintf("%-12s %s\n", r[0], r[1]))
	}
	return b.String()
}

func (m Model) statusBar() string {
	var chips []string
	for _, g := range m.filter.Groups {
		chips = append(chips, "g:"+g)
	}
	for k, v := range m.filter.Labels {
		chips = append(chips, "l:"+k+"="+v)
	}
	var b strings.Builder
	if len(chips) > 0 {
		b.WriteString(styleSelected.Render("[" + strings.Join(chips, " ") + "]"))
		b.WriteString("  ")
	}
	if m.filterOpen {
		b.WriteString(m.filterInput.View())
		b.WriteString(styleDim.Render("  Enter 应用  Esc 取消"))
	} else {
		b.WriteString(styleDim.Render("↑↓选择 ←→切栏 g/G首尾 a添加 e编辑 d删除 c列 /过滤 ?帮助 q退出"))
	}
	if m.status != "" {
		b.WriteString("  " + styleDim.Render(m.status))
	}
	return "\n" + b.String()
}

func (m Model) formView() string {
	f := m.form
	if f == nil {
		return ""
	}
	title := "添加节点"
	if f.mode == FormEdit {
		title = "编辑节点 " + f.nodeID()
	}
	var b strings.Builder
	b.WriteString("┌─ " + title + " ───────────────\n")
	for i, fd := range f.fields {
		marker := " "
		if i == f.cursor && m.mode == ModeNormal {
			marker = ">"
		}
		req := ""
		if fd.required {
			req = "*"
		}
		b.WriteString(fmt.Sprintf("%s %s%-10s %s\n", marker, req, fd.label, fd.input.View()))
	}
	if f.confirmDiscard {
		b.WriteString(styleError.Render("  有未保存修改,确认丢弃? y/n"))
	} else if f.error != "" {
		b.WriteString(styleError.Render("  " + f.error))
	} else {
		b.WriteString(styleDim.Render("  ↑↓移动 Enter编辑 s保存 Esc返回 ?帮助"))
	}
	b.WriteString("\n└─")
	return b.String()
}

func (m Model) confirmView() string {
	c := m.confirm
	if c == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("┌─ 删除节点 ─────────────\n")
	b.WriteString(fmt.Sprintf("  确定删除节点 %s (%s)?\n", c.node.ID, c.node.Name))
	if c.cursor == 0 {
		b.WriteString(styleSelected.Render("  [Delete]") + "   [Cancel]\n")
	} else {
		b.WriteString("   [Delete]  " + styleSelected.Render("[Cancel]") + "\n")
	}
	if c.error != "" {
		b.WriteString(styleError.Render("  " + c.error) + "\n")
	}
	b.WriteString(styleDim.Render("  ←→选择 Enter执行 Esc返回"))
	b.WriteString("\n└─")
	return b.String()
}

func (m Model) columnsView() string {
	cm := m.columnsModel
	if cm == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("┌─ 表格列配置 ────────────\n")
	for i, k := range cm.order {
		marker := "[ ]"
		if cm.checked[i] {
			marker = "[x]"
		}
		line := fmt.Sprintf("  %s %s", marker, k)
		if i == cm.cursor {
			line = styleSelected.Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString(styleDim.Render("  ↑↓移动 Space切换 A全选 R重置 Enter应用 Esc取消"))
	b.WriteString("\n└─")
	return b.String()
}

func sum(ns []int) int {
	total := 0
	for _, n := range ns {
		total += n
	}
	return total
}
```

`FormModel` 需新增 `nodeID()` 方法(在 form.go 追加):

```go
func (f *FormModel) nodeID() string {
	if f.base != nil {
		return f.base.ID
	}
	return f.value("id")
}
```

- [ ] **Step 4: 运行确认通过**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -v`
Expected: 全部 PASS

- [ ] **Step 5: 提交**

```bash
git add cmd/cli/cmd/tui/nodes/view.go cmd/cli/cmd/tui/nodes/view_test.go cmd/cli/cmd/tui/nodes/form.go cmd/cli/cmd/tui/nodes/model.go
git commit -m "feat(tui): View 渲染(双栏列表/表单/确认/列选择器/状态栏)"
```

---

### Task 10: App 外壳(面包屑/帮助/退出确认/插入态拦截)+ `owl tui` 入口接入

**Files:**
- Create: `cmd/cli/cmd/tui/app.go`
- Create: `cmd/cli/cmd/tui/app_test.go`
- Modify: `cmd/cli/cmd/tui/tui.go`(替换 exec 转发为原生 Program 启动)
- Test: `cmd/cli/cmd/tui/app_test.go` + 既有 `tui_test.go`

**Interfaces:**
- Consumes: `nodes.Model` 全部
- Produces:
  - `type App struct`、`func NewApp(store common.NodeStore) *App`
  - `App.Update/View/Init`(tea.Model)

- [ ] **Step 1: 写失败测试**

创建 `cmd/cli/cmd/tui/app_test.go`(`package tui_test`):

```go
package tui_test

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/tui"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/tui/nodes"
)

func newStore(t *testing.T) common.NodeStore {
	t.Helper()
	s := common.NewInMemoryNodeStoreAt(filepath.Join(t.TempDir(), "nodes.json"))
	for _, n := range []*common.NodeInfo{
		{ID: "n1", Name: "web-1", Address: "10.0.0.1", Port: 22, User: "root", Status: "online", Groups: []string{"web"}, Labels: map[string]string{"env": "prod"}},
		{ID: "n2", Name: "db-1", Address: "10.0.0.2", Port: 22, User: "admin", Status: "offline", Groups: []string{"db"}},
	} {
		if err := s.Add(n); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return s
}

func key(t tea.KeyType) tea.Msg { return tea.KeyMsg{Type: t} }

func runeKey(r rune) tea.Msg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func TestApp_BreadcrumbView(t *testing.T) {
	a := tui.NewApp(newStore(t))
	v := a.View()
	if !strings.Contains(v, "/nodes") {
		t.Fatalf("expected /nodes breadcrumb in view: %q", v)
	}
}

func TestApp_QuitInNormalMode(t *testing.T) {
	a := tui.NewApp(newStore(t))
	m, cmd := a.Update(runeKey('q'))
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
	// tea.Quit 是返回 Msg 的 Cmd,需调用后断言
	if msg := cmd(); msg != nil {
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Fatalf("expected tea.QuitMsg, got %T", msg)
		}
	} else {
		t.Fatal("expected non-nil quit msg")
	}
	_ = m
}

func TestApp_QuitBlockedInInsertMode(t *testing.T) {
	a := tui.NewApp(newStore(t))
	// 打开过滤输入(进入 Insert)
	m, _ := a.Update(runeKey('/'))
	a = m.(*tui.App)
	// Insert 态按 q 不得退出
	m, cmd := a.Update(runeKey('q'))
	if cmd != nil {
		t.Fatalf("expected no quit in insert mode, got %T", cmd)
	}
	_ = m
}

func TestApp_HelpOverlay(t *testing.T) {
	a := tui.NewApp(newStore(t))
	m, _ := a.Update(runeKey('?'))
	a = m.(*tui.App)
	v := a.View()
	if !strings.Contains(v, "Normal=命令") {
		t.Fatalf("expected help overlay: %q", v)
	}
	m, _ = a.Update(key(tea.KeyEsc))
	a = m.(*tui.App)
	if strings.Contains(a.View(), "Normal=命令") {
		t.Fatal("expected help closed after Esc")
	}
}

func TestApp_QuitConfirmWhenDirty(t *testing.T) {
	a := tui.NewApp(newStore(t))
	// 进入新增表单(深层位置 = dirty)
	m, _ := a.Update(runeKey('a'))
	a = m.(*tui.App)
	m, cmd := a.Update(runeKey('q'))
	if cmd != nil {
		t.Fatal("expected no immediate quit when dirty")
	}
	a = m.(*tui.App)
	// 确认 y → 退出
	_, cmd = a.Update(runeKey('y'))
	if cmd == nil {
		t.Fatal("expected quit cmd after confirm")
	}
	if msg := cmd(); msg != nil {
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Fatalf("expected quit after confirm, got %T", msg)
		}
	}
}

func TestApp_InsertModeBypassesAppKeys(t *testing.T) {
	a := tui.NewApp(newStore(t))
	m, _ := a.Update(runeKey('/'))
	a = m.(*tui.App)
	// Insert 态 '?' 不应开帮助
	_, cmd := a.Update(runeKey('?'))
	if cmd != nil {
		t.Fatal("expected no help toggle in insert mode")
	}
}

func TestApp_NodeCRUDFlow(t *testing.T) {
	a := tui.NewApp(newStore(t))
	// add: 打开表单,填 ID/Name/Address,保存
	m, _ := a.Update(runeKey('a'))
	a = m.(*tui.App)
	for _, r := range []rune("new-1") {
		m, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		a = m.(*tui.App)
	}
	_ = m
}
```

注意:App 的 `Update` 会把 Insert 态按键转发到 nodes;`TestApp_NodeCRUDFlow` 仅作冒烟(打开表单后输入字符不崩溃),断言可后续补全。`TestApp_HelpOverlay` 以帮助页独有的 `Normal=命令` 标记判断开合。

- [ ] **Step 2: 运行确认失败**

Run: `go test ./cmd/cli/cmd/tui/ -v`
Expected: FAIL,`undefined: NewApp`

- [ ] **Step 3: 实现 app.go**

创建 `cmd/cli/cmd/tui/app.go`:

```go
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/tui/nodes"
)

type App struct {
	nodes        nodes.Model
	help         bool
	quitConfirm  bool
}

func NewApp(store common.NodeStore) *App {
	return &App{nodes: nodes.NewModel(store)}
}

func (m App) Init() tea.Cmd { return nil }

func (m App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		return m.forward(msg)
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

func (m App) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	nm, cmd := m.nodes.Update(msg)
	m.nodes = nm.(nodes.Model)
	return m, cmd
}

func (m App) View() string {
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
```

- [ ] **Step 4: 改写 tui.go 入口**

将 `cmd/cli/cmd/tui/tui.go` 的 `runTui` 与 `findTuiExecutable` 整体替换为原生启动;删除不再使用的 `os/exec`、`path/filepath` import:

```go
package tui

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/i18n"
)

func NewTuiCmd() *cobra.Command {
	tuiCmd := &cobra.Command{
		Use:   "tui",
		Short: i18n.T("tui.cmd.short"),
		Long:  i18n.T("tui.cmd.long"),
		Run:   runTui,
	}

	return tuiCmd
}

func runTui(cmd *cobra.Command, args []string) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintln(os.Stderr, "错误: owl tui 需要在交互式终端中运行")
		os.Exit(1)
	}

	app := NewApp(common.GetNodeStore())
	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "错误: owl tui 异常:", err)
		os.Exit(1)
	}
}
```

注意:保留 `NewTuiCmd` 的 Use/Short/Long 与既有 `tui_test.go` 断言一致(仍无子命令、无 flag)。

- [ ] **Step 5: 运行确认通过**

Run: `go test ./cmd/cli/cmd/tui/ ./cmd/cli/cmd/nodes/ -v`
Expected: 全部 PASS;`go build ./cmd/cli -o build/owl` 通过

- [ ] **Step 6: 提交**

```bash
git add cmd/cli/cmd/tui/app.go cmd/cli/cmd/tui/app_test.go cmd/cli/cmd/tui/tui.go
git commit -m "feat(tui): App 外壳(面包屑/帮助/脏退出确认/Insert 拦截)接入 owl tui 原生入口"
```

---

### Task 11: pty 端到端冒烟(启动/面包屑/表单进出/退出)

**Files:**
- Create: `scripts/test-tui.sh`
- Test: 手动运行 `./scripts/test-tui.sh`

**Interfaces:**
- Consumes: `build/owl tui`
- Produces: E2E 验证脚本(进入 /nodes → a 开表单 → Esc 返回 → q 退出,退出码 0)

- [ ] **Step 1: 写脚本**

创建 `scripts/test-tui.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

echo "==> 构建 owl"
go build -o build/owl ./cmd/cli

OUT="$(mktemp)"
echo "==> E2E: 启动 owl tui,进入 /nodes 渲染列表,开表单,返回,退出"
( sleep 1.5
  printf 'a'    # 打开新增表单
  sleep 0.6
  printf '\033' # Esc 返回列表
  sleep 0.6
  printf 'q'    # 退出
) | script -q "$OUT" ./build/owl tui >/dev/null 2>&1 || true

echo "==> 校验输出"
if grep -q "/nodes" "$OUT"; then
  echo "PASS: 面包屑 /nodes 渲染"
else
  echo "FAIL: 未渲染面包屑 /nodes"
  cat "$OUT"
  exit 1
fi

if grep -q "添加节点" "$OUT"; then
  echo "PASS: 新增表单弹出"
else
  echo "FAIL: 未渲染新增表单"
  cat "$OUT"
  exit 1
fi

rm -f "$OUT"
echo "==> TUI E2E 冒烟通过"
```

`chmod +x scripts/test-tui.sh`。

- [ ] **Step 2: 运行脚本**

Run: `./scripts/test-tui.sh`
Expected: 三行 PASS;若终端类型导致时序不稳,把 sleep 从 1.5 提到 2.5 再试

- [ ] **Step 3: 全量回归**

Run: `go build ./... && go test ./cmd/cli/...`
Expected: 全部 PASS,无 vet 告警(`go vet ./cmd/cli/cmd/tui/...`)

- [ ] **Step 4: 提交**

```bash
git add scripts/test-tui.sh
git commit -m "test(tui): pty 端到端冒烟(启动/面包屑/表单/退出)"
```

---

## Self-Review

**1. Spec coverage:**
- 路径栈/面包屑 → Task 3(Path/current/push/pop)+ Task 10(App 渲染)
- Mode 隔离(Insert 不过 keymap)→ Task 4(/)、Task 6(表单隔离测试)、Task 10(App 在 Insert 态不响应 q/?)
- 双栏列表 + 详情全字段 → Task 3 + Task 9(detailPane 含 Labels/Groups/ProxyJump)
- 行过滤 g:/l:/搜索/AND → Task 2 + Task 4(chips 在 Task 9 statusBar)
- 列选择器勾选/A/R/重置/序列化 → Task 5(header 串序列化由 `selected()` 输出,App 未持久化属 Phase 1 会话内)
- 表单字段集、回卷、必填校验、Port 范围、错误回显、跳首个非法字段、重复 ID → Task 6/7
- 删除确认 ←/→/Enter/Esc → Task 8
- `common.NodeStore` 复用 + 不 os.Exit → Global Constraints + 各 task 错误回显
- 测试:pull bubbletea model 单测 + pty E2E → Task 2-11

**2. Placeholder scan:** 占位类型仅在 Task 3 明确标注为"后续任务替换",并在 Task 6/8 逐一替换,非模糊占位。所有测试断言均含实际代码。

**3. Type consistency:**
- `FormModel` 占位(IsDirty)→ Task 6 真实实现,签名一致
- `updateForm/updateConfirm/updateColumns` 返回 `(tea.Model, tea.Cmd)`,各 task 一致
- `applyFilter`/`FilterQuery`/`cellValue`/`computeColumnWidths` 签名在 Task 2/3 定义,Task 4/9 按相同签名使用
- `selectedColumns()` 在 Task 9 定义并仅在本 task 使用
- `nodeID()` 在 Task 9 追加于 form.go,与 formView 使用一致
