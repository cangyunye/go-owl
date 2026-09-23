# 设计文档 6: SFTP 文件浏览器（v1.8.0）

> 分支：`feat/v1.8.0-sftp-browser` ｜ 状态：设计评审稿，未实施 ｜ 日期：2026-09-22

## 1. 概述

### 1.1 需求

在 owl-serve 节点管理界面为每个节点增加「文件管理（SFTP）」入口，打开一个 SFTP 文件浏览器窗口，支持：

1. 浏览远端节点目录（列表、面包屑导航、进入/返回）
2. **拖拽上传**：把本地文件/文件夹拖入窗口，上传到当前浏览路径
3. **下载**：把远端文件拖出/点击下载到本地；文件夹打包下载
4. **重名文件冲突**：弹窗询问 覆盖 / 换名 / 自动 `_序号` 重命名，并提供批量处理方式
5. **文件夹递归传输**：拖入文件夹时递归上传全部内容
6. **同名文件夹**：默认覆盖 + 递归内容同步；其中每个重名文件弹窗询问，弹窗内始终提供批量处理方式（批量覆盖/批量重命名/批量跳过）（2026-09-23 评审定稿）

### 1.2 现状（已具备的能力）

| 能力 | 位置 | 说明 |
|------|------|------|
| SFTP 引擎 | `cmd/plugins/serve/handler/transfer.go` | 已引入 `github.com/pkg/sftp`，`sftpPush/sftpPull` 支持断点续传、overwrite 语义 |
| 统一 SSH 拨号 | `internal/ssh/dial.go` `Dial()` | 支持密码/密钥/ProxyJump |
| 节点凭证 | `handler/ssh_executor.go` `decryptNodeSSHInfo()` | nodes 表 password/ssh_key/proxy_jump，AES-256-GCM 加密存储 |
| WS 一次性 ticket | `handler/ws_ticket.go` | 60s TTL、一次性核销，terminal 在用 |
| 前端拖拽上传 | `web/js/pages/files.js:998-1020` | dropzone + `dataTransfer` 已实践 |
| 行内按钮范式 | `web/js/pages/nodes.js:131`（`data-term`） | SFTP 按钮照此新增 `data-sftp` |

**缺口**：远端目录列表/stat/mkdir/rename/delete 的 API；浏览器直传远端（当前上传必须先 multipart 到服务器 staging 再 `POST /transfer`）；重名冲突交互策略；文件夹递归同步。

### 1.3 关键决策：传输通道选型

| 方案 | 说明 | 结论 |
|------|------|------|
| A. REST 流式（推荐） | 目录操作走 JSON；上传 `PUT` raw body 流式落远端；下载 `GET` 流式。Bearer JWT 复用，`XMLHttpRequest.upload.onprogress` 拿进度，`AbortController` 取消 | ✅ 采用 |
| B. WebSocket 双工（仿 terminal） | ticket + ws，JSON 控制 + binary 帧 | ❌ 备选。多路复用/分帧/ACK 协议复杂，本需求无服务端推送进度需求（传输由前端发起，进度前端天然可知） |

传输进度、冲突弹窗、批量策略全部在**前端**完成；服务端只暴露无状态的分文件接口，每个文件一次 HTTP 请求，简单且天然支持并发（限 3）与单独取消。

## 2. 后端设计

### 2.1 新 handler：`cmd/plugins/serve/handler/sftp_browser.go`

复用 `getNodeInfo + decryptNodeSSHInfo` 取凭证，**必须走 `internal/ssh.Dial`**（支持 ProxyJump；顺带修正 `transfer.go:281 dialSFTP` 不支持 ProxyJump 的老问题，本迭代可一并切换，属低风险重构）。每个请求建立 `*sftp.Client`，请求结束即关连接（目录操作开销可接受；上传大文件单连接贯穿单次请求）。

### 2.2 API 一览（前缀 `/api/v1/sftp`，注册于 `server.go setupRoutes()`）

```
GET    /sftp/ls      ?node_id=&path=            → {path, items:[{name,path,is_dir,size,mtime,mode}]}
GET    /sftp/stat    ?node_id=&path=            → 存在性 + 元信息（404 表示不存在）
GET    /sftp/file    ?node_id=&path=            → 文件字节流 + Content-Disposition
GET    /sftp/archive ?node_id=&path=&fmt=tgz    → 文件夹打包流（见 2.4）
PUT    /sftp/file    ?node_id=&path=&mode=&new_name= → 上传 raw body，见 2.3
POST   /sftp/mkdir   {node_id, path}
POST   /sftp/rename  {node_id, from, to}
POST   /sftp/delete  {node_id, path, recursive}
```

