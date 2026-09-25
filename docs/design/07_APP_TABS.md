# 设计文档 7: 内建标签页（v1.9.0）

> 基线：v1.8.1 ｜ 分支：`feat/v1.9.0-app-tabs` ｜ 状态：设计评审稿，未实施 ｜ 日期：2026-09-25

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
3. **资源纪律不齐**（保活会把泄漏放大 N 倍）—— **M0 已修，见 §4 里程碑表**：
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

**决策（2026-09-25 修订）：M1/M2 用重建式；M3 改用方案 A（同文档保活），不再把 iframe 作为主路径。**

修订理由：初稿把「419 个固定 DOM id 必须作用域化」当成了方案 A 的硬成本，但那只在
**多个标签的 DOM 同时挂在文档里**时成立。方案 A 的关键是非激活标签的 DOM 从文档**摘除**
（detach），文档内任何时刻只有一个页面实例，`getElementById` 依然唯一 —— 419 处查询无需改造。
方案 A 的真实成本收敛为三件可控的事：每标签容器、失活即暂停、浮层挂到容器。
iframe（方案 B）虽然隔离彻底，但要长期背负跨帧税：快捷键在 iframe 内失效、标题/上下文要
postMessage 回传、E2E 全部要进 frame；且它同样只能活在「应用内切标签」，扛不住刷新。

原始三方案对比（保留备查）：

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

> **本期需求已达成（2026-09-25）**：最初的要求是「多标签页 —— 一个标签进节点管理、一个进剧本管理、
> 一个进系统设置、一个进监控（告警中心），也可以多个标签同时在节点管理」。这四类场景已实现并
> 由 `test/e2e_tabs_acceptance.py` 逐条验收：四个视图各占一个标签 + 两个「节点管理」标签各持
> 不同分组上下文（`节点管理 · e2e-small` / `节点管理 · e2e-big`），逐个切换渲染正常。
>
> 之后的 M3/M4 是「允许但不要求」的加固（会话保活、连接与传输减负）；**M5 不在本期需求内**，
> 保留为备选（触发条件：出现「多端同看一个会话」或「会话列表/审计」的实际需求）。



| 里程碑 | 内容 | 交付物 | 预估 | 状态 |
|--------|------|--------|------|------|
| **M0 资源纪律** | `scope.resources` 注册器；修掉 files 5s 轮询泄漏、nodes/settings keydown 泄漏、alerts 轮询取消、ai SSE abort、body 浮层挂载即登记（切页摘除，M3 走 iframe 后天然帧内隔离） | [`web/js/pagescope.js`](../../cmd/plugins/serve/web/js/pagescope.js) + app.js `beginPage()` + 8 处页面迁移 + `pagescope_test.go` + `test/e2e_tabs_m0_resources.py` 泄漏哨兵 | 2~3 人日 | **已完成**（commit 4ee636e） |
| **M1 标签模型 + 标签栏** | Tab 集合、`.tabbar` UI 与交互、URL/pushState 同步、localStorage 恢复、`snapshot/restore` 契约（列表类 8 页接入） | `web/js/tabs.js` + `app.js` 改造 + `app.css` 标签栏 + E2E | 4~6 人日 | **已完成**（commit 95e800a） |
| **M2 面板随标签 + 嵌套上下文** | 面板归属页面（`scope.panel`）、`shell.* → scope.panel.*` 迁移、嵌套标题/图标、详情类路由开新标签、`?group=`/`?cat=` 等上下文入 route | `makePanel()` + `routeContext` + `openPathInNewTab()` + 8 页面板迁移 + E2E | 3~4 人日 | **已完成**（commit 013d0d3） |
| **M3 会话类保活（方案 A：同文档保活）** | 每标签容器（视图 + 面板 DOM）随激活 attach/detach；非激活标签**失活即暂停**（scope.resources 加 pause/resume）；浮层挂到标签容器而非 body；`/terminal/:id`、`/sftp/:id`、AI 会话、剧本运行详情接入 | `tabs.js` 容器化 + `app.js` attach 生命周期 + 4 页后台行为审计 + E2E | 3~4 人日 | **已完成**（M3a 24d710e 终端；M3b cd9d6d5 sftp/ai/playbooks） |
| **M5-① 会话重连与回放（本期实现目标）** | 单进程内的终端会话注册表：不可猜 session_id、输出环形缓冲（1MiB）、WS 断开只 detach 不杀会话、空闲 10 分钟回收、每用户会话上限、复用节点范围授权；前端 sessionStorage 记 session_id + WS 自动重连（1/2/4…10s 退避）+ 回放提示 | `handler/terminal.go` + 会话注册表 + `terminal.js` | 1.5~2 人日 | 未开始 |
| **M4 共享 WS + 节流治理** | 壳层共享 WS 总线、4 处页面迁移、非激活标签轮询挂起/恢复、增量取输出（轻量列表 + 字节游标）、标签上限与内存提示 | `api.js onWS` + 4 页迁移 + `/tasks/:id/output` + E2E | 3~4 人日 | **已完成**（M4a bff470e 总线；M4b a2f3460+8ad98f7 增量输出） |

