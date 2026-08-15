# owl tui 主题系统升级 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 owl TUI 配色从"2 档 7 色"升级为主题级——5 套预设 + 三级色域降级(ANSI→256→TrueColor)+ 明暗自适应,全量迁移所有 view。

**Architecture:** theme 包重写为:预设表(presets) + 语义槽结构(types) + 自定义 `HybridColor`(hybrid,实现 lipgloss.TerminalColor,hex 主值 + 可选 256/16 覆盖) + 探测/降级(基于 `lipgloss.ColorProfile()`/`HasDarkBackground()`)。外部仅依赖 `theme.Color(key)`/`theme.Style(key)`/`theme.Title()`。旧 API 与新 API 先共存,迁移完成后删除。

**Tech Stack:** Go 1.26,github.com/charmbracelet/lipgloss v1.1.0,github.com/muesli/termenv v0.16.0(间接,测试直接引用需转为直接依赖),testify。

## Global Constraints

- lipgloss **锁定 v1.1.0,禁止升级 v2**(`charm.land/lipgloss/v2` 不可用)。
- v1.1.0 签名:`lipgloss.ColorProfile() termenv.Profile`、`lipgloss.HasDarkBackground() bool`(**无参数**)、`lipgloss.SetColorProfile(p)`、`lipgloss.SetHasDarkBackground(b)`。
- `lipgloss.TerminalColor` 接口:`color(*Renderer) termenv.Color`(方法名小写 `color`,同包可访问)+ `RGBA() (r,g,b,a uint32)`。
- `termenv.Profile` 常量:`termenv.TrueColor`/`termenv.ANSI256`/`termenv.ANSI`/`termenv.NoColor`;`p.Color(string)` 解析 hex 或索引。
- 环境变量 `OWL_TUI_THEME` 只认主题名:`catppuccin`|`nord`|`dracula`|`solarized`|`default`。旧值 `ansi`/`truecolor` 废弃,按未知处理。
- 默认主题:`catppuccin`。
- 每槽 ANSI 16 色档必填非空;ANSI256 缺省 = hex 自动降级;TrueColor(hex)必填。
- 明暗:每槽存 Light/Dark 两组 `CompleteColor`,由 `HasDarkBackground()` 取用;探测失败按暗背景。
- 不允许注释(除非用户要求)。代码风格遵循现有文件。
- 测试命令:`go test ./cmd/cli/cmd/tui/...`
- TDD:先写失败测试,再实现,再提交。
- 若 `github.com/muesli/termenv` 未在 `go.mod` 直接依赖中,先运行 `go get github.com/muesli/termenv@v0.16.0`(Task 3 前)使其成为直接依赖,`go.mod` 从 `// indirect` 注释移除。

---

### Task 1: types.go — 语义槽与数据模型

**Files:**
- Create: `cmd/cli/cmd/tui/theme/types.go`
- Test: `cmd/cli/cmd/tui/theme/types_test.go`

**Interfaces:**
- Consumes: 无(本任务定义全部结构)
- Produces: `SlotKey`(常量 `SlotSelected` 等 13 个 + `slotKeys()` 返回 13 个)、`CompleteColor{TrueColor,ANSI256,ANSI string}`、`Slot{Light,Dark CompleteColor}`、`Theme{Name Name; Slots map[SlotKey]Slot}`、`Name string`

- [ ] **Step 1: 写失败测试**

`cmd/cli/cmd/tui/theme/types_test.go`:

```go
package theme

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSlotKeySet(t *testing.T) {
	keys := slotKeys()
	assert.Len(t, keys, 13, "应有 13 个语义槽")
	for _, k := range keys {
		assert.NotEmpty(t, string(k), "SlotKey 不应为空")
	}
}

func TestCompleteColorValidate(t *testing.T) {
	ok := CompleteColor{TrueColor: "#5FD4FF", ANSI: "14"}
	assert.NoError(t, ok.Validate())
	noAnsi := CompleteColor{TrueColor: "#5FD4FF"}
	assert.Error(t, noAnsi.Validate(), "ANSI 必填")
	badHex := CompleteColor{TrueColor: "5FD4FF", ANSI: "14"}
	assert.Error(t, badHex.Validate(), "hex 需 #RRGGBB")
	badAnsi := CompleteColor{TrueColor: "#5FD4FF", ANSI: "1000"}
	assert.Error(t, badAnsi.Validate(), "ANSI 超出 0-255")
}

func TestSlotValidate(t *testing.T) {
	ok := Slot{
		Light: CompleteColor{TrueColor: "#5FD4FF", ANSI: "14"},
		Dark:  CompleteColor{TrueColor: "#5FD4FF", ANSI: "14"},
	}
	assert.NoError(t, ok.Validate())
	assert.Error(t, Slot{Light: ok.Light}.Validate(), "Dark 必填")
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./cmd/cli/cmd/tui/theme/ -run TestSlotKeySet -v`
Expected: FAIL(编译错误:SlotKey/slotKeys 未定义)

- [ ] **Step 3: 实现 types.go**

`cmd/cli/cmd/tui/theme/types.go`:

```go
package theme

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Name string

type SlotKey string

const (
	SlotSelected    SlotKey = "selected"
	SlotDim         SlotKey = "dim"
	SlotError       SlotKey = "error"
	SlotUser        SlotKey = "user"
	SlotAI          SlotKey = "ai"
	SlotHighlightFg SlotKey = "highlightFg"
	SlotHighlightBg SlotKey = "highlightBg"
	SlotSuccess     SlotKey = "success"
	SlotWarning     SlotKey = "warning"
	SlotBorder      SlotKey = "border"
	SlotTitle       SlotKey = "title"
	SlotAccent      SlotKey = "accent"
	SlotMuted       SlotKey = "muted"
)

func slotKeys() []SlotKey {
	return []SlotKey{
		SlotSelected, SlotDim, SlotError, SlotUser, SlotAI,
		SlotHighlightFg, SlotHighlightBg, SlotSuccess, SlotWarning,
		SlotBorder, SlotTitle, SlotAccent, SlotMuted,
	}
}

type CompleteColor struct {
	TrueColor string
	ANSI256   string
	ANSI      string
}

var hexRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func (c CompleteColor) Validate() error {
	if c.TrueColor == "" || !hexRe.MatchString(c.TrueColor) {
		return fmt.Errorf("TrueColor 需为 #RRGGBB, got %q", c.TrueColor)
	}
	if c.ANSI == "" {
		return fmt.Errorf("ANSI 16 色档必填")
	}
	if n, err := strconv.Atoi(c.ANSI); err != nil || n < 0 || n > 255 {
		return fmt.Errorf("ANSI 应为 0-255, got %q", c.ANSI)
	}
	if c.ANSI256 != "" {
		if n, err := strconv.Atoi(c.ANSI256); err != nil || n < 0 || n > 255 {
			return fmt.Errorf("ANSI256 应为 0-255 或空串, got %q", c.ANSI256)
		}
	}
	return nil
}

type Slot struct {
	Light CompleteColor
	Dark  CompleteColor
}

func (s Slot) Validate() error {
	if err := s.Light.Validate(); err != nil {
		return fmt.Errorf("Light: %w", err)
	}
	if err := s.Dark.Validate(); err != nil {
		return fmt.Errorf("Dark: %w", err)
	}
	return nil
}

type Theme struct {
	Name  Name
	Slots map[SlotKey]Slot
}

func (t Theme) Validate() error {
	if strings.TrimSpace(string(t.Name)) == "" {
		return fmt.Errorf("主题名不能为空")
	}
	if len(t.Slots) != len(slotKeys()) {
		return fmt.Errorf("槽位数 %d != 需要 %d", len(t.Slots), len(slotKeys()))
	}
	for _, k := range slotKeys() {
		s, ok := t.Slots[k]
		if !ok {
			return fmt.Errorf("缺少槽位 %q", k)
		}
		if err := s.Validate(); err != nil {
			return fmt.Errorf("槽位 %q: %w", k, err)
		}
	}
	return nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./cmd/cli/cmd/tui/theme/ -run 'TestSlotKeySet|TestCompleteColorValidate|TestSlotValidate' -v`
Expected: PASS

注:现有 `theme_test.go` 仍引用 `palettes`/`fg`/`ThemeANSI`/`ThemeTrueColor`,这些在旧 `theme.go` 中仍存在,本任务不删除,测试保持通过。

- [ ] **Step 5: Commit**

```bash
git add cmd/cli/cmd/tui/theme/types.go cmd/cli/cmd/tui/theme/types_test.go
git commit -m "feat(tui): 主题语义槽与数据模型(13槽+三级色值+校验)"
```

---

### Task 2: presets.go — 5 套内建预设

**Files:**
- Create: `cmd/cli/cmd/tui/theme/presets.go`
- Test: `cmd/cli/cmd/tui/theme/presets_test.go`

**Interfaces:**
- Consumes: `Name`/`SlotKey`/`CompleteColor`/`Slot`/`Theme` 及 `slotKeys()`/`Validate()`(Task 1)
- Produces: `var presets = map[Name]Theme{...}`、`func themeNames() []Name`

- [ ] **Step 1: 写失败测试**

`cmd/cli/cmd/tui/theme/presets_test.go`:

```go
package theme

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPresetsComplete(t *testing.T) {
	names := themeNames()
	assert.Equal(t, 5, len(names), "应内置 5 套主题")
	for _, n := range names {
		t.Run(string(n), func(t *testing.T) {
			p, ok := presets[n]
			assert.True(t, ok, "presets 含 %q", n)
			assert.NoError(t, p.Validate(), "主题 %q 数据完整", n)
		})
	}
}

func TestPresetANSIRequired(t *testing.T) {
	for _, n := range themeNames() {
		for _, k := range slotKeys() {
			assert.NotEmpty(t, presets[n].Slots[k].Light.ANSI, "%s/%s/Light.ANSI 必填", n, k)
			assert.NotEmpty(t, presets[n].Slots[k].Dark.ANSI, "%s/%s/Dark.ANSI 必填", n, k)
		}
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./cmd/cli/cmd/tui/theme/ -run TestPresetsComplete -v`
Expected: FAIL(presets/themeNames 未定义)

- [ ] **Step 3: 实现 presets.go**

`cmd/cli/cmd/tui/theme/presets.go` — 5 套主题,每套 13 槽 × Light/Dark。ANSI256 均留空走自动降级:

