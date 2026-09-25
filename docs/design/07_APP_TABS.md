# 设计文档 7: 内建标签页（v1.9.0 候选）

> 分支：`feat/v1.9.0-app-tabs`（待建） ｜ 状态：设计评审稿，未实施 ｜ 日期：2026-09-25
> 版本号待定：main 上已有 v1.8.0 的 SFTP 工作，若标签页排进 v1.8.0 请一并改标题与 README 索引。

## 1. 概述

### 1.1 需求

Web 控制台需要内建标签页，让多层子菜单可以同时处理。当前 SPA 是「单视图 + 全局单侧栏面板」结构，
切走即丢上下文，下列场景无法并行：

| 场景 | 现状痛点 |
|------|----------|
| 两个节点分组并行操作 | 切到另一分组再切回，分组选择、搜索词、勾选全丢 |
| 终端 + SFTP + 运行详情并存 | `/terminal/:id`、`/sftp/:id` 是路由级页面，切走即断连、重进重连 |
| 长任务边跑边干别的 | 运行中的剧本详情、文件传输、AI 流式回答切走即停摆（WS/轮询随页面卸载） |
| 同一个节点的多层下钻 | 节点详情 → 终端 → SFTP 之间来回，每次都要重新下钻 |

目标：标签页承载「一级导航 + 侧栏子菜单 + 详情/会话」这一整条上下文；切标签不丢上下文，
长任务在非激活标签里继续跑，标签集合可跨刷新恢复。

### 1.2 现状（已具备 / 阻碍）

已具备：

- 壳层是单容器单实例：`app.js:73-87` 的 `render(html, afterRender)` 直接覆盖 `.view-container`
  的 innerHTML，`afterRender` 返回的函数存进**唯一**的 `currentCleanup`（`app.js:16`）。
- 路由已支持详情/会话级深链接：`/nodes/:id`、`/tasks/:id`、`/terminal/:id`、`/sftp/:id`（`app.js:474-510`），
  查询串由页面自读（`/exec?nodes=`、`/playbooks?run=`）。
- 侧栏面板由页面自治渲染（history/nodes/exec/playbooks/files/users/alerts 各自写 `panelList`，
  `app.js:351-359` 直接跳过）。

阻碍（保活/多实例的硬约束）：

1. **固定 DOM id 全局唯一假设**：全前端 419 个 `getElementById` 唯一 id，其中 31 个被 ≥2 个页面共用
   （`exec-btn`、`panelList`、`sidebar`、`node-pagination`…），`nodes.js` 73 个、`playbooks.js` 69 个、
   `exec.js` 51 个。同页两个实例同时挂载，`document.getElementById` 必然串台。
2. **侧栏面板是全局单例**：`shell.setPanelContent/setPanelTitle` 只认唯一 `#panelList`（`app.js:52-70`、`125-130`）。
   两个标签各自的分组/分类子菜单无法共存。
3. **资源纪律不齐**（保活会把泄漏放大 N 倍）：
   - `files.js:898` 挂载即 `startAutoRefresh()`，`stopAutoRefresh()`（`files.js:789`）**全仓无调用点** → 每进一次文件页多一个 5s 轮询；
   - `nodes.js:848`、`settings.js:605` 的 `document.addEventListener('keydown', …)` 每次挂载加一条、从不移除；
   - `alerts.js:329/355/585` 递归 `setTimeout` 轮询无取消句柄；
   - `ai.js` 流式回答走 SSE（`api.js:319-337`）无 `AbortController`，卸载后继续写已移除的 DOM；
   - body 级浮层（history 详情、alerts 7 处编辑器、ai 审批浮层、sftp 输入浮层）跨页残留。
4. **每页各建 WS**：`api.connectWebSocket`（`api.js:540-574`）每次调用独立取票据、独立 3s 重连；
   后端 `WSHub`（`handler/ws.go:21-52`）**无连接上限**，每个客户端 512 条缓冲，广播发给所有人。
   N 个标签 ≈ N 条 WS + N 份重连定时器 + N 份消息 fanout。
5. **页面状态与标签不匹配**：`owl-pb-view`/`owl-pb-lib`/`owl-settings-sections` 是 localStorage 全局偏好；
   `exec_selected_nodes`/`files_selected_nodes` 是 sessionStorage **按页**存，两个 exec 标签会互相覆盖勾选。

