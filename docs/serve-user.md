# owl-serve 用户文档

## 概述

owl-serve 是 go-owl 的 Web 控制台服务，提供：

- **节点管理** — 添加/编辑/删除/搜索托管节点，支持分组（groups）和标签（labels）
- **远程执行** — 单节点/多节点、命令/脚本、分组/标签过滤、并行/串行模式
- **任务队列** — 执行历史、状态追踪、冲突检测、分页浏览
- **剧本编排** — Ansible 风格 YAML 剧本，异步逐步骤执行
- **用户管理** — 多角色 RBAC（viewer/editor/operator/admin）
- **配置管理** — 全局键值设置（admin only）
- **实时推送** — WebSocket 推送任务状态变更

## 启动

```bash
# 直接运行（首次自动生成 admin 密码）
owl-serve

# 指定端口和地址
owl-serve --port 8080 --host 0.0.0.0

# 开发模式（前端热更新）
owl-serve --dev

# 通过 go-owl CLI
owl serve
```

## 首次运行

首次启动自动完成初始化：

1. 创建 SQLite 数据库 `~/.owl/owl.db`
2. 生成 JWT 密钥（持久化到数据库中）
3. 创建 admin 用户，随机密码输出到 stdout：

```
URL:      http://127.0.0.1:8080
Username: admin
Password: rq3aemy3tUut
```

**请立即保存密码。** 后续启动不再输出。

## 重置管理员密码

如果忘记了首次生成的密码，可以重置：

```bash
owl-serve --reset-admin
# 或通过 owl CLI 同样支持：
owl serve --reset-admin
```

输出：

```
Admin password has been reset.
Username: admin
Password: 81gBl5mZ1onK

Please save this password. It will not be shown again.
```

**注意：** 此命令会生成新密码并更新数据库中的 admin 用户，然后退出。不会启动服务器。

## 手动数据库操作

如果需要完全重置数据库（删除所有节点、任务、设置等），可以直接删除数据库文件：

```bash
rm ~/.owl/owl.db
```

下次启动时会自动创建新的数据库和 admin 用户。

如果需要更精细的控制，可以直接使用 sqlite3 命令：

```bash
# 查看 admin 用户
sqlite3 ~/.owl/owl.db "SELECT * FROM users WHERE username='admin';"

# 删除 admin 用户（下次启动会重新创建）
sqlite3 ~/.owl/owl.db "DELETE FROM users WHERE username='admin';"

# 删除所有用户
sqlite3 ~/.owl/owl.db "DELETE FROM users;"

# 删除所有节点
sqlite3 ~/.owl/owl.db "DELETE FROM nodes;"

# 删除所有任务
sqlite3 ~/.owl/owl.db "DELETE FROM tasks;"

# 删除所有设置（包括 JWT 密钥）
sqlite3 ~/.owl/owl.db "DELETE FROM settings;"

# 查看数据库结构
sqlite3 ~/.owl/owl.db ".schema"
```

## API 端点

所有 API 前缀 `/api/v1`。认证通过 `Authorization: Bearer <token>` 头传递。

