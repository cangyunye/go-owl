# owl ai 命令详解

AI 助手模块，提供智能运维辅助功能。

---

## 1. 命令列表

```
owl ai - AI 助手
├── owl ai chat    - 聊天交互
├── owl ai explain - 解释命令
├── owl ai suggest - 建议操作
├── owl ai models - 模型管理
└── owl ai config - AI 配置
```

---

## 2. owl ai chat

与 AI 进行自然语言对话。

### 使用方法

```bash
owl ai chat
owl ai chat "检查所有节点的磁盘使用情况"
owl ai chat --interactive
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `<message>` | 直接发送的消息 |
| `--interactive` | 交互式聊天模式 |
| `--model` | 指定模型 |
| `--provider` | 指定 Provider |

### 示例

```bash
# 直接提问
owl ai chat "如何优化 Nginx 性能"

# 交互式聊天
owl ai chat --interactive

# 指定模型
owl ai chat "重启所有 web 服务器" --model gpt-4o
```

### 示例输出

```
$ owl ai chat "检查所有节点的磁盘使用情况"

🤖 AI 助手:

建议执行以下命令来检查磁盘使用情况:

1. 在所有 web 节点执行:
   df -h

2. 在所有 db 节点执行:
   df -h | grep -E '/$|/var|/data'

3. 查看大文件:
   du -sh /var/log/*

是否需要我帮你执行这些命令? (yes/no)
```

---

## 3. owl ai explain

解释 Shell 命令。

### 使用方法

```bash
owl ai explain "<command>"
```

### 示例

```bash
$ owl ai explain "find . -type f -mtime +30 -exec rm {} \;"

📖 命令解释:

find . -type f -mtime +30 -exec rm {} \;

- find .          : 在当前目录查找
- -type f         : 只查找文件
- -mtime +30      : 修改时间超过 30 天
- -exec rm {} \;  : 删除找到的文件

⚠️ 警告: 此命令会删除文件，请谨慎使用!
建议先使用 find . -type f -mtime +30 进行预览
```

---

## 4. owl ai suggest

根据情况建议运维操作。

### 使用方法

```bash
owl ai suggest "<situation>"
owl ai suggest --context "node=web-01,issue=high-cpu"
```

### 示例

```bash
$ owl ai suggest "服务器负载很高"

🤖 建议:

可能的原因:
1. 运行的进程过多
2. 内存不足导致 swap
3. 磁盘 I/O 瓶颈
4. 网络攻击

建议检查:
1. 查看进程: ps aux --sort=-%cpu | head
2. 查看内存: free -h
3. 查看磁盘: iostat -x 1 5
4. 查看网络: netstat -an | grep ESTABLISHED | wc -l

建议执行的操作:
1. top -c (查看占用最高的进程)
2. systemctl restart <service> (重启有问题的服务)
```

---

## 5. owl ai models

管理 AI 模型。

### 使用方法

```bash
# 列出可用模型
owl ai models

# 刷新模型列表
owl ai models --refresh

# 查看特定 Provider 的模型
owl ai models --provider anthropic
```

### 示例输出

```
📦 AI 模型列表

当前配置:
  Provider: openai
  模型:    gpt-4o

可用模型 (OpenAI):
───────────────────────────────────────
  ● gpt-4o           GPT-4o (128K 上下文)
    gpt-4o-mini     GPT-4o Mini (128K 上下文)
    gpt-4-turbo    GPT-4 Turbo (128K 上下文)
    gpt-4           GPT-4 (8K 上下文)
    gpt-3.5-turbo  GPT-3.5 Turbo (16K 上下文)

可用模型 (Anthropic):
───────────────────────────────────────
    claude-3.5-sonnet  Claude 3.5 Sonnet (200K 上下文)
    claude-3-opus      Claude 3 Opus (200K 上下文)
    claude-3-sonnet    Claude 3 Sonnet (200K 上下文)
    claude-3-haiku     Claude 3 Haiku (200K 上下文)

💡 提示: 使用 owl settings set ai.model <model-name> 切换模型
```

---

## 6. owl ai config

AI 配置管理。

### 使用方法

```bash
# 测试 API 连接
owl ai config test

# 设置 API Key
owl ai config set api-key "sk-xxx"

# 查看当前配置
owl ai config show
```

### 示例

```bash
$ owl ai config test

🔧 测试 AI 配置...

✓ Provider: openai
✓ Model: gpt-4o
✓ API Key: 已设置
✓ 连接测试: 成功
✓ 响应时间: 1.2s

配置正常!
```

---

## 7. 测试用例

### TC-AI-001: 直接聊天

```bash
# 步骤
$ owl ai chat "你好"

# 预期结果
# AI 返回问候和功能介绍
```

### TC-AI-002: 命令解释

```bash
# 步骤
$ owl ai explain "chmod +x script.sh"

# 预期结果
# 详细解释命令含义
```

### TC-AI-003: 智能建议

```bash
# 步骤
$ owl ai suggest "内存使用率 95%"

# 预期结果
# 提供可能原因和解决方案
```

### TC-AI-004: 模型列表

```bash
# 步骤
$ owl ai models

# 预期结果
# 显示所有可用模型
```

### TC-AI-005: 配置测试

```bash
# 步骤
$ owl ai config test

# 预期结果
# 测试 API 连接并显示结果
```

---

## 8. 常见问题

### Q: 需要 API Key 吗？
A: 是的，需要配置 AI Provider 的 API Key

### Q: 支持哪些 Provider？
A: OpenAI、Anthropic (Claude)、阿里云 DashScope

### Q: 如何设置 API Key？
A: `owl settings set ai.api_key "sk-xxx"` 或设置环境变量 `OWL_API_KEY`

### Q: 命令执行安全吗？
A: AI 建议的命令需要用户确认才会执行
