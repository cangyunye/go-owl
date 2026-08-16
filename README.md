# go-owl - 智能分布式运维工具

## 🦉 项目简介

**go-owl** 是一款智能 Linux 分布式运维工具，通过 SSH 对多节点进行批量命令执行、文件传输、剧本编排和 AI 辅助运维。

## ✨ 特性

- 🚀 **节点管理**: 注册、监控和管理多节点，支持分组和标签
- 💻 **批量命令执行**: 在多个节点上同时执行 Shell 命令，支持脚本执行
- 📜 **脚本执行**: 批量上传和执行自定义脚本（本地文件或 URL）
- 📋 **YAML 剧本编排**: 类 Ansible 的 YAML 剧本，支持 `pre_tasks`/`tasks`/`post_tasks`、变量、条件、标签
- 📁 **文件传输**: 支持上传、下载和自扩散传输（P2P 模式）及中继传输
- 🤖 **AI 助手**: 自然语言驱动的智能运维操作（执行命令、生成剧本、传输文件、查询节点）
- 🔒 **安全设计**: 内置危险命令识别（黑名单）和危险操作确认
- 🖥️ **交互式会话**: 支持单节点实时交互和多节点批量管理
- 📊 **会话历史**: 完整的会话和命令记录，可以随时查看
- ⏱️ **异步执行**: 支持异步任务执行与状态查询
- 🔑 **SSH 配置集成**: 自动检测和使用 `~/.ssh/config`
- 📥 **节点导入导出**: 支持 YAML/JSON 格式批量管理节点
- 🌐 **Web 控制台**: 可选的 `owl serve` Web 管理界面（需编译 serve 组件）
- 🖥️ **TUI 界面**: 可选的 `owl tui` 终端界面（需编译 tui 组件）

## 📦 安装

### 从源码构建

默认使用纯 Go 的 SQLite（modernc.org/sqlite）作为历史数据库，无 CGO 依赖，可交叉编译到所有平台：

```bash
git clone https://github.com/cangyunye/go-owl.git
cd go-owl

make build                        # 当前平台 owl CLI
make build WITH=serve,metrics,tui # 追加 Web 控制台 / metrics / TUI 组件
make build PLATFORMS="linux/amd64 windows/amd64"   # 跨平台
make build/all                    # 全平台 × 全组件（含 gscp，需网络）
make install                      # 安装到 ~/.local/bin
make help                         # 查看全部目标
```

## 🎉 快速开始

### 🐣 1. 节点管理

添加第一个节点：

```bash
owl node add web-01 \
  --name "Web Server 1" \
  --address 192.168.1.10 \
  --user root \
  --groups web \
  --label env=prod
```

查看节点列表：

```bash
owl node list
```

检查节点连通性（ping 与 SSH 检查）：

```bash
owl node ping web-01
owl node check --all
```

### ⚡ 2. 批量执行命令

在所有 web 分组节点执行命令：

```bash
owl exec run "uptime" --groups web
```

指定节点执行：

```bash
owl exec run "df -h" --nodes web-01,web-02
```

执行脚本（本地文件或 URL）：

```bash
owl exec script ./deploy.sh --groups web
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

### 📋 4. 剧本编排

仓库内自带示例剧本，位于 `playbooks/`（如 `nginx-deploy.yaml`）：

```bash
# 列出可用剧本
owl playbook list

# 执行剧本（参数为剧本文件路径）
owl playbook run playbooks/nginx-deploy.yaml --nodes web-01

# 传参并检查模式（dry-run）
owl playbook run playbooks/nginx-deploy.yaml --extra-vars nginx_port=8080 --check
```

### 🖥️ 5. 交互式会话

连接节点进行交互操作：

```bash
owl session attach web-01
```

### 🤖 6. AI 助手

```bash
# 交互式对话
owl ai

# 单次查询
owl ai "在所有 web 节点上执行 df -h"

# 查看支持的模型 / 配置
owl ai models
owl ai config show
```

## 📚 命令文档

每个命令都有详细的使用说明：

| 命令 | 文档 | 说明 |
|------|------|------|
| **节点管理** | [NODE.md](docs/user/NODE.md) | 节点的增删改查、分组、标签、导入导出 |
| **命令执行** | [EXEC.md](docs/user/EXEC.md) | 批量命令执行、脚本 |
| **文件传输** | [FILE.md](docs/user/FILE.md) | 上传、下载、扩散传输 |
| **交互会话** | [SESSION.md](docs/user/SESSION.md) | 实时 SSH 会话管理 |
| **剧本管理** | [PLAYBOOK.md](docs/user/PLAYBOOK.md) | YAML 剧本编排 |
| **系统设置** | [SETTINGS.md](docs/user/SETTINGS.md) | 配置管理和目标配置 |
| **AI 助手** | [AI.md](docs/user/AI.md) | 智能运维辅助 |
| **历史记录** | [HISTORY.md](docs/user/HISTORY.md) | 执行历史查看 |
| **TUI 界面** | [TUI.md](docs/user/TUI.md) | `owl tui` 终端界面（Nodes/Exec/File/AI 面板） |

## 🛠️ 使用示例

### 添加多个节点

```bash
# 添加 web 服务器
owl node add web-01 --name web1 --address 192.168.1.10 --user root --groups web
owl node add web-02 --name web2 --address 192.168.1.11 --user root --groups web

