# 任务列表

- [ ] 任务 1：在 common 包新增冲突检测模块 `node_conflict.go`
  - 定义 `NodeConflict` 结构体（冲突类型、DB节点、JSON节点、描述）
  - 实现 `DetectConflicts(dbNodes, jsonNodes []*NodeInfo) []NodeConflict`
  - 三种冲突检测：数据源内同名、跨源同名不同ID、跨源同ID不同字段
  - 实现 `SyncNodesJSONToDB(db *sql.DB) error` — 将 nodes.json 覆盖写入 DB
  - 实现 `EnsureNodesConsistent(db *sql.DB) error` — 懒加载统一冲突检测入口
  - 实现 `resolveNodeConflicts(db, dbNodes, jsonNodes) error` — 冲突解决核心逻辑
  - 实现 `PrintConflictReport(conflicts, dbCount, jsonCount)` — 格式化冲突报告

- [ ] 任务 2：给 NodeStoreDB 新增 `BulkUpsert` 方法 + 懒加载冲突检测
  - `BulkUpsert(nodes []*NodeInfo) error` — 使用 `INSERT OR REPLACE` 批量写入
  - 新增 `sync.Once` 字段 `checkOnce`，确保每个实例只触发一次冲突检测
  - `List()` 和 `Get()` 方法在首次调用时通过 `ensureConsistent()` 触发懒加载冲突检测
  - 将纯查询逻辑抽取为 `listInternal()`，避免递归循环

- [ ] 任务 3：修改 root.go 启动流程
  - 移除启动时的 `handleNodeConflicts()` 调用
  - 仅保留 `InitNodeStoreFromDB`，冲突检测改为懒加载

- [ ] 任务 4：修改 exec/run.go 和 exec/script.go
  - 新增 `--sync-nodes` flag
  - `handleExecNodeConflicts()` 调用 `common.EnsureNodesConsistent()` 做懒加载检查
  - `owl exec script` 同样添加 `handleExecNodeConflicts()` 调用
  - 处理三种场景：交互提示 / --sync-nodes / 非交互报错

- [ ] 任务 5：自动化测试
  - 在 `go-owl/cmd/cli/cmd/common/` 新增 `node_conflict_test.go`
  - 编写 `DetectConflicts()` 单元测试（9 个用例：无冲突、DB内同名、JSON内同名、跨源同名不同ID、跨源同ID不同字段、多类型组合、空DB、空JSON、两者皆空）
  - 编写 `SyncNodesJSONToDB()` 单元测试（3 个用例：覆盖写入、新增节点、GroupsLabels序列化）
  - 编写 `BulkUpsert()` 单元测试（4 个用例：新增、覆盖、JSON序列化、空切片）
  - 编写 `ReadNodesFromJSON` 单元测试
  - 在 `go-owl/cmd/cli/cmd/` 新增 `root_test.go` — 冲突报告输出、子命令注册
  - 在 `go-owl/cmd/cli/cmd/exec/` 新增 `run_test.go` — flag 注册、冲突报告输出
  - 所有测试使用 `:memory:` SQLite + `t.TempDir()` 隔离环境

- [ ] 任务 6：编译验证
  - `go build ./...` 确保 owl 编译通过
  - `go test ./...` 确保所有测试通过

# 任务依赖
- 任务 2 依赖任务 1（BulkUpsert 用于 SyncNodesJSONToDB，EnsureNodesConsistent 用于懒加载）
- 任务 3、4 依赖任务 1、2
- 任务 5 依赖任务 1、2、3、4（实现完成后编写测试）
- 任务 6 依赖所有前置任务
