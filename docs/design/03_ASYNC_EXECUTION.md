# 设计文档 3: 添加异步执行模式

## 1. 概述

### 1.1 问题描述

当前 go-owl 项目执行命令是同步阻塞模式，存在以下问题：

1. **长时间任务阻塞**：数据库迁移、软件编译等长时间任务会阻塞 SSH 连接
2. **SSH 超时风险**：长时间运行的命令可能被 SSH 会话超时中断
3. **资源浪费**：用户必须等待任务完成才能执行其他操作
4. **无法后台执行**：无法启动长时间任务后立即返回

### 1.2 当前实现

```go
// 当前实现：同步阻塞
func (e *Executor) Run(ctx context.Context, nodeIDs []string, command string, opts *ExecuteOptions) []CommandResult {
    results := make([]CommandResult, len(nodeIDs))
    for i, nodeID := range nodeIDs {
        results[i] = e.runOnNode(ctx, nodeID, command, opts)  // 阻塞等待
    }
    return results
}
```

### 1.3 Ansible 参考实现

Ansible 通过 `async` 和 `poll` 参数实现异步执行：

```yaml
# Ansible 异步执行示例
- name: 后台备份
  command: /usr/bin/backup.sh
  async: 3600    # 最大运行时间（秒）
  poll: 60       # 轮询间隔（秒），0 表示 fire-and-forget
  register: backup_job
```

### 1.4 目标

实现类似 Ansible 的异步执行模式，支持：
- 后台执行长时间任务
- 轮询检查任务状态
- Fire-and-forget 模式
- 异步任务管理

## 2. 设计方案

### 2.1 异步任务结构

```go
// AsyncTask 异步任务
type AsyncTask struct {
    // ID 任务唯一标识
    ID string

    // NodeID 节点ID
    NodeID string

    // Command 执行的命令
    Command string

    // StartTime 开始时间
    StartTime time.Time

    // EndTime 结束时间（如果已完成）
    EndTime time.Time

    // Status 任务状态
    Status AsyncTaskStatus

    // Pid 远程进程 PID
    Pid int

    // OutputFile 输出文件路径
    OutputFile string

    // ExitCode 退出码
    ExitCode int

    // Error 错误信息
    Error error

    // AsyncTimeout 异步超时时间
    AsyncTimeout time.Duration

    // RemoteBaseDir 远程工作目录
    RemoteBaseDir string
}

// AsyncTaskStatus 异步任务状态
type AsyncTaskStatus string

const (
    AsyncTaskStatusPending   AsyncTaskStatus = "pending"    // 等待执行
    AsyncTaskStatusRunning   AsyncTaskStatus = "running"    // 执行中
    AsyncTaskStatusSuccess   AsyncTaskStatus = "success"    // 执行成功
    AsyncTaskStatusFailed    AsyncTaskStatus = "failed"    // 执行失败
    AsyncTaskStatusTimeout   AsyncTaskStatus = "timeout"    // 超时
    AsyncTaskStatusCanceled  AsyncTaskStatus = "canceled"   // 已取消
)
```

### 2.2 异步任务配置

```go
// AsyncOptions 异步执行选项
type AsyncOptions struct {
    // Timeout 异步任务最大运行时间（默认 1 小时）
    Timeout time.Duration

    // PollInterval 轮询间隔（0 表示 fire-and-forget）
    PollInterval time.Duration

    // MaxPollCount 最大轮询次数（默认 3600）
    MaxPollCount int

    // Async 是否异步执行（true）或同步执行（false）
    Async bool

    // RemoteBaseDir 远程工作目录（默认 /tmp/owl）
    RemoteBaseDir string
}

// DefaultAsyncOptions 默认配置
func DefaultAsyncOptions() AsyncOptions {
    return AsyncOptions{
        Timeout:      1 * time.Hour,
        PollInterval: 10 * time.Second,
        MaxPollCount: 3600,
        Async:        false,
        RemoteBaseDir: "/tmp/owl",
    }
}
```

### 2.3 后台启动策略（setsid 优于 nohup）

