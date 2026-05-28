# 增强 AI 提示词 —— owl exec 命令系列的自然语言映射

## Why

当前 `execute_command` 工具仅支持 `targets`、`command`、`timeout` 三个参数，缺少 `owl exec run` 实际支持的 `--group`、`--label`、`--format`、`--mode`（parallel/serial/async），且完全没有 `owl exec script` 对应的工具。LLM 无法将 "在 web 组节点上执行 df -h" 或 "在 node1 执行脚本 deploy.sh" 等自然语言请求映射为完整的 tool_call JSON。

## What Changes

- **MODIFIED** `internal/ai/tools.go` — 增强 `ExecuteCommandTool`，补充 `group`、`label`、`format`、`mode` 参数；新增 `ExecuteScriptTool`
- **MODIFIED** `internal/ai/agent.go` — 注册新增的 `execute_script` 工具
- **MODIFIED** `internal/ai/validator.go` — 新增 `ValidateExecuteScript`；修改 `ValidateExecuteCommand` 支持 `targets` 与 `group`/`label` 互斥
- **MODIFIED** `internal/ai/prompts/prompts.go` — 重写 `SystemPrompt` 中 exec 相关部分（含完整参数表 + 丰富示例）；新增 `ExecuteCommandPrompt`、`ExecuteScriptPrompt` 操作专项提示模板

## Impact

- Affected specs: 无
- Affected code:
  - `internal/ai/tools.go` — 工具定义 (Execute + ExecuteScript)
  - `internal/ai/agent.go` — 工具注册
  - `internal/ai/validator.go` — 参数校验
  - `internal/ai/prompts/prompts.go` — 提示词

## MODIFIED Requirements

### Requirement: execute_command 工具完整参数

`execute_command` 工具 SHALL 支持以下参数，与 `owl exec run` 对齐：

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `command` | string | 是 | 要执行的 shell 命令 |
| `targets` | string[] | 否* | 目标节点名称列表 |
| `group` | string | 否* | 按分组选择节点 |
| `label` | string | 否* | 按标签选择节点（格式 "key=value"） |
| `timeout` | int | 否 | 命令超时秒数，默认 30 |
| `format` | string | 否 | 输出格式："simple"（默认）、"detail"、"json" |
| `mode` | string | 否 | 执行模式："parallel"（默认）、"serial"、"async" |

> *targets / group / label 至少提供一个（互斥关系，优先 targets > group > label）

#### Scenario: 按分组执行含输出格式
- **GIVEN** 用户说 "在 web 节点上执行 df -h，用 json 格式输出"
- **THEN** `command="df -h"`, `group="web"`, `format="json"`, `mode="parallel"`（默认）

#### Scenario: 异步执行长任务
- **GIVEN** 用户说 "在所有节点上异步执行 long-task.sh"
- **THEN** `command="long-task.sh"`, `targets=[全部节点名]`, `mode="async"`

#### Scenario: 指定节点串行执行
- **GIVEN** 用户说 "在 web-01、web-02 串行执行 systemctl restart nginx"
- **THEN** `command="systemctl restart nginx"`, `targets=["web-01","web-02"]`, `mode="serial"`

#### Scenario: 按标签执行带超时
- **GIVEN** 用户说 "在标签 env=prod 的节点执行 curl api.example.com，超时 10 秒"
- **THEN** `command="curl api.example.com"`, `label="env=prod"`, `timeout=10`

## ADDED Requirements

### Requirement: execute_script 工具（新增）

系统 SHALL 新增 `execute_script` 工具，映射 `owl exec script` 命令：

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `script` | string | 是 | 本地脚本文件路径 |
| `targets` | string[] | 否* | 目标节点名称列表 |
| `group` | string | 否* | 按分组选择节点 |
| `label` | string | 否* | 按标签选择节点 |
| `dest` | string | 否 | 远程存放目录，默认 "/tmp" |
| `args` | string | 否 | 传递给脚本的参数 |
| `timeout` | int | 否 | 脚本执行超时秒数，默认 300 |
| `inline` | bool | 否 | 直接内容执行（不留文件），默认 false |
| `keep` | bool | 否 | 执行后保留脚本文件，默认 false |

> *targets / group / label 至少提供一个

#### Scenario: 基本脚本执行
- **GIVEN** 用户说 "在 web-01 上执行脚本 deploy.sh"
- **THEN** `script="./deploy.sh"`, `targets=["web-01"]`, `dest="/tmp"`（默认）

#### Scenario: 带参数脚本执行
- **GIVEN** 用户说 "在所有 web 节点执行 setup.sh，参数 --env prod --version 1.0"
- **THEN** `script="./setup.sh"`, `group="web"`, `args="--env prod --version 1.0"`

#### Scenario: inline 安全执行
- **GIVEN** 用户说 "在 node1 上用 inline 模式执行检查脚本 check.sh"
- **THEN** `script="./check.sh"`, `targets=["node1"]`, `inline=true`

#### Scenario: 保留脚本便于调试
- **GIVEN** 用户说 "在 web-01 执行 init.sh 并保留脚本文件"
- **THEN** `script="./init.sh"`, `targets=["web-01"]`, `keep=true`

### Requirement: AI 系统提示词（exec 部分）

系统提示词中 exec 相关部分 SHALL 包含：
1. `execute_command` 和 `execute_script` 的完整参数表格
2. 每个工具至少 3 个覆盖不同参数组合的示例
3. 关键区分规则：命令执行用 `execute_command`，脚本文件执行用 `execute_script`
4. 节点选择规则：targets（指定名称）/ group（分组）/ label（标签）三选一的互斥逻辑
5. 模式选择指南：< 10s 任务用 parallel、需观察顺序用 serial、> 60s 任务用 async
6. 危险命令清单对照表（与 blacklist 包对齐）

#### Scenario: 正确区分命令和脚本
- **GIVEN** 用户说 "执行 uptime"
- **THEN** LLM 调用 `execute_command`（因为是内建命令）
- **GIVEN** 用户说 "执行 ./scripts/deploy.sh"
- **THEN** LLM 调用 `execute_script`（因为是脚本文件路径）

### Requirement: exec 操作专项提示模板

系统 SHALL 提供两个操作专项提示模板，用于多轮工具调用时动态注入：

| 模板名称 | 对应工具 | 内容 |
|----------|----------|------|
| `ExecuteCommandPrompt` | execute_command | 完整参数表、模式选择指南、危险命令清单、节点筛选示例 |
| `ExecuteScriptPrompt` | execute_script | 完整参数表、inline vs 文件模式对比、参数传递格式 |

#### Scenario: 多轮补全
- **GIVEN** 第一轮 LLM 输出了不完整的参数 `{"command":"df -h","group":"web"}`
- **WHEN** Agent 进入第二轮
- **THEN** 注入 `ExecuteCommandPrompt` 模板，提示 LLM 可补充 `format`、`mode`、`timeout` 参数
