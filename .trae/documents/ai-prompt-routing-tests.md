# AI 路由 + JSON 输出校验测试计划

## 目标

为分层提示词路由架构增加测试覆盖，验证：
1. **路由输出正确性**：RouterPrompt → LLM 返回后，Process() 能否正确分类为 node/exec/file/playbook/uncertain
2. **JSON 输出格式校验**：子命令 prompt 调用模型后，输出的 tool_calls JSON 是否合法、可解析

## 测试分层策略

```
Layer 0: 单元测试 (internal/ai/agent_test.go)
  使用 mock ChatModel，不需要 API Key
  覆盖：路由分类、JSON 解析、动态注入
  
Layer 1: 集成测试 (tests/integration/ai_test.go)  
  使用真实 owl ai 命令，需要 OWL_TEST_ENABLED=true + API Key
  覆盖：端到端路由 → 执行 → JSON 输出
  
Layer 2: Bash E2E 测试 (tests/scripts/test-ai.sh)
  使用真实 owl ai 命令，需要 OWL_TEST_ENABLED=true + API Key
  覆盖：自然语言输入 → owl ai → 校验输出 JSON
```

---

## Layer 0: Go 单元测试

**文件**: `internal/ai/agent_test.go`（追加到现有测试后）

需要新增的 mock:

```go
// mockChatModel 实现 ChatModel 接口，按 sequence 返回预定义响应
type mockChatModel struct {
    responses []string
    callCount int
}

func (m *mockChatModel) Generate(ctx context.Context, messages []Message) (string, error) {
    if m.callCount >= len(m.responses) {
        return "", fmt.Errorf("no more mock responses")
    }
    resp := m.responses[m.callCount]
    m.callCount++
    return resp, nil
}

// mockNodeMgrForAI 增强版 mock，支持返回复数节点和按分组查询
type mockNodeMgrForAI struct {
    nodes []*model.Node
}

func (m *mockNodeMgrForAI) List() []*model.Node  { return m.nodes }
func (m *mockNodeMgrForAI) GetByGroup(g string) []*model.Node {
    var result []*model.Node
    for _, n := range m.nodes {
        for _, grp := range n.Groups {
            if grp == g { result = append(result, n) }
        }
    }
    return result
}
// ... 实现 node.Manager 其余方法
```

### 测试用例清单

#### TC-AI-ROUTE-001: 路由器返回 "exec"
- Mock 返回: `"exec"`
- 断言: `Process()` 加载 `ExecSystemPrompt`，生成的消息包含 `execute_command`

#### TC-AI-ROUTE-002: 路由器返回 "node"
- Mock 返回: `"node"`
- 断言: `Process()` 加载 `NodeSystemPrompt`，生成的消息包含 `query_nodes`

#### TC-AI-ROUTE-003: 路由器返回 "file"
- Mock 返回: `"file"`
- 断言: `Process()` 加载 `FileSystemPrompt`，生成的消息包含 `transfer_file`

#### TC-AI-ROUTE-004: 路由器返回 "playbook"
- Mock 返回: `"playbook"`
- 断言: `Process()` 加载 `PlaybookSystemPrompt`，生成的消息包含 `generate_playbook`

#### TC-AI-ROUTE-005: 路由器返回 "uncertain" → 拒绝
- Mock 返回: `"uncertain"`
- 断言: `Process()` 返回 `"我不确定您要做什么"`，不进入 Phase 2

#### TC-AI-ROUTE-006: 路由器返回空字符串 → 拒绝
- Mock 返回: `""`
- 断言: 同上

#### TC-AI-ROUTE-007: 路由器返回带 markdown 的标签 → 清理
- Mock 返回: `` "```exec```" ``
- 断言: 清理后正确识别为 "exec"，加载 ExecSystemPrompt

#### TC-AI-ROUTE-008: 路由器返回带句点的标签 → 清理
- Mock 返回: `"exec."`
- 断言: 同上

#### TC-AI-ROUTE-009: 路由器返回模糊标签 → 模糊匹配
- Mock 返回: `"execute"`（包含 "exec" 子串）
- 断言: 模糊匹配到 exec 组，加载 ExecSystemPrompt

