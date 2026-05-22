# owl exec 命令详解

批量命令执行模块，支持在多个节点上同时执行命令。

---

## 1. 命令列表

```
owl exec - 批量命令执行
├── owl exec run      - 执行命令
└── owl exec script  - 执行脚本
```

---

## 2. owl exec run

在指定节点上执行 Shell 命令。

### 使用方法

```bash
owl exec run "<command>"
owl exec run "<command>" --nodes node1,node2
owl exec run "<command>" --group web
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `<command>` | 要执行的命令（必填） |
| `--nodes` | 指定节点 ID（逗号分隔） |
| `--group` | 按分组选择节点 |
| `--label` | 按标签选择节点 |
| `--status` | 按状态选择节点 |
| `--timeout` | 超时时间，默认 60s |
| `--parallel` | 并行执行，默认 true |
| `--async` | 异步执行，不等待结果 |
| `--output` | 输出格式（simple/detail/json） |

### 示例

```bash
# 基本用法（所有节点）
owl exec run "uptime"

# 指定节点
owl exec run "df -h" --nodes web-01,web-02

# 按分组执行
owl exec run "systemctl status nginx" --group web

# 按标签执行
owl exec run "free -h" --label env=prod

# 只执行在线节点
owl exec run "uptime" --status online

# 设置超时
owl exec run "sleep 30" --timeout 10s

# JSON 输出
owl exec run "uptime" --output json

# 详细输出
owl exec run "df -h" --output detail
```

### 示例输出

**simple 格式**:
```
🔧 命令: uptime
🎯 节点: 3 个
⚡ 模式: 并行执行

✅ [web-01] 成功
    10:30:00 up 100 days,  1 user,  load average: 0.15
✅ [web-02] 成功
    10:30:00 up 50 days,   2 users, load average: 0.25
❌ [db-01] 失败: 连接节点失败

📊 总结: 2 成功, 1 失败
```

**detail 格式**:
```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
节点: web-01
状态: ✅ 成功 (exit code: 0)

输出:
 10:30:00 up 100 days,  1 user,  load average: 0.15, 0.20, 0.15
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
节点: web-02
状态: ✅ 成功 (exit code: 0)

输出:
 10:30:00 up 50 days,   2 users, load average: 0.25, 0.30, 0.25
```

**json 格式**:
```json
{"node":"web-01","success":true,"output":"10:30:00 up 100 days,  1 user","exit_code":0}
{"node":"web-02","success":true,"output":"10:30:00 up 50 days,   2 users","exit_code":0}
{"node":"db-01","success":false,"output":"","exit_code":-1}
```

---

## 3. owl exec script

执行本地脚本文件。

支持两种执行方式：
- **默认方式**：先上传脚本到远程节点，赋予执行权限，再执行
- **inline 方式**：直接发送脚本内容给远程执行，不留文件痕迹

### 使用方法

```bash
owl exec script <script-file>
owl exec script ./deploy.sh --nodes web-01,web-02
owl exec script ./init.sh --nodes web-01 --inline
owl exec script ./setup.sh --nodes web-01 --keep
owl exec script ./config.sh --nodes web-01 --args "--env prod"
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `<script-file>` | 本地脚本文件路径（未来支持 URL 脚本） |
| `--nodes` | 指定节点 ID（逗号分隔） |
| `--group` | 按分组选择节点 |
| `--label` | 按标签选择节点 |
| `--dest` | 远程存放目录（默认 /tmp） |
| `--args` | 传递给脚本的参数 |
| `--timeout` | 执行超时时间（默认 5m） |
| `--inline` | 直接发送内容执行，不留脚本文件（安全模式） |
| `--keep` | 执行后保留远程脚本文件（调试模式） |

### 示例

```bash
# 基本用法 - 上传并执行（默认）
owl exec script ./deploy.sh --nodes web-01

# 传递参数
owl exec script ./deploy.sh --nodes web-01 --args "--version 1.0.0 --env prod"

# 安全执行 - 不留文件
owl exec script ./init.sh --nodes web-01 --inline

# 调试模式 - 保留脚本文件
owl exec script ./setup.sh --nodes web-01 --keep

# 自定义存放目录
owl exec script ./deploy.sh --nodes web-01 --dest /home/user/
```

### 执行方式对比

| 方式 | 特点 | 适用场景 |
|------|------|---------|
| **默认（上传+执行）** | 脚本文件保留在远程<br>支持脚本引用同目录文件<br>便于调试和复现 | 标准部署脚本<br>复杂任务<br>需要调试的场景 |
| **inline 方式** | 脚本内容不保留<br>更安全<br>无法引用同目录文件 | 快速检查<br>安全检查<br>包含敏感信息的脚本 |

### 示例输出

```
📜 脚本: ./deploy.sh
🎯 目标节点: 2 个
🚀 执行方式: 文件传输 + 执行
📂 存放目录: /tmp

⏳ 开始执行...
✅ [web-01] 成功
    输出:
      Deploying...
      Done!
✅ [web-02] 成功
    输出:
      Deploying...
      Done!

📊 总结: 2 成功, 0 失败
```

---

## 5. 测试用例

### TC-EXEC-001: 单节点命令执行

```bash
# 步骤
$ owl node add test-01 --name test --address 127.0.0.1 --user root
$ owl exec run "echo hello" --nodes test-01

# 预期结果
# ✅ [test-01] 成功
#     hello
```

### TC-EXEC-002: 多节点并行执行

```bash
# 步骤
$ owl node add test-02 --name test2 --address 127.0.0.1 --user root
$ owl exec run "hostname" --nodes test-01,test-02 --parallel

# 预期结果
# ✅ [test-01] 成功
# ✅ [test-02] 成功
# 📊 总结: 2 成功, 0 失败
```

### TC-EXEC-003: 分组执行

```bash
# 步骤
$ owl node groups add test-group --nodes test-01,test-02
$ owl exec run "whoami" --group test-group

# 预期结果
# ✅ [test-01] 成功
# ✅ [test-02] 成功
```

### TC-EXEC-004: 命令超时

```bash
# 步骤
$ owl exec run "sleep 10" --nodes test-01 --timeout 2s

# 预期结果
# ❌ [test-01] 失败: command timed out after 2s
```

### TC-EXEC-005: JSON 输出格式

```bash
# 步骤
$ owl exec run "uptime" --nodes test-01 --output json

# 预期结果
# {"node":"test-01","success":true,"output":"...","exit_code":0}
```

### TC-EXEC-006: 错误处理

```bash
# 步骤
$ owl exec run "ls /nonexistent" --nodes test-01

# 预期结果
# ❌ [test-01] 失败
# 📊 总结: 0 成功, 1 失败
# exit code: 1
```

### TC-EXEC-007: 异步执行

```bash
# 步骤
$ owl exec run "sleep 5 && echo done" --nodes test-01 --async

# 预期结果
# 🔧 命令: sleep 5 && echo done
# 🎯 节点: 1 个
# ⚡ 模式: 异步执行
# (立即返回，不等待结果)
```

---

## 6. 常见问题

### Q: 命令执行失败如何排查？
A: 使用 `--output detail` 查看详细错误信息

### Q: 如何传递复杂命令？
A: 使用引号包裹：`owl exec run "cd /tmp && ./script.sh"`

### Q: 节点连接失败会怎样？
A: 该节点标记为失败，继续执行其他节点，最终显示失败统计

### Q: 如何查看命令历史？
A: 使用 `owl history` 查看执行历史
