# 统一节点数据源 — 规格文档

## Why

`owl` 二进制中存在两套完全独立的节点解析路径：

| 命令 | 数据源 | 代码路径 |
|------|--------|---------|
| `owl node list` / `owl node add` 等 | 仅数据库 `NodeStoreDB` | `common.GetNodeStore()` → `NodeStoreDB.List()` |
| `owl exec run` | 数据库 + `nodes.json` 合并 (JSON 覆盖 DB) | `node.NewNodeResolver()` → `LocalSource.loadNodes()` |

当用户同时维护数据库（通过 `owl node add`）和 `nodes.json`（手动编辑）时，两个命令看到**不同的节点集合**：
- `node list` 看不到手动添加到 `nodes.json` 的节点
- `exec run` 中同 ID 节点的字段值与 `node list` 显示的不同（JSON 覆盖了 DB 值）
- 存在同名不同 ID 的节点时结果不可预测

## What Changes

### ADDED: 节点冲突检测器 `NodeConflictChecker`

在 `common` 包新增懒加载冲突检测逻辑，在**首次读取节点数据时**触发检查：

1. 从数据库读取全部节点（通过 `NodeStoreDB` 内部查询）
2. 从 `~/.owl/nodes.json` 读取全部节点
3. 检测以下冲突类型：
   - **同一数据源内部同名**：数据库内两个不同 ID 的节点具有相同 Name
   - **跨数据源同名不同 ID**：DB 中的节点 A (id=X, name=foo) 与 JSON 中的节点 B (id=Y, name=foo)
   - **跨数据源同 ID 不同字段**：DB 中的节点 X 与 JSON 中的节点 X 字段值不一致
4. 生成冲突报告

### ADDED: 交互式冲突解决

当检测到冲突时，如果运行时支持交互（stdin 是 TTY），提示用户选择：

```
⚠️  Node data conflict detected!

Database nodes:    5 nodes, 2 with conflicts
nodes.json:        3 nodes, 1 with conflicts

Conflicts found:
  [name] DB:node-1 (id=abc, addr=10.0.0.1) ⇔ JSON:node-1 (id=xyz, addr=10.0.0.2)
    → Same name but different IDs — these are different machines!
  [id]   DB:id=web-01 (addr=10.0.0.1:22) ⇔ JSON:id=web-01 (addr=10.0.0.1:2222)
    → Same ID but different port values

Choose action:
  [1] Overwrite database with nodes.json and continue
  [2] Exit (fix conflicts manually first)
  Enter choice (1/2):
```

- 选择 1：将 `nodes.json` 的全部节点写入数据库（INSERT OR REPLACE），然后继续执行
- 选择 2：退出进程，不做任何修改

### ADDED: `--sync-nodes` flag (用于 exec run)

`owl exec run` 新增标志：
- `--sync-nodes`：强制用 `nodes.json` 覆盖数据库，跳过交互提示

### MODIFIED: 启动流程

`root.go` 中 `Execute()` 改为（移除了启动时的冲突检测）：

```
1. 初始化数据库
2. MigrateNodesJSONToDB (保持现有一次性迁移)
3. 替换 globalStore 为 NodeStoreDB
4. 继续执行子命令（冲突检测在首次访问节点时懒加载触发）
```

关键设计变更：**冲突检测不在 `owl` 程序启动时触发**，而是在以下命令实际读取节点数据时才触发：

| 触发路径 | 触发时机 | 涵盖命令 |
|---------|---------|---------|
| `NodeStoreDB.List()` / `NodeStoreDB.Get()` | 首次调用时（`sync.Once`） | `owl node list`、`owl node add`、`owl node update`、`owl node remove`、`owl node status` 等所有 node 子命令 |
| `exec/run.go:handleExecNodeConflicts()` | `owl exec run` 命令执行前 | `owl exec run` |
| `exec/script.go:handleExecNodeConflicts()` | `owl exec script` 命令执行前 | `owl exec script` |

这样设计的好处：
- `owl settings`、`owl history`、`owl session` 等不涉及节点的命令不会触发冲突检测
- 非交互环境下，不操作节点的流水线调用不会因冲突而阻塞
- 每个命令进程只检测一次（`sync.Once` 确保）

### MODIFIED: `owl exec run` 及 `owl exec script`

在执行命令前检查节点冲突（通过 `handleExecNodeConflicts()`）：
- 如果 `--sync-nodes` 指定 → 自动用 JSON 覆盖 DB
- 如果有冲突 + 交互模式 → 提示用户
- 如果有冲突 + 非交互 + 无 `--sync-nodes` → 报错退出

### MODIFIED: `owl node list`

- 统一使用 `NodeStoreDB`（已是现状，不需要改，但需要确保迁移后 JSON 已同步到 DB）
- 当冲突解决后（用户选 1 覆盖），`node list` 自然能看到与 `exec run` 一致的节点

## ADDED Requirements

### Requirement: 懒加载冲突检测

#### Scenario: 无节点读取的命令不触发检测
- **WHEN** 运行 `owl settings`、`owl history`、`owl session` 等不涉及节点数据的命令
- **THEN** 不触发任何冲突检测，正常执行

#### Scenario: 首次读取节点时无冲突
- **WHEN** 运行 `owl node list` 或其他节点相关命令，且 DB 与 nodes.json 无冲突（或 nodes.json 不存在）
- **THEN** 正常执行，无提示

#### Scenario: 交互模式下首次读取节点时有冲突
- **WHEN** 运行 `owl node list`，stdin 为 TTY，且检测到冲突
- **THEN** 显示冲突报告和选择菜单
- **THEN** 用户选择 1 后覆盖 DB 并继续执行命令
- **THEN** 用户选择 2 后退出

