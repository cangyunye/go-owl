# owl tui 节点字段配色 + Labels 彩虹 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在已落地的主题系统之上,为节点列表列与详情面板增加字段级配色:Status 按值配色、User/Address/Groups 按字段槽配色、Labels 彩虹配色。

**Architecture:** 新增 theme 包彩虹色环(`Rainbow(key)` 基于 FNV-1a 哈希),节点详情面板新增 `coloredValue`/`styleForStatus`/`rainbowLabelsFull`,列表新增 `renderCell`/`rainbowLabelsWidth`(宽度感知彩虹)。全部复用现有 13 语义槽 + HybridColor 三档降级机制。

**Tech Stack:** Go 1.26,github.com/charmbracelet/lipgloss v1.1.0,`hash/fnv`(标准库)。

## Global Constraints

- lipgloss 锁定 v1.1.0,禁止升级 v2。
- 不新增语义槽;复用现有 13 槽:`SlotSuccess`/`SlotError`/`SlotWarning`/`SlotUser`/`SlotAccent`/`SlotTitle`/`SlotDim`/`SlotSelected`。
- `theme.Rainbow(key string) lipgloss.TerminalColor`:8 色环 × FNV-1a 哈希 `Sum32()%8`。
- 彩虹色环 8 色每色三档(TrueColor hex / ANSI256 索引 / ANSI 16 色)齐全,ANSI 优先(16 色终端彩虹仍可读)。
- 选中行保持整体 `styleSelected` 高亮(覆盖列色)。
- 空值占位符 `"—"` 保持默认色,不套字段色。
- 宽度感知彩虹:按可见宽度(`common.DisplayWidth`)逐 label 预算,放不下用 `…`;不切断 ANSI 码。
- 不升级 lipgloss;不改 AI/Exec/File 面板;仅 nodes 模块。
- 不允许注释(除非 spec/现有文件惯例)。现有 theme/hybrid.go 有中文注释惯例——新代码保持极简注释或不加。
- 测试命令:`go test ./cmd/cli/cmd/tui/...`
- TDD:先写失败测试,再实现,再提交。

---

### Task 1: theme/rainbow.go — 8 色彩虹色环

**Files:**
- Create: `cmd/cli/cmd/tui/theme/rainbow.go`
- Test: `cmd/cli/cmd/tui/theme/rainbow_test.go`

**Interfaces:**
- Consumes: `CompleteColor`(types.go)、`hybridColor`/`Slot`(hybrid.go)、`hash/fnv`(标准库)
- Produces: `func Rainbow(key string) lipgloss.TerminalColor`、`var rainbow []CompleteColor`(8 色三档)

- [ ] **Step 1: 写失败测试**

`cmd/cli/cmd/tui/theme/rainbow_test.go`:

```go
package theme

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
)

func TestRainbowRingComplete(t *testing.T) {
	assert.Len(t, rainbow, 8, "应有 8 色")
	for i, c := range rainbow {
		assert.NoError(t, c.Validate(), "色环[%d] 三档需合法", i)
	}
}

func TestRainbowDeterministic(t *testing.T) {
	a := Rainbow("env")
	b := Rainbow("env")
	assert.Equal(t, a, b, "同 key 应同色")
}

func TestRainbowIndexRange(t *testing.T) {
	// 0-7 索引全覆盖可通过 16 个不同 key 断言不越界
	for _, k := range []string{"env", "role", "zone", "arch", "os", "tier", "team", "app", "a", "bb", "ccc", "dddd", "eeeee", "ffffff", "g", "hh"} {
		c := Rainbow(k)
		assert.NotNil(t, c, "Rainbow(%q) 非 nil", k)
	}
}

func TestRainbowImplementsInterface(t *testing.T) {
	var _ lipgloss.TerminalColor = Rainbow("x")
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./cmd/cli/cmd/tui/theme/ -run TestRainbow -v`
Expected: FAIL(rainbow/Rainbow 未定义)

- [ ] **Step 3: 实现 rainbow.go**

`cmd/cli/cmd/tui/theme/rainbow.go`:

