# 限制 AI 回答范围到 owl 命令领域 Spec

## Why

当前 AI 对话中，当用户输入如"查询mac节点"时，LLM 在领域提示词阶段可能输出与 owl 无关的通用知识（如 MAC 地址介绍、macOS 操作指南），而非严格调用 owl 工具。根因有两点：1) 提示词未明确界定 owl 的功能边界，LLM 凭自身知识自由发挥；2) 代码层缺少安全网——当 LLM 未返回工具调用 JSON 而是返回长文本时，当前逻辑直接透传给用户，未做拦截。

## What Changes

* **MODIFIED** `RouterPrompt` — 新增 owl 范围定义，明确只有与节点管理/命令执行/文件传输/剧本管理相关的查询才路由到命令组，其余一律输出`uncertain`

* **MODIFIED** 4 个领域 SystemPrompt — 每个提示词开头新增"owl 范围界定"段落，声明 owl 是什么、只能回答什么、遇到无关问题必须拒绝

* **MODIFIED** `Agent.Process()` — 新增安全网：当 `parseToolCalls` 返回空且响应内容为长文本（非"我不确定您要做什么"）时，丢弃 LLM 输出并返回受控的拒绝消息

## Impact

* Affected specs: `hierarchical-prompt-routing`, `enhance-ai-dialog-capabilities`

* Affected code:

  * `internal/ai/prompts/prompts.go` — RouterPrompt + 4 个领域 SystemPrompt

  * `internal/ai/agent.go` — Process() 安全网逻辑

## MODIFIED Requirements

### Requirement: RouterPrompt 范围界定

RouterPrompt SHALL 在路由分类前先声明 owl 的功能范围，确保超出范围的查询被归类为 `uncertain`。

#### Scenario: 与 owl 无关的查询被拒绝

* **WHEN** 用户输入"MAC地址怎么查"

* **THEN** 路由输出 `uncertain`，AI 回复“不明白您的意图，我只能处理节点管理/命令执行/文件传输/剧本管理相关的动作”

#### Scenario: 域名内查询正确路由

* **WHEN** 用户输入"查询mac节点"（mac 是 owl 管理的节点名关键字）

* **THEN** 路由输出 `node`

#### Scenario: 歧义输入路由

* **WHEN** 用户输入"mac"（单字，无法确定意图）

* **THEN** 路由输出 `uncertain`

### Requirement: 领域提示词范围界定

每个领域 SystemPrompt SHALL 在开头包含 owl 范围界定段落，内容涵盖：

* owl 是什么（分布式 Linux 节点管理运维工具）

* 你只能回答什么（该领域内的工具调用）

* 遇到无关问题时必须做什么（回复"我不确定您要做什么"）

* 明确列出不属于 owl 范围的典型误识别场景（如 MAC 地址、macOS 操作、区块链节点等）

#### Scenario: 节点管理领域拒绝 MAC 地址查询

* **WHEN** 路由阶段已将"查询mac节点"归类为 `node`，进入 NodeSystemPrompt

* **THEN** LLM 在领域提示词的范围内理解"mac"是节点名称关键字，输出 `{"tool_calls":[{"name":"query_nodes","arguments":{"search":"mac"}}]}`

#### Scenario: 节点管理领域拒绝操作系统指南

* **WHEN** 路由阶段错误将"macOS怎么用"归类为 `node`

* **THEN** LLM 看到领域提示词的范围界定后输出"我不确定您要做什么"

### Requirement: 代码安全网拦截非工具调用长文本

`Agent.Process()` SHALL 在工具调用循环中，当 `parseToolCalls` 返回空时，检查响应内容：

* 如果响应包含有意义的长文本（非工具调用 JSON，也非"我不确定您要做什么"），丢弃该文本，返回受控的拒绝消息

* 如果响应是"我不确定您要做什么"或其变体，正常返回给用户

#### Scenario: LLM 输出无关长文本被拦截

* **WHEN** LLM 在领域提示词下输出了关于 MAC 地址的长篇解释（无 tool\_calls JSON）

* **THEN** 代码安全网检测到非工具调用长文本，丢弃 LLM 输出，返回"我不确定您要做什么"

#### Scenario: 合法的拒绝响应正常透传

* **WHEN** LLM 输出"我不确定您要做什么"

* **THEN** 正常返回给用户

#### Scenario: 合法的工具调用后带简短说明

* **WHEN** LLM 输出包含 `tool_calls` JSON 和简短说明文字

* **THEN** `parseToolCalls` 成功提取 JSON，正常执行工具