合计 16~23 人日（含测试）。M0→M1 即可交付「可用标签页」，M2 补齐子菜单并行，M3/M4 解决长任务与资源。

> M4 实施记录（2026-09-25）：M4a 把「每页一条 WS」收敛成一条总线（实测三页共用 1 条、
> 订阅者 2，关闭后释放）；M4b 把「长输出整段重传」换成「轻量列表 + 字节游标补差」。
> 关键设计：WS 的 task_output 广播**带上该行的起始字节偏移**，客户端据此维护精确游标——
> 按行数估算会因换行/编码差异漂移，漂移会导致补出来的输出错位。实测 108894 字节输出下，
> 对账 1 次轻量、0 次全量，并观察到 1 次按游标补输出。
> 未做：WS 的按主题过滤（服务端广播仍是全量，客户端靠 scope 失活门控丢弃）；
> 顺带发现执行页面板搜索框只过滤当前页（服务端分页 30 条），是独立小缺陷。
>
> M3b 实施记录（2026-09-25）：审计要点是「后台工作与界面更新分离」——上传 XHR 与流式
> 文本继续跑，界面更新在失活期间门控，切回靠 onResume 补。真正容易漏的是 **await 之后
> 的续写**：页面在挂载时发起请求，返回时标签已被摘除，于是写 null 抛错（实测 sftp 的
> `load()` 写 `#sftp-cwd`）。两条经验：渲染入口要守卫、await 之后要再查一次状态；
> 以及保活分支绝不能只摘 DOM 而忘了 `pause()`（漏这一步时 M3b 用例持续报错）。
>
> M3a 实施记录（2026-09-25）：机制落地后只开了终端一个页面保活 —— 它的后台工作写 xterm
> 实例，与 DOM 查询无关，最安全；sftp/ai/playbooks 失活时会用 `document.getElementById`
> 找元素（保活后取到 null），需要逐页审计（M3b）。实施中发现两个真问题：同标签内换页会把
> 新页面写进旧容器并把旧页 WS 留成孤儿（现在 beginPage 先释放本标签旧容器）；停在深层路由
> （终端/详情）时点同级导航会把会话页挤掉（改为新开标签打开列表）。
>
> M2 实施记录（2026-09-25）：`?group=`/`?cat=`/`?q=` 入 route 后，**刷新丢上下文**这个 M1 的
> 局限也一并解决（上下文来自 URL，不再只靠内存快照）；顺带修掉一个既存误导——进详情页
> （`/nodes/:id`、终端、SFTP）时面板会残留上一页的「节点分组」。截图（`docs/serve-web-ui.md`
> 26 张）已在 M2 后随 commit 35231c2 重拍。
>
> M1 实施记录（2026-09-25）：标签语义定为「导航复用已打开的同视图标签（保住上下文），
> Ctrl/中键才新开」；快照只留内存、标签集合持久化，因此刷新恢复的是标签与路由，
> 页面内筛选靠同会话内的快照恢复。实施中两个坑：一是 8 个页面里 users/exec/playbooks
> 只改了函数体没加 `scope` 参数（运行期 "scope is not defined"，已由「签名必须接住 scope」
> 断言钉住）；二是资源哨兵的基准必须取在「无监听页面」激活时，否则会把当前页应有的监听
> 误判成泄漏。
>
> M0 实施记录（2026-09-25）：泄漏哨兵在修复前实测到定时器 0→2、document 监听 1→5
> （10 个一级页走两轮），修复后三项计数全部回到基线；迁移中踩到一次「只登记不挂载」——
> `resources.overlay()` 初版只登记移除、不 append，导致弹窗全部不出现，被 issue11 回归 E2E 抓到，
> 现该方法一次完成「挂载 + 登记」，并由 Go 断言钉住。