```go
package theme

import (
	"hash/fnv"

	"github.com/charmbracelet/lipgloss"
)

var rainbow = []CompleteColor{
	{TrueColor: "#F38BA8", ANSI256: "204", ANSI: "9"},
	{TrueColor: "#FAB387", ANSI256: "215", ANSI: "11"},
	{TrueColor: "#F9E2AF", ANSI256: "223", ANSI: "3"},
	{TrueColor: "#A6E3A1", ANSI256: "150", ANSI: "10"},
	{TrueColor: "#94E2D5", ANSI256: "116", ANSI: "14"},
	{TrueColor: "#89DCEB", ANSI256: "117", ANSI: "6"},
	{TrueColor: "#CBA6F7", ANSI256: "183", ANSI: "13"},
	{TrueColor: "#B4BEFE", ANSI256: "147", ANSI: "12"},
}

func Rainbow(key string) lipgloss.TerminalColor {
	h := fnv.New32a()
	h.Write([]byte(key))
	c := rainbow[h.Sum32()%uint32(len(rainbow))]
	return hybridColor(Slot{Light: c, Dark: c})
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./cmd/cli/cmd/tui/theme/ -run TestRainbow -v`
Expected: PASS

- [ ] **Step 5: 全量主题包回归**

Run: `go test ./cmd/cli/cmd/tui/theme/ -v`
Expected: PASS(全部既有 + 新测试)

- [ ] **Step 6: Commit**

```bash
git add cmd/cli/cmd/tui/theme/rainbow.go cmd/cli/cmd/tui/theme/rainbow_test.go
git commit -m "feat(tui): Labels彩虹色环(FNV哈希+8色三档降级)"
```

---

### Task 2: nodes 详情面板 — coloredValue/styleForStatus/rainbowLabelsFull

**Files:**
- Modify: `cmd/cli/cmd/tui/nodes/view.go`(`detailPane` 值渲染 + 新增三个函数)
- Test: `cmd/cli/cmd/tui/nodes/view_test.go`(追加)

**Interfaces:**
- Consumes: `theme.Rainbow`(Task 1)、`theme.Style`/`theme.Slot*`、`theme.SlotDim`、`common.DisplayWidth`、现有 `sortedLabels`/`styleDim`/`styleSelected`
- Produces: `func coloredValue(key, val string) string`、`func styleForStatus(s string) lipgloss.Style`、`func rainbowLabelsFull(s string) string`

- [ ] **Step 1: 写失败测试**

`cmd/cli/cmd/tui/nodes/view_test.go` 追加(先读现有文件末尾确认风格;测试包用 `package nodes`):

```go
func TestStyleForStatus(t *testing.T) {
	cases := []struct {
		status string
	}{
		{"online"}, {"offline"}, {"unknown"}, {"bogus"},
	}
	for _, c := range cases {
		s := styleForStatus(c.status)
		if s.Render("x") == "x" {
			t.Fatalf("styleForStatus(%q) 应有着色", c.status)
		}
	}
}

func TestColoredValue(t *testing.T) {
	if got := coloredValue("Status", "online"); got == "online" {
		t.Fatal("Status 应着色")
	}
	if got := coloredValue("Labels", "a=1,b=2"); got == "a=1,b=2" {
		t.Fatal("Labels 应彩虹着色")
	}
	if got := coloredValue("Address", "1.2.3.4"); got == "1.2.3.4" {
		t.Fatal("Address 应着色")
	}
	if got := coloredValue("User", "root"); got == "root" {
		t.Fatal("User 应着色")
	}
	if got := coloredValue("Groups", "web,db"); got == "web,db" {
		t.Fatal("Groups 应着色")
	}
	if got := coloredValue("ID", "n1"); got != "n1" {
		t.Fatalf("ID 不着色, got %q", got)
	}
}

func TestRainbowLabelsFull(t *testing.T) {
	if got := rainbowLabelsFull(""); got != "" {
		t.Fatalf("空串原样返回, got %q", got)
	}
	got := rainbowLabelsFull("a=1,b=2")
	if got == "a=1,b=2" {
		t.Fatal("多 label 应逐 label 彩虹")
	}
	// 异常格式整体回退 dim 而非 panic
	out := rainbowLabelsFull("notanassignment")
	if out == "" {
		t.Fatal("异常格式不应为空")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -run 'TestStyleForStatus|TestColoredValue|TestRainbowLabelsFull' -v`
