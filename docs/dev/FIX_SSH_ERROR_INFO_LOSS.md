# SSH 退出码 255 错误信息丢失问题修复方案

## 问题描述

当前代码中，当系统 `ssh` 命令执行失败（如认证失败、连接拒绝等），退出码 255 对应的详细错误信息在错误传递链中被吞掉，导致用户只看到模糊的"命令执行失败，退出码 255"，而无法区分是认证失败、连接超时还是密钥不存在。

## 受影响文件

| 文件 | 行号 | 问题 |
|------|------|------|
| `internal/ssh/executor_factory.go` | L77-L81 | `ExitError` 被当成功处理，error 返回 nil |
| `internal/ssh/executor_factory.go` | L125-L128 | 同上，`ExecuteWithConfig` 中同样问题 |
| `internal/control/command/executor_v2.go` | L273-L287 | 接收到的 exitCode=255 被误判为 ErrorTypeCommand |
| `internal/control/command/executor_v2.go` | L345-L355 | `containsAny` 实现有 bug，无法匹配子串 |

## 根因分析

### 问题点 1：`ExitError` 被当成功处理

在 [executor_factory.go:77-81](file:///Volumes/ORICO2T/Users/sinvigil/Programming/owl/go-owl/internal/ssh/executor_factory.go#L77-L81)：

```go
if err != nil {
    if exitErr, ok := err.(*exec.ExitError); ok {
        return exitErr.ExitCode(), output, nil  // ← bug: error 被设为 nil！
    }
    return -1, output, err
}
```

系统 `ssh` 在认证失败时退出码为 255，且 stderr 中包含 "Permission denied (publickey)." 等关键信息。但这里把 `err` 设为了 `nil`，上层收到 `exitCode=255, error=nil`，**完全无法区分"命令执行完毕但退出码非零"和"SSH 认证失败"两种情况**。

### 问题点 2：退出码 255 被误判为 Command 错误

在 [executor_v2.go:273-287](file:///Volumes/ORICO2T/Users/sinvigil/Programming/owl/go-owl/internal/control/command/executor_v2.go#L273-L287)：

```go
if execErr != nil {
    // 这里 execErr 是 nil（因为问题1把 error 吞了），所以走不到这里
    ...
}

// 走到了这里
if exitCode != 0 {
    result.ErrorType = ErrorTypeCommand  // ← 255 被误判为命令错误
    result.ErrorDetail = fmt.Sprintf("命令执行失败，退出码 %d", exitCode)  // ← "命令执行失败，退出码 255"
}
```

SSH 退出码 255 的含义是 "SSH 连接/认证失败"，不是命令执行失败。应该映射为 `ErrorTypeAuth` 或 `ErrorTypeConnection`。

### 问题点 3：`containsAny` 子串匹配有 bug

在 [executor_v2.go:345-355](file:///Volumes/ORICO2T/Users/sinvigil/Programming/owl/go-owl/internal/control/command/executor_v2.go#L345-L355)：

```go
func containsAny(s string, substrs ...string) bool {
    for _, substr := range substrs {
        if len(substr) <= len(s) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr) {
            return true
        }
        if len(substr) <= len(s) && (s[:len(substr)] == substr || contains(s, substr)) {
            return true
        }
    }
    return false
}
```

这个方法试图在字符串 `s` 中查找子串 `substr`，但实现非常低效且容易出错。例如搜索 `"auth"` 时，第一个条件检查 s 是否**以** "auth" 开头或结尾，只有匹配不上时才会调用 `contains()` 做真正的子串搜索。但即使如此，`contains()` 的循环实现也是手写的，效率低下。

## 修复方案

### 修改 1：`executor_factory.go` — 保留退出码 255 的原始错误

**改动点：`Execute` 方法 (L77-L81)**

```go
// 修改前
if exitErr, ok := err.(*exec.ExitError); ok {
    return exitErr.ExitCode(), output, nil  // 错误被吞掉
}

// 修改后
if exitErr, ok := err.(*exec.ExitError); ok {
    // 退出码 255 表示 SSH 连接/认证失败，保留错误信息
    if exitErr.ExitCode() == 255 {
        return exitErr.ExitCode(), output, fmt.Errorf("SSH 连接失败 (exit code 255): %s", strings.TrimSpace(output))
    }
    // 其他非零退出码是远程命令执行结果，不算 Go 层面的错误
    return exitErr.ExitCode(), output, nil
}
```

**改动点：`ExecuteWithConfig` 方法 (L125-L128)** — 同上的逻辑。

### 修改 2：`executor_v2.go` — 退出码 255 映射为认证/连接错误

**改动点：`runOnNode` 方法 (L289-L299)**

```go
// 修改前
if exitCode != 0 {
    result.ErrorType = ErrorTypeCommand
    result.ErrorDetail = fmt.Sprintf("命令执行失败，退出码 %d", exitCode)
}

// 修改后
if exitCode != 0 {
    if exitCode == 255 {
        // 255 是 SSH 特有的退出码，表示连接/认证失败
        result.ErrorType = ErrorTypeAuth
        if output != "" && (strings.Contains(output, "timeout") || strings.Contains(output, "refused")) {
            result.ErrorType = ErrorTypeConnection
        }
        result.ErrorDetail = fmt.Sprintf("SSH 连接失败（退出码 255）: %s", truncateOutput(output, 512))
        result.Success = false
        result.Error = fmt.Errorf("SSH 连接失败，退出码 255")
    } else {
        result.ErrorType = ErrorTypeCommand
        result.ErrorDetail = fmt.Sprintf("命令执行失败，退出码 %d", exitCode)
    }
}
```

### 修改 3：简化 `containsAny` 实现

```go
func containsAny(s string, substrs ...string) bool {
    sLower := strings.ToLower(s)
    for _, substr := range substrs {
        if strings.Contains(sLower, strings.ToLower(substr)) {
            return true
        }
    }
    return false
}
```

### 修改 4：在输出层显示有意义的错误

**在 `run.go` 的 simple 输出格式中 (L332-L362)**，当 `result.ErrorType` 为 `ErrorTypeAuth` 时，输出更友好的提示：

```
❌ [node-1] 失败
   类型: 认证失败
   详情: SSH 连接失败（退出码 255）: Permission denied (publickey).
   💡 建议: 请检查用户名、密码或密钥配置
```

## 修复后效果对比

| 场景 | 修复前用户看到 | 修复后用户看到 |
|------|---------------|---------------|
| 密钥不存在 | 命令执行失败，退出码 255 | SSH 连接失败（退出码 255）: Permission denied (publickey). → 建议检查密钥 |
| 密码错误 | 命令执行失败，退出码 255 | SSH 连接失败（退出码 255）: Authentication failed. → 建议检查密码 |
| 连接被拒绝 | 命令执行失败，退出码 255 | SSH 连接失败（退出码 255）: Connection refused → 建议检查网络 |
| 连接超时 | 超时 | 超时（保持不变） |
| 命令本身退出码非零 | 命令执行失败，退出码 1 | 命令执行失败，退出码 1（保持不变） |
