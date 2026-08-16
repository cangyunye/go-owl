# TUI 终端界面

`owl tui` 是全屏交互式终端界面（bubbletea 实现），在终端里完成节点管理、批量命令执行、文件传输和 AI 对话，无需记忆命令行参数。

## 构建与启动

TUI 组件通过构建标签门控，默认构建不包含：

```bash
# 带 TUI 的构建
make build WITH=tui
# 或
make build-tui

# 启动
owl tui
```

纯构建（`make build` / `task build`）不注册 `owl tui` 子命令，且 tui 依赖（bubbletea/bubbles/lipgloss）不会被链接。

## 面板总览

TUI 有 4 个面板，顶部菜单栏显示当前面板：

| 面板 | 功能 | 直达键 |
|------|------|--------|
| Nodes | 节点管理（增删改查、分组、标签、ping、SSH 检查、导入导出） | `1` |
| Exec | 批量命令执行（命令/节点/分组/标签/格式/高级选项） | `2` 或 `x` |
| File | 文件传输（上传/下载/扩散传输） | `3` 或 `f` |
| AI | AI 助手对话 | `4` |

- `Tab` 循环切换面板，`1/2/3/4` 直达
- `x` 从 Nodes 面板带选择进入 Exec；`f` 带选择进入 File（勾选的节点优先，否则带入当前组/标签过滤条件）
- `?` 查看帮助，`q` 退出（Nodes 面板有未保存修改时需 `y` 确认）

## Nodes 面板

| 键 | 功能 |
|----|------|
| `↑` `↓` | 选择节点 |
| `←` `→` | 切换列 |
| `g` `G` | 跳转到列表首/尾 |
| `a` | 添加节点 |
| `e` | 编辑节点 |
| `d` | 删除节点 |
| `c` | 列配置（勾选显示哪些列，含 Labels 彩虹着色列） |
| `p` | ping 节点 |
| `k` | SSH 检查 |
| `i` | 导入导出（YAML/JSON） |
| `o` | 分组管理 |
| `l` | 标签管理 |
| `Space` | 勾选/取消多选（`x` 带入 Exec 面板作为执行目标） |
| `/` | 过滤 |
| `?` | 帮助 |

### 过滤语法

`/` 打开过滤输入，支持：

- 纯关键词：搜索节点名称/地址/ID
- `g:组`：按分组过滤
- `l:k=v`：按标签过滤
- `s:状态`：按状态过滤（online/offline/unknown）
- 空格或 `&&` 表示 AND 组合，例如 `g:web && l:env=prod` 匹配「分组含 web 且 env=prod」的节点

## Exec 面板

主表单包含 4 个字段：**命令**（必填）、**节点**、**分组**、**标签**。

| 键 | 功能 |
|----|------|
| `↑` `↓` | 移动字段（首尾回卷） |
| `Enter` | 编辑字段（`Esc` 退出输入） |
| `f` | 切换输出格式：`simple` → `detail` → `json` |
| `a` | 高级选项模态表单 |
| `r` | 执行 |
| `Esc` | 返回 Nodes 面板 |

### 目标解析

表单底部实时显示目标节点数，优先级：**节点 ID** > 分组 ∩ 标签（AND）> Nodes 面板当前可见快照。

### 高级选项（`a`）

20 个选项字段，覆盖 CLI `exec run` 的参数：超时（timeout/connect-timeout/command-timeout）、并行/串行、重试（retry/retry-interval/retry-max-interval/no-retry）、异步（async/async-timeout/async-poll-interval/async-max-poll-count/async-remote-dir）、status、no-color、debug、force、sync-nodes、silent。

- `↑` `↓` 移动，`Enter` 编辑文本字段，`Space` 切换布尔开关，`s` 保存，`Esc` 返回

### 危险命令确认

命中黑名单的命令（如 `rm -rf`）会先弹出「危险命令确认」视图，显示命中的节点与匹配模式：`y` 放行执行，`n`/`Esc` 取消。高级选项中勾选 `force` 可跳过黑名单检查。

### 结果视图

流式显示每个节点的执行结果（✓/✗、退出码、耗时、输出），`r` 重跑，`Esc` 返回表单。

## File 面板

`←` `→` 切换操作：**文件上传** / **文件下载** / **扩散传输**。

| 键 | 功能 |
|----|------|
| `↑` `↓` | 移动字段 |
| `Enter` | 编辑字段；「本地文件」字段（上传/扩散）弹出本地文件浏览器 |
| `←` `→` | 切换操作类型 |
| `a` | 高级选项 |
| `r` | 执行传输 |
| `Esc` | 返回 Nodes 面板 |

### 本地文件浏览器

在「本地文件」字段按 `Enter` 弹出，起点为启动 `owl tui` 时的工作目录：

- `↑` `↓` 选择，`Enter` 进入目录 / 选中文件回填
- `←` / `Backspace` 返回上级目录
- `/` 输入绝对路径跳转（目录或文件），`Enter` 跳转，`Esc` 取消
- `h` 切换隐藏文件显示
- `Esc` 退出浏览器，返回字段编辑态

下载操作（远程文件字段）不弹浏览器，直接编辑。

## AI 面板

| 键 | 功能 |
|----|------|
| `Enter` 或 `i` | 输入问题 |
| `Enter` | 发送 |
| `n` | 新会话 |
| `Esc` | 返回 Nodes 面板 |

## 主题

`OWL_TUI_THEME` 环境变量选择主题：`default` / `catppuccin` / `nord` / `dracula` / `solarized`（默认 `catppuccin`）。主题在 TrueColor/256/ANSI 色域自动降级，并按终端明暗背景自适应；非法值回退 `catppuccin` 并打印提示。

```bash
OWL_TUI_THEME=nord owl tui
```

## 模式说明

- **Normal 模式**：按键即命令
- **Insert 模式**：输入框编辑中，`Esc` 退出输入

顶部路径栏显示当前面板/子视图路径（如 `/exec/run/advanced`）与当前模式。
