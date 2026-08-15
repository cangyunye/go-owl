# owl tui 节点字段配色 + Labels 彩虹 设计

日期:2026-08-15
分支:owl-tui

## 目标

在已落地的主题系统(5 预设 + 三级色域 + 明暗自适应)之上,为节点列表与详情面板增加字段级配色:Status 按值配色、User/Address/Groups 按字段配色、Labels 彩虹配色。列表列与详情面板都着色。

## 背景

- 现状 `nodes/list.go` 的 `cellValue(n, key)` 返回纯文本,`listPane` 仅选中行整体 `styleSelected` 着色;`nodes/view.go` 的 `detailPane` 纯文本,无字段着色。
- `Status` 是枚举:`model.NodeStatusOnline="online"` / `NodeStatusOffline="offline"` / `NodeStatusUnknown="unknown"`。
- `Labels` 是 `map[string]string`,`sortedLabels` 按 key 排序拼成 `k=v,k=v`。
- 现有 13 语义槽全部可复用,不新增槽位。

## 决策(已与用户确认)

- **Status**:online→SlotSuccess(绿)/ offline→SlotError(红)/ unknown→SlotWarning(黄),按值配色,详情 + 列表列
- **User/Address/Groups**:User→SlotUser、Address→SlotAccent、Groups→SlotTitle(复用现有槽)
- **Labels 彩虹**:新增 8 色彩环,按 label key 哈希取色;key 用彩虹色,`=`+value 用 dim;TrueColor/256/ANSI 三档齐全,**低色域不劣化**(ANSI 优先)
- **范围**:详情面板 + 列表表格列都着色;选中行保持整体 `styleSelected` 高亮(覆盖列色,保可读性)
- **截断策略**:宽度感知彩虹——按可见宽度(`common.DisplayWidth`)逐 label 预算,能完整放下才彩虹着色,放不下用 `…` 省略;ANSI 转义码不占显示宽度,列宽计算不破坏对齐

## 彩虹色环

`cmd/cli/cmd/tui/theme/rainbow.go`,8 色 × 三档(catppuccin 系色,深浅背景均适读):

```
idx  TrueColor    256    ANSI   色名
0    #F38BA8      204    9      粉红
1    #FAB387      215    11     橙
2    #F9E2AF      223    3      黄
3    #A6E3A1      150    10     绿
4    #94E2D5      116    14     青
5    #89DCEB      117    6      湖蓝
6    #CBA6F7      183    13     紫
7    #B4BEFE      147    12     蓝
```

- 哈希:`hash/fnv` FNV-1a 32 位,对 label key 取 `Sum32() % 8`
- API:`theme.Rainbow(key string) lipgloss.TerminalColor` 返回 `HybridColor`(复现 Task 3 的 HybridColor 三档降级机制,Light/Dark 用同色)
- 色环每色可视为 `Slot{Light: c, Dark: c}`,直接喂 `hybridColor()`

## 详情面板改动(nodes/view.go `detailPane`)

`rows` 循环的 value 渲染改为按字段着色:

```go
// 新增(值着色规则)
func coloredValue(key, val string) string {
    switch key {
    case "Status":  return styleForStatus(val).Render(val)
    case "Labels":  return rainbowLabelsFull(val) // 详情,无列宽限制
    case "Address": return theme.Style(theme.SlotAccent).Render(val)
    case "User":    return theme.Style(theme.SlotUser).Render(val)
    case "Groups":  return theme.Style(theme.SlotTitle).Render(val)
    }
    return val
}

func styleForStatus(s string) lipgloss.Style {
    switch s {
    case string(model.NodeStatusOnline):  return theme.Style(theme.SlotSuccess)
    case string(model.NodeStatusOffline): return theme.Style(theme.SlotError)
    default:                              return theme.Style(theme.SlotWarning)
    }
}
```

- 空值占位符 `"—"` 保持默认色(不套字段色)
- 详情面板 Labels 无列宽限制,`rainbowLabelsFull` 直接按排序逐 label 着色:`Rainbow(k)` 渲染 key + `styleDim` 渲染 `=value`

## 列表列改动(nodes/list.go + view.go)

- 新增 `renderCell(n, key, width, selected) string`(在 `listPane` 中替换 `truncateCell(cellValue(...), w)`):
  - 先 `truncateCell` 到列宽,若 `selected` 整格 `styleSelected`
  - 否则按 key 着色(Status→按值、Labels→宽度感知彩虹、User/Address/Groups→对应槽)
- **宽度感知彩虹**(列表列):`rainbowLabelsWidth(raw string, width int)` 接收已截断到列宽的纯文本 `raw`;重新解析出排序后的 label 列表,按可见宽度逐个预算,能完整放入当前剩余宽度才追加 `Rainbow(k)`+dim,否则以 `…` 结束。保证不把 ANSI 码切断。
- 表头 `styleSelected`、分隔线、选中行逻辑不变

## 错误处理

- `rainbowLabels` 遇到非 `k=v` 格式(异常数据)整体回退 dim,不 panic
- 空 Labels(`""`)原样返回
- 哈希碰撞只影响颜色分布,无正确性问题

## 测试

- `theme/rainbow_test.go`:哈希确定性(同 key 同色)、色环索引 0-7 范围、8 色三档齐全(hex/256/ANSI 非空)、`Rainbow` 返回非 nil
- `nodes` 包:`coloredValue` 各 key 着色、`styleForStatus` 三态映射、`renderCell` 选中/非选中、宽度感知彩虹(窄列省略号、ANSI 码不切断、列宽对齐用 DisplayWidth)
- 现有 `nodes/*_test.go` 保持绿(若快照断言了纯文本 Labels 列,更新为彩虹渲染后的期望)
- 全量:`go test ./cmd/cli/cmd/tui/...`

## 范围外

- 不新增语义槽(复用现有 13 槽 + 色环)
- 不改 AI 聊天/Exec/File 面板(仅 nodes 模块)
- 不升级 lipgloss v2(延续上一主题决策)
