# 交互式会话管理方案

## 1. 需求概述

实现持久 SSH 会话管理功能，支持单节点实时交互和多节点批量管理，所有操作记录到日志和数据库。

### 1.1 核心特性

- **会话保持**: 维护持久 SSH 连接，避免频繁建立连接
- **命令补全**: 支持 Tab 键命令补全和历史命令
- **超时机制**: 30 分钟无操作自动检测连接状态
- **结果记录**: 所有命令记录到数据库和日志
- **优雅退出**: Ctrl+C 安全关闭会话

---

## 2. 功能设计

### 2.1 会话类型

#### 单节点会话 (Single Mode)
- 保持实时交互
- 显示完整命令输出
- 支持命令补全和历史

#### 多节点会话 (Multiple Mode)
- 单一交互窗口
- 表格形式显示状态汇总
- 详细结果异步记录

### 2.2 超时机制

```bash
# 进入会话时设置环境变量
export TMOUT=1800  # 30 分钟

# 30 分钟无命令时，自动发送
echo 1

# 如果返回非 0 或连接异常，自动退出
# 提示: "会话超时，连接已断开"
```

### 2.3 命令补全

支持以下补全：

| 类型 | 补全内容 |
|-----|---------|
| 内置命令 | `exit`, `help`, `clear`, `history`, `nodes` |
| 已执行命令 | 会话中的历史命令 |
| 系统命令 | `$PATH` 中的命令 |

### 2.4 历史命令

```bash
# 上下箭头切换历史命令
(up arrow)   - 显示上一条命令
(down arrow) - 显示下一条命令

# 查看历史列表
> history
  1  uptime
  2  df -h
  3  systemctl restart nginx

# 执行历史命令
> !2    # 执行第 2 条 (df -h)
> !uptime  # 执行最后一条以 uptime 开头的命令
```

---

## 3. 交互流程

### 3.1 单节点会话

```bash
$ owl session attach web1

已连接到 web1 (192.168.1.10)
会话超时: 30 分钟
─────────────────────────────────────

(web1) > uptime
 00:00:00 up 1 day,  2:30,  1 user,  load average: 0.15, 0.10, 0.05

(web1) > df -h
  Filesystem      Size  Used Avail Use% Mounted on
  /dev/sda1       50G   20G   30G  40% /

(web1) > systemctl status nginx
  ● nginx.service - A high performance web server
     Loaded: loaded (/lib/systemd/system/nginx.service)
     Active: active (running)

(web1) >

# Ctrl+C 或输入 exit 优雅退出
(web1) > exit
正在关闭会话...
─────────────────────────────────────
会话摘要:
  会话时长: 05:23
  执行命令: 12
  成功率:  100% (12/12)
─────────────────────────────────────
✓ 会话已关闭
```

### 3.2 多节点会话

```bash
$ owl session attach --nodes web1,web2,web3

已连接到 3 个节点
会话超时: 30 分钟
─────────────────────────────────────

(multi) > uptime
┌────────┬─────────┬────────┐
│  节点  │ 返回码  │  状态  │
├────────┼─────────┼────────┤
│ web1   │   00    │   ✓    │
│ web2   │   00    │   ✓    │
│ web3   │   01    │   ✗    │
└────────┴─────────┴────────┘

(multi) > df -h
┌────────┬─────────┬────────┐
│  节点  │ 返回码  │  状态  │
├────────┼─────────┼────────┤
│ web1   │   00    │   ✓    │
│ web2   │   00    │   ✓    │
│ web3   │   00    │   ✓    │
└────────┴─────────┴────────┘

(multi) > systemctl restart nginx
┌────────┬─────────┬────────┐
│  节点  │ 返回码  │  状态  │
├────────┼─────────┼────────┤
│ web1   │   00    │   ✓    │
│ web2   │   00    │   ✓    │
│ web3   │   01    │   ✗    │
└────────┴─────────┴────────┘
错误详情请查看: owl history --session-id sess-xxx

(multi) >

# Ctrl+C 或输入 exit 优雅退出
(multi) > exit
正在关闭会话...
─────────────────────────────────────
会话摘要:
  会话时长: 15:42
  执行命令: 8
  成功率:  87.5% (7/8)
─────────────────────────────────────
✓ 会话已关闭
```

---

## 4. 数据记录

### 4.1 数据库表