```go
package theme

var presets = map[Name]Theme{
	"default": {
		Name: "default",
		Slots: map[SlotKey]Slot{
			SlotSelected:    {Light: CompleteColor{TrueColor: "#1A6DA6", ANSI: "6"}, Dark: CompleteColor{TrueColor: "#5FD4FF", ANSI: "14"}},
			SlotDim:         {Light: CompleteColor{TrueColor: "#6A6A6A", ANSI: "8"}, Dark: CompleteColor{TrueColor: "#6A6A6A", ANSI: "8"}},
			SlotError:       {Light: CompleteColor{TrueColor: "#C5002B", ANSI: "9"}, Dark: CompleteColor{TrueColor: "#FF5C5C", ANSI: "9"}},
			SlotUser:        {Light: CompleteColor{TrueColor: "#1A7F37", ANSI: "2"}, Dark: CompleteColor{TrueColor: "#5AF78E", ANSI: "10"}},
			SlotAI:          {Light: CompleteColor{TrueColor: "#6A35D6", ANSI: "5"}, Dark: CompleteColor{TrueColor: "#8A6CF8", ANSI: "13"}},
			SlotHighlightFg: {Light: CompleteColor{TrueColor: "#FFFFFF", ANSI: "7"}, Dark: CompleteColor{TrueColor: "#101014", ANSI: "0"}},
			SlotHighlightBg: {Light: CompleteColor{TrueColor: "#1A6DA6", ANSI: "6"}, Dark: CompleteColor{TrueColor: "#5FD4FF", ANSI: "14"}},
			SlotSuccess:     {Light: CompleteColor{TrueColor: "#1A7F37", ANSI: "2"}, Dark: CompleteColor{TrueColor: "#5AF78E", ANSI: "10"}},
			SlotWarning:     {Light: CompleteColor{TrueColor: "#B58A00", ANSI: "3"}, Dark: CompleteColor{TrueColor: "#F3F99D", ANSI: "11"}},
			SlotBorder:      {Light: CompleteColor{TrueColor: "#6A6A6A", ANSI: "8"}, Dark: CompleteColor{TrueColor: "#3D3D3D", ANSI: "8"}},
			SlotTitle:       {Light: CompleteColor{TrueColor: "#1A6DA6", ANSI: "6"}, Dark: CompleteColor{TrueColor: "#5FD4FF", ANSI: "14"}},
			SlotAccent:      {Light: CompleteColor{TrueColor: "#1A6DA6", ANSI: "6"}, Dark: CompleteColor{TrueColor: "#5FD4FF", ANSI: "14"}},
			SlotMuted:       {Light: CompleteColor{TrueColor: "#9A9A9A", ANSI: "7"}, Dark: CompleteColor{TrueColor: "#4A4A4A", ANSI: "8"}},
		},
	},
	"catppuccin": {
		Name: "catppuccin",
		Slots: map[SlotKey]Slot{
			SlotSelected:    {Light: CompleteColor{TrueColor: "#8839EF", ANSI: "5"}, Dark: CompleteColor{TrueColor: "#CBA6F7", ANSI: "13"}},
			SlotDim:         {Light: CompleteColor{TrueColor: "#6C6F85", ANSI: "8"}, Dark: CompleteColor{TrueColor: "#6C7086", ANSI: "8"}},
			SlotError:       {Light: CompleteColor{TrueColor: "#D20F39", ANSI: "1"}, Dark: CompleteColor{TrueColor: "#F38BA8", ANSI: "9"}},
			SlotUser:        {Light: CompleteColor{TrueColor: "#40A02B", ANSI: "2"}, Dark: CompleteColor{TrueColor: "#A6E3A1", ANSI: "10"}},
			SlotAI:          {Light: CompleteColor{TrueColor: "#8839EF", ANSI: "5"}, Dark: CompleteColor{TrueColor: "#CBA6F7", ANSI: "13"}},
			SlotHighlightFg: {Light: CompleteColor{TrueColor: "#FFFFFF", ANSI: "7"}, Dark: CompleteColor{TrueColor: "#1E1E2E", ANSI: "0"}},
			SlotHighlightBg: {Light: CompleteColor{TrueColor: "#8839EF", ANSI: "5"}, Dark: CompleteColor{TrueColor: "#CBA6F7", ANSI: "13"}},
			SlotSuccess:     {Light: CompleteColor{TrueColor: "#40A02B", ANSI: "2"}, Dark: CompleteColor{TrueColor: "#A6E3A1", ANSI: "10"}},
			SlotWarning:     {Light: CompleteColor{TrueColor: "#DF8E1D", ANSI: "3"}, Dark: CompleteColor{TrueColor: "#FAE3B0", ANSI: "11"}},
			SlotBorder:      {Light: CompleteColor{TrueColor: "#BCC0CC", ANSI: "7"}, Dark: CompleteColor{TrueColor: "#45475A", ANSI: "8"}},
			SlotTitle:       {Light: CompleteColor{TrueColor: "#8839EF", ANSI: "5"}, Dark: CompleteColor{TrueColor: "#CBA6F7", ANSI: "13"}},
			SlotAccent:      {Light: CompleteColor{TrueColor: "#04A5E5", ANSI: "6"}, Dark: CompleteColor{TrueColor: "#89DCEB", ANSI: "14"}},
			SlotMuted:       {Light: CompleteColor{TrueColor: "#8C8FA1", ANSI: "7"}, Dark: CompleteColor{TrueColor: "#585B70", ANSI: "8"}},
		},
	},
	"nord": {
		Name: "nord",
		Slots: map[SlotKey]Slot{
			SlotSelected:    {Light: CompleteColor{TrueColor: "#5E81AC", ANSI: "4"}, Dark: CompleteColor{TrueColor: "#88C0D0", ANSI: "14"}},
			SlotDim:         {Light: CompleteColor{TrueColor: "#6A7A8C", ANSI: "8"}, Dark: CompleteColor{TrueColor: "#4C566A", ANSI: "8"}},
			SlotError:       {Light: CompleteColor{TrueColor: "#BF616A", ANSI: "1"}, Dark: CompleteColor{TrueColor: "#BF616A", ANSI: "9"}},
			SlotUser:        {Light: CompleteColor{TrueColor: "#A3BE8C", ANSI: "2"}, Dark: CompleteColor{TrueColor: "#A3BE8C", ANSI: "10"}},
			SlotAI:          {Light: CompleteColor{TrueColor: "#B48EAD", ANSI: "5"}, Dark: CompleteColor{TrueColor: "#B48EAD", ANSI: "13"}},
			SlotHighlightFg: {Light: CompleteColor{TrueColor: "#FFFFFF", ANSI: "7"}, Dark: CompleteColor{TrueColor: "#2E3440", ANSI: "0"}},
			SlotHighlightBg: {Light: CompleteColor{TrueColor: "#5E81AC", ANSI: "4"}, Dark: CompleteColor{TrueColor: "#88C0D0", ANSI: "14"}},
			SlotSuccess:     {Light: CompleteColor{TrueColor: "#A3BE8C", ANSI: "2"}, Dark: CompleteColor{TrueColor: "#A3BE8C", ANSI: "10"}},
			SlotWarning:     {Light: CompleteColor{TrueColor: "#EBCB8B", ANSI: "3"}, Dark: CompleteColor{TrueColor: "#EBCB8B", ANSI: "11"}},
			SlotBorder:      {Light: CompleteColor{TrueColor: "#D8DEE9", ANSI: "7"}, Dark: CompleteColor{TrueColor: "#4C566A", ANSI: "8"}},
			SlotTitle:       {Light: CompleteColor{TrueColor: "#5E81AC", ANSI: "4"}, Dark: CompleteColor{TrueColor: "#88C0D0", ANSI: "14"}},
			SlotAccent:      {Light: CompleteColor{TrueColor: "#88C0D0", ANSI: "6"}, Dark: CompleteColor{TrueColor: "#88C0D0", ANSI: "14"}},
			SlotMuted:       {Light: CompleteColor{TrueColor: "#8A9BB0", ANSI: "7"}, Dark: CompleteColor{TrueColor: "#3B4252", ANSI: "8"}},
		},
	},
	"dracula": {
		Name: "dracula",
		Slots: map[SlotKey]Slot{
			SlotSelected:    {Light: CompleteColor{TrueColor: "#6272A4", ANSI: "4"}, Dark: CompleteColor{TrueColor: "#BD93F9", ANSI: "13"}},
			SlotDim:         {Light: CompleteColor{TrueColor: "#6A6A72", ANSI: "8"}, Dark: CompleteColor{TrueColor: "#44475A", ANSI: "8"}},
			SlotError:       {Light: CompleteColor{TrueColor: "#FF5555", ANSI: "1"}, Dark: CompleteColor{TrueColor: "#FF5555", ANSI: "9"}},
			SlotUser:        {Light: CompleteColor{TrueColor: "#50FA7B", ANSI: "2"}, Dark: CompleteColor{TrueColor: "#50FA7B", ANSI: "10"}},
			SlotAI:          {Light: CompleteColor{TrueColor: "#BD93F9", ANSI: "5"}, Dark: CompleteColor{TrueColor: "#BD93F9", ANSI: "13"}},
			SlotHighlightFg: {Light: CompleteColor{TrueColor: "#FFFFFF", ANSI: "7"}, Dark: CompleteColor{TrueColor: "#282A36", ANSI: "0"}},
			SlotHighlightBg: {Light: CompleteColor{TrueColor: "#6272A4", ANSI: "4"}, Dark: CompleteColor{TrueColor: "#BD93F9", ANSI: "13"}},
			SlotSuccess:     {Light: CompleteColor{TrueColor: "#50FA7B", ANSI: "2"}, Dark: CompleteColor{TrueColor: "#50FA7B", ANSI: "10"}},
			SlotWarning:     {Light: CompleteColor{TrueColor: "#F1FA8C", ANSI: "3"}, Dark: CompleteColor{TrueColor: "#F1FA8C", ANSI: "11"}},
			SlotBorder:      {Light: CompleteColor{TrueColor: "#BDBDBD", ANSI: "7"}, Dark: CompleteColor{TrueColor: "#44475A", ANSI: "8"}},
			SlotTitle:       {Light: CompleteColor{TrueColor: "#6272A4", ANSI: "4"}, Dark: CompleteColor{TrueColor: "#BD93F9", ANSI: "13"}},
			SlotAccent:      {Light: CompleteColor{TrueColor: "#8BE9FD", ANSI: "6"}, Dark: CompleteColor{TrueColor: "#8BE9FD", ANSI: "14"}},
			SlotMuted:       {Light: CompleteColor{TrueColor: "#8A8A96", ANSI: "7"}, Dark: CompleteColor{TrueColor: "#6272A4", ANSI: "8"}},
		},
	},
	"solarized": {
		Name: "solarized",
		Slots: map[SlotKey]Slot{
			SlotSelected:    {Light: CompleteColor{TrueColor: "#268BD2", ANSI: "4"}, Dark: CompleteColor{TrueColor: "#268BD2", ANSI: "14"}},
			SlotDim:         {Light: CompleteColor{TrueColor: "#93A1A1", ANSI: "7"}, Dark: CompleteColor{TrueColor: "#586E75", ANSI: "8"}},
			SlotError:       {Light: CompleteColor{TrueColor: "#DC322F", ANSI: "1"}, Dark: CompleteColor{TrueColor: "#DC322F", ANSI: "9"}},
			SlotUser:        {Light: CompleteColor{TrueColor: "#859900", ANSI: "2"}, Dark: CompleteColor{TrueColor: "#859900", ANSI: "10"}},
			SlotAI:          {Light: CompleteColor{TrueColor: "#6C71C4", ANSI: "5"}, Dark: CompleteColor{TrueColor: "#6C71C4", ANSI: "13"}},
			SlotHighlightFg: {Light: CompleteColor{TrueColor: "#FFFFFF", ANSI: "7"}, Dark: CompleteColor{TrueColor: "#073642", ANSI: "0"}},
			SlotHighlightBg: {Light: CompleteColor{TrueColor: "#268BD2", ANSI: "4"}, Dark: CompleteColor{TrueColor: "#268BD2", ANSI: "14"}},
			SlotSuccess:     {Light: CompleteColor{TrueColor: "#859900", ANSI: "2"}, Dark: CompleteColor{TrueColor: "#859900", ANSI: "10"}},
			SlotWarning:     {Light: CompleteColor{TrueColor: "#B58900", ANSI: "3"}, Dark: CompleteColor{TrueColor: "#B58900", ANSI: "11"}},
			SlotBorder:      {Light: CompleteColor{TrueColor: "#839496", ANSI: "7"}, Dark: CompleteColor{TrueColor: "#586E75", ANSI: "8"}},
			SlotTitle:       {Light: CompleteColor{TrueColor: "#268BD2", ANSI: "4"}, Dark: CompleteColor{TrueColor: "#268BD2", ANSI: "14"}},
			SlotAccent:      {Light: CompleteColor{TrueColor: "#2AA198", ANSI: "6"}, Dark: CompleteColor{TrueColor: "#2AA198", ANSI: "14"}},
			SlotMuted:       {Light: CompleteColor{TrueColor: "#A0A9A9", ANSI: "7"}, Dark: CompleteColor{TrueColor: "#657B83", ANSI: "8"}},
		},
	},
}

func themeNames() []Name {
	return []Name{"default", "catppuccin", "nord", "dracula", "solarized"}
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./cmd/cli/cmd/tui/theme/ -run 'TestPresetsComplete|TestPresetANSIRequired' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/cli/cmd/tui/theme/presets.go cmd/cli/cmd/tui/theme/presets_test.go
git commit -m "feat(tui): 5套内建主题预设(default/catppuccin/nord/dracula/solarized)"
```

