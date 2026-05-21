# owl history 命令详解

历史记录模块，查看和管理命令执行历史。

---

## 1. 命令列表

```
owl history - 历史记录
├── owl history            - 查看历史
└── owl history clean     - 清理历史
```

---

## 2. owl history

查看命令执行历史记录。

### 使用方法

```bash
owl history
owl history --limit 100
owl history --node-id web-01
owl history --op-type command
owl history --status completed
owl history --last 24h
owl history --last 7d
owl history --start-time 2024-01-01T00:00:00Z
owl history --format json
owl history --output report.json
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `--task-id` | 按任务 ID 筛选 |
| `--node-id` | 按节点 ID 筛选 |
| `--op-type` | 按操作类型筛选 (command, file_transfer, playbook, node_manage) |
| `--status` | 按状态筛选 |
| `--start-time` | 开始时间 (ISO 格式) |
| `--end-time` | 结束时间 (ISO 格式) |
| `--last` | 相对时间 (如 1h, 24h, 7d) |
| `--limit` | 结果数量限制 (默认 50，最大 1000) |
| `--offset` | 偏移量 (分页) |
| `--format` | 输出格式 (table, json, yaml) |
| `--output` | 输出到文件 |
| `--verbose` | 显示详细信息 |

### 示例输出

```
$ owl history --last 24h

TIME                TASK ID                       OP TYPE       TARGETS              STATUS
-----               -------                       --------       -------              ------
2024-01-15 10:30   a1b2c3d4e5f6                 command       [web-01,web-02]     completed
2024-01-15 09:15   b2c3d4e5f6g7                 file_transfer [db-01]             completed
2024-01-15 08:00   c3d4e5f6g7h8                 playbook      [web-01]             failed
```

---

## 3. owl history clean

清理过期的历史记录。

### 使用方法

```bash
owl history clean --days 30
owl history clean --days 7 --force
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `--days` | 保留天数，清理早于此天数的记录 (默认 30) |
| `--force` | 跳过确认，直接清理 |

### 示例

```bash
# 清理 30 天前的历史
owl history clean --days 30

# 强制清理（跳过确认）
owl history clean --days 7 --force
```

---

## 4. 测试用例

### TC-HIST-001: 查看最近历史

```bash
# 步骤
$ owl history --limit 10

# 预期结果
# 显示最近 10 条历史记录
```

### TC-HIST-002: 按节点筛选

```bash
# 步骤
$ owl history --node-id test-01

# 预期结果
# 显示 test-01 节点的历史
```

### TC-HIST-003: 按操作类型筛选

```bash
# 步骤
$ owl history --op-type command

# 预期结果
# 显示所有命令执行历史
```

### TC-HIST-004: JSON 输出

```bash
# 步骤
$ owl history --format json --limit 5

# 预期结果
# JSON 格式输出
```

### TC-HIST-005: 相对时间筛选

```bash
# 步骤
$ owl history --last 24h

# 预期结果
# 显示 24 小时内的历史
```

### TC-HIST-006: 清理历史

```bash
# 步骤
$ owl history clean --days 7 --force

# 预期结果
# 清理 7 天前的历史记录
```

---

## 5. 常见问题

### Q: 历史记录保存在哪里？
A: 默认保存在 `~/.owl/history.db` (SQLite)

### Q: 历史记录保留多久？
A: 默认保留 90 天，可通过 `owl history clean` 清理

### Q: 如何查看某个任务的结果？
A: 使用 `owl history --task-id <id>`

### Q: 可以导出历史记录吗？
A: 可以，使用 `owl history --output file.json --format json`
