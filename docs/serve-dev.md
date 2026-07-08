# owl-serve 开发文档

## 架构

```
cmd/owl-serve/main.go          # 入口，解析 -port / -host / -dev 标志
cmd/plugins/serve/
├── server.go                   # Init / Start / 路由注册 / 静态文件服务
├── server_test.go              # 集成测试（SPA、WebSocket）
├── web/                        # 前端 SPA
│   ├── index.html              # 单页入口
│   ├── css/app.css             # 样式（~900 行）
│   └── js/
│       ├── app.js              # 路由 + parseJWT + render 函数
│       ├── api.js              # HTTP + WebSocket 客户端
│       └── pages/
│           ├── login.js        # 登录页
│           ├── dashboard.js    # 节点列表页
│           ├── node.js         # 节点详情页
│           ├── tasks.js        # 任务列表页（含 exec 模态框）
│           ├── task_detail.js  # 任务详情页（流式输出）
│           ├── playbooks.js    # 剧本页（list / run / cancel）
│           ├── settings.js     # 设置页（admin only）
│           └── users.js        # 用户管理页（admin only）
├── handler/                    # API 处理器
│   ├── auth.go                 # Login / Me / RBAC 中间件
│   ├── node.go                 # 节点 CRUD + Search + Filters
│   ├── exec.go                 # 任务执行（多节点 / 脚本 / 分组标签解析 / 分页）
│   ├── playbook.go             # 剧本 CRUD / Run / Cancel / Refresh
│   ├── user.go                 # 用户 CRUD（admin）
│   ├── settings.go             # 配置 CRUD（admin）
│   └── ws.go                   # WebSocket Hub + Handler
├── service/
│   └── auth.go                 # JWT 生成/验证 + bcrypt
├── store/
│   ├── user.go                 # 用户存储
│   ├── task.go                 # 任务存储（含冲突检测）
│   ├── playbook.go             # 剧本元数据存储（YAML 同步）
│   ├── playbook_run.go         # 剧本运行历史存储
│   └── settings.go             # 设置存储
└── model/
    ├── playbook.go             # Playbook / PlaybookRun / StepResult 模型
    └── types.go                # Role 枚举、User 模型
```

## 构建

```bash
# 当前平台
make build-serve

# 跨平台
make build-serve/linux
make build-serve/darwin-arm64
make build-serve/windows

# 安装到 ~/.local/bin
make install-serve

# 全量编译（含 server）
make build/all
```

## 管理员管理

```bash
# 首次启动 —— 自动生成随机密码并打印到终端
./build/owl-serve --port 8080

# 忘记密码 —— 重置并启动服务（不删库）
./build/owl-serve --reset-admin --port 8080
# 输出: Username: admin / Password: xxxxxxxx，然后自动启动服务
```

`--reset-admin` 在 `main.go` 中实现，调用 `Server.ResetAdmin()` 方法。

## 测试数据

内置 seed 端点 `POST /api/v1/nodes/seed`（仅 admin），创建 50 个模拟节点：

```bash
# 1. 启动服务
./build/owl-serve --port 8080

# 2. 从终端输出的密码登录并获取 token（替换 <password>）
TOKEN="$(curl -s http://localhost:8080/api/v1/login -X POST \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"<password>"}' \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["token"])')"

# 3. 注入 50 个节点
curl -X POST http://localhost:8080/api/v1/nodes/seed \
  -H "Authorization: Bearer $TOKEN"
# → {"code":200,"message":"seeded 50 nodes"}
```

生成的节点特征：
- 分组：web / db / cache / worker / monitor / gateway（每个节点 1~3 个）
- 标签：env（prod/staging/dev）、tier（frontend/backend/data）、team（platform/infra/fe-team/data）
- 状态：online / offline / unknown 随机分布
- IP：`10.0.x.x` 格式
- SSH 用户：root / admin / deploy / app

