# 节点执行日志功能设计

## 1. 功能概述

节点执行日志为每个被管理的节点维护一个独立的追加式日志文件，完整记录该节点上每一次命令执行的详细信息。所有通过 `owl exec run`、`owl exec script`、`owl playbook run` 发起的命令，在执行完成后都会自动追加到对应节点的日志文件中。

与 SQLite 历史记录数据库不同（`stdout` 被截断为 4096 字符），节点执行日志**不做截断**，完整保留 stdout/stderr 输出，适用于运维审计和事后问题排查场景。

### 1.1 核心特性

- **按节点组织**: 一个节点一个日志文件，终生追加
- **完整输出**: 不截断 stdout/stderr，保留所有执行细节
- **自动创建**: 日志文件及父目录在首次写入时自动创建
- **并发安全**: 每节点独立互斥锁，并行执行时日志不交错
- **路径可配**: 默认 `~/.owl/logs/nodes/`，支持通过环境变量自定义

---

## 2. 日志文件路径

### 2.1 默认路径

日志文件默认存储在用户主目录下的 `.owl/logs/nodes/` 目录中，文件名为 `<node_id>.log`：

```
~/.owl/logs/nodes/
├── web-01.log
├── web-02.log
├── db-01.log
└── cache-01.log
```

### 2.2 自定义路径

通过环境变量 `OWL_LOG_DIR` 可覆盖默认日志目录：

```bash
# 自定义日志目录
export OWL_LOG_DIR=/var/log/owl/nodes

# 执行命令后，日志写入 /var/log/owl/nodes/web-01.log
owl exec run uptime --nodes web-01
```

### 2.3 路径解析优先级