#### TC-AI-ROUTE-010: 路由失败 → error
- Mock 返回 error
- 断言: `Process()` 返回错误，error 信息包含 "路由失败"

#### TC-AI-JSON-001: parseToolCalls 解析合法 JSON
- 输入: ``` ```json\n{"tool_calls":[{"name":"execute_command","arguments":{"command":"uptime","targets":["node1"]}}]}\n``` ```
- 断言: 解析出 1 个 ToolCall，Name="execute_command"，arguments 包含 "command":"uptime"

#### TC-AI-JSON-002: parseToolCalls 解析多个 tool_calls
- 输入: ``` ```json\n{"tool_calls":[{"name":"execute_command","arguments":{...}},{"name":"query_nodes","arguments":{...}}]}\n``` ```
- 断言: 解析出 2 个 ToolCall

#### TC-AI-JSON-003: parseToolCalls 解析非法 JSON → 返回空
- 输入: `"some random text"`
- 断言: 返回空 slice

#### TC-AI-JSON-004: parseToolCalls 解析缺少 tool_calls 字段的 JSON
- 输入: ``` ```json\n{"foo":"bar"}\n``` ```
- 断言: 返回空 slice

#### TC-AI-JSON-005: parseToolCalls 解析缺少 closing ``` 的 JSON
- 输入: ``` ```json\n{"tool_calls":[...]} ```
- 断言: 返回空 slice（jsonEnd == -1）

#### TC-AI-DYN-001: 动态注入 execute_command 提示
- Mock 第 1 轮返回: execute_command + 参数完整 → 工具执行
- 断言: 第 2 轮 messages 包含 `ExecuteCommandPrompt`

#### TC-AI-DYN-002: 动态注入 execute_script 提示
- Mock 第 1 轮返回: execute_script + 参数完整 → 工具执行
- 断言: 第 2 轮 messages 包含 `ExecuteScriptPrompt`

#### TC-AI-DYN-003: 动态注入 generate_playbook 提示
- Mock 第 1 轮返回: generate_playbook
- 断言: 第 2 轮 messages 包含 `PlaybookPrompt`

#### TC-AI-DYN-004: 动态注入 transfer_file 提示
- Mock 第 1 轮返回: transfer_file
- 断言: 第 2 轮 messages 包含 `TransferPrompt`

#### TC-AI-DYN-005: query_nodes 不触发注入
- Mock 第 1 轮返回: query_nodes
- 断言: toolHints map 中无 query_nodes key → 不注入

---

## Layer 1: Go 集成测试

**文件**: `tests/integration/ai_test.go`（新建）

遵循现有 `exec_test.go` 模式：`TestIntegrationEnabled(t)` 守卫 + `os/exec` 调用 `owl ai`

### 测试用例清单

#### TC-AI-INT-001: 命令执行路由 + JSON 输出校验
- 前置: `OWL_TEST_ENABLED=true`, `~/.owl/config.yaml` 已配置 API Key
- 执行: `owl ai "在 self-test-1 上执行 echo hello"`
- 断言: 
  - 退出码 0
  - 输出包含 JSON 格式 `"tool_calls"` 
  - `tool_calls[].name` ∈ {"execute_command"}
  - `tool_calls[].arguments.command` 内容合法

#### TC-AI-INT-002: 节点查询路由 + JSON 输出校验
- 执行: `owl ai "列出所有节点"`
- 断言:
  - 退出码 0  
  - 输出包含 `"tool_calls"`
  - `tool_calls[].name` = "query_nodes"

#### TC-AI-INT-003: 模糊输入路由 → 拒绝
- 执行: `owl ai "随便来点什么"`
- 断言:
  - 输出包含 "不确定" 或没有 `"tool_calls"` JSON

#### TC-AI-INT-004: exec run 组内精炼校验
- 执行: `owl ai "在 self-test-1 上以 detail 格式执行 df -h"`
- 断言:
  - `tool_calls[].name` = "execute_command"
  - `tool_calls[].arguments.format` = "detail"
  - `tool_calls[].arguments.targets` 包含 "self-test-1"

#### TC-AI-INT-005: exec script 路由校验
- 执行: `owl ai "在 self-test-1 执行脚本 tests/testdata/scripts/test-script.sh"`
- 断言:
  - `tool_calls[].name` = "execute_script"
  - `tool_calls[].arguments.script` 包含 "test-script.sh"

---

## Layer 2: Bash E2E 测试

**文件**: `tests/scripts/test-ai.sh`（新建，参照 test-exec.sh 结构）

### 环境变量

```bash
OWL_TEST_NODES="${OWL_TEST_NODES:-self-test-1}"
# 需要 ~/.owl/config.yaml 已配置 AI provider/api_key/model
```

### 测试用例清单

#### test_ai_router_exec
```bash
# TC-AI-BASH-001
# 输入: owl ai "在 $first_node 执行 echo hello"
# 断言: 输出包含 "execute_command"
```

#### test_ai_router_node  
```bash
# TC-AI-BASH-002
# 输入: owl ai "列出所有在线节点"
# 断言: 输出包含 "query_nodes"
```

#### test_ai_router_uncertain
```bash
# TC-AI-BASH-003  
# 输入: owl ai "随便来点什么"
# 断言: 输出包含 "不确定" 或不包含 "tool_calls"
```

#### test_ai_json_format_exec
```bash
# TC-AI-BASH-004
# 输入: owl ai "在 $first_node 执行 uptime"
# 断言: parse JSON → tool_calls[0].name = "execute_command"
#       tool_calls[0].arguments 为合法 JSON object
```

#### test_ai_exec_script
```bash
# TC-AI-BASH-005
# 前置: 创建临时脚本 /tmp/owl-test-ai-script.sh
# 输入: owl ai "在 $first_node 执行脚本 /tmp/owl-test-ai-script.sh"
# 断言: tool_calls[0].name = "execute_script"
```

#### test_ai_router_file
```bash
# TC-AI-BASH-006 (可选，取决于 transfer_file 是否需要真实文件)
# 输入: owl ai "上传 /etc/hosts 到 $first_node"
# 断言: 输出包含 "transfer_file"
```

---

## 文件变更清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/ai/agent_test.go` | 追加 | 新增 mockChatModel + 20 个路由/JSON/注入测试 |
| `tests/integration/ai_test.go` | 新建 | 5 个集成测试（需 API Key） |
| `tests/scripts/test-ai.sh` | 新建 | 6 个 Bash E2E 测试 |
| `tests/Makefile` | 修改 | 增加 `test-bash-ai` target |
| `tests/TEST_MAPPING.md` | 修改 | AI 模块增加 L3 测试映射 |

## 实施步骤

1. **Step 1**: 扩展 `agent_test.go`，新增 `mockChatModel` + 路由测试（TC-AI-ROUTE-001 ~ TC-AI-ROUTE-010）
2. **Step 2**: 追加 JSON 解析测试（TC-AI-JSON-001 ~ TC-AI-JSON-005）
3. **Step 3**: 追加动态注入测试（TC-AI-DYN-001 ~ TC-AI-DYN-005）
4. **Step 4**: 新建 `tests/integration/ai_test.go`（TC-AI-INT-001 ~ TC-AI-INT-005）
5. **Step 5**: 新建 `tests/scripts/test-ai.sh`（TC-AI-BASH-001 ~ TC-AI-BASH-006）
6. **Step 6**: 更新 `tests/Makefile` + `tests/TEST_MAPPING.md`
7. **Step 7**: 运行全量验证：`go test ./internal/ai/...` + `make test-bash-ai`

## 依赖关系

- Step 1 需要 `internal/ai/agent.go` 中的 `Process()` 方法可见性（package private，同包测试可直接访问）
- Step 2-3 依赖 Step 1 中的 mockChatModel
- Step 4-5 依赖 `~/.owl/config.yaml` 已配置 API Key
- Step 7 依赖全部步骤完成