---

### Task 3: hybrid.go — HybridColor 实现 lipgloss.TerminalColor

**Files:**
- Modify: `go.mod`(termenv 转为直接依赖)
- Create: `cmd/cli/cmd/tui/theme/hybrid.go`
- Test: `cmd/cli/cmd/tui/theme/hybrid_test.go`

**Interfaces:**
- Consumes: `CompleteColor`/`Slot`(Task 1)、`lipgloss.TerminalColor`/`lipgloss.NewStyle`/`lipgloss.ColorProfile`/`lipgloss.HasDarkBackground`/`lipgloss.SetColorProfile`/`lipgloss.SetHasDarkBackground`(v1.1.0)、`termenv.Profile` 常量
- Produces: `type HybridColor struct{ Light, Dark CompleteColor }` 实现 `color(*lipgloss.Renderer) termenv.Color` + `RGBA() (uint32,uint32,uint32,uint32)`;`func hybridColor(s Slot) HybridColor`

- [ ] **Step 1: 确保 termenv 为直接依赖**

Run: `go get github.com/muesli/termenv@v0.16.0`
Expected: `go.mod` 中该行从 `// indirect` 变为直接依赖(无注释)。随后运行 `go mod tidy` 后确认无其它变动。

- [ ] **Step 2: 写失败测试**