## 4.5 负载归属：什么该在浏览器、什么必须留在服务端

标签页把「一个页面变多个并发上下文」之后，服务端压力会随客户端数量线性上升，因此需要明确
分工。以下是当前实测到的服务端负载与可行的客户端卸载项。

**现在压在服务端的负载（证据）**

| 负载 | 现状 | 代价 |
|------|------|------|
| 广播扇出 | `WSHub.Broadcast` 把每条消息发给**所有**客户端，每个客户端 512 条缓冲（`handler/ws.go:80-90`） | N 个标签 = N 份解码 + N 份缓冲；输出行是最重的一类消息 |
| 轮询 | files 5s、alerts 递归 1.5/2s 等由浏览器定时打 HTTP，无缓存头/ETag（`server.go:459` 只有 no-cache） | 每次轮询都触发一次 SQL + 序列化 |
| 重复拉取 | `nodesAll()` 每个标签各翻页拉一遍全量节点；列表页与选择器各拉一份 | 8 标签 × 多入口 = 数十次相同请求（节点上万时是断崖） |
| 服务端聚合 | 仪表盘的分组分布走 COUNT 查询，而 `dashboard.js` 自己也在翻页聚合 | 同一口径两处算 |
| 传输字节过网桥 | SFTP/传输：浏览器 → owl-serve → 目标节点三段转发 | 所有文件字节过服务端内存与带宽 |
| AI 生成绑定连接 | LLM 调用挂在 `c.Request.Context()`（`ai.go:281/391/526`） | 客户端一断即停，无法后台续跑/多端续看 |

**可以迁到浏览器客户端的（按收益排序）**

1. **缓存与去重**（最划算）：节点/任务/概览这类短时不变数据在浏览器缓存并与标签共享；
   `nodesAll()` 加一层共享缓存即可把「8 标签 × 多入口」降为 1 次拉取。
2. **请求节流与可见性**：非激活标签不轮询、不渲染（M4 范围）；同一 tick 的重复请求合并。
3. **增量拉取替代全量推送**：WS 只推轻量信号（task_id + 游标），浏览器按 offset 拉增量，
   非激活标签完全不拉。需服务端补一个「按 offset 取输出」端点（小改动，换来扇出大幅下降）。
4. **纯展示计算**：ANSI/markdown 渲染、时间与体积格式化、截断、颜色哈希、排序/过滤/分页切片 ——
   服务端只发原始数据（现状已大部分在前端，继续按这条线走）。
5. **本地持久化**：视图/会话状态与最近数据放 IndexedDB（`storage.js` 已有先例），
   减少刷新后的全量重拉。
6. **聚合统计**：分组/状态分布、在线率等由浏览器一次拿快照后本地聚合；服务端只保留
   跨用户一致的「大数快照」口径。

**必须留在服务端的**

- SSH/PTY/SFTP 的**执行**与凭据（浏览器永远不应接触私钥/密码明文）
- 监控与告警的定时采集（无人开浏览器时也要跑）、审计与持久化
- LLM 调用与 API Key
- 需要全局一致口径的统计（全量节点/任务计数）以服务端为准；浏览器聚合只服务「当前视图」

**比「搬计算到客户端」更有效的是绕开网桥**

- **节点到节点传输**：让目标节点自己拉（`wget/curl` 或受控 SFTP 拉取），替代
  浏览器→服务端→节点 的三段转发，文件不过服务端。
- **静态资源**：加 gzip + ETag + 长缓存（现在连 gzip 都没有）——纯服务端减负，无需改客户端。
- **大文件下载**：服务端只流式转发、不落盘。

**与 M3/M4/M5 的关系**

