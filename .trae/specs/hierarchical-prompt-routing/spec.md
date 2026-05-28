# 分层提示词路由 —— 多轮意图精确匹配与 Token 节省

## Why

当前 `SystemPrompt` 是一个单体提示词，包含所有 4 个命令组（node/exec/file/playbook）的工具定义和示例，每次 LLM 调用都发送 ~3000 字符的完整上下文。这导致两个问题：
1. **Token 浪费**：即使用户只想执行命令，也发送了节点管理、文件传输、剧本的完整工具定义
2. **意图发散**：LLM 看到过多工具选项时，容易选错工具或犹豫不决

需要设计分层路由架构：第一轮用轻量分类提示词确定命令组，后续轮次只用该组的专用提示词，并支持组内进一步精炼到子命令级别。

## What Changes

- **ADDED** `RouterPrompt` — 第一轮分类提示词（~200 字符），只要求 LLM 输出命令组标签
- **ADDED** 4 个命令组专用提示词 — 替代单一 `SystemPrompt`，每个只包含本组工具
  - `NodeSystemPrompt` — 只含 `query_nodes`
  - `ExecSystemPrompt` — 只含 `execute_command` + `execute_script`
  - `FileSystemPrompt` — 只含 `transfer_file`
  - `PlaybookSystemPrompt` — 只含 `generate_playbook`
- **MODIFIED** `Agent.Process()` — 增加路由阶段：先用 `RouterPrompt` 分类，再加载组专用提示词执行多轮工具调用
- **MODIFIED** 提示词注入机制 — 组内多轮时根据已选工具动态注入的工具专用提示词（`ExecuteCommandPrompt` / `ExecuteScriptPrompt` 等）

## Impact

- Affected specs: `enhance-ai-prompt-tool-mapping`（本 spec 复用其生成的组级/工具级提示词，重构路由架构）
- Affected code:
  - `internal/ai/prompts/prompts.go` — 新增 RouterPrompt + 4 个组专用 SystemPrompt + 原有的工具专用提示词
  - `internal/ai/agent.go` — Process() 重构为两阶段（路由 → 执行）

## ADDED Requirements

### Requirement: 路由提示词（RouterPrompt）

系统 SHALL 提供 `RouterPrompt`，字符数 ≤ 500，内容为：

```
你是 owl-AI 路由器。根据用户输入，输出以下命令组标签之一（只输出标签，无其他内容）：

node   - 节点管理（查询节点、列出节点、节点状态、节点检查）
exec   - 命令执行（在节点上执行 shell 命令或脚本）
file   - 文件传输（上传、下载、扩散传输文件）
playbook - 剧本管理（生成、执行 Ansible 剧本）

如果无法确定，输出: uncertain
```

#### Scenario: 路由到 exec 组
- **GIVEN** 用户输入 "在 web-01 上执行 df -h"
- **WHEN** 系统用 RouterPrompt 调用 LLM
- **THEN** LLM 输出 "exec"

#### Scenario: 路由到 node 组
- **GIVEN** 用户输入 "列出所有在线 web 节点"
- **WHEN** 系统用 RouterPrompt 调用 LLM
- **THEN** LLM 输出 "node"

#### Scenario: 无法确定
- **GIVEN** 用户输入 "帮我做点什么"
- **WHEN** 系统用 RouterPrompt 调用 LLM
- **THEN** LLM 输出 "uncertain" → 系统返回 "我不确定您要做什么"

### Requirement: NodeSystemPrompt（节点管理组）

`NodeSystemPrompt` SHALL 只包含 `query_nodes` 工具定义，替代原 SystemPrompt 中的节点部分：

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| group | string | 否 | 按分组过滤，如 "web"、"db" |
| labels | object | 否 | 按标签过滤，如 `{"env":"prod"}` |
| status | string | 否 | 按状态过滤: "online"、"offline"、"unknown" |
| format | string | 否 | 输出格式: "table"(默认)、"json"、"summary" |

#### Scenario: 节点查询
- **GIVEN** 路由结果为 "node"
- **WHEN** 系统加载 NodeSystemPrompt
- **THEN** LLM 只看到 query_nodes 工具，精确生成节点查询 tool_call

### Requirement: ExecSystemPrompt（命令执行组）

`ExecSystemPrompt` SHALL 只包含 `execute_command` 和 `execute_script` 两个工具定义（当前 SystemPrompt 已包含此内容，但需独立出来）。

