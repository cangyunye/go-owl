# Checklist

## 代码实现

* [x] `internal/logfile/writer.go` 实现 `NodeLogWriter`，提供 `NewNodeLogWriter` 和 `AppendEntry` 方法

* [x] 日志目录默认为 `~/.owl/logs/nodes/`，可通过 `OWL_LOG_DIR` 环境变量自定义

* [x] 日志文件按节点 ID 命名：`<node_id>.log`

* [x] `AppendEntry` 自动创建不存在的日志目录和文件

* [x] `AppendEntry` 使用 `O_APPEND` 模式打开文件，确保追加写入

* [x] 并发写入同一节点日志时使用 `sync.Mutex` 按 nodeID 加锁，防止内容交错

* [x] 日志条目格式符合 spec 定义（分隔线、时间戳、Task ID、命令、退出码、耗时、完整输出、ERROR 字段）

## 命令集成

* [x] `owl exec run` 执行后对应节点日志文件新增一条记录

* [x] `owl exec script` 执行后对应节点日志文件新增记录

* [x] `owl playbook run` 执行后每个 task 结果追加到对应节点日志

* [x] 日志写入不受 `--silent`、`--format json`、`--format detail` 模式影响（始终写入）

## 单元测试

* [x] TC-LOG-001: `NewNodeLogWriter` 默认路径（`~/.owl/logs/nodes/`）

* [x] TC-LOG-002: `OWL_LOG_DIR` 环境变量覆盖路径

* [x] TC-LOG-003: `AppendEntry` 写入一条记录，验证文件内容和格式

* [x] TC-LOG-004: `AppendEntry` 多次追加写入，验证日志条目不覆盖、顺序正确

* [x] TC-LOG-005: 自动创建不存在目录和文件

* [x] TC-LOG-006: 并发写入同一节点（多 goroutine），验证无交错

* [x] TC-LOG-007: 失败场景日志格式（exit code != 0，含 ERROR 字段）

* [x] TC-LOG-008: 空输出日志正常写入

* [x] `go test ./internal/logfile/...` 全部通过

## 开发者文档

* [x] 创建 `docs/dev/NODE_EXECUTION_LOG.md`

* [x] 文档包含：功能概述、日志文件路径规则、日志条目格式说明、并发安全机制

* [x] 文档包含：集成方式（各命令如何调用 `AppendEntry`）

* [x] 文档包含：测试用例清单（TC-LOG-001 \~ TC-LOG-008）

* [x] 更新 `docs/dev/README.md`，在文档索引中添加 `NODE_EXECUTION_LOG.md` 链接

## 回归验证

* [x] `go test ./cmd/cli/cmd/exec/...` 全部通过

* [x] `go test ./cmd/cli/cmd/playbook/...` 全部通过

* [x] `go build ./...` 全量编译通过

