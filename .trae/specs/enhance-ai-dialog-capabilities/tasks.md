# Tasks

- [x] Task 1: 在 node.Manager 接口中新增 `SearchByName` 方法
  - [x] 在 `internal/control/node/manager.go` 的 `Manager` 接口新增 `SearchByName(pattern string) []*model.Node`
  - [x] 在 `manager` 结构体实现该方法：对所有节点执行 `strings.Contains(strings.ToLower(n.Name), strings.ToLower(pattern))`
  - [x] 验证：编译通过，`go build ./...`

- [x] Task 2: query_nodes 工具新增 `search` 参数
  - [x] 在 `QueryNodesTool.Parameters()` 中新增 `search` 字段（string，可选）
  - [x] 在 `QueryNodesTool.Execute()` 中添加 search 逻辑：在 group/labels/status 过滤后，使用 `nodeMgr.SearchByName(search)` 二次过滤
  - [x] 在 `tools.go` 的 `GetToolDefinitions()` 中同步更新 query_nodes 的参数定义
  - [x] 验证：工具参数表完整，search 过滤逻辑正确

- [x] Task 3: execute_command 和 execute_script 工具新增 `search` 参数
  - [x] 在 `ExecuteCommandTool.Parameters()` 中新增 `search` 字段，描述与 targets/group/label 互斥
  - [x] 在 `ExecuteCommandTool.Execute()` 中添加 search 分支：`nodeMgr.SearchByName(search)`
  - [x] 在 `ExecuteScriptTool.Parameters()` 中新增 `search` 字段
  - [x] 在 `ExecuteScriptTool.Execute()` 中添加 search 分支
  - [x] 在 `GetToolDefinitions()` 中同步更新两个工具的参数定义
  - [x] 验证：search 参数可正确匹配节点并在 exec 场景中使用

- [x] Task 4: NodeSystemPrompt 升级 — 新增 search 参数说明和示例
  - [x] 在 `internal/ai/prompts/prompts.go` 的 `NodeSystemPrompt` 的 query_nodes 参数表新增 search 行
  - [x] 新增示例 4：用户说 "查询 mac 节点" → 使用 `search: "mac"`
  - [x] 在提示词中增加规则："当用户输入看起来像节点名但不等于已知分组名时，使用 search 而非 group 进行模糊匹配"
  - [x] 验证：提示词包含 search 参数说明和至少一个 search 示例

- [x] Task 5: ExecSystemPrompt 升级 — 新增 search 参数说明和示例
  - [x] 在 ExecSystemPrompt 的 execute_command / execute_script 参数表新增 search 行
  - [x] 更新"节点选择规则"：targets > search > group > label
  - [x] 新增示例展示使用 search 选择节点执行命令
  - [x] 验证：提示词中节点选择规则明确了 search 的存在和优先级

- [x] Task 6: FileSystemPrompt 和 PlaybookSystemPrompt 升级
  - [x] 在 FileSystemPrompt 中更新节点选择说明，补充 search 参数
  - [x] 在 PlaybookSystemPrompt 中更新节点选择说明，补充 search 参数
  - [x] 验证：两个提示词均提及 search 模糊匹配节点名

- [x] Task 7: 新增 query_database 工具
  - [x] 在 `tools.go` 中新增 `QueryDatabaseTool` 结构体，实现 `Tool` 接口
  - [x] `Parameters()` 定义参数：`query`（string，可选）、`group`（string，可选）、`labels`（object，可选）、`status`（string，可选）、`search`（string，可选）、`format`（string，可选）
  - [x] `Execute()` 支持两种模式（互斥）：
    - SQL 模式：当 `query` 提供时，仅允许 SELECT 语句，拒绝 INSERT/UPDATE/DELETE/DROP/ALTER
    - 结构化过滤模式：当 `group`/`labels`/`status`/`search` 提供时，调用 `nodeMgr.GetByGroup`/`GetByLabels`/`SearchByName` 等组合过滤（AND 逻辑），复用 `query_nodes` 的过滤路径
  - [x] 对 SELECT 查询，将节点列表转换为 in-memory table 并执行过滤/投影（简化实现：支持 SELECT * / WHERE name LIKE / WHERE group = / WHERE status = 等常见模式）
  - [x] 在 `GetToolDefinitions()` 中注册 query_database 函数定义，参数表包含 query/group/labels/status/search/format
  - [x] 在 `NewAgent()` 中注册该工具：`registry.Register(NewQueryDatabaseTool(nodeMgr))`
  - [x] 在 `NodeSystemPrompt` 中新增 query_database 工具说明（两种模式均展示示例）
  - [x] 验证：AI 可通过结构化参数或 SQL 两种方式查询节点，写操作被拒绝

- [x] Task 8: Session 多轮对话记忆 — 传递对话历史
  - [x] 在 `Agent` 中新增 `ProcessWithContext(ctx, messages []Message, onProgress)` 方法：将历史消息作为前缀传入 LLM
  - [x] 在 `agent.go` 末尾新增 `ProcessWithContext`，逻辑为：将 `messages` 作为初始 `messages` 参数传递给 generateWithRetry，然后在循环中追加新消息
  - [x] 修改 `Session.Send()` 调用 `ProcessWithContext(ctx, s.messages, s.OnProgress)` 并更新 `s.messages`
  - [x] session 的 `messages` 初始化为仅含 system prompt 的第一条消息
  - [x] 验证：交互模式下多轮对话能保持上下文，非交互模式不受影响

- [x] Task 9: pendingContext 待确认上下文追踪
  - [x] 在 `agent.go` 中新增 `PendingContext` 结构体（含 State, Action, LastToolName, LastParams, Question 字段）
  - [x] 在 `Session` 结构体中新增 `pendingContext *PendingContext` 字段
  - [x] 在 `Session.Send()` 中：每次 AI 响应后检测是否包含问句（以 "？" 或 "?" 结尾、或包含 "是否"、"要不要" 等关键词），若是则设置 `pendingContext`
  - [x] 在下一次 `Session.Send()` 中：检测用户输入是否为肯定回复（"是"、"好的"、"yes"、"对"、"ok" 等），若是且 `pendingContext != nil`，则将 pendingContext 注入为系统消息
  - [x] 验证：用户回答 "是" 后 AI 能恢复上下文继续执行

- [x] Task 10: 测试与验证
  - [x] 为 `SearchByName` 编写单元测试（匹配、不匹配、大小写、空字符串）
  - [x] 为 `query_nodes` 的 search 参数编写单元测试
  - [x] 为 `QueryDatabaseTool` 编写单元测试（SELECT 正常、写操作拒绝）
  - [x] 为 `pendingContext` 编写单元测试（肯定回复识别、上下文注入）
  - [x] 运行全部测试：`go test ./internal/ai/... ./internal/control/node/... -v`
  - [x] 验证：所有测试通过，无 regression

# Task Dependencies

- Task 2 依赖 Task 1（search 参数需要 SearchByName 方法）
- Task 3 依赖 Task 1（exec 工具的 search 需要 SearchByName 方法）
- Task 4、5、6 可与 Task 1-3 部分并行（先写提示词，后验证与代码的一致性）
- Task 7 独立，无依赖
- Task 8 独立，无依赖（但实现时需注意与 Process 的兼容性）
- Task 9 依赖 Task 8（pendingContext 需要多轮对话机制已就绪）
- Task 10 依赖 Task 1-9 全部完成
