# owl tui 主题系统升级 — 主题级配色设计

日期:2026-08-15
分支:owl-tui

## 目标

把 owl TUI 的配色从"2 档 7 色"(ANSI 16 色 / TrueColor hex)升级为**主题级配色**:5 套内置预设 + 三级色域降级(ANSI→256→TrueColor)+ 明暗自适应(AdaptiveColor)。一次性全量重构所有 view 消费新 API。

## 背景与约束

- 现状:`cmd/cli/cmd/tui/theme/theme.go` 仅 7 个语义色槽 × 2 档调色板,全是前景色;`OWL_TUI_THEME` 只接受 `ansi|truecolor` 两个值。
- lipgloss **v1.1.0 已具备**方案 C 所需全部能力(`AdaptiveColor`/`CompleteColor`/`CompleteAdaptiveColor`/`ColorProfile`/`HasDarkBackground`),**不升级到 v2**。
- 升级 v2 被否决:模块路径全换(`charm.land/lipgloss/v2`),`tea.KeyMsg`→`KeyPressMsg`(项目 66 处)、`View()` 返回类型、bubbles 连带升级,成本与配色目标不成比例。留作独立后续任务。
- 参考示例(lipgloss v2 版)已复制至 `lipgloss_examples/`,仅作 API 设计参考,不用于依赖升级。

## 决策(已与用户确认)

- **预设**:5 套 —— `default`(现 cyan 系)、`catppuccin`(深色,mocha 风味)、`nord`(深色)、`dracula`(深色)、`solarized`(浅色系)
- **配置**:仅 `OWL_TUI_THEME` 环境变量,只认主题名;废弃 `ansi`/`truecolor` 旧值
- **槽位**:颜色槽扩充至 14 个语义色(见下)
- **色域**:ANSI→256→TrueColor 三级降级;**ANSI 16 色档必填**(老系统可读性),256 档可选覆盖(缺省自动降级)
- **明暗**:预设单套,明暗两组色值由 `HasDarkBackground()` 自动取用(AdaptiveColor 语义)
- **默认主题**:`catppuccin` 作为默认风格
- **重构方式**:一次性全量迁移所有 view,不留旧 API

## 语义槽位(14 个)

```
selected     选中项/当前菜单        success   操作成功/在线状态
dim          次要文本/状态栏提示    warning   警告/离线状态
error        错误/危险操作          border    边框
user         用户消息(AI 面板)     title     标题/表头
ai           AI 消息               accent    强调/链接
highlightFg  高亮前景             muted     最弱文本
highlightBg  高亮背景
```

## 数据模型(方案 C 混合)

每槽 = `Slot{ Light, Dark CompleteColor }`,其中 `CompleteColor{ TrueColor, ANSI256, ANSI }`:

- **TrueColor**:hex 主值(`#RRGGBB`),必填
- **ANSI256**:xterm-256 索引(如 `"75"`),可选;缺省 = 由 hex 自动降级
- **ANSI**:16 色档(如 `"14"`),**必填**(老系统可读性保证)

```go
type Name string // "catppuccin" | "nord" | "dracula" | "solarized" | "default"

type CompleteColor struct {
    TrueColor string // hex  #RRGGBB
    ANSI256   string // "" = 自动降级
    ANSI      string // 必填, 16 色
}
type Slot struct {
    Light CompleteColor // 亮背景
    Dark  CompleteColor // 暗背景
}
type Theme struct {
    Name  Name
    Slots map[SlotKey]Slot
}
```

明暗用 `lipgloss.CompleteAdaptiveColor` 语义表达,运行时由 `HasDarkBackground()` 决定取 Light/Dark 组。

## 探测与降级

- **色域探测**:`lipgloss.ColorProfile()` 返回 TrueColor / ANSI256 / ANSI(替代现有手写 `terminalTrueColorCapable()`)
- **背景探测**:`lipgloss.HasDarkBackground(stdin, stdout)`;失败时按暗背景处理(Dark 组)
- **降级链**:自定义 `HybridColor` 实现 `lipgloss.TerminalColor` 接口:按 profile 取 TrueColor/ANSI256/ANSI 档;ANSI256 缺省时回落到 hex 自动降级;ANSI 恒有值(必填)
- 不依赖 `CompleteColor` 直接使用,因其一旦启用即关闭自动降级、空字段渲染为黑色;需 `HybridColor` 提供"缺省自动降级"语义

## 环境变量契约

