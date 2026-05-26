# Tasks

- [x] Task 1: 在数据库 schema 中新增 `nodes` 表
  - [x] 在 `internal/history/db_sqlite3.go` 的 `InitSchema()` 中添加 `nodes` 表 DDL（使用 `INTEGER PRIMARY KEY AUTOINCREMENT`, `TEXT` 类型，`groups`/`labels` 存为 TEXT）
  - [x] 在 `internal/history/db_duckdb.go` 的 `InitSchema()` 中添加 `nodes` 表 DDL（使用 `SEQUENCE` + `NEXTVAL()`, `VARCHAR`/`JSON` 类型）
  - [x] 两个后端的 DDL 保持字段一致：`id`, `name`, `address`, `port`, `user`, `password`, `ssh_key`, `status`, `groups`(JSON), `labels`(JSON), `proxy_jump`, `created_at`, `updated_at`, `last_check_at`
  - **验证**：编译两个版本（默认和 `-tags duckdb`），确认 `InitSchema()` 不报错

- [x] Task 2: 创建 `NodeStoreDB` 实现 `NodeStore` 接口
  - [x] 在 `cmd/cli/cmd/common/` 下新建 `node_store_db.go`，定义 `NodeStoreDB` 结构体
  - [x] 实现 `List()` — `SELECT * FROM nodes` 并反序列化 `groups`/`labels` JSON 字段
  - [x] 实现 `Get(id)` — `SELECT * FROM nodes WHERE id = ?`
  - [x] 实现 `Add(node)` — `INSERT INTO nodes (...)` 序列化 `groups`/`labels` 为 JSON
  - [x] 实现 `Remove(id)` — `DELETE FROM nodes WHERE id = ?`
  - [x] 实现 `Update(node)` — `UPDATE nodes SET ... WHERE id = ?`
  - [x] 实现 `Save()` / `Load()` — 空操作（数据库自动持久化），保持接口兼容
  - [x] `NodeStoreDB` 构造函数接收 `*sql.DB` 参数，通过 `history.GetGlobalDB().Connection()` 获取
  - **验证**：编写 `node_store_db_test.go` 单元测试，使用 SQLite 内存数据库验证 CRUD 操作

- [x] Task 3: 修改 `internal/node/local_source.go` 从数据库读取节点
  - [x] `loadFromFile()` 改为先从数据库读取节点（`SELECT * FROM nodes`），再读取 `nodes.json` 覆盖同名节点
  - [x] 数据库读取逻辑复用 `history.GetGlobalDB().Connection()`
  - [x] `nodes.json` 不存在或解析失败时静默降级，仅输出 debug/warn 日志
  - [x] 保留 `LocalSource` 的 `LocalNode` 结构体和 `GetNode`/`ListNodes`/`AddNode`/`RemoveNode` 方法签名不变
  - **验证**：单元测试验证 DB+nodes.json 合并逻辑、nodes.json 优先级覆盖

- [x] Task 4: 修改 `cmd/cli/cmd/root.go` 初始化顺序
  - [x] 在 `Execute()` 中：先 `NewDB()` 建立数据库连接，再初始化节点存储
  - [x] 将 `NewDB()` 的返回值（`DBInterface`）保留，传递给节点存储初始化
  - [x] 在 `common/node.go` 的 `init()` 中：改为延迟初始化，等待 DB 连接就绪（或移除 `init()` 改为显式调用）
  - **验证**：`owl node list` 在首次启动（无 nodes.json）时不报错，显示空列表

- [x] Task 5: 适配 CLI 节点管理子命令使用 `NodeStoreDB`
  - [x] 修改 `cmd/cli/cmd/common/node.go` 的 `GetNodeStore()` 返回 `NodeStoreDB` 实例
  - [x] 移除 `InMemoryNodeStore` 的 `Save()` 显式调用（数据库自动持久化），保留调用点但不执行文件写入
  - [x] 确保 `node add/remove/update/list/import/export/ping/check/status/groups/labels` 所有子命令正常工作
  - **验证**：运行 `tests/scripts/test-node.sh` 集成测试全部通过

- [x] Task 6: 实现首次启动自动迁移
  - [x] 在节点存储初始化时检测：数据库 `nodes` 表为空 且 `~/.owl/nodes.json` 存在
  - [x] 解析 `nodes.json` 并将所有节点 INSERT 到数据库
  - [x] 迁移完成后输出 Info 日志，不删除原 `nodes.json` 文件
  - [x] 数据库已有节点时跳过迁移
  - **验证**：准备一个 `nodes.json` 文件，删除数据库后重新运行 `owl node list`，确认节点已自动导入数据库

- [x] Task 7: 更新相关测试
  - [x] 更新 `internal/node/local_source_test.go` — 适配数据库读取逻辑
  - [x] 更新 `cmd/cli/cmd/node/node_test.go` — 适配 `NodeStoreDB`
  - [x] 更新 `cmd/cli/cmd/common/common_test.go` — 如有测试则适配
  - [x] 更新 `tests/integration/` 下的集成测试
  - **验证**：`go test ./...` 全部通过

# Task Dependencies
- Task 2 依赖 Task 1（需要 `nodes` 表存在）
- Task 3 依赖 Task 2（复用 `NodeStoreDB` 或共享全局 DB 连接）
- Task 4 依赖 Task 2、Task 3（调整初始化顺序，串联 DB 和节点存储）
- Task 5 依赖 Task 2、Task 4（CLI 命令需要 `NodeStoreDB` 和正确的初始化）
- Task 6 依赖 Task 1、Task 2（需要表和存储就绪）
- Task 7 依赖 Task 1～6（实现完成后更新测试）