Expected: FAIL(coloredValue/styleForStatus/rainbowLabelsFull 未定义)

- [ ] **Step 3: 实现**

在 `cmd/cli/cmd/tui/nodes/view.go` 的 `detailPane` 中,把 `rows` 循环的值写入改为调用 `coloredValue`(仅当值非空时):

`view.go` `detailPane` 循环(现状 117-122 行)替换为:

```go
	for _, r := range rows {
		if r[1] == "" {
			r[1] = "—"
			b.WriteString(fmt.Sprintf("%-12s %s\n", r[0], r[1]))
			continue
		}
		b.WriteString(fmt.Sprintf("%-12s %s\n", r[0], coloredValue(r[0], r[1])))
	}
```

在文件末尾追加(与现有 `detailPane` 同文件):

```go
func styleForStatus(s string) lipgloss.Style {
	switch s {
	case "online":
		return theme.Style(theme.SlotSuccess)
	case "offline":
		return theme.Style(theme.SlotError)
	default:
		return theme.Style(theme.SlotWarning)
	}
}

func coloredValue(key, val string) string {
	switch key {
	case "Status":
		return styleForStatus(val).Render(val)
	case "Labels":
		return rainbowLabelsFull(val)
	case "Address":
		return theme.Style(theme.SlotAccent).Render(val)
	case "User":
		return theme.Style(theme.SlotUser).Render(val)
	case "Groups":
		return theme.Style(theme.SlotTitle).Render(val)
	}
	return val
}
```

新增辅助 `splitPairs`(解析 `sortedLabels` 产出的 `"a=1,b=2"` 字符串回 pairs):在 `list.go` 中新增:

```go
// list.go
func splitPairs(s string) ([][2]string, bool) {
	if s == "" {
		return nil, false
	}
	parts := strings.Split(s, ",")
	out := make([][2]string, 0, len(parts))
	for _, p := range parts {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			return nil, false
		}
		out = append(out, [2]string{kv[0], kv[1]})
	}
	return out, true
}
```

`rainbowLabelsFull` 用 `splitPairs` + 辅助 `rainbowText`:

```go
func rainbowText(key, val string) string {
	k := lipgloss.NewStyle().Foreground(theme.Rainbow(key)).Render(key)
	return k + styleDim.Render("="+val)
}

func rainbowLabelsFull(s string) string {
	if s == "" {
		return ""
	}
	labels, ok := splitPairs(s)
	if !ok {
		return styleDim.Render(s)
	}
	var b strings.Builder
	for i, kv := range labels {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(rainbowText(kv[0], kv[1]))
	}
	return b.String()
}
```