`OWL_TUI_THEME` 取值:

| 值 | 行为 |
|---|---|
| `catppuccin`/`nord`/`dracula`/`solarized`/`default` | 选对应预设 |
| 未设置 | 回退 `catppuccin`(默认) |
| 未知值 | 回退 `catppuccin`,stderr 打印提示(复用 `tui.go:42` 降级提示模式) |
| `ansi`/`truecolor`(旧值) | 废弃,按未知值处理回退默认 |

主题名合法但终端色域受限 → 该主题在低色域档渲染(不降级到别的主题)。

## 工厂 API

```go
// theme 包新增
func Color(key SlotKey) lipgloss.TerminalColor // 按当前主题+探测返回色(含明暗/降级)
func Style(key SlotKey) lipgloss.Style         // 语义样式工厂
func Title(text string) string                 // 标题样式渲染(粗体 + title 色)
```

迁移范围(grep 已定位):
- `nodes/view.go` — styleError/styleDim/styleSelected + detailBox highlight(Fg+Bg 组合)
- `exec/view.go` — styleSelected/styleDim/styleError
- `ai/view.go` — styleUser/styleAI/styleDim/styleError
- `file/view.go` — styleSelected/styleDim/styleError
- `app.go:245` — menuBar activeStyle/dim

每个 `styleXxx` 由 `lipgloss.NewStyle().Foreground(theme.Fg(theme.CXxx))` 改为 `theme.Style(theme.Xxx)`;`detailBox` 高亮用 Color 工厂组合。

## 架构

```
cmd/cli/cmd/tui/theme/
  theme.go        # 主题注册表:Name 解析、当前主题、探测结果缓存
  types.go        # Theme / Slot / CompleteColor 结构 + SlotKey 语义常量
  presets.go      # 5 套内建预设(每套 14 槽 × Light/Dark)
  hybrid.go       # HybridColor:实现 lipgloss.TerminalColor(hex主值+可选256/16覆盖)
  resolve.go      # ColorProfile / HasDarkBackground 探测 + 缓存 + 降级判定
  resolve_test.go
  theme_test.go   # 预设完整性 + 降级 + 明暗 测试(重写现有)
```

外部消费方只依赖 `theme.Color(key)` / `theme.Style(key)` / `theme.Title()`,不感知内部结构。

## 错误处理

- **未知主题名**:回退 `catppuccin`,stderr 打印一条提示,不中断启动
- **预设数据缺失**(如某槽漏写 ANSI 档):编译期不可见,靠测试兜底——每主题每槽断言 ANSI 非空;运行时取不到返回 `lipgloss.NoColor{}` 而非 panic
- **探测失败**:`HasDarkBackground` 无法判定时按暗背景处理(Dark 组)

## 测试

- `theme_test.go` 重写:
  - 每主题 × 每槽:色值齐全、ANSI 必填非空、hex 格式 `#RRGGBB` 合法
  - 未知主题名 → 回退默认
  - 降级测试:模拟 ANSI/256/TrueColor profile,断言取到对应档值(256 缺省时取 hex 自动降级)
  - 明暗测试:模拟深/亮背景,断言取对 Light/Dark 组
- 现有 view 测试保持绿:渲染结果因色码变化,快照断言需更新

## 迁移顺序

1. **theme 包重写**(types/presets/hybrid/resolve + 测试)——纯新增,不动消费方
2. **工厂 API 落地**(Color/Style/Title)——兼容 `Fg()`
3. **逐文件迁移**,每步跑该包测试:
   - `nodes/view.go`(最复杂,含 highlight 组合)
   - `exec/view.go` → `ai/view.go` → `file/view.go`(同构)
   - `app.go` menuBar(最后)
4. **删除旧 API**(`Fg`/`ThemeANSI`/`ThemeTrueColor`/`C*` 常量),跑全 TUI 测试 + E2E 冒烟
5. 更新 `tui.go` 降级提示文案与 README/USAGE 的 `OWL_TUI_THEME` 文档

## 参考

- `lipgloss_examples/color/standalone` — v2 明暗探测 + 自适应示例
- `lipgloss_examples/compat/standalone` — `AdaptiveColor` 用法
- `lipgloss_examples/color/bubbletea` — bubbletea 下 `BackgroundColorMsg` + `View()` 新签名(v2,仅参考不采用)

## 实现状态
- [x] 已完成(2026-08-15),计划见 docs/superpowers/plans/2026-08-15-owl-tui-theme-system.md
