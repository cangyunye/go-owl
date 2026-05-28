# Tasks

- [x] Task 1: 创建 aichat 数据模型和数据库表 ✅
  - 在 `internal/history/` 新增 `aichat.go`，定义 `AiChat` 结构体
  - 在 `internal/history/db_sqlite3.go` 的 `InitSchema()` 中新增 `aichat` 表 DDL
  - 在 `internal/history/db_duckdb.go` 的 `InitSchema()` 中新增 `aichat` 表 DDL
  - 实现 `RecordAiChat(db, *AiChat)` 函数（INSERT）
  - 实现 `QueryAiChatSessions(db, sessionID, limit)` 函数
  - 实现 `QueryAiChatSteps(db, sessionID)` 函数
  - 实现 `CleanAiChat(db, days)` 函数
  - 实现对应全局便捷函数

- [x] Task 2: 给 Agent.Process() 增加进度回调参数 ✅
  - 定义 `ProgressCallback` 类型
  - Process() 签名增加 `onProgress ProgressCallback` 参数
  - 在 6 个关键节点调用回调：route/analyze/generate/execute/result
  - 所有现有测试更新为传 `nil`，向后兼容

- [x] Task 3: owl ai 命令进度打印 + --debug + DB 记录 ✅
  - 新增 `--debug` / `-d` 标志
  - `progressLog()` 函数实现带时间戳的 stderr 进度输出 + DB 写入
  - 直接模式：用户输入头 + 进度回调 + DB 记录
  - 交互模式：Session.OnProgress + 输入 DB 记录

- [x] Task 4: 实现 `owl ai history` 子命令 ✅
  - `owl ai history list` — 会话摘要表格
  - `owl ai history show <session-id>` — 完整对话链
  - `owl ai history clean --days N` — 清理过期记录

- [x] Task 5: 全量验证 ✅
  - `go build ./cmd/cli/...` 编译通过
  - `go test ./internal/ai/...` 全部通过
  - `go test ./internal/history/...` 全部通过

# Task Dependencies

- Task 2 依赖于 Task 1
- Task 3 依赖于 Task 1, 2
- Task 4 依赖于 Task 1
- Task 5 依赖于 Task 1-4