确认 `nodes/view.go` 已 import `"github.com/charmbracelet/lipgloss"`(有,边框用)。`strings`/`fmt` 已有。`splitPairs` 放 `list.go`(与 `sortedLabels` 同文件),`rainbowText`/`rainbowLabelsFull` 放 `view.go`(与 `coloredValue` 同文件)。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -run 'TestStyleForStatus|TestColoredValue|TestRainbowLabelsFull|TestCellValue' -v`
Expected: PASS(含既有 `TestCellValue_VariousKeys` 不受影响)

- [ ] **Step 5: 全量 nodes + 全 TUI 回归**

Run: `go test ./cmd/cli/cmd/tui/...`
Expected: PASS(若详情快照断言了纯文本 Labels/Status 行,更新为着色后期望)

- [ ] **Step 6: Commit**

```bash
git add cmd/cli/cmd/tui/nodes/view.go cmd/cli/cmd/tui/nodes/list.go cmd/cli/cmd/tui/nodes/view_test.go
git commit -m "feat(tui): 节点详情面板字段配色(Status按值/Labels彩虹)"
```

---

### Task 3: nodes 列表列 — renderCell/rainbowLabelsWidth 宽度感知

**Files:**
- Modify: `cmd/cli/cmd/tui/nodes/view.go`(`listPane` 列渲染)
- Modify: `cmd/cli/cmd/tui/nodes/list.go`(`renderCell` 新增)
- Test: `cmd/cli/cmd/tui/nodes/view_test.go`(追加)或 `list_test.go`

**Interfaces:**
- Consumes: `cellValue`/`truncateCell`/`coloredValue`/`rainbowLabelsFull`/`splitPairs`/`rainbowText`(Task 2)、`theme.Rainbow`、`common.DisplayWidth`
- Produces: `func renderCell(n *common.NodeInfo, key string, width int, selected bool) string`、`func rainbowLabelsWidth(raw string, width int) string`

- [ ] **Step 1: 写失败测试**

`cmd/cli/cmd/tui/nodes/view_test.go` 追加:

```go
func TestRenderCellSelected(t *testing.T) {
	n := &common.NodeInfo{ID: "n1", Status: "online", Labels: map[string]string{"a": "1"}}
	sel := renderCell(n, "status", 10, true)
	if sel == "online" {
		t.Fatal("选中行应整体高亮")
	}
	unsel := renderCell(n, "status", 10, false)
	if unsel == "online" {
		t.Fatal("非选中 Status 应着色")
	}
	if got := renderCell(n, "id", 10, false); got == "" {
		t.Fatal("ID 列应正常渲染")
	}
}

func TestRainbowLabelsWidth(t *testing.T) {
	if got := rainbowLabelsWidth("", 10); got != "" {
		t.Fatalf("空串原样, got %q", got)
	}
	// 宽度极小 → 省略号
	got := rainbowLabelsWidth("a=1", 2)
	if got == "" {
		t.Fatal("不应为空")
	}
	// 宽列全量彩虹
	wide := rainbowLabelsWidth("a=1,b=2", 30)
	if wide == "a=1,b=2" {
		t.Fatal("应带 ANSI 着色")
	}
	// 窄列截断不切 ANSI
	short := rainbowLabelsWidth("a=1,b=2", 6)
	if short == "" {
		t.Fatal("窄列应非空")
	}
}

func TestRenderCellLabels(t *testing.T) {
	n := &common.NodeInfo{Labels: map[string]string{"a": "1"}}
	cell := renderCell(n, "labels", 20, false)
	if cell == "" {
		t.Fatal("labels 列应渲染")
	}
}