[logfile/writer.go](file:///Volumes/ORICO2T/Users/sinvigil/Programming/owl/go-owl/internal/logfile/writer.go#L13-L25) 中的 `resolveLogDir` 函数定义了路径解析优先级：

1. 代码中显式传入的 `logDir` 参数（优先）
2. 环境变量 `OWL_LOG_DIR`
3. 默认路径 `$HOME/.owl/logs/nodes/`

```go
func resolveLogDir(dir string) string {
    if dir != "" {
        return dir
    }
    if envDir := os.Getenv("OWL_LOG_DIR"); envDir != "" {
        return envDir
    }
    home, err := homeDirFunc()
    if err != nil {
        return filepath.Join(".owl", "logs", "nodes")
    }
    return filepath.Join(home, ".owl", "logs", "nodes")
}
```

---

## 3. 日志条目格式

### 3.1 标准格式

每条执行记录包含以下字段，记录之间以分隔线隔开：

```
──────────────────────────────────────────────────────────────────────
[2026-05-28 15:30:45] TASK: 550e8400-e29b-41d4-a716-446655440000
COMMAND: uptime
EXIT CODE: 0
DURATION: 1.23s
OUTPUT:
 15:30:45 up 30 days,  2:15,  3 users,  load average: 0.00, 0.01, 0.05
──────────────────────────────────────────────────────────────────────
```

### 3.2 字段说明

| 字段 | 说明 | 来源 |
|------|------|------|
| `[时间戳]` | 写入时间，格式 `2006-01-02 15:04:05` | `time.Now()` |
| `TASK` | 本次操作的任务 ID（UUID） | `uuid.New().String()` |
| `COMMAND` | 执行的命令内容 | 用户输入的命令字符串 |
| `EXIT CODE` | 命令退出码，0 表示成功 | `result.ExitCode` |
| `DURATION` | 命令执行耗时，自动格式化 | `result.EndTime - result.StartTime` |
| `OUTPUT` | 完整 stdout 输出 | `result.Output` |
| `ERROR` | 错误信息（仅在失败时出现） | `result.Error` |

### 3.3 失败记录示例

当命令执行失败（exit code != 0 或存在 error），会额外追加 `ERROR` 字段：

```
──────────────────────────────────────────────────────────────────────
[2026-05-28 15:32:10] TASK: 661f9501-f30c-52e5-b827-557760661111
COMMAND: cat /etc/nonexistent
EXIT CODE: 1
DURATION: 120ms
ERROR: command execution failed
OUTPUT:
cat: /etc/nonexistent: No such file or directory
──────────────────────────────────────────────────────────────────────
```

### 3.4 耗时格式化规则

[formatDuration](file:///Volumes/ORICO2T/Users/sinvigil/Programming/owl/go-owl/internal/logfile/writer.go#L93-L101) 函数对耗时的格式化规则：

| 耗时范围 | 显示格式 | 示例 |
|----------|----------|------|
| `< 1s` | 毫秒 | `120ms` |
| `1s ~ 1min` | 秒（保留两位小数） | `1.23s` |
| `>= 1min` | 分钟+秒 | `2m15s` |

---

## 4. 并发安全

### 4.1 设计思路

[NodeLogWriter](file:///Volumes/ORICO2T/Users/sinvigil/Programming/owl/go-owl/internal/logfile/writer.go#L27-L31) 为每个节点 ID 维护一个独立的 `sync.Mutex`，确保同一节点在同一时刻只有一个 goroutine 能写入其日志文件。不同节点之间的写入可以并行进行。

```go
type NodeLogWriter struct {
    logDir string
    mu     sync.Mutex          // 保护 locks map 的并发访问
    locks  map[string]*sync.Mutex  // 每个节点独立的锁
}
```

### 4.2 锁获取流程

`lockNode` 方法保证锁的懒加载和线程安全获取：

```go
func (w *NodeLogWriter) lockNode(nodeID string) *sync.Mutex {
    w.mu.Lock()
    defer w.mu.Unlock()
    if mu, ok := w.locks[nodeID]; ok {
        return mu
    }
    mu := &sync.Mutex{}
    w.locks[nodeID] = mu
    return mu
}
```

### 4.3 写入流程

`AppendEntry` 方法执行完整的写入流程：

1. 获取该节点的互斥锁 → `lockNode(nodeID).Lock()`
2. 确保日志目录存在 → `os.MkdirAll(w.logDir, 0755)`
3. 以追加模式打开文件 → `os.OpenFile(logPath, O_APPEND|O_CREATE|O_WRONLY, 0644)`
4. 构造日志条目内容
5. 写入文件 → `f.WriteString(entry)`
6. 关闭文件并释放锁 → `defer nodeMu.Unlock()` + `defer f.Close()`

### 4.4 并发场景示例

假设用户同时对 `web-01` 执行两个命令：

```bash
# 并行执行
owl exec run "uptime" --nodes web-01 &
owl exec run "df -h" --nodes web-01 &
```

此时两个 goroutine 会竞争 `web-01` 的互斥锁，先获取锁的 goroutine 完整写入其日志条目后再释放锁，后获取锁的 goroutine 接着写入。最终日志文件中两条记录完整不交错：

```
──────────────────────────────────────────────────────────────────────
[2026-05-28 15:30:45] TASK: aaa...
COMMAND: uptime
EXIT CODE: 0
DURATION: 1.23s
OUTPUT:
 15:30:45 up 30 days
──────────────────────────────────────────────────────────────────────
──────────────────────────────────────────────────────────────────────
[2026-05-28 15:30:46] TASK: bbb...
COMMAND: df -h
EXIT CODE: 0
DURATION: 2.15s
OUTPUT:
Filesystem      Size  Used Avail Use% Mounted on
/dev/sda1       50G   20G   30G  40% /
──────────────────────────────────────────────────────────────────────
```

---

## 5. 集成方式

### 5.1 创建 Writer 实例

在使用 `AppendEntry` 之前，需要在命令入口处创建 `NodeLogWriter` 实例：

```go
import "github.com/cangyunye/go-owl/internal/logfile"

nodeLogWriter := logfile.NewNodeLogWriter("")
```

传入空字符串表示使用默认路径解析逻辑（`OWL_LOG_DIR` 环境变量 → `~/.owl/logs/nodes/`）。

### 5.2 exec run 集成

在 [run.go](file:///Volumes/ORICO2T/Users/sinvigil/Programming/owl/go-owl/cmd/cli/cmd/exec/run.go) 的 `processResult` 回调中，每条命令结果返回后调用 `AppendEntry`：

```go
processResult := func(result command.CommandResult) {
    if result.Success {
        success++
    } else {
        failed++
    }

    // 写入节点执行日志
    errorMsg := ""
    if result.Error != nil {
        errorMsg = result.Error.Error()
    }
    nodeLogWriter.AppendEntry(
        result.NodeID,
        taskID,
        execmd,
        result.ExitCode,
        result.Output,
        errorMsg,
        result.EndTime.Sub(result.StartTime),
    )

    // 记录到 SQLite 历史数据库 (stdout 截断为 4096)
    history.RecordCommandExecution(&history.CommandExecution{
        TaskID:     taskID,
        NodeID:     result.NodeID,
        Command:    execmd,
        ExitCode:   result.ExitCode,
        Stdout:     truncateOutput(result.Output, 4096),
        Stderr:     errorMsg,
        DurationMs: time.Since(startTime).Milliseconds(),
        Success:    result.Success,
        CreatedAt:  time.Now(),
    })

    printResult(result)
}
```

### 5.3 exec script 集成

在 [script.go](file:///Volumes/ORICO2T/Users/sinvigil/Programming/owl/go-owl/cmd/cli/cmd/exec/script.go) 的结果循环中，每个节点的脚本执行结果返回后调用 `AppendEntry`：

```go
for _, result := range results {
    if result.Success() {
        success++
    } else {
        failed++
    }

    // 写入节点执行日志
    errorMsg := ""
    if result.Error != nil {
        errorMsg = result.Error.Error()
    }
    nodeLogWriter.AppendEntry(
        result.NodeID,
        taskID,
        scriptPath,
        result.ExitCode,
        result.Output,
        errorMsg,
        result.EndTime.Sub(result.StartTime),
    )

    // 记录到历史数据库
    history.RecordCommandExecution(&history.CommandExecution{
        TaskID:     taskID,
        NodeID:     result.NodeID,
        Command:    scriptPath,
        ExitCode:   result.ExitCode,
        Stdout:     truncateString(result.Output, 4096),
        Stderr:     errorMsg,
        DurationMs: result.EndTime.Sub(result.StartTime).Milliseconds(),
        Success:    result.Success(),
        CreatedAt:  time.Now(),
    })
}
```

### 5.4 playbook run 集成

在 [playbook/run.go](file:///Volumes/ORICO2T/Users/sinvigil/Programming/owl/go-owl/cmd/cli/cmd/playbook/run.go) 的 `runPlaybookRun` 中，遍历每个 task 的执行结果，为每个节点的每项 task 调用 `AppendEntry`：

```go
for taskName, results := range execution.Results {
    for _, result := range results {
        errorMsg := ""
        if result.Error != nil {
            errorMsg = result.Error.Error()
        }

        // 写入节点执行日志
        nodeLogWriter.AppendEntry(
            result.NodeID,
            taskID,
            taskName,
            result.ExitCode,
            result.Output,
            errorMsg,
            result.EndTime.Sub(result.StartTime),
        )

        // 记录到历史数据库 (stdout 截断为 4096)
        history.RecordCommandExecution(&history.CommandExecution{
            TaskID:     taskID,
            NodeID:     result.NodeID,
            Command:    taskName,
            ExitCode:   result.ExitCode,
            Stdout:     truncateStr(result.Output, 4096),
            Stderr:     errorMsg,
            DurationMs: result.EndTime.Sub(result.StartTime).Milliseconds(),
            Success:    result.ExitCode == 0,
            CreatedAt:  time.Now(),
        })
    }
}
```

### 5.5 方法签名

```go
func (w *NodeLogWriter) AppendEntry(
    nodeID   string,        // 节点 ID，决定写入哪个日志文件
    taskID   string,        // 任务 ID（UUID）
    command  string,        // 执行的命令
    exitCode int,           // 退出码
    output   string,        // 完整 stdout
    errMsg   string,        // 错误信息（空字符串表示成功）
    duration time.Duration, // 执行耗时
) error
```

---

## 6. 核心组件

| 组件 | 文件 | 职责 |
|------|------|------|
| NodeLogWriter | `internal/logfile/writer.go` | 节点日志写入器，管理日志文件路径和并发锁 |
| NewNodeLogWriter | `internal/logfile/writer.go` | 创建 Writer 实例，解析日志目录路径 |
| AppendEntry | `internal/logfile/writer.go` | 追加一条执行记录到节点日志文件 |
| lockNode | `internal/logfile/writer.go` | 获取指定节点的互斥锁 |

---

## 7. 测试用例清单

| 用例编号 | 测试描述 | 验证点 |
|----------|----------|--------|
| TC-LOG-001 | exec run 单节点写入日志 | 命令执行后 `~/.owl/logs/nodes/<node_id>.log` 存在，包含时间戳、Task ID、命令、退出码、输出 |
| TC-LOG-002 | exec script 多节点写入日志 | 两个节点的日志文件各自包含独立的执行记录 |
| TC-LOG-003 | playbook run 多 task 写入日志 | playbook 中每个 task 在对应节点日志中各生成一条记录 |
| TC-LOG-004 | 日志文件自动创建 | 目标目录和文件不存在时，首次写入自动创建 |
| TC-LOG-005 | 失败命令记录 ERROR 字段 | exit code != 0 时日志条目包含 `ERROR:` 行 |
| TC-LOG-006 | 并发写入不交错 | 并行执行多个命令，日志文件中每条记录完整独立 |
| TC-LOG-007 | OWL_LOG_DIR 自定义路径 | 设置环境变量后日志写入指定目录 |
| TC-LOG-008 | 多次追加不出错 | 对同一节点连续多次执行命令，日志正确追加不覆盖 |

---

## 8. 数据流图

```
┌──────────┐     ┌──────────┐     ┌──────────┐
│ exec run │     │exec script│    │ playbook │
│          │     │          │     │   run    │
└────┬─────┘     └────┬─────┘     └────┬─────┘
     │                │                │
     │   result       │   result       │   result
     ▼                ▼                ▼
┌─────────────────────────────────────────────────┐
│                NodeLogWriter                    │
│                                                 │
│  AppendEntry(nodeID, taskID, cmd,               │
│              exitCode, output, err, duration)    │
│                                                 │
│  1. lockNode(nodeID) ──► 获取节点互斥锁          │
│  2. MkdirAll          ──► 确保目录存在            │
│  3. OpenFile(APPEND)  ──► 打开日志文件            │
│  4. WriteString       ──► 写入格式化条目          │
│  5. Close + Unlock    ──► 释放资源               │
└───────────────────────┬─────────────────────────┘
                        │
                        ▼
    ~/.owl/logs/nodes/<node_id>.log
```

---

## 9. 注意事项

1. **与历史数据库的关系**: 节点执行日志与 `history` 包中的 SQLite 数据库是互补关系。SQLite 库中的 `stdout` 截断为 4096 字符，适合快速查询和统计；节点日志文件保留完整输出，适合审计和问题排查。
2. **日志不自动轮转**: 当前版本不限制日志文件大小，文件会持续增长。建议运维层面设置 logrotate 等机制进行归档。
3. **权限要求**: 日志文件以 `0644` 权限创建，目录以 `0755` 权限创建。确保运行 owl 的用户对该目录有写入权限。
4. **错误处理**: 日志写入失败（如磁盘满、权限不足）会返回 error，但不会中断命令执行流程。建议在生产环境中监控日志写入异常。
