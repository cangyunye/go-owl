# owl node 命令详解

节点管理是 owl 的核心功能，用于添加、查看、更新、删除节点。

---

## 1. 命令列表

```
owl node - 节点管理
├── owl node list      - 列出节点
├── owl node add       - 添加节点
├── owl node update    - 更新节点
├── owl node remove    - 删除节点
├── owl node status    - 查看节点状态
├── owl node groups    - 管理分组
├── owl node labels    - 管理标签
├── owl node import    - 导入节点
├── owl node ping      - Ping 节点检查可达性
└── owl node check    - SSH 连接检查并更新状态
```

---

## 2. owl node list

列出所有已注册的节点。

### 使用方法

```bash
owl node list
owl node list --group web
owl node list --label env=prod
owl node list --format json
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `--group` | 按分组筛选节点 |
| `--label` | 按标签筛选节点 |
| `--status` | 按状态筛选（online/offline） |
| `--format` | 输出格式（table/json） |

### 示例输出

```
$ owl node list

  ID       Name       Address          User    Port  Groups   Status
 ──────────────────────────────────────────────────────────────────────
  web-01   web1       192.168.1.10    root    22    web     online
  web-02   web2       192.168.1.11    root    22    web     offline
  db-01    db1        192.168.1.20    admin   22    db      online
```

---

## 3. owl node add

添加新节点。

### 使用方法

```bash
owl node add <node-id> \
  --name <name> \
  --address <address> \
  --port <port> \
  --user <user>
```

### 参数说明

| 参数 | 必填 | 说明 |
|------|------|------|
| `node-id` | ✅ | 节点唯一标识 |
| `--name` | ✅ | 节点名称 |
| `--address` | ✅ | IP 地址或主机名 |
| `--port` | ❌ | SSH 端口，默认 22 |
| `--user` | ❌ | SSH 用户，默认 root |
| `--password` | ❌ | SSH 密码 |
| `--ssh-key` | ❌ | SSH 私钥路径 |
| `--groups` | ❌ | 分组列表（逗号分隔） |
| `--labels` | ❌ | 标签（key=value，可多次使用） |
| `--label` | ❌ | 标签（--labels 的别名） |
| `--proxy-jump` | ❌ | 跳板机 |

### 示例

```bash
# 基本用法
owl node add web-01 --name web1 --address 192.168.1.10 --user root

# 带分组和标签
owl node add web-02 \
  --name web2 \
  --address 192.168.1.11 \
  --user root \
  --groups web,production \
  --labels env=prod \
  --labels appname=owl \
  --labels region=us-east

# 使用 SSH 密钥
owl node add db-01 \
  --name db1 \
  --address 192.168.1.20 \
  --ssh-key ~/.ssh/id_rsa \
  --group db
```

---

## 4. owl node update

更新节点信息。

### 使用方法

```bash
owl node update <node-id> [flags]
```

### 可更新参数

| 参数 | 说明 |
|------|------|
| `--name` | 更新节点名称 |
| `--address` | 更新 IP 地址 |
| `--port` | 更新端口 |
| `--user` | 更新用户 |
| `--password` | 更新密码 |
| `--ssh-key` | 更新 SSH 密钥 |
| `--groups` | 更新分组 |
| `--labels` | 更新标签 |
| `--label` | 更新标签（--labels 的别名） |
| `--status` | 更新状态 |

### 示例

```bash
# 更新节点地址
owl node update web-01 --address 192.168.2.10

# 添加分组
owl node update web-01 --groups web,production

# 添加标签
owl node update web-01 --labels env=staging --labels tier=backend

# 批量更新
owl node update web-01 \
  --name "Web Server 1" \
  --port 2222 \
  --groups production
```

---

## 5. owl node remove

删除节点。

### 使用方法

```bash
owl node remove <node-id>
owl node remove node1 node2 node3
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `node-id` | 要删除的节点 ID（可多个） |

### 示例

```bash
# 删除单个节点
owl node remove web-01

# 删除多个节点
owl node remove web-01 web-02 db-01

# 强制删除（跳过确认）
owl node remove web-01 --force
```

---

## 6. owl node status

查看节点连接状态。

### 使用方法

```bash
owl node status
owl node status --nodes web-01,web-02
```

### 示例输出

```
$ owl node status

  ID       Address          Status    Latency   Last Check
 ─────────────────────────────────────────────────────────────
  web-01   192.168.1.10    online    12ms      2024-01-15 10:30:00
  web-02   192.168.1.11    offline   -         2024-01-15 10:29:45
  db-01    192.168.1.20    online    8ms       2024-01-15 10:30:01
```

---

## 7. owl node groups

管理节点分组。

### 使用方法

```bash
# 列出所有分组
owl node groups list

# 添加节点到分组
owl node groups add <group-name> --nodes node1,node2

# 从分组移除节点
owl node groups remove <group-name> --nodes node1

# 删除分组
owl node groups delete <group-name>
```

### 示例

```bash
# 创建分组并添加节点
owl node groups add web --nodes web-01,web-02

# 查看分组
owl node groups list

# 从分组移除节点
owl node groups remove web --nodes web-02
```

---

## 8. owl node labels

管理节点标签。

### 使用方法

```bash
# 添加标签
owl node labels add <node-id> --labels key1=value1 --labels key2=value2

# 移除标签
owl node labels remove <node-id> --labels key1

# 查看节点标签
owl node labels list <node-id>
```

### 示例

```bash
# 添加多个标签
owl node labels add web-01 --labels env=prod --labels tier=frontend --labels region=us-east

# 移除标签
owl node labels remove web-01 --labels env

# 查看标签
owl node labels list web-01
```

---

## 9. owl node import