`cmd/cli/cmd/tui/theme/hybrid_test.go`:

```go
package theme

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
)

func render(profile termenv.Profile, dark bool) string {
	lipgloss.SetColorProfile(profile)
	lipgloss.SetHasDarkBackground(dark)
	hc := HybridColor{
		Light: CompleteColor{TrueColor: "#5FD4FF", ANSI256: "75", ANSI: "14"},
		Dark:  CompleteColor{TrueColor: "#864EFF", ANSI256: "99", ANSI: "13"},
	}
	return lipgloss.NewStyle().Foreground(hc).Render("x")
}

func TestHybridColorTrueColor(t *testing.T) {
	out := render(termenv.TrueColor, true)
	assert.Contains(t, out, "38;2;134;78;255", "truecolor+暗背景应取 Dark.TrueColor")
	out = render(termenv.TrueColor, false)
	assert.Contains(t, out, "38;2;95;212;255", "truecolor+亮背景应取 Light.TrueColor")
}

func TestHybridColorANSI256(t *testing.T) {
	out := render(termenv.ANSI256, true)
	assert.Contains(t, out, "38;5;99", "256+暗背景应取 Dark.ANSI256")
	out = render(termenv.ANSI256, false)
	assert.Contains(t, out, "38;5;75", "256+亮背景应取 Light.ANSI256")
}

func TestHybridColorANSI(t *testing.T) {
	out := render(termenv.ANSI, true)
	assert.Contains(t, out, "38;5;13", "16色+暗背景应取 Dark.ANSI")
	out = render(termenv.ANSI, false)
	assert.Contains(t, out, "38;5;14", "16色+亮背景应取 Light.ANSI")
}

func TestHybridColor256Fallback(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	lipgloss.SetHasDarkBackground(true)
	hc := HybridColor{
		Light: CompleteColor{TrueColor: "#5FD4FF", ANSI256: "", ANSI: "14"},
		Dark:  CompleteColor{TrueColor: "#864EFF", ANSI256: "", ANSI: "13"},
	}
	out := lipgloss.NewStyle().Foreground(hc).Render("x")
	assert.Contains(t, out, "38;5;", "ANSI256 缺省时由 hex 自动降级")
}

func TestHybridColorImplementsInterface(t *testing.T) {
	var _ lipgloss.TerminalColor = HybridColor{}
}
```

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./cmd/cli/cmd/tui/theme/ -run TestHybridColor -v`
Expected: FAIL(HybridColor 未定义)

- [ ] **Step 4: 实现 hybrid.go**

`cmd/cli/cmd/tui/theme/hybrid.go`:

```go
package theme

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

