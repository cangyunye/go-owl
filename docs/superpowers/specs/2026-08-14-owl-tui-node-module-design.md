# owl tui 重构 — Node 管理模块(Phase 1)设计

日期:2026-08-14
分支:owl-tui

## 目标

完全重构 `owl tui`:废弃转发外部 go-owl-tui 二进制的壳子,在 go-owl 主仓库内实现原生全屏 TUI。Phase 1 只做 **Node 管理模块**,覆盖 `owl node list/add/remove/update` 四个子命令的交互。解决旧 TUI 的三大痛点:**导航/层级混乱、表单编辑难用、快捷键无模式隔离**(任何输入框输入时全局键冲突触发其他操作)。

## 决策(已与用户确认)

- **技术栈**:charmbracelet/bubbletea v1 + bubbles + lipgloss(Elm 架构,旧 TUI 同栈,团队有经验)。go-owl 主仓库新增该依赖
- **入口形态**:`owl tui` 为全屏应用,Phase 1 仅含 Node 模块;后续模块(Exec/File 等)再扩展
- **交互模型**:路径栈(Path Stack)定位层级 + 按键组隔离;箭头键做方向处理(弃用 j/k);`e` = 编辑
- **列表布局**:双栏 —— 左侧节点列表(可勾选列),右侧选中节点详情(含 Labels/Groups/ProxyJump 等全字段)
- **行过滤 vs 列配置正交**:`/` 管行(groups/labels 查询串),`c` 管列(列选择器勾选)
- **表单**:模态弹窗覆盖层,导航态/输入态两态硬隔离,字段首尾回卷
- **删除**:不占表单,单独确认视图

## 架构

```
cmd/cli/cmd/tui/
  tui.go            # owl tui 入口,启动 bubbletea Program,退出码透传
  app.go            # App 顶层 model:路径栈、Mode、模块路由(Phase 1 仅 nodes)
  keys.go           # 单一按键路由入口:先判 Mode,再按位置 keymap 分发
  nodes/
    model.go        # Node 模块 model:位置栈(list/new/:id/edit/:id/delete/columns)
    list.go         # 双栏列表(左表格 + 右详情)+ 过滤 chips 状态栏
    form.go         # add/edit 模态表单(导航态/输入态)
    confirm.go      # delete 确认视图
    columns.go      # 列选择器视图
    keymap.go       # 各位置 keymap 定义 + 上下文帮助数据
    styles.go       # lipgloss 样式
```

**位置栈(Path Stack)** 三重职责:上下文 / keymap 激活组 / 面包屑显示。

```
Path Stack(仿文件系统)
  /                    根(Phase 1 启动直接进入 /nodes)
  /nodes               节点列表(双栏)
  /nodes/new           add 表单(模态弹窗)
  /nodes/:id/edit      edit 表单(模态弹窗)
  /nodes/:id/delete    删除确认
  /nodes/columns       列选择器
```

顶栏面包屑实时显示当前路径(如 `/nodes/192.168.1.10/edit`),`Esc` 弹栈返回,`q` 退出。

## 模式隔离(核心)

两个正交维度,类 vim 的 buffer + 模式:

- **Path** → 决定激活哪组快捷键(切换菜单/层级/编辑栏 = push/pop 路径)
- **Mode(Normal/Insert)** → 决定按键是命令还是输入。`handleKey` 入口**最先判 Mode**:Insert 下任何键只进当前输入框,不经过任何位置 keymap;`Esc` 是唯一退出键

无全局 Tab/字母拦截。各位置 keymap 仅在 Normal 模式生效。

## 列表页 `/nodes`

- 左栏:节点表格,右栏:选中节点详情(全字段,Labels/Groups 排序展示)
- **方向键**:↑/↓ 移动选中,←/→ 切换左右栏焦点
- `g`/`G` 首尾跳转
- `a` 添加 · `e` 编辑 · `d` 删除 · `c` 列配置 · `?` 上下文帮助 · `q` 退出
- 数据源 `common.NodeStore.List()`,复用 `filterNodes` 的过滤语义

### 行过滤 `/`(查询串)