**问题分析**：
```
┌─────────────────────────────────────────────────────────────────┐
│                    nohup 的局限性                                │
├─────────────────────────────────────────────────────────────────┤
│  nohup 原理:                                                   │
│    - 忽略 SIGHUP 信号                                           │
│    - 重新定向 stdin/stdout/stderr                               │
│                                                                  │
│  nohup 问题:                                                   │
│    - 仍然受控制终端影响                                         │
│    - 在某些环境下可能失败                                       │
│    - 权限问题：需要当前目录写权限                               │
└─────────────────────────────────────────────────────────────────┘
```

**推荐方案：使用 setsid**
```bash
# setsid 完全脱离控制终端
setsid /path/to/command > /tmp/owl/output.log 2>&1 &
echo $!  # 获取 PID

# 或者组合使用
nohup setsid /path/to/command > /tmp/owl/output.log 2>&1 &
```

**关键代码**：
```go
func (m *AsyncTaskManager) startBackgroundTask(nodeID, command string) (pid int, outputFile string, err error) {
    // 1. 确保远程目录存在
    mkdirCmd := fmt.Sprintf("mkdir -p %s && chmod 755 %s", m.remoteBaseDir, m.remoteBaseDir)
    if _, _, err := m.executeSSH(nodeID, mkdirCmd); err != nil {
        return 0, "", fmt.Errorf("failed to create remote dir: %w", err)
    }

    // 2. 生成任务ID和输出文件
    taskID := uuid.New().String()
    outputFile = fmt.Sprintf("%s/%s.out", m.remoteBaseDir, taskID)

    // 3. 构建后台启动命令
    // 使用 setsid 完全脱离控制终端，并取消 TMOUT
    bgCommand := fmt.Sprintf(
        `cd %s && unset TMOUT && setsid %s > %s 2>&1 & echo $!`,
        m.remoteBaseDir,
        command,
        outputFile,
    )

    // 4. 设置 SSH 保活参数
    sshArgs := []string{
        "-o", "ServerAliveInterval=30",
        "-o", "ServerAliveCountMax=3",
        "-o", "BatchMode=yes",
    }

    // 5. 执行并获取 PID
    _, output, err := m.executeSSHWithArgs(nodeID, sshArgs, bgCommand)
    if err != nil {
        return 0, "", fmt.Errorf("failed to start background task: %w", err)
    }

    pid, err = strconv.Atoi(strings.TrimSpace(output))
    if err != nil {
        return 0, "", fmt.Errorf("failed to parse PID: %w", err)
    }

    return pid, outputFile, nil
}
```

### 2.4 远程工作目录设计

**目录结构**：
```
/tmp/owl/
├── tasks/           # 任务目录
│   ├── task-uuid1/  # 每个任务一个子目录
│   │   ├── command.sh
│   │   └── output.log
│   └── task-uuid2/
└── cleanup.timer    # 清理定时器（可选）
```

**权限策略**：
```bash
# 目录权限: 1777 (sticky bit) - 所有用户可创建文件，但只能删除自己的
# drwxrwxrwt  3 root root 4096 May 17 10:00 /tmp/owl

# 自动创建
ssh user@host "mkdir -p /tmp/owl && chmod 1777 /tmp/owl"
```

### 2.5 轮询机制设计

**多层超时控制**：
```
┌─────────────────────────────────────────────────────────────────┐
│                       超时控制层级                               │
├─────────────────────────────────────────────────────────────────┤
│  层级 1: 任务总超时 (AsyncTimeout)                              │
│    - 默认 1 小时                                                │
│    - 到达后强制终止进程                                         │
│                                                                  │
│  层级 2: 单次轮询超时                                           │
│    - 默认 30 秒                                                 │
│    - SSH 连接超时                                               │
│                                                                  │
│  层级 3: 轮询次数上限 (MaxPollCount)                           │
│    - 默认 3600 次                                               │
│    - 防止无限循环                                               │
│                                                                  │
│  层级 4: 连续失败上限                                          │
│    - 默认 3 次                                                  │
│    - 连续失败后标记任务失败                                     │
└─────────────────────────────────────────────────────────────────┘
```

