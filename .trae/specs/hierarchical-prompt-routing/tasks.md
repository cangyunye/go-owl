# Tasks

- [x] Task 1: 创建路由提示词 RouterPrompt
  - 在 `internal/ai/prompts/prompts.go` 中新增 `RouterPrompt` 常量
  - 内容：角色为路由器，输出 node/exec/file/playbook/uncertain 之一，含简短说明
  - 字符数 ≤ 500 ✅
  - 验证：RouterPrompt 不含任何工具参数细节，只含命令组分类规则 ✅

- [x] Task 2: 拆分 SystemPrompt 为 4 个组专用提示词
  - 在 `internal/ai/prompts/prompts.go` 中新增 4 个常量
  - `NodeSystemPrompt`：只含 query_nodes 工具（完整参数表 + 示例）✅
  - `ExecSystemPrompt`：只含 execute_command + execute_script（复用当前 SystemPrompt 中的 exec 部分）✅
  - `FileSystemPrompt`：只含 transfer_file（含完整参数表 + 示例）✅
  - `PlaybookSystemPrompt`：只含 generate_playbook（含完整参数表 + 示例）✅
  - 每个组提示词保留"输出契约"和"拒绝规则" ✅
  - 验证：4 个组提示词分别编译通过，不再使用原单体 SystemPrompt ✅

- [x] Task 3: 重构 Agent.Process() 为两阶段流程
  - 在 `internal/ai/agent.go` 的 `Process()` 方法中增加 Phase 1（路由）
  - Phase 1：用 RouterPrompt 构建单轮 messages，调用 chatModel.Generate()
  - 解析 Phase 1 响应，提取路由标签（node/exec/file/playbook/uncertain）
  - uncertain → 直接返回 "我不确定您要做什么"
  - Phase 2：根据路由标签选择对应组 SystemPrompt，运行原有 10 轮工具调用循环
  - 组内多轮注入逻辑：当 LLM 已选定工具但参数不完整时，可选注入工具专用提示词
  - 验证：`go build ./cmd/cli/...` 编译通过，`go test ./internal/ai/...` 全部通过 ✅

- [x] Task 4: 实现组内工具专用提示词动态注入
  - 修改 Process() 的多轮循环：在工具执行后（Round 2+），根据上一轮选中的工具注入对应提示词
  - execute_command → 注入 ExecuteCommandPrompt ✅
  - execute_script → 注入 ExecuteScriptPrompt ✅
  - query_nodes → 无需注入（参数已在组提示词中完整定义）✅
  - generate_playbook → 注入 PlaybookPrompt（已有）✅
  - transfer_file → 注入 TransferPrompt（已有）✅
  - 验证：多轮工具调用时第二轮 messages 中包含对应的工具专用提示词 ✅

- [x] Task 5: 清理旧代码 + 全量验证
  - 移除或废弃原 `SystemPrompt` 常量（保留为引用兼容或直接删除）✅
  - `go build ./cmd/cli/...` 编译通过 ✅
  - `go test ./internal/ai/...` 全部测试通过 ✅
  - 验证 RouterPrompt 字符数 ≤ 500 ✅
  - 验证每个组专用提示词字符数 < 2000 ✅

# Task Dependencies

- Task 2 依赖于 Task 1（组提示词需要与 RouterPrompt 的标签名对齐）
- Task 3 依赖于 Task 1, 2（Process 重构需要 RouterPrompt 和组提示词）
- Task 4 依赖于 Task 3（动态注入需要两阶段流程已就位）
- Task 5 依赖于 Task 1-4
