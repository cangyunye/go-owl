# Exec Silent Mode Spec

## Why
批量执行命令时（多节点 run 或 script），当前输出包含大量节点详情（emoji 图标、输出内容、错误信息等），在节点数量多时不利于快速扫描各节点的执行状态。需要一个简洁的 silent 模式，仅展示关键结果信息。

## What Changes
- 在 `owl exec run` 命令增加 `--silent` / `-s` 参数
- 在 `owl exec script` 命令增加 `--silent` / `-s` 参数
- silent 模式下，不打印命令/脚本执行详情（emoji、输出内容等）
- 以表格形式展示每个节点的执行结果：节点名、状态、退出码、耗时
- 每完成一个节点立即追加一行（流式输出），不等待全部完成后一次性输出
- 表格末尾打印一行汇总统计（成功/失败数量）

## Impact
- Affected specs: exec run, exec script
- Affected code: `cmd/cli/cmd/exec/run.go`, `cmd/cli/cmd/exec/script.go`, `cmd/cli/cmd/exec/run_test.go`, `cmd/cli/cmd/exec/exec_test.go`

## ADDED Requirements

### Requirement: Silent Mode for Exec Run
系统 SHALL 在 `owl exec run` 命令中提供 `--silent` / `-s` 参数，启用后仅以表格形式输出执行结果。

#### Scenario: Single node silent run
- **WHEN** 用户执行 `owl exec run uptime --nodes node1 --silent`
- **THEN** 不显示命令详情、emoji 图标、节点输出内容
- **AND** 以表格形式显示一行结果：节点名、成功/失败状态、退出码、耗时

#### Scenario: Multi-node silent run streaming
- **WHEN** 用户执行 `owl exec run uptime --nodes node1,node2,node3 --silent`
- **THEN** 不显示命令详情、emoji 图标、节点输出内容
- **AND** 每完成一个节点立即追加一行到表格（不等待全部完成）
- **AND** 表格列包含：Node、Status、Exit Code、Duration
- **AND** 表格末尾显示汇总统计行：Total: X success, Y failed

#### Scenario: Silent mode with serial execution
- **WHEN** 用户执行 `owl exec run uptime --nodes node1,node2 --silent --serial`
- **THEN** 按串行顺序逐个输出表格行，每完成一个节点追加一行

#### Scenario: Silent mode does not affect other format modes
- **WHEN** 用户同时指定 `--silent` 和 `--format json`
- **THEN** `--format json` 优先生效，按 JSON 格式输出（silent 被忽略）

### Requirement: Silent Mode for Exec Script
系统 SHALL 在 `owl exec script` 命令中提供 `--silent` / `-s` 参数，启用后仅以表格形式输出执行结果。

#### Scenario: Multi-node silent script
- **WHEN** 用户执行 `owl exec script deploy.sh --nodes node1,node2 --silent`
- **THEN** 不显示脚本信息、emoji 图标、节点输出内容
- **AND** 以表格形式显示每节点结果：节点名、状态、退出码、耗时
- **AND** 每完成一个节点立即追加一行
- **AND** 表格末尾显示汇总统计行

#### Scenario: Script with blacklist warning in silent mode
- **WHEN** 脚本包含危险命令且未使用 `--force`，用户启用 `--silent`
- **THEN** 黑名单警告和交互确认提示仍然正常显示（silent 不影响安全确认）
- **AND** 确认通过后以表格形式展示执行结果
