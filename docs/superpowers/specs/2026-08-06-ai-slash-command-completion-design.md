# AI 会话斜杠命令补全设计

日期:2026-08-06
状态:已实现(通过 E2E)
范围:Web 控制台 OPS AI 页面(`cmd/plugins/serve/web/js/pages/ai.js`)的输入框补全

## 背景与目标

OPS AI 是运维助手会话页,用户通过自然语言下达指令(查节点、执行命令、跑剧本、传文件等)。
目标是让用户以 `/` 唤起命令补全:

- 输入以 `/` 开头时,弹出下拉列表展示关键命令与常用 OWL 工具提示词模板;
- 选中任务类命令 → 展开为可编辑的提示词模板填入输入框,光标定位到第一个待填参数;
- 选中导航/系统类命令 → 直接执行动作(跳页、新建、清空、帮助);
- 非 `/` 开头的输入保持普通对话,不干扰。

## 设计决策

| 决策点 | 结论 | 理由 |
|---|---|---|
| 触发条件 | 仅当输入框开头为 `/` 时弹出 | 避免路径/正则里的斜杠误触发 |
| 交互方式 | 下拉列表 + ↑/↓ 选择 + Enter 确认 + Esc 关闭 | 类 ChatGPT 斜杠菜单,键盘友好 |
| 命令目录来源 | 前端内置静态目录 | 零网络依赖、即时响应、与前端零依赖现状一致 |
| 选中行为 | 任务类展开模板;导航类直接动作 | 模板可微调,降低误操作 |
| 命令目录归属 | 迁移到 `ai.js`(而非组件文件) | 导航动作需访问页面作用域的 `navigate`/对话状态 |
| 组件实现 | vanilla JS 单组件 `SlashMenu` | 无 npm 构建、无第三方依赖 |

## 组件结构

### `web/js/slash-menu.js` — 通用组件

`SlashMenu` 类,通过 `{ commands }` 注入目录,不内置命令:

- `refresh()`:输入事件时判断 `value.startsWith('/')`,按名称前缀过滤;无匹配即关闭
- `render()`:生成 `.ai-slash-menu` 下拉(绝对定位于输入区上方)
- `onKeyDown()`:↑/↓ 循环选中、Enter 选中、Esc 关闭;处理过的键一律 `preventDefault`
- `select()`:任务类走 `applyTemplate`,导航类执行 `action(textarea)`
- `applyTemplate()`:填入模板,`setSelectionRange` 选中第一个 `{param}`,再派发 `input` 事件
- `close()` / `destroy()`:移除 DOM、解绑监听

### `web/js/pages/ai.js` — 命令目录与页面绑定

- `SLASH_COMMANDS`:14 条命令,7 任务 + 7 导航
- 任务模板(展开后光标选中首占位符):

| 命令 | 展开模板 | 占位符 |
|---|---|---|
| `/exec` | 在 `{nodes}` 上执行命令 `{command}` | nodes, command |
| `/check` | 检查 `{nodes}` 的 SSH 连通性,找出不可达节点 | nodes |
| `/diagnose` | 对 `{target}` 进行全栈故障诊断并给出修复建议 | target |
| `/query` | 查询 `{condition}` 的节点信息 | condition |
| `/playbook` | 生成一个 playbook 实现 `{requirement}` | requirement |
| `/transfer` | 把 `{source_file}` 传输到 `{nodes}` 的 `{dest_dir}` | source_file, nodes, dest_dir |
| `/script` | 在 `{nodes}` 上运行脚本 `{script}` | nodes, script |

- 导航/系统命令: `/nodes` `/exec-page` `/playbooks` `/files` → `navigate()` 跳页;`/new` → 新建对话;`/clear` → 删除当前对话并清空消息 DOM 回到空态;`/help` → 显示命令帮助浮层

- Enter 发送逻辑改为 `!e.defaultPrevented` 判定:补全组件处理过的 Enter(选中展开)会 `preventDefault`,与监听注册顺序解耦,避免选中瞬间 `isOpen()` 已为 false 导致误发送

### `web/css/app.css` — 样式

- `.ai-slash-menu` / `.ai-slash-item`:下拉列表,`position: absolute` 悬于输入区上方,`max-height` 滚动
- `.ai-help-overlay` / `.ai-help-box`:帮助浮层,覆盖 `.ai-main`,列出命令名/标签/模板预览

## 关键交互细节

