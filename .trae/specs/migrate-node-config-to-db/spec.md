# 节点配置迁移到数据库 Spec

## Why
当前节点配置存储在 `~/.owl/nodes.json` JSON 文件中，使用两套独立且不一致的数据结构（CLI 层的 `common.InMemoryNodeStore` 和解析层的 `internal/node/LocalSource`），缺乏事务保护、并发安全性差、无法利用数据库索引查询。项目已有成熟的双数据库（SQLite3/DuckDB）架构用于历史记录，应将节点配置统一迁移到同一数据库中，同时保留 `nodes.json` 作为可选的快速覆盖配置。

## What Changes
- 在历史记录数据库（`~/.owl/history.db`）中新增 `nodes` 表，用于持久化节点配置
- 创建 `NodeStoreDB` 实现 `NodeStore` 接口，使用 `database/sql` 操作节点数据，通过 build tags 自动适配 SQLite3 或 DuckDB
- 修改 `internal/node/LocalSource` 从数据库读取节点作为主数据源
- 保留 `nodes.json` 加载能力，作为高优先级覆盖层：先加载数据库节点，再加载 `nodes.json` 覆盖同名节点
- 迁移所有 CLI 节点管理子命令（`node add/remove/update/import/export/list` 等）使用数据库
- 首次启动时自动从 `nodes.json` 迁移已有数据到数据库（如果数据库为空且 `nodes.json` 存在）
- 修改 `root.go` 的 `Execute()` 初始化流程，确保数据库连接在节点存储之前建立
- **BREAKING**：节点数据存储位置从 `nodes.json` 迁移到数据库，`nodes.json` 变为可选覆盖文件

## Impact
- Affected specs: 无（新功能）
- Affected code:
  - `internal/history/db_sqlite3.go` — 新增 `nodes` 表 schema
  - `internal/history/db_duckdb.go` — 新增 `nodes` 表 schema
  - `cmd/cli/cmd/common/node.go` — `InMemoryNodeStore` 替换为 `NodeStoreDB`
  - `internal/node/local_source.go` — 改为从数据库读取 + nodes.json 覆盖
  - `internal/node/resolver.go` — 适配新的 LocalSource 初始化
  - `cmd/cli/cmd/root.go` — 调整初始化顺序
  - `cmd/cli/cmd/node/*.go` — 适配新的存储接口

## ADDED Requirements

### Requirement: 数据库节点存储
系统 SHALL 在历史记录数据库中维护 `nodes` 表，存储所有受管节点的配置信息。

#### Scenario: 新建数据库时自动创建 nodes 表
- **WHEN** 系统首次启动并初始化数据库
- **THEN** 数据库 schema 中自动包含 `nodes` 表，字段涵盖：`id`（主键）、`name`、`address`、`port`、`user`、`password`、`ssh_key`、`status`、`groups`（JSON）、`labels`（JSON）、`proxy_jump`、`created_at`、`updated_at`、`last_check_at`

#### Scenario: SQLite3 和 DuckDB 双后端兼容
- **WHEN** 使用默认 build tags 编译（`!duckdb`）
- **THEN** 使用 SQLite3 语法创建 `nodes` 表（`INTEGER PRIMARY KEY AUTOINCREMENT`, `TEXT` 类型）
- **WHEN** 使用 `-tags duckdb` 编译
- **THEN** 使用 DuckDB 语法创建 `nodes` 表（`BIGINT PRIMARY KEY DEFAULT NEXTVAL(...)`, `VARCHAR`/`JSON` 类型）

### Requirement: NodeStoreDB 实现 NodeStore 接口
系统 SHALL 提供 `NodeStoreDB` 结构体，实现 `common.NodeStore` 接口，通过 `database/sql` 操作节点数据。

#### Scenario: 添加节点
- **WHEN** 调用 `NodeStoreDB.Add(node)`
- **THEN** 节点数据 INSERT 到 `nodes` 表，`created_at` 和 `updated_at` 设为当前时间

