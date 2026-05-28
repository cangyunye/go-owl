# Add History Audit Log Spec

## Why
OWL 的 `exec run`、`exec script`、`playbook run`、`file upload/download/transfer` 等核心操作在 CLI 层存在以下问题：
1. **playbook run 完全缺失**——当前只是模拟，没有实际执行 Playbook 引擎，更没有记录日志
2. **file upload/download 缺失历史记录**——没有任何 `RecordOperation` / `RecordFileTransfer` 调用
3. **file transfer 完全缺失**——当前为模拟，没有实际调用扩散传输引擎，没有日志
4. **exec run 和 exec script 的记录散落在 CLI 层**——没有统一的审计中间件，容易遗漏
5. **历史记录缺少关键操作信息**——如 `playbook_name`、`playbook_tags`、`file_path`、`file_size` 等字段
6. **history query 展示内容有限**——当前只显示 operation 表，不展示关联的执行明细、传输记录

需要为这些关键命令设计统一的日志/审计记录系统，确保每次操作可追溯、可审计。

## What Changes

1. **playbook run 集成实际执行**：将 `playbook run` 从模拟实现改为调用 `internal/control/playbook` 的执行引擎，并在执行过程中记录操作日志和每个 task 的执行明细
2. **file upload/download 添加历史记录**：在 upload/download 流程中注入 `RecordOperation` + `RecordFileTransfer` 调用
3. **file transfer 集成实际执行 + 历史记录**：将 `file transfer` 从模拟改为调用扩散传输引擎，并记录日志
4. **Operation 模型扩展**：新增 `PlaybookOperation` 和 `FileOperation` 字段，在 operations 表的 `command` 字段中存储结构化 JSON 元数据
5. **history query 增强**：支持 `--verbose` 展示关联的命令执行记录/文件传输记录，支持 `--op-type playbook` 筛选

## Impact

- Affected specs: history 查询能力增强，历史记录完整性提升
- Affected code:
  - `cmd/cli/cmd/exec/run.go` —— 已有记录，需规范化
  - `cmd/cli/cmd/exec/script.go` —— 已有记录，需规范化
  - `cmd/cli/cmd/playbook/run.go` —— **大幅重写**
  - `cmd/cli/cmd/file/upload.go` —— 新增 RecordOperation/RecordFileTransfer
  - `cmd/cli/cmd/file/download.go` —— 新增 RecordOperation/RecordFileTransfer
  - `cmd/cli/cmd/file/transfer.go` —— **大幅重写**
  - `internal/history/history.go` —— 新增 FileTransfer 便捷 Record 函数
  - `internal/history/db_sqlite3.go` —— operations 表无需变更，使用现有结构
  - `cmd/cli/cmd/history/history.go` —— 增强 verbose 模式展示细节

## ADDED Requirements

### Requirement: Playbook Run 实际执行 + 审计日志

The system SHALL execute YAML playbooks using `internal/control/playbook` engine and record full audit logs.

#### Scenario: playbook run 成功执行
- **WHEN** 用户执行 `owl playbook run site.yml --nodes node1,node2`
- **THEN** 系统解析 playbook YAML，调用 Playbook Executor 执行
- **AND** 在 `operations` 表记录一条 `op_type=playbook` 的操作记录
- **AND** 在 `command_executions` 表记录每个 task 在每个节点上的执行结果

#### Scenario: playbook file 不存在，保持示例行为
- **WHEN** 用户执行 `owl playbook run site.yml` 且 `site.yml` 不存在
- **THEN** 系统输出示例执行信息（保持当前 fallback 行为）

### Requirement: File Upload/Download 审计日志

The system SHALL record file transfer operations in history database.

#### Scenario: file upload 记录
- **WHEN** 用户执行 `owl file upload app.tar.gz --nodes node1 --dest /opt/`
- **THEN** 系统在 `operations` 表记录一条 `op_type=file_transfer` 的操作（status: running）
- **AND** 对每个节点的结果，在 `file_transfers` 表记录文件名称、大小、传输方式、状态
- **AND** 操作完成后更新 operation 状态为 completed/failed/partial_failure

#### Scenario: file download 记录
- **WHEN** 用户执行 `owl file download /var/log/app.log --nodes node1 --dest ./logs/`
- **THEN** 系统在 `operations` 表记录一条 `op_type=file_transfer` 的操作
- **AND** 对每个节点结果记录 `file_transfers` 明细

### Requirement: File Transfer 实际执行 + 审计日志

The system SHALL execute diffusion transfer using `internal/control/transfer/diffusion_transfer.go` and record audit logs.

#### Scenario: file transfer 成功执行
- **WHEN** 用户执行 `owl file transfer app.tar.gz --nodes node1,...,nodeN`
- **THEN** 系统构建扩散树，调用 `DiffusionScheduler` 执行
- **AND** 记录 `op_type=file_transfer` 的操作，以及每个节点的传输记录

#### Scenario: 节点数小于阈值
- **WHEN** 节点数 < threshold
- **THEN** 使用直接传输（Direct Transfer），走 TransferManager.Upload

### Requirement: History Query 增强

The system SHALL support viewing detailed audit information.

#### Scenario: verbose 模式展示执行明细
- **WHEN** 用户执行 `owl history --verbose`
- **THEN** 对每条操作记录，展示其关联的命令执行/文件传输明细

#### Scenario: 按 op-type 筛选
- **WHEN** 用户执行 `owl history --op-type playbook`
- **THEN** 只展示 playbook 类型的操作

## MODIFIED Requirements

### Requirement: 现有 `exec run` 和 `exec script` 的历史记录规范化

**Change**: 保持现有记录逻辑，统一使用 `uuid.New().String()` 生成 taskID（exec script 当前使用 `generateTaskID()` 返回 `task-<timestamp>` 格式，需统一为 UUID 格式）

### Requirement: 现有 operations 表的 command 字段支持结构化元数据

**Change**: 对于 playbook 操作，`command` 字段存储 `{"playbook":"site.yml","tags":"nginx","check":true}` 的 JSON 字符串；对于 file 操作，存储 `{"local_path":"app.tar.gz","remote_path":"/opt/app.tar.gz"}`。

## REMOVED Requirements

无
