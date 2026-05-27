# 验证清单

### 冲突检测模块 (common/node_conflict.go)
- [ ] `NodeConflict` 结构体正确导出
- [ ] `DetectConflicts()` 检测同源内同名冲突
- [ ] `DetectConflicts()` 检测跨源同名不同ID冲突
- [ ] `DetectConflicts()` 检测跨源同ID不同字段冲突
- [ ] 无冲突时返回空列表
- [ ] nodes.json 不存在时返回空列表
- [ ] `EnsureNodesConsistent(db)` 懒加载统一冲突检测入口
- [ ] `resolveNodeConflicts(db, dbNodes, jsonNodes)` 冲突解决核心逻辑
- [ ] `PrintConflictReport()` 格式化输出冲突报告
- [ ] `SyncNodesJSONToDB()` 成功覆盖数据库

### NodeStoreDB 懒加载 (common/node_store_db.go)
- [ ] `BulkUpsert` 正确处理 groups 和 labels 的 JSON 序列化
- [ ] `sync.Once` 确保冲突检测每个实例仅触发一次
- [ ] `List()` 首次调用时触发 `ensureConsistent()`
- [ ] `Get()` 首次调用时触发 `ensureConsistent()`
- [ ] `listInternal()` 纯查询不触发检测（避免递归）

### 启动流程 (root.go)
- [ ] 启动时仅做 InitNodeStoreFromDB，**不触发**冲突检测
- [ ] `owl settings`、`owl history` 等非节点命令不触发任何冲突检测

### 懒加载触发时机
- [ ] `owl node list` — 首次 List() 时触发
- [ ] `owl node update` — 首次 Get() 时触发
- [ ] `owl node remove` — 首次 Get() 时触发
- [ ] `owl node add` — 首次 Get() (查重) 时触发
- [ ] `owl node check` / `owl node ping` — 首次 List() 时触发
- [ ] `owl exec run` — 通过 `handleExecNodeConflicts()` 触发
- [ ] `owl exec script` — 通过 `handleExecNodeConflicts()` 触发
- [ ] 交互模式有冲突时显示菜单
- [ ] 用户选择 1 后 DB 被覆盖且继续执行命令
- [ ] 用户选择 2 后进程退出
- [ ] 非交互模式 node 命令有冲突时日志警告不阻塞
- [ ] 非交互模式 exec 命令有冲突时报错退出

### exec run / exec script
- [ ] `--sync-nodes` flag 已注册
- [ ] 指定 `--sync-nodes` 时自动覆盖并执行
- [ ] 交互模式有冲突时提示用户
- [ ] 非交互无 `--sync-nodes` 时报错退出

### 自动化测试

#### 冲突检测单元测试 (`node_conflict_test.go`)
- [ ] `TestDetectConflicts_NoConflicts` — DB 和 JSON 一致时返回空
- [ ] `TestDetectConflicts_DuplicateNameInDB` — 检测 DB 内同名不同ID
- [ ] `TestDetectConflicts_DuplicateNameInJSON` — 检测 JSON 内同名不同ID
- [ ] `TestDetectConflicts_CrossSourceSameNameDiffID` — 检测跨源同名不同ID
- [ ] `TestDetectConflicts_CrossSourceSameIDDiffFields` — 检测跨源同ID不同字段
- [ ] `TestDetectConflicts_MultipleConflictTypes` — 同时检测多种冲突
- [ ] `TestDetectConflicts_EmptyDB` — DB 为空时无冲突
- [ ] `TestDetectConflicts_EmptyJSON` — JSON 为空时无冲突
- [ ] `TestDetectConflicts_BothEmpty` — 两者为空时无冲突

#### SyncNodesJSONToDB 单元测试
- [ ] `TestSyncNodesJSONToDB_Success` — 覆盖写入成功
- [ ] `TestSyncNodesJSONToDB_Overwrite` — 覆盖同ID旧数据

#### BulkUpsert 单元测试
- [ ] `TestBulkUpsert_InsertNewNodes` — 批量新增
- [ ] `TestBulkUpsert_ReplaceExistingNode` — 覆盖已存在节点
- [ ] `TestBulkUpsert_GroupsLabelsJSON` — JSON 序列化/反序列化一致
- [ ] `TestBulkUpsert_EmptySlice` — 空切片无错误

#### ReadNodesFromJSON 单元测试
- [ ] `TestReadNodesFromJSON_FileNotExist` — 文件不存在返回 nil
- [ ] `TestReadNodesFromJSON_ValidFile` — 正确解析 JSON 文件

#### 根命令结构测试 (`root_test.go`)
- [ ] `TestPrintConflictReport` — 冲突报告输出格式正确
- [ ] `TestRootCmdHasSubcommands` — 所有子命令已注册

#### exec 命令测试 (`run_test.go`)
- [ ] `TestPrintConflictReportFromCommon` — 冲突报告格式正确
- [ ] `TestNewRunCmd_HasSyncNodesFlag` — --sync-nodes flag 已注册

#### 测试基础设施
- [ ] 所有测试使用 `:memory:` SQLite 隔离数据库
- [ ] 文件操作使用 `t.TempDir()` 避免污染真实文件系统
- [ ] 测试文件遵循 `_test.go` 命名规范
- [ ] 仅使用标准库 `testing`，不引入第三方断言库

### 全局
- [ ] `owl node list` 在同步后显示与 exec 一致的节点
- [ ] `go build ./...` 编译通过
- [ ] `go test ./...` 全部测试通过
