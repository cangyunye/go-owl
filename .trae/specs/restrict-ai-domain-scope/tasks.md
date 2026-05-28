# Tasks

- [x] Task 1: 强化 RouterPrompt 范围界定
  - 在 `RouterPrompt` 顶部新增 owl 功能范围声明，阐明 owl 是什么（分布式节点管理运维工具）及其 4 个能力域
  - 添加明确的 `uncertain` 触发条件：与 owl 功能无关的查询一律输出 `uncertain`
  - 添加典型误识别示例：MAC 地址查询、macOS 操作指南等不属于 owl 范围
  - 修改文件：`internal/ai/prompts/prompts.go` 第 3-10 行

- [x] Task 2: 为 4 个领域 SystemPrompt 添加范围界定段落
  - 在 `NodeSystemPrompt`、`ExecSystemPrompt`、`FileSystemPrompt`、`PlaybookSystemPrompt` 开头各添加统一的 owl 范围界定段落
  - 段落内容包括：owl 是什么、该领域能做什么、遇到无关问题时必须回复"我不确定您要做什么"
  - 针对节点管理领域，明确指出"mac"在 owl 语境下指节点名称关键字，非 MAC 地址或 macOS
  - 修改文件：`internal/ai/prompts/prompts.go`

- [x] Task 3: 在 Agent.Process() 添加安全网拦截非工具调用长文本
  - 在工具调用循环中，当 `parseToolCalls` 返回空时，检查 LLM 响应是否为有意义的非工具调用长文本
  - 判断逻辑：如果响应长度超过 100 字符且不包含 `tool_calls` 关键字，则判定为无关回复
  - 判定为无关回复时，丢弃 LLM 输出，返回受控的拒绝消息"我不确定您要做什么"
  - 保留"我不确定您要做什么"等合法拒绝响应的正常透传
  - 修改文件：`internal/ai/agent.go`

# Task Dependencies

- Task 1、Task 2、Task 3 相互独立，可并行实施
- Task 2 中各领域提示词的修改可批量完成