### 1.3 关键决策：标签内容如何隔离与保活

| 方案 | 做法 | 优点 | 代价 |
|------|------|------|------|
| **A. 同文档保活** | 非激活标签的容器 DOM 从文档摘除（detach），切回再插回；页面查询改作用域化 | 无 iframe，共享运行时，内存最省，可共享一条 WS | 需要改造 419 个 `getElementById` 为「标签根节点内查询」+ body 级浮层全部内联 → 改动面最大、回归风险最高 |
| **B. iframe 保活** | 每个保活标签一个 `<iframe src="/nodes?embed=1">`，iframe 内只渲染视图（不渲染壳层） | 零页面改造即得彻底隔离（id/监听/WS/浮层都关在帧内）；主题仅需同步 `data-theme`；崩溃互不影响 | 每帧一份 JS 堆（约 10~30MB）；需新增 `embed=1` 精简模式；跨帧调试、E2E 选择器要进 frame |
| **C. 重建式标签**（不保活） | 标签只记住路由 + 一小份可序列化状态（snapshot/restore），切回时重新挂载 | 风险最低，改动集中在壳层 + 每页一个 `snapshot/restore` | 会话类视图（终端/SFTP/AI 流/运行中详情）切走即断，长任务场景仍不满足 |

**决策：分阶段 C → B，不做 A。**

- M1/M2 用 C 落地标签模型与标签栏：列表类页面（dashboard/nodes/playbooks/history/users/settings/alerts/exec/files）
  只需「路由 + snapshot/restore」，先拿到「多上下文并行」的主要价值，风险可控。
- M3 对会话类页面（terminal/sftp/ai/剧本运行详情）启用 B（`tab.keepAlive = true` 时以 iframe 挂载）：
  这是唯一不重写 16 个页面就能保住 WS/xterm 实例的路子。
- A 只在将来要收敛内存或统一 WS 时才评估；届时页面查询作用域化可逐页渐进，不必大爆炸。

## 2. 前端设计

### 2.1 标签模型

```js
Tab = {
  id,                // tab-<n>，会话内唯一
  view,              // 'nodes' | 'exec' | ... （NAV_ITEMS 的 id）
  route,             // 该标签当前路由，含查询串：'/nodes?group=web'、'/playbooks?run=abc'
  title,             // 标签显示名：'节点管理 · web'（一级 + 嵌套上下文）
  icon,              // 沿 NAV_ITEMS 的 icon
  nested,            // 嵌套上下文（分组/分类/角色/节点 id…），用于 snapshot/restore 与标题
  keepAlive,         // true → iframe 挂载（M3），false → 重建式（M1）
  state,             // 重建式标签的快照（页面 snapshot() 的返回值），关闭即释放
  scrollTop,         // 可选：视图滚动位置
}
```

标签集合存 `localStorage['owl-tabs'] = { tabs:[…], activeId, seq }`；页面级偏好（`owl-pb-view` 等）
保持全局，勾选类状态（`exec_selected_nodes` 等）改为 `owl-tab-<tabId>-<page>` 命名空间。

### 2.2 标签栏 UI

- 位置：`.main-area` 内、`.topbar` 下方新增一行 `.tabbar`（`app.css` 现有 flex 纵向结构直接加一行即可），
  高度 `--tabbar-h: 34px`；三套主题（深空/青瓷/暖阳）各配一组颜色变量。
- 交互：`+` 新建（默认仪表盘）、单击切换、中键/`×` 关闭、`Ctrl/⌘+点击` 或右键「在新标签打开」复制当前
  上下文、拖拽排序、右键菜单（关闭其他/关闭右侧/全部关闭）、标签数上限 8（超出提示先关）、
  溢出横向滚动（不折叠成下拉，先简单）；关闭激活标签后激活右邻（无则左邻）。
- 快捷键（避开浏览器占用：`Ctrl+W`/`Ctrl+T` 无法拦截）：`Alt+T` 新建、`Alt+W` 关闭当前、
  `Alt+←/→` 或 `Alt+PageUp/PageDown` 前后切换；现有 `Alt+1..7` 保留给一级导航。
