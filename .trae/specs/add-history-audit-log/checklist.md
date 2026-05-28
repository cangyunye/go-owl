# Checklist

## Task 1: file upload/download 添加历史审计日志
- [x] file upload 执行前记录 RecordOperation(op_type=file_transfer, status=running)
- [x] file upload 对每个结果记录 RecordFileTransfer（含文件名、大小、传输方式、状态、错误）
- [x] file upload 完成后更新 operation 状态 (completed/failed/partial_failure)
- [x] file download 执行前记录 RecordOperation
- [x] file download 对每个结果记录 RecordFileTransfer
- [x] file download 完成后更新 operation 状态
- [x] 缺失的 import 已添加（history, logger, uuid, time）

## Task 2: playbook run 集成实际执行引擎 + 审计日志
- [x] playbook 文件存在时，调用 parser.ParsePlaybook 解析 YAML
- [x] playbook 文件存在时，调用 playbook executor 实际执行
- [x] 执行前记录 RecordOperation(op_type=playbook)
- [x] 对每个 task 在每个节点记录 RecordCommandExecution
- [x] 操作完成后更新 operation 状态
- [x] playbook 文件不存在时，保留示例执行 fallback 行为
- [x] command 字段存储结构化元数据 JSON

## Task 3: file transfer 集成扩散传输引擎 + 审计日志
- [x] 节点数 < threshold 时走 TransferManager.Upload（直接传输）
- [x] 节点数 >= threshold 时构建扩散树，调用 DiffusionScheduler
- [x] 记录 RecordOperation(op_type=file_transfer)
- [x] 记录每个节点的 RecordFileTransfer
- [x] 节点解析从 common.NodeStore 迁移到 node.NodeResolver
- [x] 上传/扩散状态正确追踪

## Task 4: exec script 的 taskID 格式统一为 UUID
- [x] generateTaskID() 替换为 uuid.New().String()
- [x] 已添加 uuid import

## Task 5: history query 增强
- [x] --verbose 模式正确展示关联的 command_executions
- [x] --verbose 模式正确展示关联的 file_transfers
- [x] --op-type 筛选正确传递到 QueryOptions
- [x] 表格输出格式清晰可读