- M4（节流 + 共享 WS 总线）吸收上表第 1–3 项：把「每标签一份连接与轮询」收敛为
  「一条共享连接 + 一套缓存 + 只服务可见标签」。
- M5（会话服务端化）让浏览器进一步回归薄客户端：会话生命周期在服务端，
  客户端只做渲染与缓存，并顺带解决多端同看同一会话。

### 4.6 M5-① 终端会话重连与回放（实现规格）

**买什么**：刷新页面、网络抖动后接回**同一个 shell**（cwd/env/前台进程都在），并回放最近输出。

**边界（约束决定，不实现）**：owl-serve 重启后会话仍失效 —— PTY 是它的子进程，节点侧不许放任何常驻物。

**服务端**
- 会话注册表：`map[sessionID]*termSession{ client, session, ringBuf(1MiB), cols, rows, owner, createdAt, lastActive }`
- `sessionID` 用 `crypto/rand` 32 字节、base64url（**不可猜**，否则拿到 id 就能接管别人的 shell）
- `GET /api/v1/session/terminal?ticket=…&node_id=…[&session_id=…][&cols=&rows=]`：
  带 `session_id` → 校验归属（同一用户 + 该节点在其授权范围内，复用 `ScopeChecker`）→ attach（先回放 ringBuf，再进实时流）；
  不带 → 新建会话并把 `session_id` 通过 WS 首帧下发给前端
- WS 断开 → **detach**（保留 PTY 与 SSH 连接，记 `lastActive`），不起新进程
- 回收：空闲 10 分钟无 attach → 关闭 PTY/SSH；每用户会话上限 5、进程总上限 64（超限拒绝并提示）
- 输出写入 ringBuf（按字节截断头部）并照常推流，不改变现有广播路径

**前端（terminal.js）**
- `sessionStorage['owl-term-<nodeId>']` 记住 session_id；挂载时若存在则带上去请求 attach
- `ws.onclose` → 自动重连（1s/2s/4s/8s/10s 退避，最多 6 次），重连时带同一个 session_id
- 回放完成后在终端里打一行提示（如「已接回会话 #ab12，回放 320 行」）；「重连」按钮改为「重开会话」（清 session_id 重新起）

**测试**
- Go：注册表 create/attach/replay/detach/reap/上限/越权 attach 拒绝（纯内存，快）
- E2E：① `export PROBE=xyz` → 刷新页面 → `echo $PROBE` 得 `xyz`（同一 shell 的铁证，也可用 `ps` 看刷新前的 `sleep` 进程仍在）；② 模拟网络抖动（页面内断开 WS）→ 自动重连且回放补齐；③ 把空闲超时改小 → 会话被回收（进程消失）

---

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

**未来计划：多端同看同一会话（M5-②，本期不做）**

触发条件：出现「手机/另一台电脑同时看同一个 shell」或「会话列表 / 会话审计」的真实需求。

在 M5-① 之上要补的东西（合计约 +1.5 人日）：
- 多路 attach：一个会话挂多个 WS，输出扇出；写冲突按「最后写赢」并给其他端提示
- 尺寸仲裁：以最近一次 attach 的 cols/rows 为准（或投票/只读端不参与）
- 只读模式：旁观端只收不写（用于教学/排障）
- 会话列表与运维：`GET /sessions`（自己创建的会话 + 节点 + 空闲时间）、强制踢出、空闲策略可视化
- 审计：会话打开/attach/detach/回收写审计记录（谁在什么时候接了哪个节点的会话）
- 权限：attach 复用节点范围授权（已有 `ScopeChecker`），并明确「同一用户才能接自己的会话」还是「同角色可接」



- `Ctrl+W`/`Ctrl+T`/`Ctrl+Tab` 在浏览器里无法拦截，故用 `Alt` 系快捷键；用户若装了浏览器扩展冲突需自改键位。
- 保活标签上限 8 + 内存提示；iframe 模式下字体/主题同步需逐项验证（现三套主题 + `deviceScaleFactor` 截图口径）。
- 每标签独立历史栈、标签分组/固定、跨设备恢复、AI 会话与标签一对一绑定：后续版本再评估。
- 移动端/窄屏：标签栏在 <768px 下退化为「当前标签 + 列表抽屉」，随响应式一起做。
