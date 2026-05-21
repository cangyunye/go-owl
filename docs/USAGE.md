# Go-Owl AI 助手使用指南

## 📖 简介

Go-Owl 的 AI 助手经过优化，现在可以严格地将自然语言请求映射到 4 种核心操作上，不会生成超出范围的内容。

## 🎯 支持的操作类型

AI 助手只能帮助您执行以下 4 种操作：

### 1. 查询节点信息
查看节点状态、分组、标签等信息。

**常见关键词**：查询、查看、列出、list、show、节点、状态

### 2. 执行命令
在指定节点上运行 shell 命令。

**常见关键词**：执行、运行、命令、execute、run、shell、命令名（如 uptime、df）

### 3. 生成并执行剧本
根据需求生成 Ansible-like YAML 剧本并执行。

**常见关键词**：生成、创建、剧本、playbook、安装、部署、nginx、apache

### 4. 传输文件
向节点分发文件。

**常见关键词**：传输、上传、下载、文件、transfer、copy、.tar、.gz、.zip

## 💡 使用示例

### 查询节点信息示例

```
> 列出所有在线节点
> 查看 web 分组的节点
> 有多少个节点？
> 显示节点状态
```

### 执行命令示例

```
> 在所有节点上执行 uptime
> 在 web1 和 web2 上运行 df -h
> 执行 systemctl status nginx
> 检查内存使用情况
```

### 生成并执行剧本示例

```
> 在 web 节点上安装 nginx
> 部署应用程序
> 安装 redis 并启动服务
> 配置 apache
```

### 传输文件示例

```
> 把 app.tar.gz 传到所有节点
> 上传 config.yml 到 /opt 目录
> 分发文件到 db 分组
> 复制文件到 web1
```

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

## 🔧 AI 助手核心组件

### 1. 意图分类器 ([intent_classifier.go](internal/ai/intent_classifier.go))
- 关键词匹配
- 置信度评估
- 特殊场景识别（文件路径、安装、命令）

### 2. 参数提取器 ([param_extractor.go](internal/ai/param_extractor.go))
- 自动提取节点目标
- 命令识别
- 文件路径检测
- 目标目录识别

### 3. 参数验证器 ([validator.go](internal/ai/validator.go))
- 必填参数检查
- 数据类型验证
- 取值范围校验
- 格式验证

### 4. 响应格式化器 ([response_formatter.go](internal/ai/response_formatter.go))
- 友好的确认信息
- 错误信息格式化
- 帮助提示

## 📋 命令行使用

### 交互式模式
```bash
owl ai
```

### 单次查询模式
```bash
owl ai "在所有节点上执行 uptime"
```

### 配置 LLM 提供商
在 `~/.owl/config.yml` 中配置：
```yaml
ai:
  provider: qwen  # openai / anthropic / qwen / deepseek
  model: qwen-turbo
  api-key: your-api-key
```

## 🎯 优化目标

本次优化主要实现：

- ✅ 严格限制自然语言映射到 4 种操作
- ✅ 不生成超出范围的内容
- ✅ 提供清晰的交互反馈
- ✅ 支持命令行和配置文件方式
- ✅ 提供完整的示例

## 📚 相关文档

- [README.md](README.md) - 项目主文档
- [AI_OPTIMIZATION_PLAN.md](AI_OPTIMIZATION_PLAN.md) - AI 优化详细计划
- [LOGGING_PLAN.md](LOGGING_PLAN.md) - 日志系统文档
