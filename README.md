# go-owl - 智能分布式运维工具

## 🦉 项目简介

**go-owl** 是一款智能 Linux 分布式运维工具，为你带来智能运维体验。

## ✨ 特性

- 🚀 **节点管理**: 注册、监控和管理多节点，支持分组和标签
- 💻 **批量命令执行**: 在多个节点上同时执行 Shell 命令
- 📜 **脚本传输执行**: 批量上传和执行自定义脚本
- 📋 **Ansible-like 剧本**: 支持 Ansible 风格的 YAML 剧本编排
- 📁 **文件传输**: 支持直接和自扩散传输（P2P 模式）
- 🤖 **AI 助手**: 自然语言驱动的智能运维操作
- 🔒 **安全设计**: 内置安全检查和危险命令识别
- 🖥️ **交互式会话**: 支持单节点实时交互和多节点批量管理
- 📊 **会话历史**: 完整的会话和命令记录，可以随时查看
- 🔑 **SSH 配置集成**: 自动检测和使用 `~/.ssh/config`
- 📥 **节点导入导出**: 支持 YAML/JSON 格式批量管理节点

## 📦 安装

### 从源码构建

项目支持 **DuckDB**（默认）和 **SQLite3** 两种数据库，可根据环境选择：

```bash
git clone https://github.com/cangyunye/go-owl.git
cd go-owl

# 使用 DuckDB 构建（默认）
go build -o owl-duckdb ./cmd/cli/main.go

# 使用 SQLite3 构建（适用于不支持 DuckDB 的环境）
go build -tags sqlite3 -o owl-sqlite3 ./cmd/cli/main.go
```

或者使用 Makefile：

```bash
make build-duckdb    # DuckDB 版本
make build-sqlite3   # SQLite3 版本
make all             # 构建所有版本
```

## 🎉 快速开始

### 🐣 1. 节点管理

添加第一个节点：

```bash
owl node add web-01 \
  --name "Web Server 1" \
  --address 192.168.1.10 \
  --user root \
  --group web \
  --label env=prod
```

查看节点列表：

```bash
owl node list
```

### ⚡ 2. 批量执行命令

在所有节点执行命令：

```bash
owl exec run "uptime" --group web
```

指定节点执行：

```bash
owl exec run "df -h" --nodes web-01,web-02
```

### 📁 3. 文件传输

上传文件到节点：

```bash
owl file upload app.tar.gz --nodes web-01,web-02 --dest /opt/
```

从节点下载文件：

```bash
owl file download /var/log/app.log --node web-01 --dest ./logs/
```

### 🖥️ 4. 交互式会话

连接节点进行交互操作：

```bash
owl session attach web-01
```

## 📚 命令文档

每个命令都有详细的使用说明和测试用例：

| 命令 | 文档 | 说明 |
|------|------|------|
| **节点管理** | [NODE.md](docs/commands/NODE.md) | 节点的增删改查、分组、标签 |
| **命令执行** | [EXEC.md](docs/commands/EXEC.md) | 批量命令执行、剧本、脚本 |
| **文件传输** | [FILE.md](docs/commands/FILE.md) | 上传、下载、扩散传输 |
| **交互会话** | [SESSION.md](docs/commands/SESSION.md) | 实时 SSH 会话管理 |
| **剧本管理** | [PLAYBOOK.md](docs/commands/PLAYBOOK.md) | Ansible-like 剧本编排 |
| **系统设置** | [SETTINGS.md](docs/commands/SETTINGS.md) | 配置管理和目标配置 |
| **AI 助手** | [AI.md](docs/commands/AI.md) | 智能运维辅助 |
| **历史记录** | [HISTORY.md](docs/commands/HISTORY.md) | 执行历史查看 |

## 🛠️ 使用示例

### 添加多个节点

```bash
# 添加 web 服务器
owl node add web-01 --name web1 --address 192.168.1.10 --user root --group web
owl node add web-02 --name web2 --address 192.168.1.11 --user root --group web

# 添加数据库服务器
owl node add db-01 --name db1 --address 192.168.1.20 --user admin --group db

# 按标签分组
owl node labels add web-01 --labels env=prod,tier=frontend
```

### 批量运维操作

```bash
# 所有 web 节点执行命令
owl exec run "systemctl status nginx" --group web

# 按标签筛选
owl exec run "free -h" --label env=prod

# 部署应用
owl file upload ./app.tar.gz --nodes web-01,web-02 --dest /opt/app/
owl exec run "systemctl restart myapp" --nodes web-01,web-02
```

### 剧本编排

```bash
# 列出可用剧本
owl playbook list

# 执行部署剧本
owl playbook run deploy-app --vars version=v1.2.0 --nodes web-01

# 查看剧本详情
owl playbook info deploy-app
```

### AI 助手

```bash
# 智能问答
owl ai chat "如何优化 Nginx 性能"

# 命令解释
owl ai explain "find . -type f -mtime +30 -exec rm {} \;"

# 智能建议
owl ai suggest "服务器负载很高"
```

## 📂 项目结构

```
go-owl/
├── cmd/cli/cmd/        # CLI 命令实现
│   ├── node/          # 节点管理
│   ├── exec/          # 命令执行
│   ├── file/          # 文件传输
│   ├── session/       # 会话管理
│   ├── playbook/      # 剧本管理
│   ├── settings/      # 设置管理
│   ├── ai/           # AI 助手
│   └── history/      # 历史记录
├── internal/          # 内部包
│   ├── node/         # 节点解析
│   ├── ssh/          # SSH 连接
│   ├── control/      # 控制层
│   ├── ai/           # AI 模块
│   └── session/      # 会话管理
└── docs/             # 文档
    └── commands/     # 命令文档
```

## ⚙️ 配置

配置文件位于 `~/.owl/config.yaml`：

```yaml
ai:
  provider: openai
  model: gpt-4o
  api_key: ${OWL_API_KEY}

ssh:
  port: 22
  timeout: 30s
```

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 许可

MIT License