type HybridColor struct {
	Light CompleteColor
	Dark  CompleteColor
}

func hybridColor(s Slot) HybridColor { return HybridColor{Light: s.Light, Dark: s.Dark} }

// color 实现 lipgloss.TerminalColor。注意:使用传入的 Renderer 方法
// (r.ColorProfile()/r.HasDarkBackground()),与 v1.1.0 的 ssh 示例一致。
func (c HybridColor) color(r *lipgloss.Renderer) termenv.Color {
	p := r.ColorProfile()
	cc := c.Light
	if r.HasDarkBackground() {
		cc = c.Dark
	}
	switch p {
	case termenv.TrueColor:
		return p.Color(cc.TrueColor)
	case termenv.ANSI256:
		if cc.ANSI256 != "" {
			return p.Color(cc.ANSI256)
		}
		return p.Color(cc.TrueColor)
	case termenv.ANSI:
		return p.Color(cc.ANSI)
	default:
		return termenv.NoColor{}
	}
}

func (c HybridColor) RGBA() (r, g, b, a uint32) {
	return lipgloss.Color(c.Light.TrueColor).RGBA()
}
```

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./cmd/cli/cmd/tui/theme/ -run TestHybridColor -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum cmd/cli/cmd/tui/theme/hybrid.go cmd/cli/cmd/tui/theme/hybrid_test.go
git commit -m "feat(tui): HybridColor 实现三级色域+明暗自适应降级"
```

---

### Task 4: theme.go 新增工厂 API(保留旧 API 共存)

**Files:**
- Modify: `cmd/cli/cmd/tui/theme/theme.go`(末尾追加新函数,不删旧函数)
- Modify: `cmd/cli/cmd/tui/theme/theme_test.go`(追加新测试,不删旧测试)

**Interfaces:**
- Consumes: `Name`/`SlotKey`/`Theme`/`presets`/`themeNames()`(Task 1-2)、`HybridColor`/`hybridColor`(Task 3)、旧 `theme.go` 中已有的 `EnvTheme` 常量
- Produces: `const DefaultTheme Name = "catppuccin"`、`func parseThemeName(s string) Name`、`func Current() Name`、`func Color(key SlotKey) lipgloss.TerminalColor`、`func Style(key SlotKey) lipgloss.Style`、`func Title(text string) string`

注意:本任务**不删除**旧 API(`Fg`/`palettes`/`ThemeANSI`/`ThemeTrueColor`/`C*`/`IsTrueColor` 等),保证所有 view 与 `tui.go` 继续编译。旧 `EnvTheme` 常量名保留复用。

- [ ] **Step 1: 写失败测试**

`cmd/cli/cmd/tui/theme/theme_test.go` 末尾追加:

```go
func TestParseThemeName(t *testing.T) {
	cases := []struct {
		in   string
		want Name
	}{
		{"", DefaultTheme},
		{"catppuccin", "catppuccin"},
		{"CATPPUCCIN", "catppuccin"},
		{"nord", "nord"},
		{"dracula", "dracula"},
		{"solarized", "solarized"},
		{"default", "default"},
		{"bogus", DefaultTheme},
		{"ansi", DefaultTheme},
		{"truecolor", DefaultTheme},
		{" ", DefaultTheme},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			assert.Equal(t, c.want, parseThemeName(c.in))
		})
	}
}

func TestThemeNamesRegistered(t *testing.T) {
	for _, n := range themeNames() {
		_, ok := presets[n]
		assert.True(t, ok, "主题 %q 已注册", n)
	}
}

func TestColorReturnsColor(t *testing.T) {
	c := Color(SlotSelected)
	assert.NotNil(t, c, "Color 应返回非 nil TerminalColor")
}

func TestStyleHasColor(t *testing.T) {
	got := Style(SlotSelected).Render("x")
	assert.NotEmpty(t, got)
}

func TestTitleBold(t *testing.T) {
	got := Title("标题")
	assert.Contains(t, got, "标题")
}

func TestInvalidEnvFallsBackToDefault(t *testing.T) {
	t.Setenv(EnvTheme, "bogus")
	assert.Equal(t, DefaultTheme, Current())
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./cmd/cli/cmd/tui/theme/ -run TestParseThemeName -v`
Expected: FAIL(parseThemeName/DefaultTheme 未定义)

- [ ] **Step 3: 追加新 API 到 theme.go**

在 `cmd/cli/cmd/tui/theme/theme.go` 文件末尾(保留现有全部旧代码)追加:

