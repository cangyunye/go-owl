# 设计文档 1: 连接超时与命令超时分离

## 1. 概述

### 1.1 问题描述

当前 go-owl 项目使用单一的 timeout 参数同时控制连接超时和命令执行超时，这在实际使用中存在以下问题：

1. **难以区分错误类型**：无法判断是"连接不上"还是"命令执行慢"
2. **用户体验差**：连接超时和命令执行超时返回相同的错误信息
3. **现有节点状态管理**：Node 子命令已有 ping/check/update 能力，无需在 exec 阶段重复

### 1.2 当前实现

```go
// 当前实现：使用同一个 timeout
func (e *RemoteNodeExecutorWithInfo) Execute(command string, timeout time.Duration) (int, string, error) {
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()

    // SSH 连接和命令执行共用一个 context
    args := e.connInfo.BuildSSHCommand(command)
    cmd := exec.CommandContext(ctx, "ssh", args...)

    err := cmd.Run()
    // ...
}
```

### 1.3 目标

将连接超时和命令执行超时分离，提供更细粒度的超时控制。

**设计原则**：
- 利用现有 `node ping/check` 的连接检测能力
- 利用现有节点状态（`Node.Status`）避免重复连接探测
- 单次 SSH 调用完成命令执行，不额外增加开销

## 2. 设计方案

### 2.1 超时配置结构

```go
// TimeoutConfig 超时配置
type TimeoutConfig struct {
    // ConnectTimeout 连接建立超时（默认 10 秒）
    ConnectTimeout time.Duration

    // CommandTimeout 命令执行超时（默认 30 秒）
    CommandTimeout time.Duration
}

// DefaultTimeoutConfig 默认超时配置
func DefaultTimeoutConfig() TimeoutConfig {
    return TimeoutConfig{
        ConnectTimeout: 10 * time.Second,
        CommandTimeout: 30 * time.Second,
    }
}
```

### 2.2 错误类型定义

```go
// TimeoutType 超时类型
type TimeoutType string

const (
    // TimeoutConnect 连接超时
    TimeoutConnect TimeoutType = "connect"
    // TimeoutCommand 命令执行超时
    TimeoutCommand TimeoutType = "command"
)

// TimeoutError 超时错误
type TimeoutError struct {
    Type    TimeoutType
    NodeID  string
    Timeout time.Duration
    Cause   error
}

func (e *TimeoutError) Error() string {
    return fmt.Sprintf("%s timeout after %v for node %s", e.Type, e.Timeout, e.NodeID)
}

func (e *TimeoutError) Unwrap() error {
    return e.Cause
}
```

### 2.3 执行策略（单次 SSH 调用）

利用 SSH 的 `-o ConnectTimeout` 和命令超时，**单次 SSH 调用**实现超时分离：

```go
func (e *RemoteNodeExecutorWithInfo) ExecuteWithConfig(command string, config *TimeoutConfig) (int, string, error) {
    if config == nil {
        config = &DefaultTimeoutConfig()
    }

    // 1. 设置 SSH 参数
    args := []string{
        "-o", fmt.Sprintf("ConnectTimeout=%d", int(config.ConnectTimeout.Seconds())),
        "-o", "BatchMode=yes",
        "-o", "StrictHostKeyChecking=no",
    }

    // 2. 构建完整命令
    sshArgs := e.connInfo.BuildSSHCommand(command)
    args = append(args, sshArgs...)

    // 3. 执行命令，总超时 = ConnectTimeout + CommandTimeout
    totalTimeout := config.ConnectTimeout + config.CommandTimeout
    ctx, cancel := context.WithTimeout(context.Background(), totalTimeout)
    defer cancel()

    cmd := exec.CommandContext(ctx, "ssh", args...)

    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    err := cmd.Run()
    output := stdout.String()
    if stderr.Len() > 0 {
        output += "\n" + stderr.String()
    }

    // 4. 解析超时类型
    if err != nil {
        if exitErr, ok := err.(*exec.ExitError); ok {
            // 命令执行完成但返回非零退出码
            return exitErr.ExitCode(), output, nil
        }

        // 判断超时类型
        if ctx.Err() == context.DeadlineExceeded {
            // 总超时，需要进一步区分是连接超时还是命令执行超时
            // 通过 stderr 或退出信号判断
            errMsg := strings.ToLower(output)
            if strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "connect") {
                return -1, output, &TimeoutError{
                    Type:    TimeoutConnect,
                    NodeID:  e.connInfo.NodeID,
                    Timeout: config.ConnectTimeout,
                    Cause:   err,
                }
            }
            return -1, output, &TimeoutError{
                Type:    TimeoutCommand,
                NodeID:  e.connInfo.NodeID,
                Timeout: config.CommandTimeout,
                Cause:   err,
            }
        }

        return -1, output, err
    }

    return 0, output, nil
}
```