RBAC 分组：**全部端点（含 ls/stat/下载）挂 writer 组（editor 及以上）**，viewer 不能打开 SFTP 页面、不能下载（2026-09-23 评审决定）。所有端点过 `ScopeChecker` 校验节点授权范围，与 terminal 一致。前端 `nodes.js` 的「文件」按钮按角色隐藏。

### 2.3 上传冲突语义（`mode` 参数）

| mode | 行为 |
|------|------|
| `overwrite` | 存在即截断覆盖 |
| `skip` | 存在则返回 `409 {exists:true}`，不写入（前端记为"跳过"） |
| `auto_rename` | 服务端找第一个可用名：`name.txt → name_1.txt → name_2.txt`（序号插在扩展名之前；目录同理 `dir → dir_1`） |
| `rename` + `new_name=xx` | 用前端弹窗里用户输入的新名；若新名仍存在则按 auto_rename 规则继续 |

响应头/体返回 `final_path`，前端队列据此显示实际落盘名。上传路径 = 前端拼好的远端完整路径（当前浏览目录 + 相对子路径），服务端只做清洗校验（见 2.5），**不做冲突预判**——是否弹窗由前端先 `stat` 或直接把 `mode=auto_rename` 交给服务端。为减少往返：文件夹同步时前端一次性 `ls` 远端目标目录，本地比对得出冲突清单，仅冲突文件弹窗。

### 2.4 文件夹下载

浏览器不能直接把远端目录写到本地磁盘（沙箱限制），统一为**打包单文件下载**：

- 首选：SSH exec `tar -C <parent> -czf - <name>`，stdout 管道进 HTTP 响应（无远端落盘）；Windows 节点无 tar 时 fallback 到 SFTP 递归拉取 + 本地（serve 端）临时 zip，用完即删
- 单文件下载：`GET /sftp/file` 流式，前端用 `fetch → blob` 还是直接 `<a href>`？大文件禁止进内存——**下载统一走一次性 ticket 的 `<a>` 直链或 `window.open`**（同 terminal ticket 方案），避免 Bearer 无法用于 `<a>` 的问题

> 前端"拖远端文件到本地"：浏览器无法监听拖出到桌面，实现为拖到窗口内的「下载区」= 点击下载，交互上等价；UI 说明文案标注。

### 2.5 安全约束

1. **路径清洗**：`path.Clean` 后必须为绝对路径，拒绝 `\0`、拒绝含 `..` 段（清洗后仍含 `..` 即 400）；不限制只能 home 目录（运维场景需要系统路径，权限由 RBAC + 节点账号本身控制）
2. 凭证读取必须经 `decryptNodeSSHInfo`，日志中不得输出密码/密钥内容
3. 上传大小上限：settings 表新增 `sftp_max_upload_mb`（默认 2048），超限 413；`io.Copy` 全程流式，不进内存
4. 每节点并发传输限制：handler 内 `semaphore`（默认 8），超限 429
5. `HostKeyCallback` 维持现状 `InsecureIgnoreHostKey`，文档标注为已知技术债，不属本迭代

## 3. 前端设计

### 3.1 入口与窗口形态

- `nodes.js` 行内操作区新增「文件」按钮（`data-sftp`，紧邻 `data-term`，`:131` 附近），点击 `navigate('/sftp/'+id)`；批量工具栏不加（单节点操作）
- `app.js` router（`:296-332`）新增 `/sftp/:nodeId` → 新页面 `js/pages/sftp.js`
- 形态选**独立页面**（与 terminal 一致），不选弹窗：文件树 + 传输队列 + 冲突弹窗需要空间，且独立 URL 便于刷新恢复。页面头部显示节点名 + 地址 + 返回按钮

### 3.2 页面布局（sftp.js）

```
┌─ 节点: web-01 (10.0.0.5)          [返回] ─┐
│ 面包屑: /home/app ▸ logs                    │
│ ┌─ 文件列表 ─────────────────────────────┐ │
│ │ 名称        大小   修改时间    操作      │ │
│ │ ▸ config/   -     ...        重命名 删除 │ │
│ │ ▓ app.log   2.1M  ...        下载       │ │
│ │              （拖入区 = 整个列表区）      │ │
│ └────────────────────────────────────────┘ │
│ ┌─ 传输队列 ─────────── [全部取消] ───────┐ │
│ │ ✔ data.csv        1.2MB/1.2MB          │ │
│ │ ▶ big.tar.gz      45%  ▓▓▓░  [取消]     │ │
│ │ ⏸ (待决策) report.pdf  [弹窗挂起中]      │ │
│ └────────────────────────────────────────┘ │
└─────────────────────────────────────────────┘
```

