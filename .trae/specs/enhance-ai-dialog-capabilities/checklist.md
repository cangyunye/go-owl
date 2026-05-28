# Checklist

## 节点名称模糊搜索

- [x] `node.Manager` 接口包含 `SearchByName(pattern string) []*model.Node` 方法签名
- [x] `manager` 实现对节点名称做大小写不敏感子串匹配
- [x] `QueryNodesTool.Parameters()` 包含 `search` 参数定义
- [x] `QueryNodesTool.Execute()` 在 group/labels/status 过滤后执行 search 二次过滤
- [x] `ExecuteCommandTool.Parameters()` 包含 `search` 参数定义
- [x] `ExecuteCommandTool.Execute()` 支持 search 分支，调用 `nodeMgr.SearchByName(search)`
- [x] `ExecuteScriptTool.Parameters()` 包含 `search` 参数定义
- [x] `ExecuteScriptTool.Execute()` 支持 search 分支
- [x] `GetToolDefinitions()` 中 query_nodes、execute_command、execute_script 均包含 search 参数
- [x] `NodeSystemPrompt` 中 query_nodes 参数表包含 search 行
- [x] `NodeSystemPrompt` 包含至少一个 search 示例
- [x] `NodeSystemPrompt` 包含"当用户输入像节点名但非已知分组名时用 search"的规则
- [x] `ExecSystemPrompt` 的节点选择规则更新为 targets > search > group > label
- [x] `ExecSystemPrompt` 包含 search 参数的示例用法
- [x] `FileSystemPrompt` 提及 search 参数
- [x] `PlaybookSystemPrompt` 提及 search 参数

## 会话多轮对话记忆

- [x] `Agent` 新增 `ProcessWithContext(ctx context.Context, messages []Message, onProgress ProgressCallback) ([]Message, string, error)` 方法
- [x] `ProcessWithContext` 将传入的 messages 作为初始上下文传递给 LLM
- [x] `Session.Send()` 调用 `ProcessWithContext` 并传入 `s.messages`
- [x] `Session.Send()` 每次调用后更新 `s.messages` 列表（包含 AI 响应和工具调用结果）
- [x] `Session` 的 `messages` 正确初始化为空
- [x] 非交互模式（单次 `agent.Process()` 调用）行为不变

## pendingContext 待确认上下文

- [x] `PendingContext` 结构体定义正确（State, Action, LastToolName, LastParams, Question 字段）
- [x] `Session` 包含 `pendingContext *PendingContext` 字段
- [x] AI 响应包含问句时正确设置 `pendingContext`
- [x] 用户肯定回复（是/好的/yes/对/ok）时正确检测并注入上下文
- [x] 注入上下文后 `pendingContext` 被重置
- [x] 非肯定回复时 `pendingContext` 被清除

## query_database 工具

- [x] `QueryDatabaseTool` 实现 `Tool` 接口（Name, Description, Parameters, Validate, Execute）
- [x] `QueryDatabaseTool.Parameters()` 包含 `query`（string）、`group`（string）、`labels`（object）、`status`（string）、`search`（string）、`format`（string）参数
- [x] `QueryDatabaseTool.Execute()` 拒绝 INSERT/UPDATE/DELETE/DROP/ALTER 语句
- [x] `QueryDatabaseTool.Execute()` 结构化过滤模式支持 group 精确过滤
- [x] `QueryDatabaseTool.Execute()` 结构化过滤模式支持 labels 精确过滤
- [x] `QueryDatabaseTool.Execute()` 结构化过滤模式支持 search 模糊搜索
- [x] `QueryDatabaseTool.Execute()` 结构化过滤模式多条件组合为 AND 逻辑
- [x] `QueryDatabaseTool.Execute()` SQL 模式支持 SELECT * FROM nodes
- [x] `QueryDatabaseTool.Execute()` SQL 模式支持 WHERE name LIKE '%pattern%'
- [x] `QueryDatabaseTool.Execute()` SQL 模式支持 WHERE group = 'xxx'
- [x] `QueryDatabaseTool.Execute()` SQL 模式支持 WHERE status = 'xxx'
- [x] `GetToolDefinitions()` 包含 query_database 函数定义，参数表含 query/group/labels/status/search/format
- [x] `NewAgent()` 注册了 `QueryDatabaseTool`

## 测试覆盖

- [x] `SearchByName` 有单元测试：精确匹配、子串匹配、大小写不敏感、无匹配、空字符串
- [x] `QueryNodesTool` search 参数有单元测试
- [x] `QueryDatabaseTool` 有单元测试：SELECT 正常、写操作拒绝
- [x] `pendingContext` 肯定回复识别有单元测试
- [x] 全部已有测试无 regression：`go test ./internal/ai/... ./internal/control/node/... -v`