### 2.4 简化方案（备选）

利用 Go 1.18+ 的 `context` 能力，通过底层库检测超时阶段：

```go
func (e *RemoteNodeExecutorWithInfo) ExecuteWithConfig(command string, config *TimeoutConfig) (int, string, error) {
    if config == nil {
        config = &DefaultTimeoutConfig()
    }

    // 设置连接超时
    connArgs := []string{
        "-o", fmt.Sprintf("ConnectTimeout=%d", int(config.ConnectTimeout.Seconds())),
        "-o", "BatchMode=yes",
    }

    // 执行命令
    sshArgs := e.connInfo.BuildSSHCommand(command)
    allArgs := append(connArgs, sshArgs...)

    cmd := exec.Command("ssh", allArgs...)

    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    // 使用独立的 context 计时
    startTime := time.Now()
    err := cmd.Start()
    if err != nil {
        return -1, "", &TimeoutError{
            Type:    TimeoutConnect,
            NodeID:  e.connInfo.NodeID,
            Timeout: config.ConnectTimeout,
            Cause:   err,
        }
    }

    // 等待命令完成或超时
    done := make(chan error, 1)
    go func() {
        done <- cmd.Wait()
    }()

    select {
    case err := <-done:
        elapsed := time.Since(startTime)
        output := stdout.String()
        if stderr.Len() > 0 {
            output += "\n" + stderr.String()
        }

        if err != nil {
            if exitErr, ok := err.(*exec.ExitError); ok {
                return exitErr.ExitCode(), output, nil
            }
            return -1, output, err
        }

        return 0, output, nil

    case <-time.After(config.CommandTimeout):
        cmd.Process.Kill()
        return -1, stderr.String(), &TimeoutError{
            Type:    TimeoutCommand,
            NodeID:  e.connInfo.NodeID,
            Timeout: config.CommandTimeout,
        }
    }
}
```

### 2.5 配置层级

```
┌─────────────────────────────────────────────────────────────┐
│                     配置优先级（从高到低）                   │
├─────────────────────────────────────────────────────────────┤
│  1. ExecuteOptions.TimeoutConfig     (代码级别)               │
│  2. 命令行 --connect-timeout, --command-timeout (用户级别)   │
│  3. 全局默认配置                        (系统级别)            │
└─────────────────────────────────────────────────────────────┘
```

## 3. 代码改动

### 3.1 新增文件

| 文件 | 描述 |
|------|------|
| `internal/ssh/timeout.go` | 超时配置和错误定义 |
| `internal/ssh/timeout_test.go` | 单元测试 |

### 3.2 修改文件

| 文件 | 改动点 |
|------|--------|
| `internal/ssh/executor_factory.go` | 添加 ExecuteWithConfig 方法 |
| `internal/ssh/native_executor.go` | 支持 TimeoutConfig |
| `cmd/cli/cmd/exec/run.go` | 添加 `--connect-timeout`, `--command-timeout` 参数 |

### 3.3 详细改动点

#### 3.3.1 internal/ssh/timeout.go（新增）