**轮询实现**：
```go
func (m *AsyncTaskManager) PollTaskStatus(ctx context.Context, task *AsyncTask, pollInterval time.Duration, maxPollCount int) (*AsyncTask, error) {
    pollCount := 0
    consecutiveFailures := 0

    ticker := time.NewTicker(pollInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return m.GetTask(task.ID)

        case <-ticker.C:
            pollCount++

            // 检查是否超过最大轮询次数
            if maxPollCount > 0 && pollCount >= maxPollCount {
                m.forceTerminate(task)
                task.Status = AsyncTaskStatusTimeout
                task.Error = fmt.Errorf("max poll count exceeded: %d", maxPollCount)
                return task, nil
            }

            // 检查任务总超时
            if task.AsyncTimeout > 0 && time.Since(task.StartTime) > task.AsyncTimeout {
                m.forceTerminate(task)
                task.Status = AsyncTaskStatusTimeout
                task.Error = fmt.Errorf("task timeout after %v", task.AsyncTimeout)
                return task, nil
            }

            // 检查进程是否还在运行
            running, err := m.checkProcessRunning(task.NodeID, task.Pid)
            if err != nil {
                consecutiveFailures++
                if consecutiveFailures >= 3 {
                    task.Status = AsyncTaskStatusFailed
                    task.Error = fmt.Errorf("poll failed %d times: %w", consecutiveFailures, err)
                    return task, nil
                }
                continue
            }
            consecutiveFailures = 0

            if !running {
                // 进程已结束，读取退出码和输出
                m.collectResults(task)
                return task, nil
            }
        }
    }
}
```

### 2.6 进程终止策略

**安全终止流程**：
```
┌─────────────────────────────────────────────────────────────────┐
│                       进程终止流程                               │
├─────────────────────────────────────────────────────────────────┤
│  步骤 1: 发送 SIGTERM (优雅终止)                                │
│    - 允许进程清理资源                                           │
│    - 等待最多 5 秒                                              │
│                                                                  │
│  步骤 2: 检查进程状态                                          │
│    - 如果进程已退出，收集退出码                                 │
│                                                                  │
│  步骤 3: 发送 SIGKILL (强制终止)                               │
│    - 如果进程仍在运行                                           │
│    - 立即终止，不等待                                           │
│                                                                  │
│  步骤 4: 清理资源                                              │
│    - 删除输出文件（可选）                                       │
│    - 更新任务状态                                               │
└─────────────────────────────────────────────────────────────────┘
```

**实现代码**：
```go
func (m *AsyncTaskManager) forceTerminate(task *AsyncTask) error {
    // 1. 发送 SIGTERM (优雅终止)
    killCmd := fmt.Sprintf("kill -TERM %d 2>/dev/null || true", task.Pid)
    m.executeSSH(task.NodeID, killCmd)

    // 2. 等待进程退出（最多 5 秒）
    deadline := time.Now().Add(5 * time.Second)
    for time.Now().Before(deadline) {
        if !m.isProcessRunning(task.NodeID, task.Pid) {
            // 进程已退出，读取退出码
            task.ExitCode = m.getExitCode(task.NodeID, task.Pid)
            return nil
        }
        time.Sleep(100 * time.Millisecond)
    }

    // 3. 发送 SIGKILL (强制终止)
    killCmd = fmt.Sprintf("kill -KILL %d 2>/dev/null || true", task.Pid)
    m.executeSSH(task.NodeID, killCmd)

    // 4. 等待进程消失
    time.Sleep(500 * time.Millisecond)

    // 5. 清理临时文件（可选，保留日志用于调试）
    // cleanupCmd := fmt.Sprintf("rm -f %s", task.OutputFile)
    // m.executeSSH(task.NodeID, cleanupCmd)

    task.Status = AsyncTaskStatusCanceled
    task.ExitCode = -1
    task.Error = fmt.Errorf("task force terminated")

    return nil
}

// isProcessRunning 检查进程是否在运行
func (m *AsyncTaskManager) isProcessRunning(nodeID string, pid int) bool {
    cmd := fmt.Sprintf("kill -0 %d 2>/dev/null && echo running || echo stopped", pid)
    output, err := m.executeSSH(nodeID, cmd)
    if err != nil {
        return false
    }
    return strings.Contains(output, "running")
}
```

### 2.7 Zombie 进程防护

**Zombie 产生原因**：
```
┌─────────────────────────────────────────────────────────────────┐
│                    Zombie 进程的产生机制                         │
├─────────────────────────────────────────────────────────────────┤
│  正常流程:                                                      │
│    父进程 fork → 子进程运行 → 子进程退出 → 父进程 wait() → 回收 │
│                                                                  │
│  Zombie 流程:                                                  │
│    SSH fork → 子进程运行 → SSH 断开 → 无 wait() → Zombie       │
│                                                                  │
│  防护方案:                                                      │
│    1. 使用 setsid 完全脱离控制终端                               │
│    2. 显式等待子进程退出                                        │
│    3. 使用 waitpid() 非阻塞回收                                  │
└─────────────────────────────────────────────────────────────────┘
```