#### Scenario: 查询节点列表
- **WHEN** 调用 `NodeStoreDB.List()`
- **THEN** 返回 `nodes` 表中所有节点的 `[]*NodeInfo` 切片

#### Scenario: 按 ID 获取节点
- **WHEN** 调用 `NodeStoreDB.Get(id)`
- **THEN** 返回对应 ID 的 `*NodeInfo`，不存在时返回错误

#### Scenario: 更新节点
- **WHEN** 调用 `NodeStoreDB.Update(node)`
- **THEN** 更新对应 ID 的记录，`updated_at` 设为当前时间

#### Scenario: 删除节点
- **WHEN** 调用 `NodeStoreDB.Remove(id)`
- **THEN** 从 `nodes` 表删除对应 ID 的记录

### Requirement: nodes.json 作为覆盖配置层
系统 SHALL 保留从 `~/.owl/nodes.json` 读取节点的能力，作为数据库节点的高优先级覆盖层。

#### Scenario: nodes.json 节点覆盖数据库同名节点
- **WHEN** 数据库中已有节点 `node1`（address=192.168.1.1），且 `nodes.json` 中也存在 `node1`（address=192.168.1.2）
- **THEN** 解析节点时返回 `nodes.json` 中的配置（address=192.168.1.2），即 nodes.json 具有更高优先级

#### Scenario: nodes.json 不存在时静默降级
- **WHEN** `~/.owl/nodes.json` 文件不存在
- **THEN** 系统仅从数据库读取节点，不产生错误

#### Scenario: nodes.json 解析失败时报告警告
- **WHEN** `~/.owl/nodes.json` 存在但 JSON 格式错误
- **THEN** 系统输出警告日志，继续使用数据库中的节点数据

### Requirement: 自动迁移已有数据
系统 SHALL 在首次启动时检测是否有遗留的 `nodes.json` 数据，自动迁移到数据库。

#### Scenario: 数据库为空且 nodes.json 存在时自动导入
- **WHEN** 数据库 `nodes` 表为空，且 `~/.owl/nodes.json` 存在且可解析
- **THEN** 将 `nodes.json` 中的所有节点导入到数据库 `nodes` 表，迁移完成后不删除原文件

#### Scenario: 数据库已有数据时跳过迁移
- **WHEN** 数据库 `nodes` 表非空
- **THEN** 不执行任何迁移操作

### Requirement: CLI 子命令适配数据库存储
所有节点管理子命令 SHALL 通过 `NodeStoreDB` 操作节点数据，不再直接读写 `nodes.json`。

#### Scenario: node add 写入数据库
- **WHEN** 执行 `owl node add <id> --address <addr>`
- **THEN** 节点数据写入数据库 `nodes` 表，而非 `nodes.json` 文件

#### Scenario: node remove 从数据库删除
- **WHEN** 执行 `owl node remove <id>`
- **THEN** 从数据库 `nodes` 表删除对应记录

#### Scenario: node list 从数据库查询
- **WHEN** 执行 `owl node list`
- **THEN** 显示数据库 `nodes` 表中所有节点

#### Scenario: node export 从数据库导出
- **WHEN** 执行 `owl node export --output <file>`
- **THEN** 从数据库读取所有节点并导出为 JSON/YAML 文件

#### Scenario: node import 导入到数据库
- **WHEN** 执行 `owl node import --file <file>`
- **THEN** 解析文件中的节点数据并写入数据库 `nodes` 表，冲突时提示是否覆盖

### Requirement: 初始化顺序调整
系统 SHALL 在 `root.go` 的 `Execute()` 中先初始化数据库连接，再初始化节点存储，确保节点存储可复用已有的数据库连接。

#### Scenario: 启动顺序正确
- **WHEN** 执行任何 `owl` 子命令
- **THEN** 先建立数据库连接（`NewDB`），再将连接传递给节点存储初始化，节点存储使用同一数据库连接
