# 设计文档 2: 添加命令重试机制

## 1. 概述

### 1.1 问题描述

当前 go-owl 项目在执行命令时没有重试机制，存在以下问题：

1. **网络波动导致失败**：短暂的网络抖动会导致命令直接失败
2. **用户体验差**：用户需要手动重试失败的命令
3. **批量操作风险大**：在大规模节点操作时，单点故障影响整体

### 1.2 当前实现

```go
// 当前实现：直接执行，无重试
func (e *Executor) runOnNode(...) CommandResult {
    executor, err := e.pool.Get(nodeInfo)
    if err != nil {
        return CommandResult{Error: err}  // 直接返回错误
    }

    exitCode, output, err := executor.Execute(fullCommand, timeout)
    if err != nil {
        return CommandResult{Error: err}  // 直接返回错误
    }

    return CommandResult{Success: exitCode == 0}
}
```

### 1.3 目标

添加智能重试机制，提高命令执行的可靠性。

## 2. 设计方案

### 2.1 重试配置结构

```go
// RetryConfig 重试配置
type RetryConfig struct {
    // MaxRetries 最大重试次数（默认 3）
    MaxRetries int

    // InitialInterval 初始重试间隔（默认 1 秒）
    InitialInterval time.Duration

    // MaxInterval 最大重试间隔（默认 30 秒）
    MaxInterval time.Duration

    // BackoffMultiplier 退避乘数（默认 2.0）
    BackoffMultiplier float64

    // RetryableErrors 可重试的错误列表
    RetryableErrors []string

    // EnableExponentialBackoff 是否启用指数退避
    EnableExponentialBackoff bool
}

// DefaultRetryConfig 默认重试配置
func DefaultRetryConfig() RetryConfig {
    return RetryConfig{
        MaxRetries:           3,
        InitialInterval:      1 * time.Second,
        MaxInterval:          30 * time.Second,
        BackoffMultiplier:    2.0,
        RetryableErrors:      []string{"connection refused", "timeout", "temporary failure"},
        EnableExponentialBackoff: true,
    }
}
```

### 2.2 可重试错误类型

```go
// RetryableError 可重试的错误
type RetryableError struct {
    OriginalError error
    RetryCount   int
    NextRetryAt  time.Time
}

func (e *RetryableError) Error() string {
    return fmt.Sprintf("retryable error after %d attempts: %v", e.RetryCount, e.OriginalError)
}

// IsRetryable 判断错误是否可重试
func IsRetryable(err error, config *RetryConfig) bool {
    if config == nil {
        return false
    }

    errMsg := strings.ToLower(err.Error())

    for _, pattern := range config.RetryableErrors {
        if strings.Contains(errMsg, strings.ToLower(pattern)) {
            return true
        }
    }

    // 内置可重试错误检测
    switch {
    case strings.Contains(errMsg, "connection refused"):
        return true
    case strings.Contains(errMsg, "timeout"):
        return true
    case strings.Contains(errMsg, "temporary failure"):
        return true
    case strings.Contains(errMsg, "i/o timeout"):
        return true
    case strings.Contains(errMsg, "network is unreachable"):
        return true
    default:
        return false
    }
}
```

### 2.3 重试结果

```go
// RetryResult 重试结果
type RetryResult struct {
    // CommandResult 最终的命令结果
    CommandResult

    // TotalAttempts 总尝试次数
    TotalAttempts int

    // RetryHistory 重试历史
    RetryHistory []RetryAttempt

    // FinalError 最终错误（如果全部失败）
    FinalError error

    // Retried 是否进行了重试
    Retried bool
}

// RetryAttempt 单次重试尝试
type RetryAttempt struct {
    Attempt    int
    StartTime  time.Time
    EndTime    time.Time
    Error      error
    Duration   time.Duration
}
```

### 2.4 重试策略

#### 策略 1：指数退避（推荐）

