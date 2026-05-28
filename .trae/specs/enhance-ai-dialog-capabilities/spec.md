# 增强 AI 对话能力 Spec

## Why

用户在实际使用 owl AI 对话时发现 3 个核心问题：1) 节点名称无法模糊匹配，AI 将 "mac" 当作分组名查询失败；2) AI 询问用户确认后，用户回复 "是" 时 AI 遗忘了上下文；3) AI 缺少直接查询 owl 数据库的能力，只能通过有限的工具参数检索节点。

## What Changes

- **query_nodes 工具新增 `search` 参数**：支持按节点名称关键字模糊搜索（大小写不敏感、子串匹配）
- **execute_command / execute_script 工具新增 `search` 参数**：在节点选择中支持名称模糊搜索，同样应用于 transfer_file 和 generate_playbook
- **NodeSystemPrompt / ExecSystemPrompt 等提示词升级**：补充 `search` 参数的用法说明和示例，强调模糊匹配规则
- **Session 会话记忆系统**：`Session.Send()` 传递完整对话历史给 LLM，支持多轮对话上下文；新增 `pendingContext` 字段追踪 AI 待确认问题，用户回复肯定语句时恢复上下文
- **新增 query_database 工具**：允许 AI 直接执行类 SQL 查询 owl 数据库中的节点表，支持按名称、分组、标签、状态等全部维度组合检索
- **node.Manager 接口新增 `SearchByName` 方法**：支持按名称模式匹配节点

## Impact

- Affected specs: `hierarchical-prompt-routing`, `enhance-ai-prompt-tool-mapping`
- Affected code:
  - `internal/ai/prompts/prompts.go` — 全部 4 组 SystemPrompt 升级
  - `internal/ai/tools.go` — `QueryNodesTool`、`ExecuteCommandTool`、`ExecuteScriptTool` 新增 search 参数，新增 `QueryDatabaseTool`
  - `internal/ai/agent.go` — `Session.Send()` 改造为多轮对话，新增 `pendingContext`
  - `internal/control/node/manager.go` — 接口新增 `SearchByName` 方法

## ADDED Requirements

### Requirement: 节点名称模糊搜索

系统 SHALL 在 `query_nodes`、`execute_command`、`execute_script`、`transfer_file`、`generate_playbook` 工具中支持 `search` 参数，对节点名称执行大小写不敏感的模糊匹配。

#### Scenario: 用户按节点名称关键字搜索 — query_nodes

- **WHEN** 用户说 "查询 mac 节点"
- **THEN** AI 生成 `{"tool_calls":[{"name":"query_nodes","arguments":{"search":"mac"}}]}`
- **THEN** 系统返回名称包含 "mac" 的所有节点（如 mac-mini-m4），而非按 group="mac" 查空

#### Scenario: 用户在 exec 中模糊匹配节点 — execute_command

- **WHEN** 用户说 "在 mac 节点上执行 uptime"
- **THEN** AI 生成 `{"tool_calls":[{"name":"execute_command","arguments":{"command":"uptime","search":"mac"}}]}`
- **THEN** 系统匹配名称含 "mac" 的节点执行命令

#### Scenario: search 与 group/label 组合使用

- **WHEN** search 与其他过滤参数同时提供
- **THEN** 先按 group/label/status 过滤，再在结果集上按 search 做名称模糊匹配

#### Scenario: search 无匹配结果

- **WHEN** search 参数无法匹配任何节点
- **THEN** 系统返回 "No matching nodes found for search: <keyword>"

### Requirement: 会话多轮对话记忆

系统 SHALL 在交互模式下将完整对话历史（包括用户消息、AI 回复、工具调用结果）传递给 LLM，使 LLM 能够理解多轮对话上下文。

#### Scenario: AI 提问后用户确认

- **WHEN** AI 返回 "没有找到名为'mac'的分组，是否需要我列出全部节点详情？"
- **AND** 用户回复 "是"
- **THEN** AI 理解用户确认，执行列出全部节点的操作
- **AND** 不会回复 "我不确定您要做什么"

#### Scenario: 多轮对话中保持上下文

- **WHEN** 用户第一轮说 "查询 mac 节点"
- **AND** AI 回复后用户第二轮说 "那在线的那台呢"
- **THEN** AI 理解 "那台" 指代 mac-mini-m4，继续按在线状态过滤

#### Scenario: 非交互模式不受影响

- **WHEN** 通过管道输入 `echo "查询节点" | owl ai`
- **THEN** 单次调用 `agent.Process()` 行为不变，不引入额外上下文

### Requirement: query_database 数据库查询工具

系统 SHALL 提供 `query_database` 工具，允许 AI 直接查询 owl 数据库获取节点信息。支持两种查询模式：SQL 查询（`query` 参数）和结构化过滤（`group`/`labels`/`status`/`search` 参数），两种模式互斥。

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| query | string | 否* | 类 SQL 查询语句（仅允许 SELECT） |
| group | string | 否* | 按分组精确过滤 |
| labels | object | 否* | 按标签精确过滤 |
| status | string | 否* | 按状态过滤：online/offline/unknown |
| search | string | 否* | 按节点名称模糊搜索（大小写不敏感、子串匹配） |
| format | string | 否 | 输出格式：table(默认)/json/summary |

