# Go-Owl AI 助手使用指南

## 简介

Go-Owl 的 AI 助手通过自然语言交互执行分布式运维操作。AI 助手经过优化，可以严格地将自然语言请求映射到 4 种核心操作上，不会生成超出范围的内容。

## 支持的操作类型

AI 助手支持以下 4 种核心操作：

### 1. 查询节点信息
查看节点状态、分组、标签等信息。

**常见关键词**：查询、查看、列出、list、show、节点、状态、分组

### 2. 执行命令
在指定节点上运行 shell 命令。

**常见关键词**：执行、运行、命令、execute、run、shell、uptime、df、systemctl

### 3. 生成剧本
根据需求生成 Ansible-like YAML 剧本。

**常见关键词**：生成、创建、剧本、playbook、安装、部署、nginx、apache

### 4. 传输文件
向节点分发文件。

**常见关键词**：传输、上传、下载、文件、transfer、copy、.tar、.gz、.zip

### 不确定意图的响应

如果 AI 助手无法确定您的意图，它会提供帮助信息：

```
抱歉，我无法确定您要执行的具体操作。

我可以帮助您：
  1. 查询节点信息 - 查看节点状态、分组、标签
  2. 执行命令 - 在指定节点上运行 shell 命令
  3. 生成并执行剧本 - 自动化部署操作
  4. 传输文件 - 向节点分发文件

请告诉我您具体要做什么？

例如：
  - "列出所有在线节点"
  - "在 web 节点上执行 uptime"
  - "安装 nginx"
  - "把 app.tar.gz 传到所有节点"
```

## 工作原理

```
用户输入 → AI 理解意图 → 选择工具 → 验证参数 → 执行操作 → 返回结果
```

## 命令行使用

### 交互式模式
```bash
owl ai
```

### 单次查询模式
```bash
owl ai "在所有节点上执行 uptime"
owl ai "查询所有在线节点"
owl ai "帮我生成一个部署 nginx 的剧本"
```

### 列出可用模型
```bash
owl ai models
```

## AI 助手核心组件

| 组件 | 文件 | 功能 |
|------|------|------|
| 意图分类器 | `intent_classifier.go` | 关键词匹配、置信度评估、特殊场景识别 |
| 参数提取器 | `param_extractor.go` | 自动提取节点目标、命令识别、文件路径检测 |
| 参数验证器 | `validator.go` | 必填参数检查、数据类型验证、格式验证 |
| 响应格式化器 | `response_formatter.go` | 友好的确认信息、错误格式化、帮助提示 |

## 配置

### 配置文件

在 `~/.owl/config.yml` 中配置：
```yaml
ai:
  provider: qwen  # openai / anthropic / qwen / deepseek
  model: qwen-turbo
  api-key: your-api-key
```

### 命令行参数

| 参数 | 说明 |
|------|------|
| `--model` | AI 模型名称 |
| `--provider` | AI 提供商 (openai, anthropic, dashscope, qwen, deepseek) |
| `--api-key` | API Key |
| `--base-url` | API Base URL |
| `--timeout` | 请求超时时间 |
| `--session` | 会话 ID |

### 环境变量

| 变量 | 说明 |
|------|------|
| `OWL_API_KEY` | AI API Key |
| `OWL_BASE_URL` | API Base URL |
| `OWL_MODEL` | 默认模型 |
| `OWL_PROVIDER` | 默认 Provider |

## 常见问题

### Q: 需要 API Key 吗？
A: 是的，需要配置 AI Provider 的 API Key。

### Q: 支持哪些 Provider？
A: OpenAI、Anthropic (Claude)、阿里云 DashScope (Qwen)、DeepSeek。

### Q: AI 执行命令安全吗？
A: AI 建议的命令需要用户确认才会执行。

### Q: 如何查看 API Key 是否配置正确？
A: 使用 `owl ai models` 命令测试连接。