功能：ls 渲染（目录优先排序）、双击进入目录、面包屑跳转、新建文件夹、重命名、删除（确认框）、按 mtime/名称排序。

### 3.3 上传管线（核心交互）

拖入 drop 事件 → `DataTransfer.items[i].webkitGetAsEntry()` 递归展开成**条目队列**（文件 + 相对路径 + 目录结构，`FileSystemFileEntry.file()` 逐块读，不经内存堆积）。

**单文件流程**：
1. 入队（上限并发 3，`XMLHttpRequest` PUT，`upload.onprogress` 更新进度条，每行可取消 = `abort()`）
2. 冲突决策见 3.4

**文件夹条目流程（同名冲突处理，2026-09-23 评审定稿）**：

拖入本地目录 `reports/`，远端目标下已存在 `reports/` 时：**直接默认进入「覆盖 + 递归内容同步」**——不再弹三选窗，整棵树递归上传；仅当递归过程中遇到**同名文件**才弹文件级冲突窗。

- 目录本身不判"冲突即停"：远端同名目录直接作为同步目标，本地没有的远端子项**不删除**（增量覆盖同步）
- 子目录递归重复同一逻辑
- 每个重名文件的弹窗**必须自带批量选项**（见 3.4）：批量覆盖 / 批量自动 _序号 / 批量跳过，选择后本批次剩余同类冲突静默处理，不再逐个弹

**文件级冲突弹窗**（队列中每个重名文件触发，含文件夹同步内部）：

```
文件已存在: /data/reports/q3.pdf      [应用到全部剩余冲突 ▾]
  本地 2.4MB · 今天12:03   vs   远端 1.1MB · 昨天09:12   [大小/时间对比]
  ○ 覆盖   ○ 换名（输入框，预填 q3_1.pdf）   ○ 自动加 _序号   ○ 跳过
  [记住我的选择: 仅本次 | 本批次全部]      [取消整个传输]
```

批量处理：勾选"本批次全部"后，当前队列/同一目录同步内剩余冲突按该策略静默执行，不再弹窗；队列面板保留"本批已全部覆盖"之类的可撤销汇总行（可选增强，M4 再定）。

### 3.4 下载管线

远端行「下载」或拖入下载区：单文件 → ticket 直链 `window.open('/api/v1/sftp/file?...&ticket=')`；目录 → `archive` 直链下载 tgz。复用 `ws_ticket.go` 签票接口（`handler/ws_ticket.go:39`，需扩一个 `purpose=download` 或复用现票）。

## 4. 数据库

无新表。可选：传输记录复用现有 `transfers` 表加 `source='browser'` 标记（用于审计列表区分），M1 决定是否值得。

## 5. 里程碑拆分（提交原子性对齐）

| 阶段 | 内容 | 验收 |
|------|------|------|
| M1 | 后端目录操作 API（ls/stat/mkdir/rename/delete）+ 路径清洗 + RBAC | 单测 + `curl` 集成（真实 sshd 容器） |
| M2 | 后端流式下载（file/archive）+ ticket | 单测 |
| M3 | 后端 PUT 上传 + 四种冲突 mode | 单测覆盖 auto_rename 序号规则、409/413/429 |
| M4 | 前端 sftp.js 浏览器 UI（浏览/新建/重命名/删除/下载） | 手工 E2E |
| M5 | 前端拖拽上传 + 队列进度 + 文件冲突弹窗/批量 | E2E |
| M6 | 文件夹递归同步 + 目录冲突弹窗 | E2E |
| M7 | 文档（serve-user.md 章节）+ nodes.js 按钮图标打磨 | — |

每阶段 E2E 通过后独立 commit（对齐仓库 "Git commit After E2E" 规则）。

## 6. 测试策略

- **单元**：冲突 mode、auto 序号（含扩展名/无扩展名/目录）、路径清洗（`..`、空字节、相对路径注入）、`webkitGetAsEntry` 展开逻辑（前端可用简化 fixture）
- **集成**：本地起 `atmoz/sftp`（或 sshd + internal sftp server）容器，覆盖上传/下载/并发限制/超限 413
- **E2E**：Browser 页面上拖文件、拖文件夹、触发三种冲突路径，校验远端落盘结果与队列显示

## 7. 已知限制与后续方向

1. 不同步删除远端文件（非双向 mirror）
2. 拖出到桌面不可行，以「点击下载区」等价
3. 断点续传不在本期（浏览器侧中断即整文件重传；服务端 `sftpPush` 的 Resume 能力留给 CLI transfer 链路）
4. Windows OpenSSH 节点 tar 打包 fallback 体验一般，后续可考虑 zip
5. HostKey 校验为全仓技术债，另行立项