```go
package ssh

import "time"

type TimeoutConfig struct {
    ConnectTimeout time.Duration
    CommandTimeout time.Duration
}

func DefaultTimeoutConfig() TimeoutConfig {
    return TimeoutConfig{
        ConnectTimeout: 10 * time.Second,
        CommandTimeout: 30 * time.Second,
    }
}

type TimeoutType string

const (
    TimeoutConnect TimeoutType = "connect"
    TimeoutCommand TimeoutType = "command"
)

type TimeoutError struct {
    Type    TimeoutType
    NodeID  string
    Timeout time.Duration
    Cause   error
}
```

#### 3.3.2 internal/ssh/executor_factory.go（修改）

```go
// ExecuteWithConfig 执行命令（带超时配置）
func (e *RemoteNodeExecutorWithInfo) ExecuteWithConfig(command string, config *TimeoutConfig) (int, string, error) {
    // 实现超时分离
}

// NodeExecutor 节点执行器接口
type NodeExecutor interface {
    // Execute 执行命令（保持向后兼容）
    Execute(command string, timeout time.Duration) (int, string, error)

    // ExecuteWithConfig 执行命令（带超时配置）
    ExecuteWithConfig(command string, config *TimeoutConfig) (int, string, error)
}
```

#### 3.3.3 internal/ssh/native_executor.go（修改）

```go
type ExecuteOptions struct {
    Parallel      bool
    Timeout       time.Duration     // 保留，向后兼容
    TimeoutConfig *TimeoutConfig    // 新增
    WorkingDir    string
    Env           map[string]string
}

func (e *Executor) runOnNode(ctx context.Context, nodeID, command string, opts *ExecuteOptions) CommandResult {
    nodeInfo, err := e.nodeResolver.Resolve(nodeID)
    if err != nil {
        return CommandResult{
            NodeID:   nodeID,
            Output:   "",
            ExitCode: -1,
            Error:    fmt.Errorf("获取节点信息失败: %w", err),
            Success:  false,
        }
    }

    executor, err := e.pool.Get(nodeInfo)
    if err != nil {
        return CommandResult{
            NodeID:   nodeID,
            Output:   "",
            ExitCode: -1,
            Error:    fmt.Errorf("连接节点失败: %w", err),
            Success:  false,
        }
    }
    defer e.pool.Put(nodeID)

    var exitCode int
    var output string
    var execErr error

    if opts != nil && opts.TimeoutConfig != nil {
        exitCode, output, execErr = executor.ExecuteWithConfig(command, opts.TimeoutConfig)
    } else if opts != nil && opts.Timeout > 0 {
        exitCode, output, execErr = executor.Execute(command, opts.Timeout)
    } else {
        exitCode, output, execErr = executor.Execute(command, 30*time.Second)
    }

    if execErr != nil {
        return CommandResult{
            NodeID:   nodeID,
            Output:   output,
            ExitCode: exitCode,
            Error:    execErr,
            Success:  false,
        }
    }

    return CommandResult{
        NodeID:   nodeID,
        Output:   output,
        ExitCode: exitCode,
        Error:    nil,
        Success:  exitCode == 0,
    }
}
```

#### 3.3.4 cmd/cli/cmd/exec/run.go（修改）

```go
var (
    connectTimeoutFlag time.Duration
    commandTimeoutFlag time.Duration
)

func init() {
    runCmd.Flags().DurationVarP(&connectTimeoutFlag, "connect-timeout", "C", 10*time.Second, "SSH connection timeout")
    runCmd.Flags().DurationVarP(&commandTimeoutFlag, "command-timeout", "t", 30*time.Second, "Command execution timeout")
    // 保持原有的 --timeout 参数作为兼容（设置为两者之和）
    runCmd.Flags().DurationVarP(&timeoutFlag, "timeout", "", 0, "Combined timeout (deprecated, use --connect-timeout and --command-timeout)")
}

func buildExecuteOptions() (*command.ExecuteOptions, error) {
    opts := &command.ExecuteOptions{
        Parallel:   parallelFlag,
        WorkingDir: workingDirFlag,
    }

    if connectTimeoutFlag > 0 || commandTimeoutFlag > 0 {
        opts.TimeoutConfig = &ssh.TimeoutConfig{
            ConnectTimeout: connectTimeoutFlag,
            CommandTimeout: commandTimeoutFlag,
        }
    } else if timeoutFlag > 0 {
        // 兼容模式
        opts.Timeout = timeoutFlag
    }

    return opts, nil
}
```

