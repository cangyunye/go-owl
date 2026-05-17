# owl history 命令详解

历史记录模块，查看和管理命令执行历史。

---

## 1. 命令列表

```
owl history - 历史记录
├── owl history          - 查看历史
├── owl history exec     - 命令执行历史
└── owl history session  - 会话历史
```

---

## 2. owl history

查看命令执行历史。

### 使用方法

```bash
owl history
owl history --limit 100
owl history --node web-01
owl history --since "2024-01-01"
owl history --format json
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `--limit` | 限制显示条数 |
| `--node` | 按节点筛选 |
| `--command` | 按命令筛选 |
| `--status` | 按状态筛选（success/failed） |
| `--since` | 起始时间 |
| `--until` | 结束时间 |
| `--format` | 输出格式（table/json） |

### 示例输出

```
  时间                节点      命令                状态    耗时   用户
 ──────────────────────────────────────────────────────────────────────────────
  2024-01-15 10:30  web-01    uptime              成功    0.5s   root
  2024-01-15 10:28  web-02    df -h              成功    0.8s   root
  2024-01-15 10:25  web-01    systemctl restart   成功    2.1s   root
  2024-01-15 10:20  db-01     mysqldump ...      成功    15.3s  root
  2024-01-15 10:15  web-01    git pull            失败    1.2s   root
```

---

## 3. owl history exec

查看命令执行详细历史。

### 使用方法

```bash
owl history exec
owl history exec --id <history-id>
owl history exec --last 10
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `--id` | 历史记录 ID |
| `--last` | 最近 N 条 |
| `--node` | 按节点筛选 |
| `--output` | 显示完整输出 |

### 示例输出

```
$ owl history exec --id abc123

历史 ID: abc123
─────────────────────────────────────

节点:      web-01
命令:      df -h
状态:      成功
耗时:      0.8s
执行时间:  2024-01-15 10:28:00
用户:      root

输出:
Filesystem      Size  Used Avail Use% Mounted on
/dev/sda1       50G   20G   30G  40% /
/dev/sdb1      100G   60G   40G  60% /data
tmpfs           16G     0   16G   0% /dev/shm
```

---

## 4. owl history session

查看会话历史。

### 使用方法

```bash
owl history session
owl history session --node web-01
owl history session --limit 50
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `--node` | 按节点筛选 |
| `--session-id` | 会话 ID |
| `--limit` | 限制条数 |

### 示例输出

```
$ owl history session --node web-01

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

## 5. 导出历史

### 使用方法

```bash
owl history export --format csv --output history.csv
owl history export --since "2024-01-01" --until "2024-01-31"
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `--format` | 导出格式（csv/json） |
| `--output` | 输出文件 |
| `--since` | 起始时间 |
| `--until` | 结束时间 |

---

## 6. 清理历史

### 使用方法

```bash
# 清理 30 天前的历史
owl history clean --days 30

# 清理指定节点的历史
owl history clean --node web-01

# 清理所有历史
owl history clean --all
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `--days` | 保留天数 |
| `--node` | 清理指定节点 |
| `--all` | 清理所有 |

---

## 7. 测试用例

### TC-HIST-001: 查看历史

```bash
# 步骤
$ owl history --limit 10

# 预期结果
# 显示最近 10 条历史记录
```

### TC-HIST-002: 按节点筛选

```bash
# 步骤
$ owl history --node test-01

# 预期结果
# 显示 test-01 节点的历史
```

### TC-HIST-003: JSON 输出

```bash
# 步骤
$ owl history --format json --limit 5

# 预期结果
# JSON 格式输出
```

### TC-HIST-004: 查看详情

```bash
# 步骤
$ owl history exec --last 1

# 预期结果
# 显示最新命令的详细信息
```

### TC-HIST-005: 清理历史

```bash
# 步骤
$ owl history clean --days 7

# 预期结果
# 清理 7 天前的历史记录
```

---

## 8. 常见问题

### Q: 历史记录保存在哪里？
A: 默认保存在 `~/.owl/history.db` (SQLite)

### Q: 历史记录保留多久？
A: 默认保留 90 天，可通过配置修改

### Q: 如何查看某个命令的输出？
A: 使用 `owl history exec --id <id>`

### Q: 可以导出历史记录吗？
A: 可以，使用 `owl history export` 命令

### Q: 如何查找某个命令？
A: 使用 `owl history --command "uptime"`
