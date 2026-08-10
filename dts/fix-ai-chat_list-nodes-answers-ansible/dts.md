---
id: "fix-ai-chat_list-nodes-answers-ansible"
domain: "fix-ai-chat"
slug: "list-nodes-answers-ansible"
title: "为什么在 AI 对话中询问\"列出所有节点\"没有执行 owl node list,而是返回了 ansible 命令提示(用了 ansible-inventory)"
status: "resolved"
created: "2026-08-07T00:16:56+08:00"
resolved: "2026-08-07T00:31:55+08:00"
commit: "5901172"
branch: "feat/ui-redesign-phase0"
platform: "darwin"
session: "ses_028230b87ffeNVb393w4GItvl0"
---

# fix-ai-chat_list-nodes-answers-ansible

## 问题

为什么在 AI 对话中询问"列出所有节点"没有执行 owl node list,而是返回了 ansible 命令提示(用了 ansible-inventory)?

## 环境

| 项 | 值 |
|----|----|
| git commit | 5901172 |
| 分支 | feat/ui-redesign-phase0 |
| 平台 | darwin |
| 建档时间 | 2026-08-07T00:16:56+08:00 |
| 会话 | ses_028230b87ffeNVb393w4GItvl0 |

## 调查过程

- [00:16] 建档
- [00:18] 记录日志 (chat): 根因: 配了 API key 时 Chat 走裸 LLM 分支,未走 agent 工具链
- [00:30] 记录日志 (chat): 修复完成: Web AI 对话改走 agent 全链路, 真实返回节点列表
- [00:31] 记录证据 1 项
- [00:31] 记录终端文本快照
- [00:31] 结案

## 日志与摘录

### [chat] 2026-08-07T00:18:54+08:00 · 根因: 配了 API key 时 Chat 走裸 LLM 分支,未走 agent 工具链

```
前端 ai.js sendMsg(): 只要用户保存过 API key,就会发 encrypted_api_key + model + base_url + api_type。

服务端 handler/ai.go Chat() (cmd/plugins/serve/handler/ai.go:65-130):
- 若 req.EncryptedAPIKey != "" && req.Model != "" → 走"Try LLM"分支
- 该分支只用一个通用 systemPrompt("你是 OWL Agent,一个运维智能助手...") + 用户消息 直接调 CallLLM
- 成功则原样返回 LLM 自然语言回复,**不经过** agent/router/工具
- agent 框架(session.Send → agent.Process → RouterPrompt → NodeListSystemPrompt → query_nodes 工具 → WebExecutor.QueryNodes 查真实 DB)只在 LLM 调用失败时才作为 fallback

所以配了 API key 时,"列出所有节点" 被裸 LLM 当作普通问答回答,LLM 看不到 owl 节点库、没有 query_nodes 工具、没有 JSON 输出契约,于是幻觉出 ansible all --list-hosts / ansible-inventory --list。
```

### [chat] 2026-08-07T00:30:58+08:00 · 修复完成: Web AI 对话改走 agent 全链路, 真实返回节点列表

```
修复方案:
1. handler/ai.go Chat(): 删除裸 LLM 分支(只带通用 systemPrompt 直接调 CallLLM)。改为: 配了 key+model 时, 用 webLLMChatModel(把 CallLLM 适配成 ai2.ChatModel) + dbNodeStoreAdapter(查 serve nodes 表) 构建 per-session agent, 走 session.Send(agent.Process) 全链路(路由→NodeListSystemPrompt→query_nodes 工具→WebExecutor.QueryNodes 查真实 DB)。LLM 失败时降级到默认 rule-based agent(fallback session)。
2. 新增 handler/ai_node.go: dbNodeStoreAdapter 实现 ai2.NodeStoreAdapter。
3. 新增 AIHandler.newChatAgent 工厂字段(测试可注入 mock)。

E2E(真实 DeepSeek key, 服务器 + 3个种子节点 + 已有节点):
- "列出所有节点" → 返回 DB 中真实节点表(ID/Name/Address/Status), 无 ansible、无"我不确定您要做什么"
- 同 session 追问 "列出在线的节点" → 只返回 online 节点(多轮正常)
```

## 测试场景与 E2E 用例

| # | 用例 | 步骤 | 预期 | 结果 |
|---|------|------|------|------|

## 证据截图

[文本快照: E2E: 真实 DeepSeek key 调用 /api/v1/ai/chat "列出所有节点" 的返回](shots/001-003104.txt)

## 修复方案

Web /ai/chat 不再走裸 LLM: 配了 key+model 时用 webLLMChatModel(把 CallLLM 适配为 ai2.ChatModel)+dbNodeStoreAdapter 构建 per-session agent, 走 session.Send(路由→NodeListSystemPrompt→query_nodes 工具→WebExecutor.QueryNodes 查真实 DB), 与 CLI owl ai 一致; LLM 失败降级 rule-based agent。E2E 用真实 DeepSeek key 验证 "列出所有节点" 返回真实节点表。

## 复盘

根因: Chat() 为"用户自带 key 直接聊天"引入的裸 LLM 分支绕过了 agent 工具链, LLM 看不到节点库也没工具可调, 于是对运维指令幻觉出 ansible 命令。教训: 带工具/上下文的 agent 流程不能被裸 LLM 捷径替换; 任何新 LLM 调用点都应复用 agent 框架(路由+工具+节点清单)。
