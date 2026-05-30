# owl ai 命令详解

AI 智能助手模块，通过自然语言交互执行分布式运维操作。

---

## 1. 命令列表

```
owl ai - AI 智能助手
├── owl ai              - 启动 AI 交互模式
├── owl ai models      - 列出可用模型
└── owl ai config      - AI 配置管理
    ├── owl ai config init    - 初始化配置文件
    └── owl ai config show    - 显示当前配置
```

---

## 2. owl ai

启动 AI 智能助手交互模式，通过自然语言执行运维操作。

### 使用方法

```bash
owl ai
owl ai "检查所有节点的磁盘使用情况"
owl ai --model gpt-4o
owl ai --provider dashscope
owl ai --session <session-id>
owl ai "查询节点" --verbose  # 显示详细调试信息
echo "查询所有在线节点" | owl ai
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `--model` | AI 模型名称，默认 gpt-4o |
| `--provider` | AI 提供商: openai, anthropic, dashscope, qwen, deepseek |
| `--api-key` | API Key（也可通过环境变量 OWL_API_KEY 设置） |
| `--base-url` | API Base URL（用于代理或自定义端点） |
| `--timeout` | 请求超时时间（秒），默认 120 |
| `--session` | 会话 ID（用于恢复历史会话） |
| `--verbose` / `-v` | 详细调试模式，显示完整的 AI 交互过程和日志 |
| `--debug` | 详细调试模式（--verbose 的别名，保持向后兼容性） |

### AI 支持的功能

AI 助手通过工具调用实现以下功能：

| 功能 | 说明 |
|------|------|
| 查询节点 | 查询节点状态、分组、标签 |
| 执行命令 | 在指定节点上执行命令 |
| 生成剧本 | 根据需求生成 Ansible-like YAML 剧本 |
| 文件传输 | 传输文件到指定节点 |

### 工作原理

```
用户输入 → AI 理解意图 → 选择工具 → 验证参数 → 调用 owl 子命令 → 返回原始结果
```

AI 会自动：
1. 理解自然语言请求
2. 选择合适的工具
3. 验证参数有效性
4. **直接调用对应的 owl 子命令**（如 `owl node list`, `owl exec run` 等）
5. 返回与直接使用命令相同的原始输出，确保一致性

### 示例

```bash
# 启动交互式对话
owl ai

# 直接提问
owl ai "查询所有在线节点"

# 执行批量命令
owl ai "在所有 web 节点执行 df -h"

# 生成剧本
owl ai "帮我生成一个部署 nginx 的剧本"

# 传输文件
owl ai "把本地的 config.yaml 传到 web-01 的 /opt/config/ 目录"

# 指定模型
owl ai "重启所有 web 服务器" --model gpt-4o

# 使用阿里云 DashScope
owl ai "检查数据库状态" --provider dashscope --api-key sk-xxx

# 恢复历史会话
owl ai --session sess-abc123

# 详细调试模式
owl ai "查询所有节点" --verbose
```

### 示例输出

```
$ owl ai "查询所有在线节点"
[08:14:50] 用户：查询所有在线节点
[08:14:51] owl-ai: 确认用户调用子命令为 node_list 相关
[08:14:51] owl-ai: 请求模型生成执行 JSON...
[08:14:52] owl-ai: JSON 校验通过 (query_nodes)
[08:14:52] owl-ai: 开始执行操作

ID                   Name                      Address                   User       Status       Groups               Labels                         Last Check          
-------------------------------------------------------------------------------------------------------------------------------------------------------------------------
web-01               web1                      192.168.1.10:22           root       online       web                  env=prod                       2026-05-22T23:22:48Z
web-02               web2                      192.168.1.11:22           root       online       web                  env=prod                       2026-05-22T23:22:49Z
db-01                db1                       192.168.1.20:22           root       online       db                   env=prod                       2026-05-22T23:22:50Z

Total: 3 nodes
```

```
$ owl ai "在 web 分组执行 uptime"
[08:14:53] 用户：在 web 分组执行 uptime
[08:14:54] owl-ai: 确认用户调用子命令为 exec_run 相关
[08:14:54] owl-ai: 请求模型生成执行 JSON...
[08:14:55] owl-ai: JSON 校验通过 (execute_command)
[08:14:55] owl-ai: 开始执行操作

✅ [web-01] 成功
   10:30:00 up 100 days,  1 user,  load average: 0.15, 0.20, 0.15

✅ [web-02] 成功
   10:30:00 up 50 days,   2 users, load average: 0.25, 0.30, 0.25

📊 总结: 2 成功, 0 失败
```

#### 使用 --verbose 详细模式查看完整调试信息：
```
$ owl ai "查询所有节点" --verbose
[08:14:53] DEBUG: 用户输入: 查询所有节点
[08:14:53] 用户：查询所有节点
2026-05-30T08:14:55.126+0800    DEBUG   ai-debug        internal/ai/agent.go:62  路由原始响应: node_list
2026-05-30T08:14:55.126+0800    DEBUG   ai-debug        internal/ai/agent.go:62  路由标签: node_list
[08:14:55] owl-ai: 确认用户调用子命令为 node_list 相关
...
# 完整的 AI 交互过程和所有调试日志
```

---

## 3. owl ai models

列出可用的 AI 模型。

### 使用方法

```bash
owl ai models
owl ai models --provider openai
owl ai models --provider qwen --api-key sk-xxx
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `--provider` | 指定 Provider，默认使用全局配置的 Provider |
| `--api-key` | API Key（必需） |
| `--base-url` | API Base URL |

### 示例输出