**防护实现**：
```go
// 在远程节点执行时，确保父进程正确处理子进程
func (m *AsyncTaskManager) startSafeBackgroundTask(nodeID, command string) (int, string, error) {
    // 方案 1: setsid 完全脱离
    bgCommand := fmt.Sprintf(
        `setsid bash -c '%s' > /tmp/owl/output.log 2>&1 &
         echo $!`,
        command,
    )

    // 方案 2: 使用 subshell 确保 wait 正确处理
    bgCommand := fmt.Sprintf(
        `( %s & ) && disown && echo $!`,
        command,
    )

    // 方案 3: nohup + setsid 组合
    bgCommand := fmt.Sprintf(
        `cd /tmp/owl && nohup setsid %s > /tmp/owl/output.log 2>&1 &
         echo $!`,
        command,
    )

    output, err := m.executeSSH(nodeID, bgCommand)
    // ...
}
```

### 2.8 SSH 连接保持（轮询模式）

**TMOUT 问题**：
```bash
# SSH 服务器端 TMOUT 配置
# /etc/ssh/sshd_config: ClientAliveInterval 60

# 问题：长时间轮询时，SSH 连接可能因 TMOUT 断开
```

**解决方案**：
```go
func (m *AsyncTaskManager) executeSSHWithKeepAlive(nodeID, command string, timeout time.Duration) (string, error) {
    // 设置 SSH 保活参数
    args := []string{
        "-o", "ServerAliveInterval=30",  // 每 30 秒发送心跳
        "-o", "ServerAliveCountMax=3",   // 最多 3 次无响应
        "-o", "ConnectTimeout=10",       // 连接超时
        "-o", "BatchMode=yes",          // 批处理模式
    }

    // 构建完整命令
    fullArgs := append(args, m.buildNodeSSHArgs(nodeID)...)
    fullArgs = append(fullArgs, command)

    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()

    cmd := exec.CommandContext(ctx, "ssh", fullArgs...)
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    err := cmd.Run()
    if err != nil {
        return "", err
    }

    return stdout.String(), nil
}
```

### 2.9 异步任务管理器

```go
// AsyncTaskManager 异步任务管理器
type AsyncTaskManager struct {
    mu    sync.RWMutex
    tasks map[string]*AsyncTask

    // 配置
    remoteBaseDir string
    maxConcurrent int
    cleanupAfter  time.Duration
}

func NewAsyncTaskManager(opts *AsyncOptions) *AsyncTaskManager {
    if opts == nil {
        opts = &DefaultAsyncOptions()
    }

    return &AsyncTaskManager{
        tasks:          make(map[string]*AsyncTask),
        remoteBaseDir:  opts.RemoteBaseDir,
        maxConcurrent:  100,
        cleanupAfter:   24 * time.Hour,
    }
}

// StartAsync 启动异步任务
func (m *AsyncTaskManager) StartAsync(ctx context.Context, nodeID, command string, opts *AsyncOptions) (*AsyncTask, error) {
    if opts == nil {
        opts = &DefaultAsyncOptions()
    }

    task := &AsyncTask{
        ID:            uuid.New().String(),
        NodeID:        nodeID,
        Command:        command,
        StartTime:     time.Now(),
        Status:        AsyncTaskStatusPending,
        AsyncTimeout:  opts.Timeout,
        RemoteBaseDir: opts.RemoteBaseDir,
    }

    m.mu.Lock()
    if len(m.tasks) >= m.maxConcurrent {
        m.mu.Unlock()
        return nil, fmt.Errorf("max concurrent async tasks exceeded: %d", m.maxConcurrent)
    }
    m.tasks[task.ID] = task
    m.mu.Unlock()

    // 启动异步执行
    go m.runTask(ctx, task, opts)

    return task, nil
}

func (m *AsyncTaskManager) runTask(ctx context.Context, task *AsyncTask, opts *AsyncOptions) {
    task.Status = AsyncTaskStatusRunning

    // 启动后台任务
    pid, outputFile, err := m.startBackgroundTask(task.NodeID, task.Command, opts.RemoteBaseDir)
    if err != nil {
        task.Status = AsyncTaskStatusFailed
        task.Error = err
        return
    }

    task.Pid = pid
    task.OutputFile = outputFile

    // 如果是 fire-and-forget (pollInterval=0)，立即返回
    if opts.PollInterval == 0 {
        return
    }

    // 轮询等待任务完成
    completed, err := m.PollTaskStatus(ctx, task, opts.PollInterval, opts.MaxPollCount)
    if err != nil {
        task.Error = err
    }
    *task = *completed
}
```