```
重试间隔 = min(InitialInterval * (BackoffMultiplier ^ attempt), MaxInterval)

示例（InitialInterval=1s, BackoffMultiplier=2.0, MaxInterval=30s）:
- 第 1 次重试: 1s
- 第 2 次重试: 2s
- 第 3 次重试: 4s
- 第 4 次重试: 8s
- 第 5 次重试: 16s
- 第 N 次重试: min(1 * 2^n, 30)s
```

#### 策略 2：线性退避

```
重试间隔 = InitialInterval * attempt
```

#### 策略 3：抖动（Jitter）

在指数退避基础上添加随机抖动，避免惊群效应：

```go
func calculateJitter(interval time.Duration) time.Duration {
    jitter := time.Duration(rand.Float64() * float64(interval))
    return interval + jitter/2
}
```

### 2.5 重试执行器

```go
// RetryExecutor 带重试的执行器
type RetryExecutor struct {
    executor *Executor
    config   *RetryConfig
}

// NewRetryExecutor 创建重试执行器
func NewRetryExecutor(executor *Executor, config *RetryConfig) *RetryExecutor {
    if config == nil {
        config = &DefaultRetryConfig()
    }
    return &RetryExecutor{
        executor: executor,
        config:   config,
    }
}

// RunWithRetry 执行命令并重试
func (e *RetryExecutor) RunWithRetry(ctx context.Context, nodeIDs []string, command string, opts *ExecuteOptions) []RetryResult {
    results := make([]RetryResult, len(nodeIDs))

    for i, nodeID := range nodeIDs {
        results[i] = e.runOnNodeWithRetry(ctx, nodeID, command, opts)
    }

    return results
}

func (e *RetryExecutor) runOnNodeWithRetry(ctx context.Context, nodeID, command string, opts *ExecuteOptions) RetryResult {
    var history []RetryAttempt
    totalAttempts := 0

    for attempt := 0; attempt <= e.config.MaxRetries; attempt++ {
        totalAttempts++
        startTime := time.Now()

        // 执行命令
        result := e.executor.runOnNode(ctx, nodeID, command, opts)

        endTime := time.Now()
        duration := endTime.Sub(startTime)

        history = append(history, RetryAttempt{
            Attempt:   totalAttempts,
            StartTime: startTime,
            EndTime:   endTime,
            Error:     result.Error,
            Duration:  duration,
        })

        // 检查是否成功
        if result.Success {
            return RetryResult{
                CommandResult: result,
                TotalAttempts: totalAttempts,
                RetryHistory: history,
                Retried:      attempt > 0,
            }
        }

        // 检查是否可以重试
        if attempt < e.config.MaxRetries && IsRetryable(result.Error, e.config) {
            // 计算等待时间
            interval := e.calculateInterval(attempt)
            select {
            case <-time.After(interval):
                continue
            case <-ctx.Done():
                return RetryResult{
                    CommandResult: result,
                    TotalAttempts: totalAttempts,
                    RetryHistory: history,
                    FinalError:   ctx.Err(),
                    Retried:      attempt > 0,
                }
            }
        } else {
            // 不可重试或已达最大次数
            return RetryResult{
                CommandResult: result,
                TotalAttempts: totalAttempts,
                RetryHistory: history,
                FinalError:   result.Error,
                Retried:      attempt > 0,
            }
        }
    }

    return RetryResult{
        CommandResult: CommandResult{Success: false, Error: fmt.Errorf("max retries exceeded")},
        TotalAttempts: totalAttempts,
        RetryHistory:  history,
        FinalError:   fmt.Errorf("max retries exceeded"),
        Retried:      true,
    }
}

func (e *RetryExecutor) calculateInterval(attempt int) time.Duration {
    interval := float64(e.config.InitialInterval)
    if e.config.EnableExponentialBackoff {
        interval *= math.Pow(e.config.BackoffMultiplier, float64(attempt))
    } else {
        interval *= float64(attempt + 1)
    }

    // 添加抖动
    jitter := time.Duration(rand.Float64() * interval * 0.1)
    interval += float64(jitter)

    // 限制最大间隔
    if time.Duration(interval) > e.config.MaxInterval {
        interval = float64(e.config.MaxInterval)
    }

    return time.Duration(interval)
}
```