### 认证

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/login` | 登录，返回 JWT token |
| GET  | `/me` | 获取当前用户信息 |

### 节点

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET  | `/nodes` | 节点列表（支持 ?page, ?page_size, ?status, ?group, ?label） | viewer+ |
| GET  | `/nodes/search?q=` | 搜索节点 | viewer+ |
| GET  | `/nodes/filters` | 获取可用的分组/标签/用户过滤器 | viewer+ |
| GET  | `/nodes/:id` | 节点详情（隐藏密码/SSH key） | viewer+ |
| POST | `/nodes` | 创建节点 | editor+ |
| PUT  | `/nodes/:id` | 更新节点 | editor+ |
| DELETE | `/nodes/:id` | 删除节点 | admin |

创建/更新节点请求体：

```json
{
  "id": "my-server",
  "address": "192.168.1.100",
  "port": 22,
  "user": "root",
  "password": "optional",
  "ssh_key": "optional",
  "groups": ["web", "prod"],
  "labels": {"env": "production"}
}
```

### 任务执行

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET  | `/tasks` | 任务列表 | viewer+ |
| GET  | `/tasks/:id` | 任务详情（含输出） | viewer+ |
| POST | `/exec` | 执行命令（多节点/脚本/分组标签） | operator+ |
| DELETE | `/tasks/:id` | 取消任务 | admin |

### 执行命令

支持多种执行模式：

**单节点命令：**
```json
{ "node_id": "my-server", "command": "uptime" }
```

**多节点命令：**
```json
{ "node_ids": ["web-01", "web-02"], "command": "systemctl restart nginx", "force": "true" }
```

**按分组执行：**
```json
{ "group": "web", "command": "uptime" }
```

**按标签执行：**
```json
{ "labels": {"env": "prod"}, "command": "uptime" }
```

**脚本执行（内联）：**
```json
{
  "group": "web",
  "script_content": "#!/bin/bash\necho 'Deploy started'\nsystemctl restart app\necho 'Done'",
  "script_name": "deploy.sh",
  "script_args": "--verbose"
}
```

`force=true` 覆盖冲突检测强制执行（跳过 queued/running 状态检查）。

任务状态流转: `queued → running → completed | failed | cancelled`

### 分页

任务列表支持分页：

```
GET /api/v1/tasks?page=1&page_size=50
```

返回包含 `total`, `page`, `page_size`, `data` 的响应。

### 剧本

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET  | `/playbooks` | 剧本列表 | operator+ |
| GET  | `/playbooks/:name` | 剧本详情 | operator+ |
| POST | `/playbooks/:name/run` | 执行剧本 | operator+ |
| GET  | `/playbook/runs` | 运行历史 | operator+ |
| GET  | `/playbook/runs/:id` | 运行详情 | operator+ |
| GET  | `/playbook/settings/path` | 获取剧本目录路径 | operator+ |
| POST | `/playbook/refresh` | 刷新剧本目录 | admin |
| DELETE | `/playbook/runs/:id` | 取消运行 | admin |

### 用户管理

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET  | `/users` | 用户列表 | admin |
| GET  | `/users/:id` | 用户详情 | admin |
| POST | `/users` | 创建用户 | admin |
| PUT  | `/users/:id` | 更新用户（角色/密码） | admin |
| DELETE | `/users/:id` | 删除用户 | admin |

### 设置

| 方法 | 路径 | 说明 | 权限 |
|------|------|------|------|
| GET  | `/settings` | 所有设置列表 | admin |
| GET  | `/settings/:key` | 获取单个设置 | admin |
| PUT  | `/settings/:key` | 设置值 | admin |

## RBAC 角色

| 角色 | 节点查看 | 节点编辑 | 命令执行 | 剧本管理 | 用户管理 | 设置管理 |
|------|----------|----------|----------|----------|----------|----------|
| viewer | ✓ | | | | | |
| editor | ✓ | ✓ | | | | |
| operator | ✓ | ✓ | ✓ | ✓ | | |
| admin | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

## 前端 SPA

| 路由 | 页面 | 说明 |
|------|------|------|
| `/login` | 登录 | 用户名密码登录 |
| `/nodes` | 节点管理 | 搜索、筛选、批量操作 |
| `/nodes/:id` | 节点详情 | 查看/编辑节点 |
| `/tasks` | 任务列表 | 执行命令（多节点/脚本/分组过滤）、查看历史、结果面板 |
| `/tasks/:id` | 任务详情 | 流式输出、退出码、实时计时、取消 |
| `/playbooks` | 剧本管理 | 剧本列表、执行、取消（operator+） |
| `/settings` | 设置管理 | 键值设置（admin） |
| `/users` | 用户管理 | CRUD + 角色分配（admin） |

### 节点管理功能

- **搜索** — 输入节点名称/IP/标签，100ms 防抖自动过滤
- **筛选** — 左侧面板按分组复选过滤，顶部下拉按状态（在线/离线/告警）过滤
- **批量操作** — 勾选节点后支持依次标签（合并到各节点）、同批标签（统一替换）、删除
- **标签格式校验** — 输入时自动校验 `key:value` 格式、非法字符、重复键
- **分组收缩** — 左侧分组面板可折叠/展开，折叠按钮始终可见
- **彩虹色标签** — 分组和标签通过彩虹色阶区分，相同值颜色一致

### 键盘快捷键

| 快捷键 | 功能 |
|--------|------|
| `Alt+1` | 仪表盘 |
| `Alt+2` | 节点管理 |
| `Alt+3` | 命令执行 |
| `Alt+4` | 剧本管理 |
| `Alt+5` | 文件传输 |
| `Alt+6` | AI 助手 |
| `Alt+7` | 任务历史 |

所有非 API 路径 fallback 到 `index.html`（SPA 路由）。前端页面通过 `/static/*` 提供，支持 `-dev` 模式热更新。

## WebSocket

连接 `ws://host:port/api/v1/ws?token=<jwt>` 接收实时任务状态推送。

消息格式：
```json
{"type": "task_update", "data": {"id": "...", "status": "running", ...}}
```

前端通过 `api.connectWebSocket(onMessage)` 自动处理重连，任务列表页和详情页均订阅 `task_update` 消息。

## 数据库

- 位置: `~/.owl/owl.db`
- 类型: SQLite
- 表: `users`, `nodes`, `tasks`, `settings`, `playbooks`, `playbook_runs`, `playbook_step_results`

备份可以直接复制该文件。

## 命令行参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--port`（`-p`） | 8080 | HTTP 监听端口 |
| `--host` | 127.0.0.1 | HTTP 监听地址 |
| `--dev` | false | 开发模式（前端从文件系统加载） |
| `--reset-admin` | false | 重置 admin 密码并退出 |
| `--ai-debug` | false | AI 调试模式（记录完整提示词/回复） |