#### Scenario: 非交互模式下首次读取节点时有冲突 (node 命令)
- **WHEN** 运行 `owl node list` 于非 TTY 环境（脚本/管道），且检测到冲突
- **THEN** 记录日志警告，继续执行（不阻塞脚本化调用）

#### Scenario: 同进程多次读取节点仅检测一次
- **WHEN** 同一进程中多次调用 `GetNodeStore().List()`
- **THEN** 冲突检测仅执行一次（`sync.Once` 保证）

### Requirement: exec run / exec script 执行前冲突检测

#### Scenario: exec run 有冲突且交互
- **WHEN** 用户在终端直接运行 `owl exec run "command"`
- **THEN** 先检测冲突，有冲突则提示用户选择

#### Scenario: exec run 有冲突且指定 --sync-nodes
- **WHEN** 运行 `owl exec run --sync-nodes "command"`
- **THEN** 自动用 nodes.json 覆盖 DB，然后执行

### Requirement: 冲突检测规则

#### 冲突类型 1：同数据源内同名
- 数据库内：`SELECT name, COUNT(*) FROM nodes GROUP BY name HAVING COUNT(*) > 1`
- nodes.json 内：遍历 JSON 数组检查重复 name

#### 冲突类型 2：跨源同名不同 ID
- 遍历 DB 的 name→id 映射和 JSON 的 name→id 映射
- 找到 name 相同但 id 不同的对

#### 冲突类型 3：跨源同 ID 不同字段
- 对于同 ID 的节点，比较 address、port、user、ssh_key、password 等关键字段
- 任一字段不同即为冲突

## Testing

自动化测试覆盖以下层级：

### 单元测试

#### `node_conflict_test.go` — 冲突检测逻辑

使用 mock 数据（不依赖真实数据库或文件系统），覆盖所有冲突检测规则：

- **`TestDetectConflicts_NoConflicts`**：DB 和 JSON 完全一致，返回空列表
- **`TestDetectConflicts_DuplicateNameInDB`**：DB 内部存在同名不同 ID 节点
- **`TestDetectConflicts_DuplicateNameInJSON`**：JSON 内部存在同名不同 ID 节点
- **`TestDetectConflicts_CrossSourceSameNameDiffID`**：DB 和 JSON 之间存在同名不同 ID
- **`TestDetectConflicts_CrossSourceSameIDDiffFields`**：同 ID 节点，字段值不同（address、port、user 等）
- **`TestDetectConflicts_MultipleConflictTypes`**：同时存在多种冲突类型
- **`TestDetectConflicts_EmptyDB`**：DB 为空，仅 JSON 有数据
- **`TestDetectConflicts_EmptyJSON`**：JSON 为空（或文件不存在），仅 DB 有数据
- **`TestDetectConflicts_BothEmpty`**：两者均为空

#### `node_conflict_test.go` — SyncNodesJSONToDB 逻辑

- **`TestSyncNodesJSONToDB_Success`**：验证 JSON 节点成功写入 DB，覆盖同 ID 旧数据
- **`TestSyncNodesJSONToDB_NewNodes`**：JSON 中存在 DB 没有的新节点，写入后 DB 包含全部节点
- **`TestSyncNodesJSONToDB_GroupsLabelsJSON`**：验证 `groups` 和 `labels` 字段的 JSON 序列化/反序列化正确

### 集成测试

#### `root_test.go` — 根命令结构

- **`TestPrintConflictReport`**：验证 `common.PrintConflictReport` 正确格式化输出
- **`TestRootCmdHasSubcommands`**：验证所有子命令正确注册

#### `exec_run_test.go` — exec 命令

- **`TestPrintConflictReportFromCommon`**：验证冲突报告格式
- **`TestNewRunCmd_HasSyncNodesFlag`**：验证 `--sync-nodes` flag 已注册

### 测试基础设施

- 使用 `database/sql` 打开 `:memory:` SQLite 数据库，隔离测试环境
- `NodeStoreDB` 已通过 `NewNodeStoreDB(db)` 支持注入 `*sql.DB`，无需改动
- 对 `nodes.json` 读写使用 `t.TempDir()` 创建临时目录，避免污染实际文件系统
- 测试文件放在与被测源文件相同的 package 目录下，使用 `_test.go` 后缀
- 使用标准库 `testing` 包，不引入第三方断言库

## Impact

- Affected code:
  - `go-owl/cmd/cli/cmd/common/node_conflict.go` — 新增冲突检测、懒加载 `EnsureNodesConsistent`、`PrintConflictReport`
  - `go-owl/cmd/cli/cmd/common/node_store_db.go` — 新增 `BulkUpsert` 方法，`sync.Once` 懒加载冲突检测
  - `go-owl/cmd/cli/cmd/root.go` — 仅保留 `InitNodeStoreFromDB`，移除启动冲突检测
  - `go-owl/cmd/cli/cmd/exec/run.go` — 新增 `--sync-nodes` flag 和 `handleExecNodeConflicts`
  - `go-owl/cmd/cli/cmd/exec/script.go` — 新增 `handleExecNodeConflicts()` 调用
- Affected tests:
  - `go-owl/cmd/cli/cmd/common/node_conflict_test.go` — 冲突检测单元测试（新增）
  - `go-owl/cmd/cli/cmd/root_test.go` — 根命令结构测试（新增）
  - `go-owl/cmd/cli/cmd/exec/run_test.go` — exec 命令测试（新增）