### 2.6 配置层级

```
┌─────────────────────────────────────────────────────────────┐
│                     配置优先级（从高到低）                   │
├─────────────────────────────────────────────────────────────┤
│  1. ExecuteOptions.RetryConfig     (代码级别)               │
│  2. 命令行 --retry 参数             (用户级别)              │
│  3. 配置文件 ~/.owl/config.yaml    (全局级别)               │
│  4. 默认配置                       (系统级别)               │
└─────────────────────────────────────────────────────────────┘
```

## 3. 代码改动

### 3.1 新增文件

| 文件 | 描述 |
|------|------|
| `internal/control/command/retry.go` | 重试机制核心实现 |
| `internal/control/command/retry_test.go` | 单元测试 |

### 3.2 修改文件

| 文件 | 改动点 |
|------|--------|
| `internal/control/command/executor_v2.go` | 添加 RetryConfig 支持 |
| `cmd/cli/cmd/exec/run.go` | 添加 `--retry` 参数 |

### 3.3 详细改动点

#### 3.3.1 internal/control/command/retry.go（新增）

```go
package command

type RetryConfig struct {
    MaxRetries              int
    InitialInterval         time.Duration
    MaxInterval             time.Duration
    BackoffMultiplier       float64
    RetryableErrors        []string
    EnableExponentialBackoff bool
}
```

#### 3.3.2 internal/control/command/executor_v2.go（修改）

```go
type ExecuteOptions struct {
    Parallel    bool
    Timeout     time.Duration
    RetryConfig *RetryConfig  // 新增
    WorkingDir  string
    Env         map[string]string
}

func (e *Executor) Run(ctx context.Context, nodeIDs []string, command string, opts *ExecuteOptions) []CommandResult {
    if opts != nil && opts.RetryConfig != nil {
        return e.runWithRetry(ctx, nodeIDs, command, opts)
    }
    // 原逻辑
}
```

#### 3.3.3 cmd/cli/cmd/exec/run.go（修改）

```go
var (
    retryCount      int
    retryInterval   time.Duration
    retryMaxInterval time.Duration
)

func init() {
    runCmd.Flags().IntVar(&retryCount, "retry", 3, "Max retry attempts")
    runCmd.Flags().DurationVar(&retryInterval, "retry-interval", 1*time.Second, "Initial retry interval")
    runCmd.Flags().DurationVar(&retryMaxInterval, "retry-max-interval", 30*time.Second, "Max retry interval")
}
```

## 4. CLI 使用示例

```bash
# 使用默认重试配置（3次重试）
owl exec run "apt update" --retry

# 自定义重试次数
owl exec run "apt upgrade" --retry=5

# 自定义重试间隔
owl exec run "service restart" --retry=3 --retry-interval=2s --retry-max-interval=60s

# 禁用重试
owl exec run "dangerous_command" --no-retry

# 显示重试详情
owl exec run "curl api" --retry -v
# 输出示例:
# [Node: web-01] Attempt 1/3: success
# [Node: db-01] Attempt 1/3: failed (connection refused)
# [Node: db-01] Attempt 2/3: failed (timeout)
# [Node: db-01] Attempt 3/3: success
```

## 5. 风险评估

### 5.1 向后兼容性

| 影响项 | 风险等级 | 说明 | 缓解措施 |
|--------|----------|------|----------|
| ExecuteOptions 字段 | 🟢 低 | 添加了新字段，原字段仍有效 | 向后兼容 |
| 执行结果格式 | 🟡 中 | RetryResult 扩展了 CommandResult | 添加兼容字段 |
| CLI 参数 | 🟢 低 | 添加了新参数 | 向后兼容 |

### 5.2 功能风险