也可通过分组过滤验证：
```bash
# 查看所有分组
curl -s http://localhost:8080/api/v1/nodes/filters -H "Authorization: Bearer $TOKEN" \
  | python3 -m json.tool

# 按单分组过滤
curl -s "http://localhost:8080/api/v1/nodes?group=web&page_size=5" \
  -H "Authorization: Bearer $TOKEN"

# 按多分组过滤（逗号分隔）
curl -s "http://localhost:8080/api/v1/nodes?group=web,db" \
  -H "Authorization: Bearer $TOKEN"
```

## 开发模式

```bash
# 从文件系统加载前端文件（支持热更新）
owl-serve -dev

# 指定端口
owl-serve -port 8080 -dev

# 或通过 Cobra
owl serve -dev
```

`-dev` 模式下静态文件从 `cmd/plugins/serve/web/` 目录读取，修改 JS/CSS 后刷新浏览器即可，无需重新编译。

## 测试

```bash
# 全部测试
go test ./cmd/plugins/serve/... -count=1

# 单包
go test ./cmd/plugins/serve/handler/... -v -count=1

# 集成测试
go test ./cmd/plugins/serve -run "TestServer_" -v -count=1

# 覆盖率
go test ./cmd/plugins/serve/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

当前 100 tests，4 个包全部通过（handler 71 + store 12 + service 5 + main 12）。

## 核心概念

参见 `CONTEXT.md` 领域术语表：

| 概念 | 说明 |
|------|------|
| **Task** | 单节点执行记录，每个 `exec` 调用对 N 个节点产生 N 个 Task |
| **Batch** | UI 分组概念，无后端实体；同一 `owl exec` 产生的 Task 共享 `batch_id` |
| **Node** | SSH 可访问的机器，支持 `groups`（字符串列表）和 `labels`（键值对）|
| **Command vs Script** | Command 为单行 shell 字符串；Script 为多行内容通过 `echo \| ssh bash` 内联执行 |

## 添加新前端页面

1. 在 `web/js/pages/` 创建 `your_page.js`，导出 `renderYourPage(render, navigate, user, api, ...params)`
2. 在 `web/js/app.js` 添加 import + route 匹配
3. 如果页面需要 nav 链接，在 `app.js` 的 `renderNav()` 中添加（权限控制：`user.role === 'admin'` 等）
4. 页面返回的 afterRender 中可返回 cleanup 函数（切换页面时自动调用）

## 添加新 API 端点

1. 在 `handler/` 创建或扩展现有 handler
2. 在 `server.go` 的 `setupRoutes()` 中注册路由，挂载到相应中间件组（viewer / editor / operator / admin）
3. 写 handler 测试
4. 更新前端 `api.js` 添加方法

## 任务执行流程

```
POST /api/v1/exec
  ↓
resolveNodeIDs(db, req)        # 按 node_ids > group > labels > status 解析
  ↓
for each node_id:
  verify node exists
  conflict check (queued/running → merge/skip/409 unless force)
  create Task (queued)
  ↓
go executeTask(task, ch)       # goroutine: SSH 执行 → 更新 Task 状态
  ↓
WSHub.BroadcastTaskUpdate(task) # WebSocket 实时推送
```

支持 payload：
```json
{ "node_ids": ["n1","n2"], "command": "uptime" }
{ "group": "web", "script_content": "#!/bin/bash\necho hello" }
{ "labels": {"env":"prod"}, "force": "true" }
```

## WebSocket 实时推送

- 客户端连接 `GET /api/v1/ws?token=<jwt>`
- 服务端通过 `WSHub.BroadcastTaskUpdate(task)` 推送
- 推送格式: `{"type":"task_update","data":{...}}`
- 前端 `api.connectWebSocket(onMessage)` 自动处理重连
- 详见 `handler/ws.go`

## Playbook 系统

- 剧本目录：`playbooks/`（可配置，通过 API `GET /playbook/settings/path` 获取）
- 刷新：`POST /api/v1/playbook/refresh`（admin）扫描目录 → upsert DB → 标记已删除
- 执行：`POST /api/v1/playbooks/:name/run` → 创建 PlaybookRun → 异步逐 step 执行 → 实时 WebSocket 推送
- 取消：`DELETE /api/v1/playbook/runs/:id`（admin）