# 添加数据库服务器
owl node add db-01 --name db1 --address 192.168.1.20 --user admin --groups db

# 设置标签
owl node labels set web-01 env=prod tier=frontend
```

### 批量运维操作

```bash
# 所有 web 节点执行命令
owl exec run "systemctl status nginx" --groups web

# 按标签筛选
owl exec run "free -h" --label env=prod

# 部署应用
owl file upload ./app.tar.gz --nodes web-01,web-02 --dest /opt/app/
owl exec run "systemctl restart myapp" --nodes web-01,web-02
```

### 异步执行

```bash
# 后台异步执行长任务
owl exec run "long-task.sh" --nodes web-01 --async

# 查看异步任务
owl async list
owl async status <task-id>
owl async wait <task-id>
```

### 剧本编排

```bash
# 列出可用剧本
owl playbook list

# 验证剧本语法
owl playbook validate playbooks/nginx-deploy.yaml

# 执行部署剧本
owl playbook run playbooks/nginx-deploy.yaml --extra-vars nginx_port=8080 --nodes web-01

# 剧本模板管理
owl playbook template list
```

### AI 助手

```bash
# 智能问答
owl ai "如何优化 Nginx 性能"

# 自然语言执行命令
owl ai "在 web 分组上执行 uptime"

# AI 配置
owl ai config setup        # 交互式配置向导
owl ai config show         # 查看当前配置
owl ai models              # 列出支持的大模型
```

## 📂 项目结构

```
go-owl/
├── cmd/cli/             # CLI 入口
│   └── cmd/             # 子命令实现
│       ├── node/       # 节点管理
│       ├── exec/       # 命令/脚本执行
│       ├── file/       # 文件传输
│       ├── session/    # 会话管理
│       ├── playbook/   # 剧本管理
│       ├── settings/   # 设置管理
│       ├── ai/         # AI 助手
│       ├── history/    # 历史记录
│       ├── async/      # 异步执行
│       ├── serve/      # Web 控制台
│       ├── metrics/    # 性能指标
│       └── tui/        # TUI 界面
├── cmd/owl-serve/       # Web 控制台服务端
├── internal/            # 内部包
│   ├── node/           # 节点解析与选择
│   ├── ssh/            # SSH 连接
│   ├── control/        # 控制层（命令/剧本/传输/任务调度）
│   ├── ai/             # AI 模块
│   ├── session/        # 会话管理
│   ├── history/        # 历史记录数据库
│   ├── i18n/           # 国际化（zh-CN / en-US）
│   └── ...
├── playbooks/           # 示例剧本
└── docs/                # 文档
    ├── user/           # 用户文档（命令详解）
    ├── design/         # 设计文档
    ├── dev/            # 开发文档
    └── reference/      # 参考文档
```

## ⚙️ 配置

配置文件位于 `~/.owl/config.yaml`：

```yaml
ai:
  provider: openai      # openai / anthropic / qwen(dashscope) / deepseek
  model: gpt-4o
  api_key: ""           # 留空时使用环境变量 OWL_API_KEY / OPENAI_API_KEY / DASHSCOPE_API_KEY
  base_url: ""          # 可选，自定义 API 地址
  timeout: 120          # 秒

settings:
  output:
    format: table       # table / json / simple
    color: true
  default:
    timeout: 60s
    parallel: true
  target:
    groups: ""          # 默认目标分组（逗号分隔）
    label: ""
    nodes: ""
```

历史数据库默认位于 `~/.owl/owl.db`，可通过环境变量 `OWL_DB_PATH` 覆盖。

`OWL_TUI_THEME` 选择主题：`default` / `catppuccin` / `nord` / `dracula` / `solarized`（默认 `catppuccin`）。
主题在 TrueColor/256/ANSI 色域自动降级，并按终端明暗背景自适应。

## 🚀 发布

推送 `v*` 标签触发 GitHub Actions（`.github/workflows/release.yml`）自动构建并发布：

```bash
git tag v1.2.3
git push origin v1.2.3
```

工作流会构建 `linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64` 五个平台的
`owl`（含 TUI 组件）/ `owl-serve` / `gscp` 二进制，并创建带自动生成 changelog 的 GitHub Release。

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 许可

MIT License