组内多轮行为：
- 第一轮：LLM 根据用户输入选择 `execute_command` 或 `execute_script`
- 第二轮：如果参数不完整，系统注入对应工具专用提示词（`ExecuteCommandPrompt` 或 `ExecuteScriptPrompt`）辅助补全

#### Scenario: 组内精炼到 execute_command
- **GIVEN** 路由结果为 "exec"，用户说 "执行 uptime"
- **WHEN** ExecSystemPrompt 加载后 LLM 第一轮输出 `{"tool_calls":[{"name":"execute_command","arguments":{"command":"uptime","targets":[...]}}]}`
- **THEN** 工具执行后返回结果，无第二轮

#### Scenario: 组内精炼到 execute_script + 参数补全
- **GIVEN** 路由结果为 "exec"，用户说 "执行脚本 deploy.sh"
- **WHEN** LLM 第一轮输出 `{"tool_calls":[{"name":"execute_script","arguments":{"script":"./deploy.sh","targets":[...]}}]}`
- **AND** 参数缺少 dest、timeout 等
- **THEN** 第二轮系统注入 `ExecuteScriptPrompt`，提示 LLM 可补充参数

### Requirement: FileSystemPrompt（文件传输组）

`FileSystemPrompt` SHALL 只包含 `transfer_file` 工具定义：

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| source_file | string | 是 | 源文件路径 |
| targets | string[] | 是 | 目标节点名称列表 |
| dest_dir | string | 是 | 目标远程目录，默认 "/tmp" |
| mode | string | 否 | 传输模式: "direct"、"diffusion"、auto(≥5节点默认diffusion) |
| permission | string | 否 | 文件权限，默认 "0644" |
| overwrite | bool | 否 | 覆盖已存在文件 |

#### Scenario: 文件传输
- **GIVEN** 路由结果为 "file"，用户说 "上传 app.tar.gz 到所有 web 节点"
- **THEN** LLM 输出 `{"tool_calls":[{"name":"transfer_file","arguments":{"source_file":"app.tar.gz","dest_dir":"/tmp",...}}]}`

### Requirement: PlaybookSystemPrompt（剧本管理组）

`PlaybookSystemPrompt` SHALL 只包含 `generate_playbook` 工具定义：

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| requirement | string | 是 | 用户需求描述 |
| targets | string[] | 否 | 目标节点名称列表 |
| group | string | 否 | 按分组选择节点 |
| label | object | 否 | 按标签选择节点 |
| extra_vars | object | 否 | 额外变量 |
| become | bool | 否 | 是否提权执行，默认 true |
| timeout | int | 否 | 超时秒数，默认 300 |

#### Scenario: 剧本生成
- **GIVEN** 路由结果为 "playbook"，用户说 "在 web 节点安装 nginx"
- **THEN** LLM 输出 `{"tool_calls":[{"name":"generate_playbook","arguments":{"requirement":"Install nginx on web nodes","group":"web"}}]}`

## MODIFIED Requirements

### Requirement: Agent.Process() 两阶段流程

`Agent.Process()` SHALL 重构为：

```
Phase 1: Route
├── 用 RouterPrompt + 用户输入调用 LLM（单轮）
├── 解析响应，提取命令组标签（node/exec/file/playbook）
└── 如果是 "uncertain"，直接返回拒绝文案

Phase 2: Execute
├── 加载对应的组专用 SystemPrompt
├── 注入 {{.ToolDescriptions}} 和 {{.NodeInfo}}
├── 运行现有多轮工具调用循环（最多 10 轮）
└── 组内每轮根据上一轮的工具选择，可选注入工具专用提示词
```

#### Scenario: 完整两阶段流程
- **GIVEN** 用户输入 "在 web 组执行 df -h"
- **WHEN** Phase 1 路由 → 输出 "exec"
- **AND** Phase 2 加载 ExecSystemPrompt
- **THEN** LLM 输出 `{"tool_calls":[{"name":"execute_command","arguments":{"command":"df -h","group":"web"}}]}`

## REMOVED Requirements

### Requirement: 单一 SystemPrompt（废弃）

**Reason**：单体 SystemPrompt 包含所有 4 个命令组工具定义，Token 浪费严重，意图发散。

**Migration**：原有 `SystemPrompt` 常量移除，拆分为 RouterPrompt + 4 个组专用提示词。已有的 `ExecuteCommandPrompt` 和 `ExecuteScriptPrompt` 保留为工具专用提示词。