### 2.10 Fire-and-Forget 模式

```go
// StartAndForget 启动异步任务并立即返回（不轮询）
func (m *AsyncTaskManager) StartAndForget(ctx context.Context, nodeID, command string, opts *AsyncOptions) (string, error) {
    if opts == nil {
        opts = &DefaultAsyncOptions()
    }

    // 设置 pollInterval=0 表示不轮询
    opts.PollInterval = 0

    task, err := m.StartAsync(ctx, nodeID, command, opts)
    if err != nil {
        return "", err
    }

    return task.ID, nil
}

// WaitForAll 等待所有指定任务完成
func (m *AsyncTaskManager) WaitForAll(ctx context.Context, taskIDs []string, pollInterval time.Duration) []AsyncTask {
    results := make([]AsyncTask, len(taskIDs))

    var wg sync.WaitGroup
    wg.Add(len(taskIDs))

    for i, taskID := range taskIDs {
        go func(idx int, id string) {
            defer wg.Done()
            task := m.GetTask(id)
            if task == nil {
                return
            }

            // 轮询等待
            completed, _ := m.PollTaskStatus(ctx, task, pollInterval, 0)
            if completed != nil {
                results[idx] = *completed
            }
        }(i, taskID)
    }

    wg.Wait()
    return results
}
```

## 3. 代码改动

### 3.1 新增文件

| 文件 | 描述 |
|------|------|
| `internal/control/async/task.go` | 异步任务结构定义 |
| `internal/control/async/manager.go` | 异步任务管理器 |
| `internal/control/async/manager_test.go` | 单元测试 |

### 3.2 修改文件

| 文件 | 改动点 |
|------|--------|
| `internal/control/command/executor_v2.go` | 添加 RunAsync 方法 |
| `cmd/cli/cmd/exec/run.go` | 添加 `--async` 参数 |
| `cmd/cli/cmd/async/` | 新增 async 子命令 |

### 3.3 详细改动点

#### 3.3.1 internal/control/async/task.go（新增）

```go
package async

type AsyncTask struct {
    ID            string
    NodeID        string
    Command       string
    StartTime     time.Time
    EndTime       time.Time
    Status        AsyncTaskStatus
    Pid           int
    OutputFile    string
    ExitCode      int
    Error         error
    AsyncTimeout  time.Duration
    RemoteBaseDir string
}
```

#### 3.3.2 internal/control/async/manager.go（新增）

```go
package async

type AsyncTaskManager struct {
    mu            sync.RWMutex
    tasks         map[string]*AsyncTask
    remoteBaseDir string
    maxConcurrent int
    cleanupAfter  time.Duration
}

func NewAsyncTaskManager(opts *AsyncOptions) *AsyncTaskManager
func (m *AsyncTaskManager) StartAsync(ctx context.Context, nodeID, command string, opts *AsyncOptions) (*AsyncTask, error)
func (m *AsyncTaskManager) PollTaskStatus(ctx context.Context, task *AsyncTask, pollInterval time.Duration, maxPollCount int) (*AsyncTask, error)
func (m *AsyncTaskManager) GetTask(taskID string) *AsyncTask
func (m *AsyncTaskManager) CancelTask(taskID string) error
func (m *AsyncTaskManager) forceTerminate(task *AsyncTask) error
func (m *AsyncTaskManager) startBackgroundTask(nodeID, command, remoteBaseDir string) (int, string, error)
func (m *AsyncTaskManager) isProcessRunning(nodeID string, pid int) bool
```

#### 3.3.3 internal/control/command/executor_v2.go（修改）

```go
func (e *Executor) RunAsync(ctx context.Context, nodeIDs []string, command string, opts *AsyncOptions) ([]AsyncTask, error)
```

