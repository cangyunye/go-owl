# Node Execution Log Spec

## Why
当前 `owl exec run`、`owl exec script`、`owl playbook run` 等批量执行命令的历史记录仅存储在 SQLite 数据库中，且 `stdout` 被截断为 4096 字符。运维场景中需要完整保留每个节点上的命令执行记录（完整的 stdout/stderr），以便事后审计、排查问题。需要一个按节点 ID 组织的日志文件系统，追加式记录每次执行的完整信息。

## What Changes
- 新增节点执行日志文件系统：每次在节点上执行命令后，将完整记录追加写入 `~/.owl/logs/nodes/<node_id>.log`
- 日志包含：时间戳、Task ID、命令内容、退出码、耗时、完整 stdout/stderr
- 覆盖范围：`owl exec run`、`owl exec script`、`owl playbook run`
- 日志文件按节点 ID 组织，一个节点一个日志文件，终生追加
- 并发安全：使用文件级写锁，确保并行执行时日志不交错

## Impact
- Affected specs: exec run, exec script, playbook run
- Affected code:
  - `cmd/cli/cmd/exec/run.go` — 在 `processResult` 中追加写入
  - `cmd/cli/cmd/exec/script.go` — 在结果循环中追加写入
  - `cmd/cli/cmd/playbook/run.go` — 在每个 task 结果中追加写入
  - 新增 `internal/logfile/` 包 — 节点日志文件写入工具
- Affected docs:
  - 新增 `docs/dev/NODE_EXECUTION_LOG.md` — 节点执行日志功能设计文档

## ADDED Requirements

### Requirement: Per-Node Execution Log File
系统 SHALL 在 `~/.owl/logs/nodes/<node_id>.log`（可通过环境变量 `OWL_LOG_DIR` 配置）为每个节点维护一个追加式执行日志文件。

#### Scenario: exec run 写入节点日志
- **WHEN** 用户执行 `owl exec run uptime --nodes web-01`
- **THEN** 命令执行完成后，在 `~/.owl/logs/nodes/web-01.log` 追加一条记录
- **AND** 记录包含：时间戳、Task ID、命令 `uptime`、退出码、耗时、完整 stdout/stderr

#### Scenario: exec script 写入节点日志
- **WHEN** 用户执行 `owl exec script deploy.sh --nodes web-01,web-02`
- **THEN** 每个节点执行完成后，分别追加到 `web-01.log` 和 `web-02.log`

#### Scenario: playbook run 写入节点日志
- **WHEN** 用户执行 `owl playbook run site.yml --nodes web-01`
- **THEN** playbook 中每个 task 在每个节点上执行完成后，追加到对应节点的日志文件

#### Scenario: 日志文件自动创建
- **WHEN** 某节点的日志文件尚不存在
- **THEN** 自动创建日志文件及父目录 `~/.owl/logs/nodes/`

### Requirement: Log Entry Format
每条日志 SHALL 包含完整的执行上下文，格式清晰可读。

#### Scenario: 日志条目格式
- **WHEN** 一条执行记录被写入日志文件
- **THEN** 格式如下（每条记录以分隔线隔开）：

```
──────────────────────────────────────────────────────────────────────
[2026-05-28 15:30:45] TASK: 550e8400-e29b-41d4-a716-446655440000
COMMAND: uptime
EXIT CODE: 0
DURATION: 1.23s
OUTPUT:
 15:30:45 up 30 days,  2:15,  3 users,  load average: 0.00, 0.01, 0.05
──────────────────────────────────────────────────────────────────────
```

- **AND** 如果执行失败（exit code != 0 或有 error），追加 ERROR 字段

### Requirement: Concurrent Write Safety
系统 SHALL 保证并发执行时（多个 goroutine 同时向同一节点写入日志）日志内容不会交错。

#### Scenario: 并行执行不交错
- **WHEN** 用户在并行模式下对同一节点执行多个命令
- **THEN** 同一时刻只有一个 goroutine 能写入该节点的日志文件
- **AND** 不同 goroutine 的日志条目完整、不交错

### Requirement: Configurable Log Directory
系统 SHALL 支持通过环境变量 `OWL_LOG_DIR` 配置日志目录，默认 `~/.owl/logs/nodes/`。

#### Scenario: 自定义日志目录
- **WHEN** 用户设置 `OWL_LOG_DIR=/var/log/owl/nodes`
- **THEN** 日志文件写入 `/var/log/owl/nodes/<node_id>.log`