```go
const DefaultTheme Name = "catppuccin"

func parseThemeName(s string) Name {
	n := strings.ToLower(strings.TrimSpace(s))
	if n == "" {
		return DefaultTheme
	}
	if _, ok := presets[Name(n)]; ok {
		return Name(n)
	}
	return DefaultTheme
}

func Current() Name {
	return parseThemeName(os.Getenv(EnvTheme))
}

func Color(key SlotKey) lipgloss.TerminalColor {
	t := presets[Current()]
	if s, ok := t.Slots[key]; ok {
		return hybridColor(s)
	}
	return lipgloss.NoColor{}
}

func Style(key SlotKey) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(Color(key))
}

func Title(text string) string {
	return Style(SlotTitle).Bold(true).Render(text)
}
```

检查 `theme.go` 顶部 import 是否已含 `"strings"`(旧代码因 `parseEnv` 已 import,若没有则补)。`lipgloss` 与 `os` 已 import。

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./cmd/cli/cmd/tui/theme/ -v`
Expected: 全部 PASS(含旧测试与新测试)

Run: `go build ./cmd/cli/...`
Expected: 编译成功(旧 API 仍存在,view 与 tui.go 未破坏)

- [ ] **Step 5: Commit**

```bash
git add cmd/cli/cmd/tui/theme/theme.go cmd/cli/cmd/tui/theme/theme_test.go
git commit -m "feat(tui): 主题工厂API(Color/Style/Title)与默认catppuccin"
```

---

### Task 5: 迁移消费方 — nodes/exec/ai/file/app

**Files:**
- Modify: `cmd/cli/cmd/tui/nodes/view.go:16-18,96-97`
- Modify: `cmd/cli/cmd/tui/exec/view.go:14-16`
- Modify: `cmd/cli/cmd/tui/ai/view.go:13-16`
- Modify: `cmd/cli/cmd/tui/file/view.go:13-15`
- Modify: `cmd/cli/cmd/tui/app.go:245-246`
- Test: 各包现有测试(`nodes/view_test.go`、`exec/exec_test.go`、`ai/view_test.go`、`file/view_test.go`、`app_test.go`)

**Interfaces:**
- Consumes: `theme.Color`/`theme.Style`/`theme.Title`(Task 4)
- Produces: 各 view 使用 `theme.Style(key)`;highlight 组合用 `theme.Color(key)`;menuBar 用 `theme.Style(theme.SlotSelected)`+`theme.Style(theme.SlotDim)`

- [ ] **Step 1: 迁移 nodes/view.go**

`cmd/cli/cmd/tui/nodes/view.go:16-18` 替换为:

```go
var (
	styleListBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	styleDetail     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	styleError      = theme.Style(theme.SlotError)
	styleDim        = theme.Style(theme.SlotDim)
	styleSelected   = theme.Style(theme.SlotSelected)
)
```

`view.go:96-97` 替换为:

```go
		detailBox = lipgloss.NewStyle().
			Foreground(theme.Color(theme.SlotHighlightFg)).
			Background(theme.Color(theme.SlotHighlightBg)).
			Render(m.detailPane())
```

确认 `nodes/view.go` 顶部 `theme` import 仍被使用(其余 `theme.Style` 调用需要它)。

- [ ] **Step 2: 迁移 exec/ai/file 的 view.go**

`cmd/cli/cmd/tui/exec/view.go:14-16`:

```go
	styleSelected = theme.Style(theme.SlotSelected)
	styleDim      = theme.Style(theme.SlotDim)
	styleError    = theme.Style(theme.SlotError)
```

`cmd/cli/cmd/tui/ai/view.go:13-16`:

```go
	styleUser  = theme.Style(theme.SlotUser)
	styleAI    = theme.Style(theme.SlotAI)
	styleDim   = theme.Style(theme.SlotDim)
	styleError = theme.Style(theme.SlotError)
```

`cmd/cli/cmd/tui/file/view.go:13-15`:

```go
	styleSelected = theme.Style(theme.SlotSelected)
	styleDim      = theme.Style(theme.SlotDim)
	styleError    = theme.Style(theme.SlotError)
```

各文件确认 `lipgloss` import:若替换后该文件不再使用 `lipgloss`(如 exec/ai/file 的 view.go 只剩 styleXxx),需删除 `"github.com/charmbracelet/lipgloss"` import;若仍使用(如 `lipgloss.NewStyle()` 边框)则保留。

- [ ] **Step 3: 迁移 app.go menuBar**

`cmd/cli/cmd/tui/app.go:245-246` 替换为:

```go
	activeStyle := theme.Style(theme.SlotSelected)
	dim := theme.Style(theme.SlotDim)
