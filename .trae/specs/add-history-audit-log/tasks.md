# Tasks

- [x] Task 1: file upload/download 添加历史审计日志
  - [x] 1.1 在 `cmd/cli/cmd/file/upload.go` 的 `runUpload` 中：生成 UUID taskID，执行前记录 `RecordOperation(op_type=file_transfer, status=running)`
  - [x] 1.2 对每个上传结果，调用 `RecordFileTransfer` 记录文件名称、大小、传输方式、状态
  - [x] 1.3 操作完成后更新 `RecordOperation` 状态 (completed/failed/partial_failure)
  - [x] 1.4 同理在 `cmd/cli/cmd/file/download.go` 的 `runDownload` 中实现相同的记录逻辑
  - [x] 1.5 添加缺失的 `history` 和 `logger` import

- [x] Task 2: playbook run 集成实际执行引擎 + 审计日志
  - [x] 2.1 在 `cmd/cli/cmd/playbook/run.go` 中：解析 playbook YAML 文件，调用 `internal/control/playbook` 的解析器和执行器
  - [x] 2.2 生成 UUID taskID，执行前记录 `RecordOperation(op_type=playbook, status=running)`，在 command 字段存储 playbook 元数据 JSON
  - [x] 2.3 对每个 task 在每个节点上的执行结果，调用 `RecordCommandExecution` 记录
  - [x] 2.4 操作完成后更新 `RecordOperation` 状态
  - [x] 2.5 保留 playbook 文件不存在的 fallback 示例执行行为

- [x] Task 3: file transfer 集成扩散传输引擎 + 审计日志
  - [x] 3.1 在 `cmd/cli/cmd/file/transfer.go` 中：节点数 < threshold 时走 `TransferManager.Upload`
  - [x] 3.2 节点数 >= threshold 时构建扩散树，调用 `DiffusionScheduler` 执行
  - [x] 3.3 生成 UUID taskID，记录 `RecordOperation` + `RecordFileTransfer`
  - [x] 3.4 更新节点解析方式：从 `common.NodeStore` 迁移到 `node.NodeResolver`（与其他命令保持一致）

- [x] Task 4: exec script 的 taskID 格式统一为 UUID
  - [x] 4.1 将 `cmd/cli/cmd/exec/script.go` 中的 `generateTaskID()` 调用改为 `uuid.New().String()`
  - [x] 4.2 添加 `github.com/google/uuid` import

- [x] Task 5: history query 增强 — verbose 模式展示关联明细
  - [x] 5.1 在 `cmd/cli/cmd/history/history.go` 的 `printTable` 中添加 verbose 模式
  - [x] 5.2 `--verbose` 时，对每条 operation 展示其关联的 command_executions 或 file_transfers
  - [x] 5.3 确保 `--op-type` 筛选能正确过滤（当前未传递 opType 到 QueryOptions）

## Task Dependencies

- [Task 1] 无依赖
- [Task 2] 无依赖，但与 Task 1/3 可以并行
- [Task 3] 无依赖
- [Task 4] 无依赖
- [Task 5] 依赖 Task 1~4 完成（需要确认新增的记录被正确存储后才能验证查询）