## 4. CLI 使用示例

```bash
# 分别设置连接超时和命令超时
owl exec run "df -h" --connect-timeout=5s --command-timeout=60s

# 简写参数
owl exec run "apt update" -C 5s -t 120s

# 仅设置命令超时，连接使用默认值
owl exec run "sleep 30" --command-timeout=60s

# 兼容原有的 --timeout 参数（设置两者之和）
owl exec run "long_script" --timeout=90s

# 错误提示示例（区分连接超时和命令执行超时）
# [Node: web-01] connect timeout after 5s
# [Node: db-01] command timeout after 60s
```

## 5. 风险评估

### 5.1 向后兼容性

| 影响项 | 风险等级 | 说明 | 缓解措施 |
|--------|----------|------|----------|
| Execute 方法签名 | 🟢 低 | 添加了新方法，原方法保持不变 | 向后兼容 |
| ExecuteOptions 字段 | 🟢 低 | 添加了新字段，原字段仍有效 | 向后兼容 |
| CLI 参数 | 🟢 低 | 添加了新参数，原参数仍有效 | 向后兼容 |

### 5.2 功能风险

| 风险 | 等级 | 描述 | 影响 |
|------|------|------|------|
| 超时类型误判 | 🟡 中 | 通过 stderr 区分超时时可能误判 | 增加更稳健的检测 |

### 5.3 性能影响

| 影响项 | 风险等级 | 说明 |
|--------|----------|------|
| SSH 调用次数 | 🟢 低 | 单次调用，没有额外开销 |
| 内存占用 | 🟢 低 | 无额外内存占用 |

### 5.4 测试覆盖

需要添加以下测试用例：

```go
func TestTimeoutConnect(t *testing.T) {
    // 测试连接超时
}

func TestTimeoutCommand(t *testing.T) {
    // 测试命令执行超时
}

func TestTimeoutErrorType(t *testing.T) {
    // 测试错误类型区分
}

func TestBackwardCompatibility(t *testing.T) {
    // 测试向后兼容性
}
```

## 6. 实施计划

### 6.1 阶段划分

| 阶段 | 任务 | 预计工作量 |
|------|------|------------|
| 阶段 1 | 创建 TimeoutConfig 和错误类型 | 0.5 天 |
| 阶段 2 | 实现 ExecuteWithConfig 方法 | 1.5 天 |
| 阶段 3 | 修改 Executor 支持新配置 | 0.5 天 |
| 阶段 4 | 添加 CLI 参数 | 0.5 天 |
| 阶段 5 | 添加单元测试 | 1 天 |
| 阶段 6 | 集成测试 | 1 天 |

**总预计工作量：约 5 个工作日**

### 6.2 实施顺序

```
阶段1 ──▶ 阶段2 ──▶ 阶段3 ──▶ 阶段4 ──▶ 阶段5 ──▶ 阶段6
   │         │         │         │         │         │
   ▼         ▼         ▼         ▼         ▼         ▼
创建类型   实现核心   集成到    CLI参数   单元测试   集成测试
           方法       Executor
```

## 7. 回滚方案

1. **功能开关**：添加环境变量 `OWL_DISABLE_TIMEOUT_CONFIG=1` 禁用新配置
2. **配置降级**：检测到异常时自动使用原有 timeout 参数
3. **向后兼容**：`--timeout` 参数继续有效

## 8. 文档更新

需要更新以下文档：

- [ ] `docs/usage/EXEC.md` - 添加超时配置说明
- [ ] `docs/configuration/README.md` - 添加超时配置示例
- [ ] 代码注释 - 添加 API 文档

## 9. 与现有 Node 命令的配合

建议的工作流：

```bash
# 1. 先 ping/check 确认节点状态
owl node check --all --update

# 2. 根据状态执行命令
owl exec run "task" --nodes="status=online" --connect-timeout=5s --command-timeout=60s
```