#### 3.3.4 cmd/cli/cmd/exec/run.go（修改）

```go
var (
    asyncFlag       bool
    asyncTimeout    time.Duration
    pollInterval    time.Duration
    maxPollCount    int
    remoteBaseDir   string
)

func init() {
    runCmd.Flags().BoolVar(&asyncFlag, "async", false, "Run command asynchronously")
    runCmd.Flags().DurationVar(&asyncTimeout, "async-timeout", 1*time.Hour, "Async task timeout")
    runCmd.Flags().DurationVar(&pollInterval, "poll-interval", 10*time.Second, "Poll interval for async tasks (0 for fire-and-forget)")
    runCmd.Flags().IntVar(&maxPollCount, "max-poll-count", 3600, "Maximum poll count")
    runCmd.Flags().StringVar(&remoteBaseDir, "remote-dir", "/tmp/owl", "Remote working directory")
}
```

#### 3.3.5 cmd/cli/cmd/async/async.go（新增）

```bash
owl async list              # 列出异步任务
owl async status <task-id>  # 查看任务状态
owl async wait <task-id>    # 等待任务完成
owl async cancel <task-id>   # 取消任务
owl async cleanup           # 清理已完成任务
```

## 4. CLI 使用示例

### 4.1 基本异步执行

```bash
# 异步执行（轮询模式）
owl exec run "apt upgrade" --async --async-timeout=2h --poll-interval=30s

# Fire-and-forget 模式（不等待结果）
owl exec run "nohup long_script.sh" --async --poll-interval=0

# 输出示例：
# [Async] Task started: task-abc123
# [Node: web-01] Status: running (PID: 12345)
# [Node: db-01] Status: running (PID: 12346)
# Press Ctrl+C to cancel...
```

### 4.2 异步任务管理

```bash
# 列出异步任务
owl async list

# 查看任务状态
owl async status task-abc123

# 等待任务完成
owl async wait task-abc123

# 取消任务
owl async cancel task-abc123

# 清理已完成任务
owl async cleanup
```

### 4.3 批量异步执行

```bash
# 并行启动多个异步任务
owl exec run "heavy_script.sh" --nodes=node1,node2,node3 --async --poll-interval=0

# 等待所有任务完成
owl async wait-all --tasks=task-1,task-2,task-3 --poll-interval=30s
```

## 5. 风险评估

### 5.1 向后兼容性

| 影响项 | 风险等级 | 说明 | 缓解措施 |
|--------|----------|------|----------|
| Run 方法签名 | 🟢 低 | 新增 RunAsync，原方法保持不变 | 向后兼容 |
| CLI 参数 | 🟢 低 | 新增 --async 参数 | 向后兼容 |

### 5.2 功能风险

| 风险 | 等级 | 描述 | 缓解措施 |
|------|------|------|----------|
| Zombie 进程 | 🟡 中 | setsid 可有效避免 | 监控并清理 |
| SSH TMOUT | 🟡 中 | 轮询模式需要心跳 | ServerAliveInterval |
| 进程终止失败 | 🟡 中 | 某些进程无法被 SIGTERM | SIGKILL 强制终止 |
| /tmp 权限问题 | 🟡 中 | 某些环境 /tmp 不可写 | 备用目录或创建失败 |
| PID 回收重用 | 🟡 中 | 长任务可能遇到 | 双重验证进程 |

### 5.3 性能影响

| 影响项 | 风险等级 | 说明 |
|--------|----------|------|
| 任务存储 | 🟢 低 | Map 存储任务，占用少量内存 |
| 轮询开销 | 🟡 中 | 每个任务轮询增加 SSH 调用 |
| 并发限制 | 🟡 中 | 需要限制最大并发数 |

### 5.4 测试覆盖

需要添加以下测试用例：

```go
func TestAsyncStart(t *testing.T) {
    // 测试异步任务启动
}

func TestAsyncPoll(t *testing.T) {
    // 测试轮询机制
}

func TestAsyncTimeout(t *testing.T) {
    // 测试超时处理
}

func TestAsyncCancel(t *testing.T) {
    // 测试任务取消
}

func TestFireAndForget(t *testing.T) {
    // 测试 fire-and-forget 模式
}

func TestProcessZombie(t *testing.T) {
    // 测试僵尸进程防护
}

func TestCleanup(t *testing.T) {
    // 测试任务清理
}

func TestForceTerminate(t *testing.T) {
    // 测试强制终止
}

func TestServerAliveInterval(t *testing.T) {
    // 测试 SSH 保活
}
```

