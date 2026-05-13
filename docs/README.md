# go-owl 文档目录

本文档目录包含 go-owl 项目的所有设计文档和使用指南。

## 📚 核心文档

### 使用指南

- [USAGE.md](USAGE.md) - 通用使用指南
- [SESSION_USAGE.md](SESSION_USAGE.md) - 交互式会话功能使用指南
- [SSH_USAGE.md](SSH_USAGE.md) - SSH 配置和使用说明
- [DATABASE.md](DATABASE.md) - 数据库配置说明

### 设计文档

- [implementation_design.md](implementation_design.md) - 整体架构设计文档
- [SESSION_DESIGN.md](SESSION_DESIGN.md) - 会话功能设计文档
- [SSH_CONFIG.md](SSH_CONFIG.md) - SSH 配置解析设计文档
- [AI_OPTIMIZATION_PLAN.md](AI_OPTIMIZATION_PLAN.md) - AI 助手优化计划
- [LOGGING_PLAN.md](LOGGING_PLAN.md) - 日志系统设计文档

## 📖 快速导航

### 新手入门
1. 查看 [USAGE.md](USAGE.md) 了解基本使用方法
2. 尝试添加节点并执行命令
3. 使用 AI 助手进行自然语言运维

### 会话功能
1. 阅读 [SESSION_USAGE.md](SESSION_USAGE.md)
2. 了解单节点和多节点会话模式
3. 使用 `session history` 查看历史记录

### SSH 配置
1. 查看 [SSH_CONFIG.md](SSH_CONFIG.md) 了解配置解析
2. 参考 [SSH_USAGE.md](SSH_USAGE.md) 进行配置
3. 会话功能会自动使用 `~/.ssh/config`

### 数据库
1. 阅读 [DATABASE.md](DATABASE.md) 了解数据库选项
2. 支持 DuckDB（默认）和 SQLite3
3. 使用 `sqlite3` 构建标签切换数据库

## 🔧 高级功能

### AI 助手优化
- 参见 [AI_OPTIMIZATION_PLAN.md](AI_OPTIMIZATION_PLAN.md)
- 支持多种 LLM 提供商
- 严格的意图识别机制

### 日志系统
- 参见 [LOGGING_PLAN.md](LOGGING_PLAN.md)
- 完整的操作审计
- 可配置的日志级别

## 📝 文档贡献

欢迎贡献文档！请确保：
- 使用清晰的中文表达
- 包含实际使用示例
- 保持文档与代码同步更新