```
📦 AI 模型列表

当前配置:
  Provider: openai
  模型:    gpt-4o

正在获取可用模型列表...

可用模型 (OpenAI):
───────────────────────────────────────
  ● gpt-4o           GPT-4o (128K 上下文)
    gpt-4o-mini     GPT-4o Mini (128K 上下文)
    gpt-4-turbo    GPT-4 Turbo (128K 上下文)
    gpt-4           GPT-4 (8K 上下文)
    gpt-3.5-turbo  GPT-3.5 Turbo (16K 上下文)
```

---

## 4. owl ai config

AI 配置管理模块，帮助用户初始化和查看配置文件。

### 4.1 owl ai config init

快速初始化配置文件到 `~/.owl/config.yaml`。

#### 使用方法

```bash
owl ai config init
```

#### 功能说明
- 检查配置文件是否已存在
- 如果不存在，创建默认配置文件
- 包含完整的 AI、提示词、安全配置

#### 示例输出

```
$ owl ai config init
✓ 配置文件已创建: ~/.owl/config.yaml

下一步：
  1. 编辑配置文件设置 API Key
  2. 或使用 'owl ai models' 检查连接
```

#### 生成的配置文件内容

```yaml
ai:
    provider: openai
    model: gpt-4o
    api_key: ""
    base_url: ""
    timeout: 120

prompts:
    system: system.md
    playbook: playbook.md
    command: command.md
    transfer: transfer.md

safety:
    confirm_dangerous: true
    allowed_commands: []
    blocked_commands:
        - rm -rf /
        - rm -rf /*
        - ':(){:|:&};:'
        - '>/dev/sda'
        - dd if=/dev/zero of=/dev/sda
```

### 4.2 owl ai config show

显示当前的 AI 配置信息，API Key 会被隐藏保护。

#### 使用方法

```bash
owl ai config show
```

#### 示例输出

```
$ owl ai config show
当前配置:

  Provider:    openai
  Model:       gpt-4o
  API Key:     sk-****-xxxx
  Base URL:    https://api.openai.com/v1
  Timeout:     120s
```

---

## 6. 支持的 AI 提供商

### OpenAI

| 模型 | 说明 |
|------|------|
| gpt-4o | GPT-4o，默认模型 |
| gpt-4o-mini | GPT-4o Mini，性价比高 |
| gpt-4-turbo | GPT-4 Turbo |
| gpt-4 | GPT-4 |
| gpt-3.5-turbo | GPT-3.5 Turbo |

### Anthropic

| 模型 | 说明 |
|------|------|
| claude-3-5-sonnet-latest | Claude 3.5 Sonnet，推荐 |
| claude-3-opus-latest | Claude 3 Opus |
| claude-3-sonnet-latest | Claude 3 Sonnet |
| claude-3-haiku-latest | Claude 3 Haiku |

### 阿里云 DashScope (Qwen)

| 模型 | 说明 |
|------|------|
| qwen-plus | 通义千问 Plus |
| qwen-max | 通义千问 Max |
| qwen-turbo | 通义千问 Turbo |

### DeepSeek

| 模型 | 说明 |
|------|------|
| deepseek-chat | DeepSeek Chat |
| deepseek-coder | DeepSeek Coder |

---

## 7. 环境变量

| 变量 | 说明 |
|------|------|
| `OWL_API_KEY` | AI API Key |
| `OWL_BASE_URL` | API Base URL（用于代理） |
| `OWL_MODEL` | 默认模型 |
| `OWL_PROVIDER` | 默认 Provider |

---

## 8. 测试用例

### TC-AI-001: 查询节点

```bash
# 步骤
$ owl ai "查询所有节点"

# 预期结果
# AI 返回节点列表
```

### TC-AI-002: 执行命令

```bash
# 步骤
$ owl ai "在 web-01 执行 df -h"

# 预期结果
# AI 调用工具执行命令，返回结果
```

### TC-AI-003: 模型列表

```bash
# 步骤
$ owl ai models

# 预期结果
# 显示所有可用模型
```

### TC-AI-004: 使用自定义 Provider

```bash
# 步骤
$ owl ai --provider dashscope --api-key sk-xxx "查询节点"

# 预期结果
# 使用阿里云 DashScope 执行查询
```

### TC-AI-005: 初始化配置

```bash
# 步骤
$ owl ai config init

# 预期结果
# 成功创建 ~/.owl/config.yaml 配置文件
```

### TC-AI-006: 显示配置

```bash
# 步骤
$ owl ai config show

# 预期结果
# 显示当前配置，API Key 被隐藏
```

---

## 9. 常见问题

### Q: 需要 API Key 吗？
A: 是的，需要配置 AI Provider 的 API Key。可通过 `--api-key` 参数或设置 `OWL_API_KEY` 环境变量。

### Q: 支持哪些 Provider？
A: OpenAI、Anthropic (Claude)、阿里云 DashScope (Qwen)、DeepSeek。

### Q: AI 执行命令安全吗？
A: AI 建议的命令需要用户确认才会执行。

### Q: 如何查看 API Key 是否配置正确？
A: 使用 `owl ai models` 命令测试连接。

### Q: 支持流式输出吗？
A: 是的，支持实时流式响应。

### Q: 如何恢复之前的会话？
A: 使用 `--session <session-id>` 参数恢复历史会话。

### Q: 请求超时怎么办？
A: 使用 `--timeout` 参数增加超时时间，默认 120 秒。

### Q: 如何快速初始化配置文件？
A: 使用 `owl ai config init` 命令快速生成默认配置文件到 `~/.owl/config.yaml`。

### Q: 如何查看当前配置？
A: 使用 `owl ai config show` 查看当前配置，API Key 会被隐藏保护。

### Q: 配置文件在哪里？
A: 配置文件位于 `~/.owl/config.yaml`。