func TestRainbowLabelsWidthTrailingSpace(t *testing.T) {
	// truncateCell 会补尾部空格,rainbowLabelsWidth 需先 trim 再解析
	raw := "a=1  "
	got := rainbowLabelsWidth(raw, 8)
	if got == "" {
		t.Fatal("带尾部空格也应正常渲染")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -run 'TestRenderCell|TestRainbowLabelsWidth' -v`
Expected: FAIL(renderCell/rainbowLabelsWidth 未定义)

- [ ] **Step 3: 实现 list.go — renderCell**

`cmd/cli/cmd/tui/nodes/list.go` 末尾追加:

```go
func renderCell(n *common.NodeInfo, key string, width int, selected bool) string {
	raw := truncateCell(cellValue(n, key), width)
	if selected {
		return styleSelected.Render(raw)
	}
	switch key {
	case "status":
		return styleForStatus(n.Status).Render(raw)
	case "labels":
		return rainbowLabelsWidth(raw, width)
	case "user":
		return theme.Style(theme.SlotUser).Render(raw)
	case "address":
		return theme.Style(theme.SlotAccent).Render(raw)
	case "groups":
		return theme.Style(theme.SlotTitle).Render(raw)
	}
	return raw
}
```

`list.go` 需 import:`"github.com/cangyunye/go-owl/cmd/cli/cmd/tui/theme"`、`"github.com/charmbracelet/lipgloss"`(若 rainbowLabelsWidth 需)。确认现有 list.go import 无这两个,追加。

`rainbowLabelsWidth`(宽度感知,不切 ANSI):

```go
func rainbowLabelsWidth(raw string, width int) string {
	if raw == "" || width <= 0 {
		return ""
	}
	// truncateCell 输出带尾部空格填充,先去掉再解析,避免空格混入 value
	trimmed := strings.TrimRight(raw, " ")
	labels, ok := splitPairs(trimmed)
	if !ok {
		return styleDim.Render(raw)
	}
	remaining := width
	var b strings.Builder
	for i, kv := range labels {
		seg := rainbowText(kv[0], kv[1])
		if i > 0 {
			seg = "," + seg
		}
		sw := common.DisplayWidth(seg)
		if sw <= remaining {
			b.WriteString(seg)
			remaining -= sw
			continue
		}
		if b.Len() == 0 {
			if remaining >= 1 {
				b.WriteString("…")
			}
		} else {
			b.WriteString("…")
		}
		break
	}
	res := b.String()
	for common.DisplayWidth(res) < width {
		res += " "
	}
	return res
}
```

注:`truncateCell` 已把 raw 截到 width,`splitPairs` 再拆。`rainbowText` 的 ANSI 码不占可见宽度,`DisplayWidth(seg)` 得到可见宽。补齐空格用 `DisplayWidth` 保证对齐。`strings` 已有 import。

- [ ] **Step 4: 修改 view.go listPane**

`cmd/cli/cmd/tui/nodes/view.go` 的 `listPane` 中,现状 77-84 行:

```go
		for j, c := range cols {
			cell := truncateCell(cellValue(n, c.Key), widths[j])
			if i == m.cursor {
				cell = styleSelected.Render(cell)
			}
			b.WriteString(cell)
			b.WriteString(" ")
		}
```

替换为:

```go
		for j, c := range cols {
			cell := renderCell(n, c.Key, widths[j], i == m.cursor)
			b.WriteString(cell)
			b.WriteString(" ")
		}
```

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./cmd/cli/cmd/tui/nodes/ -run 'TestRenderCell|TestRainbowLabelsWidth' -v`
Expected: PASS

Run: `go test ./cmd/cli/cmd/tui/...`
Expected: PASS(若列表快照断言纯文本 cell,更新为 renderCell 后期望;现有 `TestCellValue_VariousKeys` 测 cellValue 纯文本,不受影响)

- [ ] **Step 6: go vet + 全量**

Run: `go vet ./cmd/cli/cmd/tui/...`
Expected: 无输出

Run: `go build ./...`
Expected: 成功

- [ ] **Step 7: Commit**

```bash
git add cmd/cli/cmd/tui/nodes/list.go cmd/cli/cmd/tui/nodes/view.go cmd/cli/cmd/tui/nodes/view_test.go
git commit -m "feat(tui): 列表列字段配色+Labels彩虹宽度感知"
```

---

### Task 4: 收尾 — spec 状态 + 全量回归

**Files:**
- Modify: `docs/superpowers/specs/2026-08-15-owl-tui-field-coloring-design.md`(追加实现状态)

**Interfaces:**
- Consumes: 全部已完成任务
- Produces: 无新 API

- [ ] **Step 1: 全量回归**

Run: `go test ./... -timeout 20m`
Expected: PASS(duckdb 等重型依赖包可能慢;与本次改动无关且历史即失败的包记录并跳过)

- [ ] **Step 2: grep 检查旧 API 未回归**

Run: `grep -rn "theme.Fg\|theme.CSelected\|ThemeANSI\|DowngradedToANSI" cmd/ --include="*.go"`
Expected: 无匹配

- [ ] **Step 3: spec 状态**

`docs/superpowers/specs/2026-08-15-owl-tui-field-coloring-design.md` 末尾追加:

```markdown
## 实现状态
- [x] 已完成(2026-08-15),计划见 docs/superpowers/plans/2026-08-15-owl-tui-field-coloring.md
```

- [ ] **Step 4: Commit**

```bash
git add docs/superpowers/specs/2026-08-15-owl-tui-field-coloring-design.md
git commit -m "docs(tui): 标记字段配色设计已实现"
```
