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

或者直接运行：

```bash
go install github.com/cangyunye/go-owl/cmd/cli@latest
```

> **提示**：如果编译 DuckDB 版本遇到问题，可以切换到 SQLite3 版本。

## 🎉 快速开始

### 🐣 1. 节点管理

```bash
# 添加节点（指定用户名）
owl node add --name web1 --address 192.168.1.10 --port 22 --user root
owl node add --name db1 --address 192.168.1.20 --port 22 --user admin

# 查看所有节点
owl node list

# 添加分组和标签
owl node group add web1 --nodes web1,web2

# 删除节点
owl node remove web1
```

### 📊 2. 批量执行命令

```bash
# 在所有节点执行命令
owl exec --command "uptime"

# 在指定节点执行
owl exec --nodes web1,web2 --command "df -h"

# 按分组执行
owl exec --group web --command "systemctl status nginx"

# 执行脚本
owl exec --nodes web1 --script ./deploy.sh
```

### 📜 3. 剧本执行

编写一个 YAML 剧本：

```yaml
# deploy.yml
- name: 部署应用
  hosts: web
  become: yes
  tasks:
    - name: 安装依赖
      shell: apt-get install -y nginx
    - name: 启动服务
      systemd:
        name: nginx
        state: started
```

执行剧本：

```bash
owl playbook run deploy.yml

# 其他剧本命令
owl playbook list       # 列出所有剧本
owl playbook validate   # 验证剧本语法
```

### 📁 4. 文件传输

```bash
# 简单上传
owl file upload app.tar.gz --nodes web1,web2 --dest /opt/app/

# 自扩散传输（多节点时自动使用）
owl file transfer app.tar.gz --nodes web1,web2,web3,web4,web5 --dest /opt/app/

# 下载文件
owl file download --nodes web1 --source /var/log/app.log --dest ./logs/
```

### 🤖 5. AI 助手

```bash
# 交互式模式
owl ai

# 单次查询
owl ai "在所有 web 节点上执行 uptime"

# 指定提供商
owl ai --provider openai --model gpt-4o "查看数据库状态"
```

### 🖥️ 6. 交互式会话

```bash
# 单节点实时交互
owl session attach root@192.168.1.10

# 指定 SSH 密钥
owl session attach --key ~/.ssh/id_rsa web1

# 多节点批量管理
owl session attach --nodes web1,web2,web3

# 查看会话历史
owl session history

# 查看特定会话详情
owl session history --session-id sess-abc123

# 列出所有会话
owl session list
```

> 会话功能支持自动检测 `~/.ssh/config`，优先使用密钥认证。

### ⚙️ 7. 设置管理

```bash
# 查看当前设置
owl settings show

# 更新设置
owl settings set ai.provider openai
owl settings set ai.model gpt-4o

# 重置设置
owl settings reset
```

## 🔧 高级配置

### SSH 配置集成

会话功能支持自动读取 `~/.ssh/config`：

```bash
# Host myserver
#     HostName 192.168.1.100
#     User ubuntu
#     IdentityFile ~/.ssh/id_rsa

owl session attach myserver  # 自动使用配置的用户和密钥
```

### 配置文件

配置文件默认位置：`~/.owl/config.yml`

#### AI 配置示例

```yaml
ai:
  provider: openai
  model: gpt-4o
  api-key: your-openai-api-key
  base-url: https://api.openai.com/v1
  timeout: 120
```

**支持的 LLM 提供商：**
- OpenAI (GPT 系列)
- Anthropic (Claude 系列)
- Qwen (阿里通义千问)
- DeepSeek

#### 环境变量配置

```bash
export OWL_API_KEY=your-api-key
export OWL_BASE_URL=https://your-proxy-endpoint
owl ai --provider openai --model gpt-4o
```

## 📚 详细文档

更多详细使用说明请参考：

- [docs/USAGE.md](docs/USAGE.md) - 通用使用指南
- [docs/SESSION_USAGE.md](docs/SESSION_USAGE.md) - 交互式会话功能指南
- [docs/SSH_USAGE.md](docs/SSH_USAGE.md) - SSH 配置和使用说明
- [docs/DATABASE.md](docs/DATABASE.md) - 数据库配置说明
- [docs/implementation_design.md](docs/implementation_design.md) - 架构设计文档

## 💡 架构设计

```
┌─────────────────────────────────────────────────────┐
│                      owl CLI                        │
├─────────────────────────────────────────────────────┤
│  node  │  exec  │  playbook  │  file  │  session  │
├────────┼────────┼────────────┼────────┼───────────┤
│                  SSH Connection Pool                │
├─────────────────────────────────────────────────────┤
│              History Database (DuckDB/SQLite3)     │
└─────────────────────────────────────────────────────┘
```

## 🤝 贡献

欢迎任何形式的贡献！

## 📄 许可证

本项目采用 **MIT License** 开源许可证，详见 [LICENSE](LICENSE) 文件。