- 标签视觉：图标 + `title` + 关闭按钮；`keepAlive` 标签在标题前加小圆点（表示后台仍在跑）；
  运行中（剧本 run、传输任务、AI 流式回答）用现有 `.dot-indicator` 脉冲样式提示。

### 2.3 路由与 URL

- **激活标签 = URL**：切换标签 `history.pushState(null,'',tab.route)`，浏览器前进/后退仍按现有
  `popstate → router()` 走（`app.js:541`），即后退等价于回上一个激活标签，行为与今天一致。
- **刷新恢复**：启动时若 `owl-tabs` 存在则重建标签集合与激活项；`route` 与 `location.pathname` 不一致时以 URL 为准。
- **深链接开新标签**：`navigate('/nodes/abc')` 默认在当前标签内跳；壳层新增
  `navigate(path, { newTab:true })`，供「中键点击列表行」「右键在新标签打开」使用。
- 每标签独立历史栈（Chrome 式）**v1 不做**，列入后续方向。

### 2.4 侧栏子菜单归属（回应「多层子菜单同时处理」）

面板从全局单例改为**每标签一个容器**：`.tab-content` 内部结构为 `[侧栏面板][视图容器]`，
`shell.setPanelContent` 升级为 `tab.panel.setContent()/setTitle()`（按 `tabId` 写入该标签的 `panelList`），
页面已有的面板渲染代码只需把 `shell.*` 换成注入的 `tab.panel.*`。这样：

- 标签 A 的「节点管理 · web 分组」与标签 B 的「节点管理 · db 分组」各自保留自己的分组子菜单与勾选；
- 节点详情/终端/SFTP 作为独立标签时，面板标题显示「节点管理 · node-01」，多级面包屑在标签上可见。

### 2.5 页面契约升级（向后兼容）

```js
// 现在：render(html, afterRender) → cleanup
renderPlaybooks(render, navigate, user, api, shell)

// 之后：注入 scope，渲染不变
renderPlaybooks(render, navigate, user, api, shell, scope)
//   scope.tab         当前标签（id/title/nested）
//   scope.panel.setContent(html) / setTitle(t)
//   scope.snapshot()  可选：返回可序列化状态（重建式标签用）
//   scope.restore(s)  可选：恢复状态（重建式标签切回时调用）
//   scope.resources   资源注册器：interval/timeout/listener/ws/observer 自动回收
```

- `scope` 缺省时（老页面未升级）退化为今天的行为：`scope.panel` 写全局面板，`snapshot` 为空 → 切回重新挂载。
  **16 个页面可逐页升级，不必一次性改完。**
- M0 先把「资源注册器」做出来并用于修掉 1.2 里列出的泄漏点，再谈保活。

### 2.6 WS 与后台任务

- M4 把「每页各建 WS」收敛为壳层级**共享 WS 总线**：`api.onWS(handler) → unsubscribe`，
  壳层维护唯一连接（票据 + 重连）并把消息 fan-out 给订阅者。页面改动：把
  `wsCleanup = api.connectWebSocket(fn)` 换成 `wsCleanup = api.onWS(fn)`（playbooks/history/task_detail/exec 四处）。
  收益：N 标签不再 N 条连接、N 份 512 缓冲与重连定时器。
- 非激活标签**节流**：`scope.panel`/`scope.resources` 统一登记轮询与定时器，标签失活时挂起、
  激活时立即补一次（alerts 递归轮询、files 5s 轮询、exec 3s 对账都走这条）。
- 终端/SFTP 保持各自独立 WS/xterm（会话流，必须独立）；标签上限 8 即为其资源上限。

## 3. 后端设计

无强制改动。可选项（M4 视数据量决定）：

- `WSHub` 广播消息带 `task_id`/`run_id` 字段 + 订阅时可选过滤，减少 N 标签 fanout；
- 每用户 WS 连接数上限与 `/metrics` 计数（现在 `handler/ws.go` 无上限）；
- 终端会话按连接独立（`handler/terminal.go:97-168`），多标签=多 SSH 会话，无需改后端，但需在文档标注资源含义。

## 4. 里程碑拆分（提交原子性对齐）