`/` 打开底部过滤输入框,**自动进入 Insert**;语法与 CLI flag 语义一一对应:

- `g:web,db` → 按组过滤(等价 `--groups web,db`)
- `l:env=prod,os=debian` → 按标签过滤(等价 `--label env=prod --label os=debian`)
- 裸文本 `foo` → 快速搜索(ID/Name/Address 模糊匹配)
- 空格分隔多 token = **AND 叠加**
- 输入即过滤(live);`Enter` 应用回 Normal,`Esc` 取消/清空
- 底部状态栏显示生效过滤 chips:`[g:web] [l:env=prod]`

### 列配置 `c`(勾选)

`/nodes/columns`:`↑/↓` 移动、`Space`/`Enter` 切换、`A` 全选、`R` 重置默认、`Enter` 应用弹栈、`Esc` 取消。勾选顺序 = 列显示顺序,实时生效。字段集与 `FieldWidthMap` 一致(id/name/address/port/user/status/groups/labels/last_check/metadata)。

- 勾选集序列化为 `id,name,address,...` 字符串,等价 CLI `--header`
- TUI 列宽**自动适配**(按勾选集合 + 终端宽度),不复用 CLI 固定字符宽 `:width` 语法
- 默认列集:`id, name, address, status`(双栏下左栏紧凑,详情栏补全)

## 表单 `/nodes/new` `/nodes/:id/edit`

模态弹窗覆盖列表。字段集与 CLI flag 一一对应:

`ID* · Name* · Address* · Port · User · Password · SSHKey · ProxyJump · Groups(逗号分隔) · Labels(k=v,k2=v2)`

- 编辑态 `ID` 只读(等价 CLI `update <node-id>`);新增态 `ID` 可编辑且必填
- **导航态**:↑/↓ 移动字段(**首尾回卷**:顶按 ↑ 跳底,底按 ↓ 跳顶),`Enter` 进入当前字段输入,`s` 保存,`Esc` 返回取消,`?` 帮助
- **输入态**:任意按键全部进输入框(↑/↓ 变文本内光标移动),`Esc` 唯一退出键,任何表单导航/保存/全局动作不可触发
- **保存数据流**:
  1. 导航态 `s` → 校验(加:ID/Name/Address 必填;改:Name/Address 非空;Port ∈ 1-65535)
  2. 校验失败 → 底部错误行红字回显 + 光标跳首个非法字段,不弹栈
  3. 通过 → `NodeStore.Add/Update` + `Save()`,弹栈回列表并刷新
  4. 重复 ID / store 错误 → 错误行回显,表单留在原地
  5. Esc 有未保存改动 → 确认丢弃(`y/n`,导航态)

## 删除 `/nodes/:id/delete`

`←/→` 选 [Delete]/[Cancel],`Enter` 执行,Esc 等同 Cancel。成功/失败在列表页状态栏回显,失败不弹栈。

## 错误处理

- TUI 内**不用 os.Exit**;所有错误渲染到当前视图状态栏/错误行,`Esc` 可离开
- 全局 `q` 退出需确认(脏状态时)

## 测试

- bubbletea model 单测:Update() 喂 tea.KeyMsg,断言 Mode/位置栈/列表过滤结果
  - 过滤解析:g:/l:/裸文本/AND 组合
  - 列选择器:切换/全选/重置/序列化 header 串
  - 表单:回卷导航、导航/输入态隔离(输入态下按任意键不触发导航/保存)、校验失败回显与跳转、保存/取消数据流
  - 删除:确认/取消
  - 位置栈:push/pop/面包屑渲染
- pty 端到端(script 命令喂真实按键)验证完整 CRUD 流程 + 过滤 + 列配置
- E2E 对齐 `tests_owl_nodes.sh` 的 node 数据模型

## 边界

- 空列表 / 过滤无结果:列表区显示空态提示
- 终端过窄(< 某宽度):隐藏详情栏仅留列表,或列表列截断省略号
- 无终端(TTY 检测):`owl tui` 报错提示需在交互终端运行
- 位置栈最多 3 层(list → form/confirm/columns),杜绝旧版多层嵌套
