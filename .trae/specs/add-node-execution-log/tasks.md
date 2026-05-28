# Tasks

- [x] Task 1: 创建 `internal/logfile/` 节点日志写入包
  - [x] SubTask 1.1: 创建 `internal/logfile/writer.go`，实现 `NodeLogWriter` 结构体
  - [x] SubTask 1.2: 实现 `NewNodeLogWriter(logDir string)` 构造函数，支持默认路径 `~/.owl/logs/nodes/` 和 `OWL_LOG_DIR` 环境变量覆盖
  - [x] SubTask 1.3: 实现 `AppendEntry(nodeID, taskID, command string, exitCode int, output string, errMsg string, duration time.Duration)` 方法，格式化并追加写入日志
  - [x] SubTask 1.4: 实现并发安全：使用 `sync.Mutex` map 按 nodeID 加锁，防止同一节点日志写入交错
  - [x] SubTask 1.5: 自动创建日志目录和文件（`os.MkdirAll` + `os.OpenFile` with `O_APPEND|O_CREATE|O_WRONLY`）

- [x] Task 2: 在 `owl exec run` 中集成节点日志写入
  - [x] SubTask 2.1: 在 `runExecRun()` 初始化 `logfile.NewNodeLogWriter("")`
  - [x] SubTask 2.2: 在 `processResult` 闭包中调用 `nodeLogWriter.AppendEntry(...)` 写入日志
  - [x] SubTask 2.3: 确认 silent 模式、json 模式、detail 模式均能正常写入日志（日志写入不受输出模式影响）

- [x] Task 3: 在 `owl exec script` 中集成节点日志写入
  - [x] SubTask 3.1: 在 `runScript()` 初始化 node log writer
  - [x] SubTask 3.2: 在结果循环中调用 `AppendEntry` 写入每条结果

- [x] Task 4: 在 `owl playbook run` 中集成节点日志写入
  - [x] SubTask 4.1: 在 `runPlaybookRun()` 初始化 node log writer
  - [x] SubTask 4.2: 在每个 task 的每个 node result 中调用 `AppendEntry` 写入日志
  - [x] SubTask 4.3: playbook task 的 command 字段使用 `task_name: action args` 格式

- [x] Task 5: 编写 `internal/logfile/` 包的单元测试
  - [x] SubTask 5.1: 创建 `internal/logfile/writer_test.go`
  - [x] SubTask 5.2: 测试用例 TC-LOG-001：`NewNodeLogWriter` 默认路径（`~/.owl/logs/nodes/`）
  - [x] SubTask 5.3: 测试用例 TC-LOG-002：`OWL_LOG_DIR` 环境变量覆盖路径
  - [x] SubTask 5.4: 测试用例 TC-LOG-003：`AppendEntry` 写入一条记录，验证文件内容和格式
  - [x] SubTask 5.5: 测试用例 TC-LOG-004：`AppendEntry` 多次追加写入，验证日志条目不覆盖、顺序正确
  - [x] SubTask 5.6: 测试用例 TC-LOG-005：自动创建不存在目录和文件
  - [x] SubTask 5.7: 测试用例 TC-LOG-006：并发写入同一节点（多 goroutine），验证无交错
  - [x] SubTask 5.8: 测试用例 TC-LOG-007：失败场景日志格式（exit code != 0，含 ERROR 字段）
  - [x] SubTask 5.9: 测试用例 TC-LOG-008：空输出日志正常写入

- [x] Task 6: 编写开发者文档
  - [x] SubTask 6.1: 创建 `docs/dev/NODE_EXECUTION_LOG.md`
  - [x] SubTask 6.2: 文档内容包含：功能概述、日志文件路径规则、日志条目格式说明、并发安全机制、集成方式（各命令如何调用）、测试用例清单（TC-LOG-001 ~ TC-LOG-008）
  - [x] SubTask 6.3: 更新 `docs/dev/README.md`，在文档索引中添加 `NODE_EXECUTION_LOG.md` 的链接

- [x] Task 7: 运行测试验证
  - [x] SubTask 7.1: 运行 `go test ./internal/logfile/...` 确保新包测试通过
  - [x] SubTask 7.2: 运行 `go test ./cmd/cli/cmd/exec/...` 确保现有测试通过
  - [x] SubTask 7.3: 运行 `go test ./cmd/cli/cmd/playbook/...` 确保现有测试通过
  - [x] SubTask 7.4: 运行 `go build ./...` 确保全量编译通过

# Task Dependencies
- Task 2, 3, 4 均依赖 Task 1（需要 `internal/logfile/` 包先实现）
- Task 2, 3, 4 之间无依赖，可并行实现
- Task 5 依赖 Task 1（测试 `logfile` 包本身）
- Task 6 依赖 Task 1（文档需要描述已实现的包结构）
- Task 7 依赖 Task 1~6