- 空 `/` 显示全部 14 条;输入前缀实时过滤(如 `/exec` 命中 `exec` 与 `exec-page` 两项)
- 任务类排在导航类之前
- 展开后光标全选第一个占位符,直接输入即替换;后续 `{param}` 由用户手动补齐
- `/check` 占位符填入「所有节点 / 分组 / 具体节点名」即可覆盖全量、分组、指定节点三种检查场景;模板内含「连通性」「找出不可达」命中 router 的 `node_check` 触发短语(见 `internal/ai/prompts/prompts.go`)

## 边界与异常处理

- 无匹配前缀 → 菜单自动关闭,输入继续走普通对话
- Esc 关闭菜单、点击外部关闭
- 页面切走时 `destroy()` 解绑监听,避免泄漏
- 帮助浮层可点 ✕ 或点击遮罩关闭;再次 `/help` 切换

## 测试

E2E(Playwright,20 用例)覆盖:

1. 7 个任务命令展开模板逐一断言
2. `/` 显示全部 14 条;任务类在导航类之前
3. 前缀过滤(`/exec` → 2 项,首个为 `exec`)
4. `/help` 浮层显示与关闭
5. `/new` 回到空态
6. 发送消息后 `/clear` 清空
7. `/nodes` 跳转到节点管理页

## 后续可迭代项

- 模板内多占位符用 Tab 跳转依次填充
- 命令目录支持用户自定义(设置页维护)
- 模糊匹配(如 `/dgn` 命中 `/diagnose`)

---

## CLI 端实现(2026-08-08)

范围扩展:CLI `owl ai` 交互模式的斜杠命令补全,交互设计对齐上述 Web 端 SlashMenu。

### 技术方案

| 决策点 | 结论 | 理由 |
|---|---|---|
| 输入实现 | 手写轻量行编辑器(新包 `cmd/cli/cmd/ai/input`) | 零第三方依赖(仅用已有 `golang.org/x/term`),避免引入 readline 库膨胀二进制;可精确复刻 Web 端交互(实时菜单 + Enter 确认,而非 Tab 循环补全) |
| 按键事件 | `keys.go`:字节流 → `Key`(UTF-8/ANSI CSI/控制键) | 支持中文多字节与方向键/Home/End/Delete 转义序列 |
| 菜单逻辑 | `slash.go`:`SlashCommand` 目录 + `SlashMenu` 状态机(前缀过滤、↑↓ 循环、占位符定位) | 纯逻辑无 IO,可独立单测 |
| 行编辑器 | `editor.go`:`Editor.ReadLine()` 状态机 | 菜单打开时 ↑↓ 选命令,关闭时 ↑↓ 走会话历史 |
| 终端适配 | `term.go`:`Terminal` 封装 x/term raw mode | Windows/Unix 均支持 |

### CLI 命令目录(映射 Web 端并适配)

- 任务类(7,展开模板):`/exec` `/check` `/diagnose` `/query` `/playbook` `/transfer` `/script` —— 与 Web 端模板一致
- 动作类(4,直接执行):`/help` → 打印帮助;`/new` → 重建会话(`SessionManager.CreateSession` 覆盖);`/clear` → 重建会话清上下文;`/quit` → 退出交互
- Web 端导航类(`/nodes` 等跳页)在 CLI 无对应页面,已剔除

### 关键交互细节

- 展开模板后光标**选中**第一个 `{arg}` 占位符(选区替换):直接输入即替换占位符,与 Web 端 `setSelectionRange` 行为一致
- `Esc` 取消 slash 输入并清空当前行;无匹配前缀时菜单自动关闭,输入继续走普通对话
- 提交/动作输出前补 `\r\n`,避免 raw mode 下 AI 响应粘在输入行

### 跨平台与边界处理

- **转义序列跨读**:SSH/慢速终端可能把 `ESC` 与 `[A` 分开发送 —— 单独 `ESC` 在底层支持 `SetReadDeadline`(Unix TTY)时等待 50ms 确认;Windows console 无 deadline,立即返回 `ESC`(其转义序列通常整段到达)
- `parseCSI` 对不完整序列返回"等待更多字节",不丢弃;未知 CSI 整体丢弃;`ESC` 后跟普通字符时 `ESC` 独立返回、字符保留
- 非交互输入(管道/重定向)回退 `bufio.Scanner` 逐行读取,slash 菜单仅 TTY 生效;原有 `quit`/`exit`/`help`/`!` 前缀保持兼容

### 测试

`input` 包 47 个单测:按键解析(UTF-8 拆分、CSI 拆分/不完整/未知、单独 Esc 超时与回退)、SlashMenu 状态机(过滤/循环/占位符)、行编辑器(中文输入、光标编辑、模板展开与占位符替换、action 回调、Esc 取消、Tab 确认、历史导航、中断、渲染行定位与清理);E2E 管道模式验证通过。