*注: query 与 group/labels/status/search 互斥，必须提供其中一组。

#### Scenario: SQL 查询所有节点详情

- **WHEN** AI 调用 `{"tool_calls":[{"name":"query_database","arguments":{"query":"SELECT * FROM nodes"}}]}`
- **THEN** 返回数据库中所有节点的完整信息（名称、IP、端口、状态、分组、标签等）

#### Scenario: SQL 按名称模糊查询

- **WHEN** AI 调用 `{"tool_calls":[{"name":"query_database","arguments":{"query":"SELECT * FROM nodes WHERE name LIKE '%mac%'"}}]}`
- **THEN** 返回名称包含 "mac" 的所有节点

#### Scenario: 结构化参数按分组过滤

- **WHEN** AI 调用 `{"tool_calls":[{"name":"query_database","arguments":{"group":"test"}}]}`
- **THEN** 返回 "test" 分组下的所有节点（如 mac-mini-m4）

#### Scenario: 结构化参数按标签过滤

- **WHEN** AI 调用 `{"tool_calls":[{"name":"query_database","arguments":{"labels":{"env":"prod"}}}}]`
- **THEN** 返回标签 env=prod 的所有节点

#### Scenario: 结构化参数按名称模糊搜索

- **WHEN** AI 调用 `{"tool_calls":[{"name":"query_database","arguments":{"search":"mac"}}]}`
- **THEN** 返回名称包含 "mac" 的所有节点

#### Scenario: 结构化参数多条件组合

- **WHEN** AI 调用 `{"tool_calls":[{"name":"query_database","arguments":{"group":"test","status":"online"}}]}`
- **THEN** 返回 test 分组中在线状态的节点（AND 逻辑）

#### Scenario: 不安全查询被拒绝

- **WHEN** AI 尝试执行 INSERT/UPDATE/DELETE/DROP/ALTER 等写操作
- **THEN** 工具返回错误信息 "Only SELECT queries are allowed"，不执行任何修改

## MODIFIED Requirements

### Requirement: query_nodes 工具参数扩展（原仅支持 group/labels/status/format）

`query_nodes` 工具 SHALL 新增 `search` 参数（string 类型，可选），当提供时对节点名称执行大小写不敏感子串匹配。其他参数（group、labels、status）先执行精确过滤，再在结果上按 search 二次过滤。

**参数表更新为**：
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| group | string | 否 | 按分组精确过滤 |
| labels | object | 否 | 按标签精确过滤 |
| status | string | 否 | 按状态精确过滤 |
| search | string | 否 | 按节点名称模糊搜索（大小写不敏感、子串匹配） |
| format | string | 否 | 输出格式 |

### Requirement: execute_command 工具参数扩展

`execute_command` 工具 SHALL 新增 `search` 参数（string 类型，可选），与 targets/group/label 互斥。当提供 search 时，对所有节点名称做模糊匹配得到目标节点列表。

### Requirement: execute_script 工具参数扩展

`execute_script` 工具 SHALL 新增 `search` 参数（string 类型，可选），与 targets/group/label 互斥。当提供 search 时，对所有节点名称做模糊匹配得到目标节点列表。

### Requirement: NodeSystemPrompt 升级

`NodeSystemPrompt` SHALL 新增以下内容：
1. `search` 参数的说明（模糊匹配，用于用户只知道部分节点名时）
2. 一个模糊搜索的示例
3. 明确当用户指定了看起来像节点名但不是精确分组名时，应使用 search 而非 group

### Requirement: ExecSystemPrompt 升级

`ExecSystemPrompt` SHALL 在"节点选择规则"部分新增：
1. `search` 参数：按节点名称关键字模糊匹配
2. 优先级规则更新为：targets > search > group > label
3. 示例展示 search 的用法

### Requirement: Session.Send() 支持多轮对话

`Session.Send()` SHALL 修改为：
1. 将 session 中累积的 `messages`（包含历史 tool call 结果）传递给 `agent.ProcessWithContext()`
2. 每次调用后更新 `messages` 列表
3. 新增 `pendingContext` 字段（结构体），用于追踪 AI 是否正在等待用户确认
  
`pendingContext` 结构：
```go
type PendingContext struct {
    State       string // "awaiting_confirmation" | ""
    Action      string // 待执行的操作描述
    LastToolName string // 上一轮使用的工具名
    LastParams  map[string]interface{} // 上一轮的参数
    Question    string // AI 向用户提的问题
}
```

当 AI 响应中包含问句时，设置 `pendingContext`。当用户回复肯定语句时，将 `pendingContext` 注入为系统消息，让 LLM 恢复上下文继续执行。
