# go-owl 文档目录

本文档目录包含 go-owl 项目的所有设计文档和使用指南，按受众分类。

---

## 👤 用户文档

面向终端用户的使用指南。

| 文档 | 说明 |
|------|------|
| [user/QUICKSTART.md](user/QUICKSTART.md) | 快速入门指南 |
| [user/README.md](user/README.md) | 用户文档索引 |
| [user/USAGE.md](user/USAGE.md) | AI 助手使用指南 |

### 命令详解

| 文档 | 说明 |
|------|------|
| [user/NODE.md](user/NODE.md) | 节点管理（添加、查看、更新、删除、ping、check） |
| [user/EXEC.md](user/EXEC.md) | 批量命令执行 |
| [user/PLAYBOOK.md](user/PLAYBOOK.md) | Playbook 剧本管理 |
| [user/FILE.md](user/FILE.md) | 文件传输（上传、下载、扩散传输） |
| [user/SESSION.md](user/SESSION.md) | 交互式会话管理 |
| [user/SESSION_USAGE.md](user/SESSION_USAGE.md) | 会话功能详细使用说明 |
| [user/AI.md](user/AI.md) | AI 助手 |
| [user/HISTORY.md](user/HISTORY.md) | 执行历史记录 |
| [user/SETTINGS.md](user/SETTINGS.md) | 系统设置 |

---

## 👨‍💻 开发文档

面向开发者的架构设计和实现文档。

| 文档 | 说明 |
|------|------|
| [dev/README.md](dev/README.md) | 开发文档索引 |
| [dev/ARCHITECTURE.md](dev/ARCHITECTURE.md) | 整体架构设计 |
| [dev/SESSION_DESIGN.md](dev/SESSION_DESIGN.md) | 会话功能设计 |
| [dev/FILE_TRANSFER_ARCHITECTURE.md](dev/FILE_TRANSFER_ARCHITECTURE.md) | 文件传输架构 |
| [dev/SSH_CONFIG.md](dev/SSH_CONFIG.md) | SSH 配置解析设计 |
| [dev/AI_CONFIG_DESIGN.md](dev/AI_CONFIG_DESIGN.md) | AI 配置方案 |
| [dev/TEST_IMPLEMENTATION_REPORT.md](dev/TEST_IMPLEMENTATION_REPORT.md) | 测试用例报告 |

### 已完成的修复记录

| 文档 | 说明 | 状态 |
|------|------|------|
| [dev/FIX_PASSWORD_AUTH_CHAIN.md](dev/FIX_PASSWORD_AUTH_CHAIN.md) | 密钥→密码认证回退链路修复 | ✅ 已实现 |
| [dev/FIX_SSH_ERROR_INFO_LOSS.md](dev/FIX_SSH_ERROR_INFO_LOSS.md) | SSH 退出码错误信息丢失修复 | ✅ 已通过架构重构解决 |

---

## 🔧 高可靠执行设计

| 文档 | 说明 | 状态 |
|------|------|------|
| [design/01_TIMEOUT_SEPARATION.md](design/01_TIMEOUT_SEPARATION.md) | 连接超时与命令超时分离 | ✅ 已实现 |
| [design/02_RETRY_MECHANISM.md](design/02_RETRY_MECHANISM.md) | 命令重试机制 | ✅ 已实现 |
| [design/03_ASYNC_EXECUTION.md](design/03_ASYNC_EXECUTION.md) | 异步执行模式 | ⚠️ 部分实现 |
| [design/PLAYBOOK_ACTION_OPTIONS.md](design/PLAYBOOK_ACTION_OPTIONS.md) | Playbook Action 超时重试配置 | ✅ 已实现 |
| [design/PLAYBOOK_TEMPLATE_SYSTEM.md](design/PLAYBOOK_TEMPLATE_SYSTEM.md) | Playbook 模板系统方案 | ⚠️ 未实现 |

---

## 📖 参考文档

配置和技术参考文档。

| 文档 | 说明 |
|------|------|
| [reference/README.md](reference/README.md) | 参考文档索引 |
| [reference/SSH_USAGE.md](reference/SSH_USAGE.md) | SSH 配置和使用 |
| [reference/DATABASE.md](reference/DATABASE.md) | 数据库配置 |
| [reference/API_NODE_SOURCE.md](reference/API_NODE_SOURCE.md) | API 节点源集成 |

---

## 📂 目录结构

```
docs/
├── README.md           ← 本文档
├── user/               ← 用户文档
│   ├── README.md
│   ├── QUICKSTART.md   ← 快速入门
│   ├── USAGE.md
│   ├── NODE.md
│   ├── EXEC.md
│   ├── PLAYBOOK.md
│   ├── FILE.md
│   ├── SESSION.md
│   ├── SESSION_USAGE.md
│   ├── AI.md
│   ├── HISTORY.md
│   └── SETTINGS.md
├── dev/                ← 开发文档
│   ├── README.md
│   ├── ARCHITECTURE.md
│   ├── SESSION_DESIGN.md
│   ├── FILE_TRANSFER_ARCHITECTURE.md
│   ├── SSH_CONFIG.md
│   ├── AI_CONFIG_DESIGN.md
│   ├── TEST_IMPLEMENTATION_REPORT.md
│   ├── FIX_PASSWORD_AUTH_CHAIN.md
│   └── FIX_SSH_ERROR_INFO_LOSS.md
├── design/             ← 高可靠设计
│   ├── 01_TIMEOUT_SEPARATION.md
│   ├── 02_RETRY_MECHANISM.md
│   ├── 03_ASYNC_EXECUTION.md
│   ├── PLAYBOOK_ACTION_OPTIONS.md
│   └── PLAYBOOK_TEMPLATE_SYSTEM.md
└── reference/          ← 参考文档
    ├── README.md
    ├── SSH_USAGE.md
    ├── DATABASE.md
    └── API_NODE_SOURCE.md
```

---

## 🚀 快速导航

### 新手入门
1. 阅读 [user/QUICKSTART.md](user/QUICKSTART.md)
2. 添加第一个节点
3. 执行第一条命令

### 节点管理
1. 使用 `owl node add` 添加节点
2. 使用 `owl node list` 查看节点
3. 使用 `owl node ping` 检查可达性
4. 使用 `owl node check` 更新状态

### 命令执行
1. 使用 `owl exec run` 执行命令
2. 使用 `owl playbook run` 执行剧本
3. 查看 `owl exec` 参数

### 文件传输
1. 使用 `owl file upload` 上传文件
2. 使用 `owl file download` 下载文件

### AI 助手
1. 配置 AI Provider
2. 使用 `owl ai` 对话

### 开发者
1. 阅读 [dev/ARCHITECTURE.md](dev/ARCHITECTURE.md)
2. 了解高可靠设计文档
3. 查看 [测试报告](dev/TEST_IMPLEMENTATION_REPORT.md)
