# AI 对话日志 —— 数据库记录 + 调试输出

## Why

当前 `owl ai` 命令没有数据库持久化——每次 LLM 调用的请求和响应都不被记录，调试困难；流程进度也不向用户展示，用户无法感知当前处于路由阶段还是执行阶段。需要：
1. 在数据库中新增 `aichat` 表，记录每次 AI 对话的完整信息
2. 在终端打印带时间戳的流程进度（路由→选组→JSON→校验→执行）
3. 通过 `--debug` 参数控制是否记录详细信息到数据库

## What Changes

- **ADDED** `aichat` 表到 SQLite3 和 DuckDB 的 InitSchema
- **ADDED** `AiChat` 数据模型 + `RecordAiChat` / `QueryAiChat` 函数到 `internal/history/`
- **MODIFIED** `owl ai` 命令新增 `--debug` 标志
- **MODIFIED** `runAI` 入口增加进度打印（带时间戳）
- **MODIFIED** `Agent.Process()` 增加进度打印回调注入点
- **ADDED** `owl ai history` 子命令（`owl ai history list` / `owl ai history show` / `owl ai history clean`）

## Impact

- Affected specs: `hierarchical-prompt-routing`（Agent.Process 增加回调参数）
- Affected code:
  - `internal/history/` — 新增 aichat 表、模型、记录/查询函数
  - `cmd/cli/cmd/ai/ai.go` — `--debug` 标志 + 进度打印 + 历史子命令

## ADDED Requirements

### Requirement: aichat 数据库表

系统 SHALL 在 SQLite3 和 DuckDB 后端各新增 `aichat` 表：

```
aichat:
  id            TEXT PRIMARY KEY    -- UUID
  session_id    TEXT                -- 会话 ID，同一会话共享
  step          TEXT                -- 流程阶段: route/analyze/generate/execute/result
  role          TEXT                -- user/assistant/system/tool
  prompt        TEXT                -- 发送给 LLM 的系统提示词（仅 --debug）
  input         TEXT                -- 用户输入或工具结果
  output        TEXT                -- LLM 原始输出
  tool_calls    TEXT                -- 解析出的工具调用 JSON
  tool_results  TEXT                -- 工具执行结果（截断至 4096 字符）
  duration_ms   INTEGER            -- 本步骤耗时毫秒
  error         TEXT                -- 错误信息（如有）
  metadata      TEXT                -- 额外元数据 JSON: {provider, model, retries, route_label}
  created_at    TEXT                -- ISO8601 时间戳
```

#### Scenario: 一次 AI 对话产生多条记录
- **GIVEN** 用户执行 `owl ai "查询所有节点" --debug`
- **THEN** `aichat` 表写入 3~4 条记录：
  1. step=route, role=user, input="查询所有节点"
  2. step=analyze, role=assistant, output="node", metadata={"route_label":"node"}
  3. step=generate, role=assistant, output=`{"tool_calls":[...]}`
  4. step=execute, tool_calls=..., tool_results=...

#### Scenario: 常规模式不记录详细信息
- **GIVEN** 用户执行 `owl ai "查询所有节点"`（无 `--debug`）
- **THEN** `aichat` 表写入 1~2 条精简记录（仅含 step/role/input/output/tool_calls/tool_results），不含 prompt 和 metadata

### Requirement: 终端进度打印

系统 SHALL 在终端打印带时间戳的流程进度，格式为 `[HH:MM:SS] owl-ai: <描述>`：

```
[10:30:01] 用户：查询所有节点
[10:30:01] owl-ai: 分析用户调用子命令意图
[10:30:02] owl-ai: 确认用户调用子命令为 node 相关
[10:30:02] owl-ai: 请求模型生成查询 JSON...
[10:30:03] owl-ai: JSON 校验通过
[10:30:03] owl-ai: 开始执行查询操作
```

#### Scenario: 路由到 exec 的命令执行
- **GIVEN** 用户执行 `owl ai "在 web-01 执行 uptime"`
- **THEN** 终端输出：
```
[10:30:01] 用户：在 web-01 执行 uptime
[10:30:01] owl-ai: 分析用户调用子命令意图
[10:30:02] owl-ai: 确认用户调用子命令为 exec 相关
[10:30:02] owl-ai: 请求模型生成执行 JSON...
[10:30:03] owl-ai: JSON 校验通过 (execute_command)
[10:30:03] owl-ai: 开始执行命令
```

#### Scenario: 路由失败（uncertain）
- **GIVEN** 用户执行 `owl ai "随便来点什么"`
- **THEN** 终端输出：
```
[10:30:01] 用户：随便来点什么
[10:30:01] owl-ai: 分析用户调用子命令意图
[10:30:02] owl-ai: 无法确定用户意图，请更明确地描述
```

### Requirement: --debug 标志

`owl ai` 命令 SHALL 新增 `--debug` 标志（`-d` 短标志）：

- **常规模式**（默认）：终端打印流程进度，数据库记录精简信息（不含 prompt/metadata）
- **debug 模式**（`--debug`）：额外将完整 prompt、LLM 原始输出、metadata 写入数据库，终端仍打印流程进度

#### Scenario: debug 模式写入完整 prompt
- **GIVEN** 用户执行 `owl ai "查询所有节点" --debug`
- **THEN** `aichat` 记录的 `prompt` 字段包含完整的 SystemPrompt 内容
- **AND** `metadata` 字段包含 `{"provider":"openai","model":"gpt-4o","retries":0}`

### Requirement: owl ai history 子命令

系统 SHALL 新增 `owl ai history` 子命令组：

```
owl ai history list    — 列出最近的 AI 对话会话摘要（session_id, 用户输入, 耗时, 步骤数）
owl ai history show    — 显示指定会话的完整对话链
owl ai history clean   — 清理过期的 AI 聊天记录
```

| 命令 | 参数 | 说明 |
|------|------|------|
| `list` | `--limit` (默认 20), `--session` (按会话ID过滤) | 按时间倒序列出会话摘要 |
| `show` | `<session-id>` (必填) | 完整展示一次对话的所有步骤（time + role + step + content） |
| `clean` | `--days` (默认 30) | 删除超过 N 天的记录 |

#### Scenario: 列出最近会话
- **GIVEN** 已有 5 次 AI 对话记录
- **WHEN** 执行 `owl ai history list --limit 3`
- **THEN** 输出最近 3 次对话的摘要表格（时间/用户输入/工具调用/步骤数）

## MODIFIED Requirements

### Requirement: Agent.Process() 增加进度回调

`Agent.Process()` SHALL 接受可选的进度回调函数：

```go
type ProgressCallback func(step string, detail string)

func (a *Agent) Process(ctx context.Context, userInput string, onProgress ProgressCallback) (string, error)
```

回调时机：
| step | detail | 触发点 |
|------|--------|--------|
| `"route"` | 路由标签，如 `"node"` | Phase 1 路由完成 |
| `"analyze"` | `"正在生成 JSON..."` | Phase 2 开始 |
| `"generate"` | 工具名，如 `"query_nodes"` | 解析出 tool_call |
| `"execute"` | 工具名 | 开始执行工具 |
| `"result"` | `"完成"` 或 `"失败"` | 全部完成 |

**向后兼容**：`Process()` 保留无回调的旧签名（内部传 nil）。

## REMOVED Requirements

无。