| 里程碑 | 内容 | 交付物 | 预估 |
|--------|------|--------|------|
| **M0 资源纪律** | `scope.resources` 注册器；修掉 files 5s 轮询泄漏、nodes/settings keydown 泄漏、alerts 轮询取消、ai SSE abort、body 浮层改为视图内浮层 | `web/js/pagescope.js` + 8 处页面修补 + Go 断言测试 + Playwright 泄漏哨兵 | 2~3 人日 |
| **M1 标签模型 + 标签栏** | Tab 集合、`.tabbar` UI 与交互、URL/pushState 同步、localStorage 恢复、`snapshot/restore` 契约（列表类 8 页接入） | `web/js/tabs.js` + `app.js` 改造 + `app.css` 标签栏 + E2E | 4~6 人日 |
| **M2 面板随标签 + 嵌套上下文** | 每标签面板容器、`shell.* → scope.panel.*` 迁移、嵌套标题/图标、详情类路由开新标签、`?group=`/`?cat=` 等上下文入 route | 面板容器化 + 8 页面板迁移 + E2E | 3~4 人日 |
| **M3 会话类保活** | `embed=1` 精简模式（iframe 内只渲染视图）、`keepAlive` 标签的 iframe 挂载与生命周期、主题同步、`/terminal/:id`、`/sftp/:id`、AI 会话、剧本运行详情接入 | `app.js` embed 分支 + 4 页接入 + E2E（切走任务继续跑） | 4~6 人日 |
| **M4 共享 WS + 节流治理** | 壳层共享 WS 总线、4 处页面迁移、非激活标签轮询挂起/恢复、标签上限与内存提示、（可选）后端 WS 过滤 | `api.js onWS` + 4 页迁移 + E2E | 3~4 人日 |

合计 16~23 人日（含测试）。M0→M1 即可交付「可用标签页」，M2 补齐子菜单并行，M3/M4 解决长任务与资源。

## 5. 测试策略

沿用现有两层：Go 静态断言（`webui_static_test.go`/`playbooksui_test.go` 风格）+ Playwright E2E（`test/e2e_*.py`）。

**Go 断言**（防回退，快）：

- 壳层必须存在标签栏与 Tab 模型的锚点（`.tabbar`、`localStorage['owl-tabs']`、`scope.panel`）；
- 页面不得再用裸 `document.addEventListener` 注册需回收的监听（用 `scope.resources`）；
- `api.connectWebSocket` 只允许出现在 terminal/sftp 与 `api.onWS` 实现内。

**Playwright E2E**（`test/e2e_tabs_open_switch_keep.py` 等）：

1. 打开 3 个标签（含两个带不同分组上下文的节点页）→ 切走切回，断言各自的勾选/搜索词/滚动位置仍在；
2. **id 唯一性哨兵**：`document.querySelectorAll('[id]')` 不得有重复（多实例串台的直接体检）；
3. **泄漏哨兵**：`addInitScript` 计数 `setInterval`/`WebSocket` 实例，N 次挂载-卸载后计数恒定；
4. 刷新页面 → 标签集合与激活项恢复；
5. 中键点击列表行 → 新标签打开详情，原标签上下文不变；
6. 关闭标签 → WS/定时器数下降（M3/M4 后），终端标签切走时任务仍在输出（保活验证）；
7. 回归：现有 `test/e2e_issue*.py` 全绿。

**文档**：`docs/serve-web-ui.md` 的 26 张截图需重拍（新增标签栏与面板容器），按
`.agent/skills/serve-screenshots` 流程一次性更新。

## 6. 已知限制与后续方向

- `Ctrl+W`/`Ctrl+T`/`Ctrl+Tab` 在浏览器里无法拦截，故用 `Alt` 系快捷键；用户若装了浏览器扩展冲突需自改键位。
- 保活标签上限 8 + 内存提示；iframe 模式下字体/主题同步需逐项验证（现三套主题 + `deviceScaleFactor` 截图口径）。
- 每标签独立历史栈、标签分组/固定、跨设备恢复、AI 会话与标签一对一绑定：后续版本再评估。
- 移动端/窄屏：标签栏在 <768px 下退化为「当前标签 + 列表抽屉」，随响应式一起做。