## 6. 实施计划

### 6.1 阶段划分

| 阶段 | 任务 | 预计工作量 |
|------|------|------------|
| 阶段 1 | 创建异步任务结构和状态定义 | 0.5 天 |
| 阶段 2 | 实现后台启动（setsid + /tmp/owl） | 2 天 |
| 阶段 3 | 实现轮询机制（多层超时） | 1.5 天 |
| 阶段 4 | 实现强制终止（SIGTERM + SIGKILL） | 1 天 |
| 阶段 5 | 实现 fire-and-forget 模式 | 0.5 天 |
| 阶段 6 | 添加 CLI 命令 | 1 天 |
| 阶段 7 | 添加单元测试 | 1.5 天 |
| 阶段 8 | 集成测试 | 2 天 |

**总预计工作量：约 10 个工作日**

### 6.2 实施顺序

```
阶段1 ──▶ 阶段2 ──▶ 阶段3 ──▶ 阶段4 ──▶ 阶段5 ──▶ 阶段6 ──▶ 阶段7 ──▶ 阶段8
   │         │         │         │         │         │         │         │
   ▼         ▼         ▼         ▼         ▼         ▼         ▼         ▼
创建结构   后台启动   轮询机制   强制终止   F&F模式   CLI命令   单元测试   集成测试
           + 目录              + Zombie防护
```

## 7. 与其他功能的交互

### 7.1 与重试机制结合

```go
// 异步执行 + 重试
asyncOpts := &AsyncOptions{
    Async:       true,
    Timeout:     2 * time.Hour,
    PollInterval: 30 * time.Second,
}

retryOpts := &RetryConfig{
    MaxRetries: 2,
    // ...
}

// 先重试连接，再异步执行
```

### 7.2 与超时分离结合

```go
// 异步模式下的超时分离
asyncOpts := &AsyncOptions{
    Timeout:      2 * time.Hour,  // 任务总超时
    PollInterval: 30 * time.Second,
}

timeoutConfig := &TimeoutConfig{
    ConnectTimeout: 10 * time.Second,  // SSH 连接超时
    CommandTimeout: 30 * time.Second,  // SSH 命令超时（用于轮询）
}
```

## 8. 回滚方案

1. **功能开关**：`OWL_ASYNC_ENABLED=0` 禁用异步功能
2. **模式降级**：异步失败时自动降级为同步执行
3. **优雅关闭**：收到关闭信号时等待运行中的任务完成

## 9. 文档更新

需要更新以下文档：

- [ ] `docs/usage/ASYNC.md` - 新增异步执行文档
- [ ] `docs/usage/EXEC.md` - 添加 --async 参数说明
- [ ] `docs/architecture/ASYNC.md` - 异步架构设计文档

## 10. 关键设计决策总结

```
┌─────────────────────────────────────────────────────────────────┐
│                      关键设计决策                               │
├─────────────────────────────────────────────────────────────────┤
│  Q1: 使用什么方式启动后台任务？                                  │
│  A:  setsid > nohup（完全脱离控制终端）                        │
│                                                                  │
│  Q2: 远程工作目录放在哪里？                                     │
│  A:  /tmp/owl（权限 1777，所有用户可写）                        │
│                                                                  │
│  Q3: 如何防止 Zombie 进程？                                     │
│  A:  setsid + 显式 wait()                                       │
│                                                                  │
│  Q4: 轮询超时如何控制？                                         │
│  A:  四层控制：总超时、单次超时、次数上限、连续失败             │
│                                                                  │
│  Q5: 如何强制终止进程？                                         │
│  A:  SIGTERM(5s等待) → SIGKILL                                  │
│                                                                  │
│  Q6: 如何防止 SSH TMOUT？                                       │
│  A:  ServerAliveInterval=30                                     │
└─────────────────────────────────────────────────────────────────┘
```

## 11. 未来扩展

- [ ] 异步任务持久化（支持重启恢复）
- [ ] 异步任务 Webhook 通知
- [ ] 异步任务链（任务依赖）
- [ ] Screen/Tmux 集成（进一步增强可靠性）
- [ ] 异步任务优先级