```

- [ ] **Step 4: 跑全 TUI 测试**

Run: `go test ./cmd/cli/cmd/tui/...`
Expected: 全部 PASS(已确认现有测试不断言具体 ANSI 色码,不需更新快照)

- [ ] **Step 5: Commit**

```bash
git add cmd/cli/cmd/tui/nodes/view.go cmd/cli/cmd/tui/exec/view.go cmd/cli/cmd/tui/ai/view.go cmd/cli/cmd/tui/file/view.go cmd/cli/cmd/tui/app.go
git commit -m "feat(tui): view组件迁移至主题工厂API(theme.Style/Color)"
```

---

### Task 6: 删除旧 API + 降级提示 + 文档

**Files:**
- Modify: `cmd/cli/cmd/tui/theme/theme.go`(删除旧 API)
- Modify: `cmd/cli/cmd/tui/theme/theme_test.go`(删除旧测试)
- Modify: `cmd/cli/cmd/tui/tui.go:40-44`
- Modify: `README.md`、`USAGE.md`(OWL_TUI_THEME 文档)

**Interfaces:**
- Consumes: `theme.Current()`/`theme.DefaultTheme`/`theme.EnvTheme`(Task 4)
- Produces: 清理后的 theme 包;更新后的 `runTui` 提示逻辑;README/USAGE 环境变量文档

- [ ] **Step 1: 删除旧 API**

`cmd/cli/cmd/tui/theme/theme.go` 删除以下旧内容(保留 `EnvTheme` 常量与 Task 4 新增部分):

- `palettes` map
- `CSelected` 等旧常量(`CDim`/`CError`/`CUser`/`CAI`/`CHighlightFg`/`CHighlightBg`)
- `current`/`requested`/`downgraded` 变量与 `initTheme`
- `parseEnv`/`resolveTheme`/`terminalTrueColorCapable`
- `IsTrueColor`/`RequestedTrueColor`/`DowngradedToANSI`
- `fg`/`Fg`

删除后 import 若不再需要 `"strings"`/`"os"`/`"lipgloss"` 需调整(新代码使用 `strings`/`os`/`lipgloss`,保留)。

同步删除 `theme_test.go` 中引用旧符号的测试:`TestParseEnv`/`TestFgANSI`/`TestFgTrueColor`/`TestFgUnknownName`/`TestResolveTheme`/`TestTerminalTrueColorCapable`。

- [ ] **Step 2: 更新 tui.go 提示逻辑**

`cmd/cli/cmd/tui/tui.go:40-44` 替换为:

```go
	// 请求了主题但环境变量值不可识别时回退默认并提示
	if name := os.Getenv(theme.EnvTheme); name != "" && theme.Current() == theme.DefaultTheme && strings.ToLower(strings.TrimSpace(name)) != string(theme.DefaultTheme) {
		fmt.Fprintln(os.Stderr, "提示: OWL_TUI_THEME=\""+name+"\" 不是可用主题,已回退默认(catppuccin)。")
		fmt.Fprintln(os.Stderr, "      可用: default / catppuccin / nord / dracula / solarized")
	}
```

`tui.go` 顶部 import 增加 `"strings"`(当前已有 `fmt`/`os`)。

- [ ] **Step 3: 全量测试确认**

Run: `go test ./cmd/cli/cmd/tui/...`
Expected: PASS
Run: `go build ./cmd/cli/...`
Expected: 成功

- [ ] **Step 4: 更新 README.md**

Run: `grep -n "OWL_TUI_THEME" README.md USAGE.md`
若 README 有该段落,替换为:

```markdown
`OWL_TUI_THEME` 选择主题:`default` / `catppuccin` / `nord` / `dracula` / `solarized`(默认 `catppuccin`)。
主题在 TrueColor/256/ANSI 色域自动降级,并按终端明暗背景自适应。
```

若 README 无此段落,在 TUI 使用说明处补上。

- [ ] **Step 5: 更新 USAGE.md**

`USAGE.md` 中 OWL_TUI_THEME 段落替换为:

```markdown
`OWL_TUI_THEME` 主题名:`default|catppuccin|nord|dracula|solarized`(默认 catppuccin)。
```

- [ ] **Step 6: 构建 + E2E 冒烟**

Run: `go build ./...`
Expected: 成功

按 AGENTS.md 冒烟流程:
```bash
./build/owl-serve --reset-admin --port 8080
```
启动后按 AGENTS.md 用 seed 接口造 50 节点,进入 `owl tui` 验证配色正常显示(无头环境则仅确认程序可启动退出)。

- [ ] **Step 7: Commit**

```bash
git add cmd/cli/cmd/tui/theme/theme.go cmd/cli/cmd/tui/theme/theme_test.go cmd/cli/cmd/tui/tui.go README.md USAGE.md
git commit -m "feat(tui): 删除旧主题API+降级提示+环境变量文档更新"
```

---

### Task 7: 全量回归 + 收尾

**Files:**
- Test: 整个 `./cmd/...` 与 `./internal/...` 相关测试

**Interfaces:**
- Consumes: 全部已完成任务
- Produces: 无新 API

- [ ] **Step 1: 全量测试**

Run: `go test ./...`
Expected: PASS(duckdb/arrow 等重型依赖测试可能较慢;若个别包与本次改动无关且历史即失败,记录并跳过)

- [ ] **Step 2: go vet**

Run: `go vet ./cmd/cli/cmd/tui/...`
Expected: 无输出

- [ ] **Step 3: 确认无残留旧 API**

Run: `grep -rn "theme.Fg\|theme.CSelected\|ThemeANSI\|ThemeTrueColor\|DowngradedToANSI\|RequestedTrueColor" cmd/ --include="*.go"`
Expected: 无匹配

- [ ] **Step 4: 更新 spec 实现状态**

在 `docs/superpowers/specs/2026-08-15-owl-tui-theme-system-design.md` 末尾追加:

```markdown
## 实现状态
- [x] 已完成(2026-08-15),计划见 docs/superpowers/plans/2026-08-15-owl-tui-theme-system.md
```

- [ ] **Step 5: Commit**

```bash
git add docs/superpowers/specs/2026-08-15-owl-tui-theme-system-design.md
git commit -m "docs(tui): 标记主题系统设计已实现"
```
