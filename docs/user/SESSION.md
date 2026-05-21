# owl session 命令详解

交互式会话模块，提供实时 SSH 会话管理。

---

## 1. 命令列表

```
owl session - 交互式会话
├── owl session attach - 连接会话
├── owl session list   - 列出会话
└── owl session history - 会话历史
```

---

## 2. owl session attach

连接到节点并进入交互式会话。

### 使用方法

```bash
owl session attach <node-id>
owl session attach node1 node2 node3
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `<node-id>` | 节点 ID（可多个） |
| `--mode` | 会话模式（single/multi） |

### 会话模式

| 模式 | 说明 |
|------|------|
| `single` | 单会话模式 |
| `multi` | 多会话模式（分屏） |

### 示例

```bash
# 单节点会话
owl session attach web-01

# 多节点会话（分屏）
owl session attach web-01 web-02 web-03 --mode multi
```

### 会话内命令

在交互式会话中，输入以 `/` 开头的命令为本地程序命令：

| 命令 | 说明 |
|------|------|
| `/help` | 显示帮助 |
| `/exit` | 退出会话 |
| `/status` | 显示连接状态 |
| `/clear` | 清屏 |
| `/broadcast` | 广播模式 |
| `/history` | 命令历史 |

其他命令直接发送到远程节点执行。

### 示例输出

```
─────────────────────────────────────
已连接到 1 个节点
会话 ID: sess-abc123
─────────────────────────────────────

📌 程序命令（以 / 开头）:
  /help     - 显示帮助
  /exit     - 退出会话
  /status   - 显示状态
  /clear    - 清屏
  /broadcast - 广播模式

💡 提示: 以 / 开头的命令在本地执行
        其他命令发送到 SSH 会话执行

[user@web-01 ~]$ uptime
 10:30:00 up 100 days,  1 user,  load average: 0.15, 0.20, 0.15

[user@web-01 ~]$ /exit
```

---

## 3. owl session list

列出当前活动会话。

### 使用方法

```bash
owl session list
```

### 示例输出

```
  会话 ID        节点数  模式    状态    创建时间
 ─────────────────────────────────────────────────────────
  sess-abc123    3       multi   active  2024-01-15 10:30:00
  sess-def456    1       single  active  2024-01-15 09:15:00
```

---

## 4. owl session history

查看会话历史记录。

### 使用方法

```bash
owl session history
owl session history <session-id>
owl session history --node web-01
owl session history --last 50
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `<session-id>` | 会话 ID |
| `--node` | 按节点筛选 |
| `--last` | 最近 N 条 |
| `--verbose` | 显示详细信息 |

### 示例输出

```
会话历史 - web-01
─────────────────────────────────────

会话 ID: sess-abc123
开始时间: 2024-01-15 10:00:00
结束时间: 2024-01-15 10:30:00
命令数:   15
状态:     正常结束

最近命令:
  10:15  uptime
  10:18  cd /opt
  10:20  ls -la
  10:25  systemctl restart nginx
  10:30  exit
```

---

## 5. 测试用例

### TC-SESSION-001: 单节点会话

```bash
# 步骤
$ owl session attach test-01
# 在会话中输入: uptime
# 输入: /exit

# 预期结果
# 进入交互式会话
# 命令正常执行
# /exit 退出到命令行
```

### TC-SESSION-002: 会话内帮助

```bash
# 步骤
$ owl session attach test-01
# 输入: /help
# 输入: /exit

# 预期结果
# 显示帮助信息
# 包含程序命令说明
```

### TC-SESSION-003: 会话历史

```bash
# 步骤
$ owl session attach test-01
# 输入: uptime
# 输入: df -h
# 输入: /exit
$ owl session history

# 预期结果
# 显示会话历史
# 包含执行的命令和输出
```

### TC-SESSION-004: 多节点会话

```bash
# 步骤
$ owl session attach test-01 test-02 --mode multi
# 输入: /broadcast
# 输入: uptime
# 输入: /exit

# 预期结果
# 分屏显示多个终端
# /broadcast 切换广播模式
```

---

## 6. 常见问题

### Q: 如何退出会话？
A: 输入 `/exit` 或按 `Ctrl+C`

### Q: 多会话模式如何使用？
A: 使用 Tab 切换不同节点的终端，使用 `/broadcast` 发送命令到所有节点

### Q: 会话中断怎么办？
A: 使用 `owl session list` 查看状态，断开的会话会自动重连

### Q: 可以复用会话吗？
A: CLI 模式下每次命令都是新的连接，如需复用请保持会话不退出