```sql
-- 会话表
CREATE TABLE sessions (
    id VARCHAR PRIMARY KEY,
    mode VARCHAR,              -- 'single' 或 'multiple'
    node_ids JSON,             -- 节点 ID 列表
    status VARCHAR,            -- 'active', 'closed', 'timeout'
    created_at TIMESTAMP,
    closed_at TIMESTAMP,
    command_count INTEGER,     -- 执行命令总数
    success_count INTEGER,     -- 成功命令数
    error_count INTEGER        -- 失败命令数
);

-- 会话命令表
CREATE TABLE session_commands (
    id BIGINT PRIMARY KEY AUTOINCREMENT,
    session_id VARCHAR,
    command VARCHAR,
    targets JSON,              -- 目标节点列表
    results JSON,              -- 各节点结果
    executed_at TIMESTAMP,
    FOREIGN KEY (session_id) REFERENCES sessions(id)
);
```

### 4.2 日志格式

```json
{
  "level": "info",
  "type": "session_command",
  "session_id": "sess-20260101-001",
  "timestamp": "2026-05-13T10:00:00Z",
  "command": "uptime",
  "targets": ["web1", "web2", "web3"],
  "results": [
    {
      "node": "web1",
      "exit_code": 0,
      "stdout": "00:00:00 up 1 day",
      "duration_ms": 120
    },
    {
      "node": "web2",
      "exit_code": 0,
      "stdout": "00:00:00 up 2 days",
      "duration_ms": 115
    }
  ]
}
```

---

## 5. CLI 命令

```bash
# 进入会话
owl session attach <node-id>                    # 单节点
owl session attach --nodes web1,web2,web3      # 多节点

# 会话管理
owl session list                                # 列出活动会话
owl session history [session-id]               # 查看会话历史
owl session history --node web1                 # 按节点筛选
owl session history --last 1h                   # 最近 1 小时

# 内置命令 (在会话中)
help       - 显示帮助
history    - 显示命令历史
nodes      - 显示连接的节点
clear      - 清屏
exit/quit  - 优雅退出会话
```

---

## 6. 技术实现

### 6.1 核心组件

| 组件 | 文件 | 职责 |
|-----|-----|-----|
| 会话管理器 | `internal/session/manager.go` | 会话生命周期管理 |
| SSH 连接池 | `internal/session/connection.go` | SSH 连接维护 |
| 命令解析器 | `internal/session/parser.go` | 命令解析和补全 |
| 历史管理器 | `internal/session/history.go` | 命令历史记录 |
| 定时器 | `internal/session/timeout.go` | 超时检测 |

### 6.2 会话状态机

```
创建 ──→ 连接中 ──→ 已连接 ──→ 执行命令 ──→ 等待输入
  │         │           │           │            │
  │         │           │           │            ↓
  │         │           │           └─────→ 超时检测 ──→ 超时退出
  │         │           │
  │         │           └─────→ Ctrl+C/Exit ──→ 关闭中 ──→ 已关闭
  │         │
  └─────→ 连接失败 ──→ 错误退出
```

### 6.3 命令补全实现

```go
type CommandCompleter struct {
    history    []string
    builtins   []string
    systemCmds []string
}

func (c *CommandCompleter) Complete(input string) []string {
    // 1. 检查是否为内置命令
    // 2. 检查历史命令前缀匹配
    // 3. 检查系统命令路径
    // 4. 返回匹配结果
}
```

---

## 7. 实现计划

### 阶段 1: 核心框架
- 会话管理器
- SSH 连接池
- 基础交互循环

### 阶段 2: 交互增强
- 命令补全
- 历史命令
- 超时检测

### 阶段 3: 多节点支持
- 并发执行
- 结果汇总
- 表格输出

### 阶段 4: 数据持久化
- 数据库记录
- 日志集成
- 历史查询

---

## 8. 配置项

```yaml
session:
  timeout: 30m          # 会话超时时间
  keepalive: 60s        # SSH 保活间隔
  max_history: 100      # 最大历史记录数
  auto_reconnect: true  # 断线自动重连
```

---

## 9. 注意事项

1. **安全性**: 所有 SSH 连接使用密钥认证，不支持密码交互
2. **资源限制**: 单会话最多 50 个节点
3. **输出限制**: 单命令输出最大 1MB，超出截断
4. **日志保留**: 会话日志默认保留 30 天
