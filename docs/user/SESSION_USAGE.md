# go-owl 会话功能说明文档

## 概述

go-owl 提供了交互式会话功能，支持在多个 Linux 节点上执行命令并查看结果。主要支持：
- **单节点实时交互模式** - 针对单个节点的实时 SSH 会话
- **多节点批量模式** - 针对多个节点的批量命令执行，输出汇总结果
- **会话历史记录** - 保存会话和命令记录，可以随时查看

## 快速开始

### 基础使用

```bash
# 单节点连接
owl session attach root@192.168.1.10

# 多节点连接
owl session attach --nodes web1,web2,web3

# 指定 SSH 密钥
owl session attach --key ~/.ssh/id_rsa web1
```

### 查看会话历史

```bash
# 查看最近 20 个会话
owl session history

# 查看指定会话的详情
owl session history --session-id sess-1234567890
```

## 功能详细说明

### 1. 数据库支持

项目支持 **DuckDB** 和 **SQLite3** 两种数据库，可以通过构建标签进行选择：

```bash
# 默认使用 DuckDB 构建
go build -o owl ./cmd/cli/main.go

# 使用 SQLite3 构建
go build -tags sqlite3 -o owl ./cmd/cli/main.go
```

**数据库表结构**:
- `sessions` 表 - 保存会话基本信息
- `session_commands` 表 - 保存会话中的命令记录
- 以及其他原有历史记录表

### 2. SSH 配置集成

会话功能支持自动检测和使用 `~/.ssh/config` 的配置：

```bash
# 使用默认 SSH config
owl session attach web-server

# 显式指定 SSH config
owl session attach --ssh-config ~/.ssh/alternate_config web-server
```

系统会优先按照以下顺序尝试 SSH 认证:
1. SSH config 中指定的密钥文件
2. 命令行提供的 `--key` 参数
3. 默认用户目录下的私钥 (id_rsa, id_ed25519, id_ecdsa)
4. SSH Agent

### 3. 单节点实时交互

**单节点连接:**

```bash
owl session attach root@192.168.1.10
```

连接后会进入交互式终端:
```
[owl] connected to web1 (single-node mode)
(owl-sess-1234) > help
  help        显示帮助信息
  nodes       列出连接的节点
  history     显示命令历史
  clear       清屏
  exit        退出会话
(owl-sess-1234) > uptime
 14:30:25 up 25 days,  3:15,  0 users,  load average: 0.12, 0.07, 0.01
(owl-sess-1234) > 
```

### 4. 多节点批量模式

**多节点连接:**

```bash
owl session attach --nodes web1,web2,web3
```

连接后进入多节点批量执行模式：
```
[owl] connected to 3 nodes (multi-node mode)
  - web1 (192.168.1.10)
  - web2 (192.168.1.11)
  - web3 (192.168.1.12)
(owl-sess-456) > uptime
┌────────┬────────┬────────┬──────────┐
│ 节点   │ 返回码│  状态  │ 耗时(ms)│
├────────┼────────┼────────┼──────────┤
│ web1   │  0     │   ✓    │      120│
│ web2   │  0     │   ✓    │      118│
│ web3   │  0     │   ✓    │      132│
└────────┴────────┴────────┴──────────┘

执行汇总:
  目标节点: 3 个
  成功:     3 个
  失败:     0 个
  平均耗时: 123.00 ms
```

多节点特点:
- **并行执行** - 命令会同时在所有节点执行
- **表格化输出** - 清晰的执行状态和结果
- **执行汇总** - 显示成功/失败统计和平均耗时

### 5. 会话超时与心跳

- 默认超时: 30 分钟
- 可使用 `--timeout` 参数设置
```bash
owl session attach --timeout 1h web1
```

- 当空闲超时后，会自动发送心跳确认连接状态
- 如果心跳失败，会话会被安全关闭

### 6. 会话历史查询

**查询最近的会话:**

```bash
owl session history --limit 10
```

输出:
```
最近的会话:
─────────────────────────────────────────────────────────────────────────────
● sess-abc123 | 05-13 14:30 | web1,web2,web3 | 100% | 5 cmd
○ sess-def456 | 05-13 12:15 | db1           | 80%  | 10 cmd
```

**查询特定会话详情:**

```bash
owl session history --session-id sess-abc123
```

输出:
```
─────────────────────────────────────
会话 ID:    sess-abc123
模式:      multi-node
状态:      closed
创建时间:  2026-05-13 14:30:25
关闭时间:  2026-05-13 14:35:40
节点:      web1, web2, web3
─────────────────────────────────────
命令数:    5
成功:      5
失败:      0
─────────────────────────────────────

命令历史:
─────────────────────────────────────────────────────────────────────────────
[1] ✓ 14:30:40 uptime
[2] ✓ 14:30:45 df -h
[3] ✓ 14:30:50 free -h
```

## 节点管理

### 添加节点

```bash
owl node add --name web1 --address 192.168.1.10 --port 22 --user root
```

添加节点时支持:
- 指定用户名 (`--user`)
- 指定 SSH 端口 (`--port`)
- 添加分组标签 (`--group`, `--label`)

### 节点列表

```bash
owl node list
```

## 命令参考

### session attach

```
Usage:
  owl session attach [node-id] [flags]

Flags:
  -h, --help                help for attach
      --key string          SSH 私钥文件路径
      --nodes string        多节点模式，指定节点列表（逗号分隔）
      --ssh-config string   SSH config 路径（默认: ~/.ssh/config）
      --timeout string      会话超时时间（如: 30m, 1h） (default "30m")
```

### session history

```
Usage:
  owl session history [session-id] [flags]

Flags:
  -h, --help             help for history
      --session-id string   指定会话 ID
      --node string        按节点筛选
      --last string        查看最近时间（如: 1h, 30m, 1d）
  -v, --verbose           显示详细输出
  -n, --limit int        显示最近 N 条记录 (default 20)
```

## 文件修改列表

主要修改和新增文件：

- `internal/history/db_duckdb.go` - 增加会话表
- `internal/history/db_sqlite3.go` - 增加会话表
- `internal/history/session.go` - 会话数据操作
- `internal/history/interface.go` - DB接口定义
- `internal/session/manager.go` - 会话管理器（并发执行）
- `internal/session/interactive.go` - 交互式会话逻辑
- `cmd/cli/cmd/session/attach.go` - attach命令（SSH配置集成）
- `cmd/cli/cmd/session/history.go` - history命令
- `cmd/cli/cmd/root.go` - 初始化数据库

## 架构图

```
┌─────────────────────────┐
│   owl CLI (root)        │
└──────────────┬──────────┘
               │
┌─────────────────────────┐
│  session attach/history │
└──────────────┬──────────┘
               │
┌─────────────────────────┐     ┌─────────────────┐
│  Session Manager        │─────│ Connection Pool │
└──────────────┬──────────┘     └─────────────────┘
               │
┌─────────────────────────┐
│  Interactive Loop       │
└──────────────┬──────────┘
               │
┌─────────────────────────┐
│  History (DB)           │
└─────────────────────────┘
```

## 注意事项

- 会话超时后不要关闭终端，系统会自动进行心跳检测
- 多节点模式下，单个命令失败不影响其他节点执行
- 所有命令执行会被记录，可以通过 history 命令查询
- 支持 Ctrl+C 安全退出会话
