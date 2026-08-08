# AI 提示词路由线路图

> 供开发者阅读：用户输入 → 路由 → 提示词 → 工具 → CLI 命令的完整链路，
> 标注代码文件与函数对应关系（不含代码行）。配套实现见
> `internal/ai` 各文件与 `cmd/cli/cmd/ai/ai.go`。

## 入口

```
用户输入
 ├─ 单次模式  owl ai "问题"            → cmd/cli/cmd/ai/ai.go      runAI() 单次分支
 │                                        （注册 RejectWriteOpsGate 拒绝写操作）
 └─ 交互模式  owl ai（REPL 循环）       → cmd/cli/cmd/ai/ai.go      runAI() REPL 分支
 │                                         → Session.Send()
 └─ Web OPS AI（serve 进程）            → cmd/plugins/serve/handler/ai.go  Chat()
                                          → ai2.Session.Send()（同一 Session 链路）
```

## 会话层（internal/ai/agent.go）

| 环节 | 函数 | 说明 |
|---|---|---|
| 会话入口 | `Session.Send` | 单 pending 确认队列；首次走 Process，多轮走 ProcessWithContext |
| 确认门 | `Session.SetDefaultConfirmGate` / `Agent.SetConfirmGate` | 写操作集合 `confirmRequiredTools`；拦截时把 ToolCall 存入 `PendingContext` |
| 确认重放 | `Session.Send` 确认分支 → `Agent.ExecuteToolCall` | 用户「是」→ 直接重放保存的 ToolCall（确定性，不经过 LLM） |
| 会话记忆 | `Session.operationMemory` / `dialogueMemory` / `buildMemory` | 最近操作记录 + 最近对话，经 `Agent.SetSessionMemory` 注入路由消息 |
| 单次模式拒绝 | `RejectWriteOpsGate` | 非交互环境写操作返回「请进入交互模式」 |

## 路由与提示词（internal/ai/agent.go + internal/ai/prompts/prompts.go）

```
Session.Send
   ↓
Agent.Process / Agent.ProcessWithContext
   ↓ 构造 routerMessages（RouterPrompt + 会话记忆 + 用户输入）
   ↓
LLM 路由 → 标签（如 node_list / exec_run / settings_show ...）
   ↓ 归一化：uncertain/exec/playbook/node 别名映射
   ↓ 豁免检查：unsupportedRouteLabels（session/serve/tui/metrics/node_sample）
   │       命中 → 返回「该功能不支持 AI 操作」
   ↓
groupPrompts 查表（agent.go）：
   ├─ 专属 SystemPrompt（node/exec/file/playbook 主类别）
   └─ 未定制类别 → GenericToolSystemPrompt（prompts.go，运行时注入
       {{.ToolDescriptions}} + {{.NodeInfo}}）
   ↓
LLM 生成工具调用 JSON → Agent.parseToolCalls
   ↓
Agent.confirmToolCall（确认门，未注册时写操作默认拒绝）
   ↓
Agent.executeToolCall → ToolRegistry 白名单 → Tool.Validate → Tool.Execute
```

## 工具 → CLI 命令映射表

工具实现位于 `internal/ai/tools.go`（既有）、`node_tools.go`、`file_tools.go`、
`playbook_tools.go`、`misc_tools.go`。写操作经确认门，读操作直通。

| 工具 | 对应 CLI 子命令 | 确认门 |
|---|---|---|
| query_nodes / query_database | node list（含过滤） | 否 |
| node_add | node add | 是 |
| node_remove | node remove | 是 |
| node_update | node update | 是 |
| node_status | node status | 否 |
| node_ping | node ping | 否 |
| node_groups | node groups add/remove/list/show | 是（写） |
| node_labels | node labels set/remove/show | 是（写） |
| node_check | node check | 否 |
| node_import / node_export | node import / node export | 是（写） |
| execute_command | exec run（mode: async → --async） | 是 |
| execute_script | exec script | 是 |
| transfer_file | file upload / transfer（diffusion） | 是 |
| file_download | file download | 是 |
| list_playbooks | playbook list | 否 |
| run_playbook | playbook run | 是 |
| validate_playbook | playbook validate | 否 |
| playbook_generate | 本地生成 + 保存 ~/.owl/playbooks | 是（保存） |
| playbook_template_list / info / export | playbook template list/info/export | 否 |
| playbook_scaffold | playbook scaffold | 否 |
| playbook_state_list / show | playbook state list/show | 否 |
| async_list / status / cancel | async list / status / cancel | cancel 是 |
| settings_show / set | settings show / set | set 是 |
| history_list / clean | history / history clean | clean 是 |

## 执行通道（Executor）

| 通道 | 实现位置 | 说明 |
|---|---|---|
| CLI 子进程 | `internal/ai/tools.go` CLIExecutor → runOwlCommand | 生产路径，调 `owl <子命令>` |
| Web 进程内 | `cmd/plugins/serve/handler/aiexecutor.go` WebExecutor | DB/存储实现；节点写操作明确拒绝并引导管理页面 |
| 本地回退 | 各工具 Execute 的 fallback | nodeMgr 内存实现（查询/状态）；测试环境 DisableRealCommands |

## 边界约束

- 工具执行唯一入口 `Agent.executeToolCall`：未知工具名 → 拒绝（注册表白名单）
- 写操作必经确认门；未注册确认门的调用方（如直接调 Agent.Process）默认拒绝写操作
- turn-0 无工具调用时 LLM 自由文本不透出（返回固定「我不确定您要做什么」）
- 路由未识别类别走 GenericToolSystemPrompt，不再硬拒绝
- 一致性测试（`internal/ai/route_consistency_test.go`）防止
  registry ↔ RouterPrompt ↔ groupPrompts 三方漂移

## 已删除的过时路径

- `owl playbook generate` / `owl playbook info`：CLI 从未存在；AI 侧改为
  playbook_generate（本地生成+保存）与 playbook_template_info
- 提示词中 add_node/remove_node/update_node/import_nodes 等陈旧工具名已对齐 registry
