# Tasks

- [x] Task 1: 增强 ExecuteCommandTool 并新增 ExecuteScriptTool
  - 在 `internal/ai/tools.go` 中为 `ExecuteCommandTool.Parameters()` 补充 `group`、`label`、`format`、`mode` 参数定义
  - 新增 `ExecuteScriptTool` 结构体，实现 `Tool` 接口（Name="execute_script"）
  - `ExecuteScriptTool.Parameters()` 定义 `script`、`targets`、`group`、`label`、`dest`、`args`、`timeout`、`inline`、`keep` 参数 Schema
  - 同步更新 `GetToolDefinitions()` 添加 `execute_script` 定义
  - 在 `ExecuteCommandTool.Execute()` 中处理 `group` 和 `label` 参数（调用 nodeMgr 解析出 targets）
  - 在 `ExecuteCommandTool.Execute()` 中处理 `format` 和 `mode` 参数（影响输出格式和执行方式描述）
  - `ExecuteScriptTool.Execute()` 实现：验证脚本文件存在性、解析目标节点、构建执行摘要
  - 验证：参数 Schema JSON 合法，`go build ./cmd/cli/...` 编译通过 ✅

- [x] Task 2: 更新 Validator 校验逻辑
  - 修改 `ValidateExecuteCommand`：当 `group` 或 `label` 存在时 `targets` 不再必填
  - `targets`、`group`、`label` 至少提供一个的互斥校验
  - 新增 `ValidateExecuteScript` 方法：校验 `script` 非空、`targets`/`group`/`label` 至少一个
  - `dest` 必须是绝对路径；`mode` 枚举值校验 "direct"|"diffusion"|"auto"
  - 在 `ValidateParams` 的 switch 中增加对 `execute_script` 意图的处理
  - 验证：`go test ./internal/ai/...` 全部通过 ✅

- [x] Task 3: 注册新工具到 Agent
  - 在 `internal/ai/agent.go` 的 `NewAgent()` 中注册 `execute_script` 工具
  - 在 `internal/ai/intent_classifier.go` 中新增 `IntentExecuteScript` 意图类型及关键词
  - 在 `internal/ai/param_extractor.go` 中新增 `execute_script` 的参数提取逻辑
  - 在 `defaultChatHandler` 的 switch 中增加 `IntentExecuteScript` 的处理分支
  - 验证：回退模式（无 API Key）下 `owl ai "执行脚本 deploy.sh"` 能正确分类并生成 tool_call ✅

- [x] Task 4: 重写 exec 相关提示词
  - 重写 `internal/ai/prompts/prompts.go` 中 `SystemPrompt` 的 exec 部分，包含 `execute_command` 和 `execute_script` 完整参数表格
  - 每个工具编写至少 3 个覆盖不同参数组合的示例（中英文混合）
  - 新增 `ExecuteCommandPrompt` 操作专项提示（含危险命令清单、模式选择指南）
  - 新增 `ExecuteScriptPrompt` 操作专项提示（含 inline vs 文件模式对比、参数传递格式）
  - 验证：SystemPrompt 总字符数 ~3046 < 8000，示例中参数名与 tools.go 定义一致 ✅

- [x] Task 5: 全量验证
  - `go build ./cmd/cli/...` 编译通过 ✅
  - `go test ./internal/ai/...` 全部测试通过 ✅
  - 手动检查 SystemPrompt 示例与 `tests/scripts/test-exec.sh` 测试用例的对齐度 ✅
  - 危险命令清单与 `internal/control/blacklist` 包一致 ✅

# Task Dependencies

- Task 2 依赖于 Task 1（新增参数需要对应的校验逻辑）
- Task 3 依赖于 Task 1, 2（工具注册和回退模式需要完整工具和校验）
- Task 4 可与 Task 1-3 并行（提示词是独立文件，但需 Task 5 阶段对齐）
- Task 5 依赖于 Task 1-4