从文件导入节点。

### 使用方法

```bash
owl node import <file>
owl node import nodes.yaml --overwrite
```

### 支持格式

**YAML 格式**:
```yaml
nodes:
  - id: web-01
    name: web1
    address: 192.168.1.10
    user: root
    groups: [web]
    labels:
      env: prod
```

**JSON 格式**:
```json
{
  "nodes": [
    {
      "id": "web-01",
      "name": "web1",
      "address": "192.168.1.10",
      "user": "root",
      "groups": ["web"]
    }
  ]
}
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `--overwrite` | 覆盖已存在的节点 |
| `--format` | 文件格式（yaml/json/auto） |

---

## 10. 测试用例

### TC-NODE-001: 添加节点

```bash
# 步骤
$ owl node add test-01 \
  --name "Test Node" \
  --address 127.0.0.1 \
  --user root

# 预期结果
# ✓ 节点添加成功
# $ owl node list | grep test-01
# test-01  Test Node  127.0.0.1  root  22  -  -
```

### TC-NODE-002: 列出节点

```bash
# 步骤
$ owl node list --format json

# 预期结果
# 返回 JSON 格式的节点列表
```

### TC-NODE-003: 更新节点

```bash
# 步骤
$ owl node update test-01 --name "Updated Node"

# 预期结果
# ✓ 节点名称更新成功
```

### TC-NODE-004: 删除节点

```bash
# 步骤
$ owl node remove test-01

# 预期结果
# ✓ 节点删除成功
# $ owl node list | grep test-01
# (无结果)
```

### TC-NODE-005: 分组管理

```bash
# 步骤
$ owl node groups add test-group --nodes test-01
$ owl node list --group test-group

# 预期结果
# 显示 test-group 分组中的节点
```

### TC-NODE-006: 标签管理

```bash
# 步骤
$ owl node labels add test-01 --labels env=dev --labels tier=backend
$ owl node labels list test-01

# 预期结果
# env=dev
# tier=backend
```

### TC-NODE-007: 导入节点

```bash
# 步骤
$ cat > /tmp/nodes.yaml <<EOF
nodes:
  - id: import-01
    name: Import Node
    address: 192.168.1.100
    user: root
EOF
$ owl node import /tmp/nodes.yaml

# 预期结果
# ✓ 导入成功
# $ owl node list | grep import-01
# import-01  Import Node  192.168.1.100  root  22
```

---

## 11. 常见问题

### Q: 节点添加成功但状态显示离线？
A: 检查网络连通性：`ping <node-address>`

### Q: SSH 连接失败？
A: 检查 SSH 端口和认证方式：`ssh -p <port> <user>@<address>`

### Q: 如何使用跳板机？
A: 使用 `--proxy-jump` 参数指定跳板机

---

## 12. owl node ping

通过 ICMP Ping 检查节点的可达性。

### 使用方法

```bash
owl node ping <node-id> [node-id...]
owl node ping --all
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `--all` | Ping 所有节点 |
| `--timeout`, `-t` | Ping 超时时间，默认 3 秒 |

### 示例

```bash
# Ping 单个节点
owl node ping node1

# Ping 多个节点
owl node ping node1 node2 node3

# Ping 所有节点，设置超时为 5 秒
owl node ping --all --timeout 5s
```

### 示例输出

```
开始 Ping 3 个节点 (超时: 3s)...

✓ node1 (192.168.1.10): 可达 - 12ms
✓ node2 (192.168.1.11): 可达 - 15ms
✗ node3 (192.168.1.12): 不可达 - dial tcp 192.168.1.12: i/o timeout

统计: 2 可达, 1 不可达, 总计 3
```

---

## 13. owl node check

通过 SSH 连接测试节点是否可达，并可选择性地更新节点状态。

### 使用方法

```bash
owl node check <node-id> [node-id...]
owl node check --all
owl node check --all --update
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `--all` | 检查所有节点 |
| `--timeout`, `-t` | SSH 连接超时时间，默认 10 秒 |
| `--update`, `-u` | 更新节点状态为 online/offline |
| `--workers`, `-w` | 并发检查的工作协程数，默认 5 |

### 示例

```bash
# 检查单个节点
owl node check node1

# 检查多个节点
owl node check node1 node2 node3

# 检查所有节点并更新状态
owl node check --all --update

# 设置更长的超时时间
owl node check --all --timeout 30s --update
```

### 示例输出

```
开始 SSH 连接检查 3 个节点 (超时: 10s, 并发: 5)...

✓ node1 (192.168.1.10:22): 在线
  → 状态已更新为: online
✗ node2 (192.168.1.11:22): 离线 - dial tcp 192.168.1.11:22: connection refused
  → 状态已更新为: offline
✓ node3 (192.168.1.12:22): 在线
  → 状态已更新为: online

统计: 2 在线, 1 离线, 总计 3
节点状态已保存
```

### 使用场景

1. **批量检查节点状态**：使用 `owl node check --all` 快速检查所有节点
2. **更新节点状态**：使用 `--update` 参数自动更新节点的 online/offline 状态
3. **定时巡检**：配合 cron 定时执行，记录节点可用性
4. **故障排查**：快速定位哪些节点无法连接

---

## 14. 测试用例

### TC-NODE-008: Ping 单个节点

```bash
# 步骤
$ owl node add ping-test --name "Ping Test" --address 8.8.8.8
$ owl node ping ping-test

# 预期结果
# ✓ ping-test (8.8.8.8): 可达 - XXms
```

### TC-NODE-009: Check 并更新状态

```bash
# 步骤
$ owl node check node1 --update
$ owl node status node1

# 预期结果
# 显示 node1 的最新状态
```