| 风险 | 等级 | 描述 | 影响 |
|------|------|------|------|
| 幂等性问题 | ⚠️ 高 | 非幂等命令重试可能导致问题 | 添加幂等性检测 |
| 资源耗尽 | 🟡 中 | 频繁重试可能耗尽资源 | 添加全局限流 |
| 惊群效应 | 🟡 中 | 多节点同时重试可能造成负载 | 添加抖动 |
| 超时叠加 | 🟡 中 | 重试间隔可能导致总超时 | 设置总超时限制 |

### 5.3 性能影响

| 影响项 | 风险等级 | 说明 |
|--------|----------|------|
| 重试延迟 | 🟡 中 | 最坏情况增加 MaxRetries * MaxInterval |
| 内存占用 | 🟢 低 | RetryHistory 占用少量内存 |

### 5.4 测试覆盖

需要添加以下测试用例：

```go
func TestRetrySuccess(t *testing.T) {
    // 测试首次成功
}

func TestRetryOnFailure(t *testing.T) {
    // 测试失败后重试成功
}

func TestMaxRetriesExceeded(t *testing.T) {
    // 测试达到最大重试次数
}

func TestNonRetryableError(t *testing.T) {
    // 测试不可重试错误
}

func TestExponentialBackoff(t *testing.T) {
    // 测试指数退避
}

func TestJitter(t *testing.T) {
    // 测试抖动
}

func TestContextCancellation(t *testing.T) {
    // 测试上下文取消
}
```

## 6. 实施计划

### 6.1 阶段划分

| 阶段 | 任务 | 预计工作量 |
|------|------|------------|
| 阶段 1 | 创建 RetryConfig 和错误类型 | 0.5 天 |
| 阶段 2 | 实现重试核心逻辑 | 2 天 |
| 阶段 3 | 添加重试历史记录 | 0.5 天 |
| 阶段 4 | 添加 CLI 参数 | 0.5 天 |
| 阶段 5 | 添加单元测试 | 1 天 |
| 阶段 6 | 集成测试 | 1 天 |

**总预计工作量：约 5.5 个工作日**

### 6.2 实施顺序

```
阶段1 ──▶ 阶段2 ──▶ 阶段3 ──▶ 阶段4 ──▶ 阶段5 ──▶ 阶段6
   │         │         │         │         │         │
   ▼         ▼         ▼         ▼         ▼         ▼
创建类型   实现核心   历史记录   CLI参数   单元测试   集成测试
           逻辑
```

### 6.3 关键路径

```
用户命令 ──▶ 解析参数 ──▶ 构建 RetryConfig ──▶ 执行重试循环
                              │
                              ▼
                       首次执行 ──▶ 成功? ──▶ 返回结果
                              │
                              ▼ (失败)
                         可重试? ──▶ 否 ──▶ 返回错误
                              │
                              ▼ (是)
                         等待间隔 ──▶ 重试 ──▶ 循环
```

## 7. 与超时分离的交互

重试机制与超时分离设计配合使用时：

```go
// 推荐组合配置
opts := &ExecuteOptions{
    Timeout: 60 * time.Second,
    TimeoutConfig: &TimeoutConfig{
        ConnectTimeout: 10 * time.Second,
        CommandTimeout: 30 * time.Second,
    },
    RetryConfig: &RetryConfig{
        MaxRetries:      3,
        InitialInterval: 1 * time.Second,
        MaxInterval:     30 * time.Second,
    },
}
```

**执行时间预算**：
- 单次执行：ConnectTimeout (10s) + CommandTimeout (30s) = 40s
- 最坏情况：40s + 3 * 30s = 130s

## 8. 回滚方案

1. **参数开关**：添加 `--no-retry` 禁用重试
2. **环境变量**：`OWL_DISABLE_RETRY=1` 禁用重试
3. **智能降级**：检测到资源紧张时自动禁用重试

## 9. 文档更新

需要更新以下文档：

- [ ] `docs/usage/EXEC.md` - 添加重试配置说明
- [ ] `docs/configuration/README.md` - 添加重试配置示例
- [ ] 命令行帮助 - 添加重试参数说明
