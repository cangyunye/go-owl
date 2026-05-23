> ✅ **状态：已实现** — 方案 B（crypto/ssh 原生实现）已完全落地，`internal/ssh/native_executor.go` 实现了密钥优先→密码兜底的认证链。

# 密钥→密码认证回退链路缺失问题修复方案

## 问题描述

当前 `owl exec run` 的 SSH 执行链路中，存在两个严重问题：

1. **`nodes.json` 中配置的 `ssh_key` 字段在整个 exec 流程中从未被使用**——`ConnectionPool.Get` 没把 `SSHKey` 传给工厂
2. **密码认证路径完全不存在**——`ConnectionInfo` 没有 `Password` 字段，`BuildSSHCommand` 也不支持密码认证

导致的结果：用户只有在 Linux 上恰好有 `~/.ssh/id_rsa` 且 SSH config 中有正确配置时才能连接，其他任何方式（指定密钥路径、配置密码）都会静默失败。

## 受影响文件及数据链路

```
用户节点配置 (nodes.json)
  ├── SSHKey:  "/home/user/.ssh/my_key"     ← 已配置但丢失
  └── Password: "my_password"                ← 已配置但丢失
       ↓
common.NodeInfo { SSHKey, Password }         ← 字段存在 (cmd/cli/cmd/common/node.go)
       ↓
node.ResolvedNode { SSHKey, SSHPassword }    ← 字段存在 (internal/node/resolver.go)
       ↓
ConnectionPool.Get(nodeInfo *ResolvedNode)   ← ⚠️ 只取了 ID/Address/Port/User，忽略 SSHKey 和 SSHPassword
       ↓
NodeExecutorFactory.GetExecutorForNode(id, addr, port, user)  ← ⚠️ 方法签名就没有 key 和 pwd 参数
       ↓
ResolveConnection(id, addr, port, user, ...) ← ⚠️ 没传 key/pwd，只能从 SSH config 读取
       ↓
ConnectionInfo { User, Address, Port, KeyFile }  ← ⚠️ 没有 Password 字段
       ↓
BuildSSHCommand(command) → []string         ← ⚠️ 只加了 -i key_file，没有密码传递机制
       ↓
exec.Command("ssh", args...)                 ← 系统 ssh 默认只试密钥，密码不会自动尝试
```

## 根因分析

### 问题点 1：`ConnectionInfo` 缺少 `Password` 字段

[connection_manager.go:9-16](file:///Volumes/ORICO2T/Users/sinvigil/Programming/owl/go-owl/internal/ssh/connection_manager.go#L9-L16)

```go
type ConnectionInfo struct {
    User      string
    Address   string
    Port      int
    KeyFile   string
    UseConfig bool
    // ⚠️ 没有 Password 字段
}
```

缺少 `Password` 字段意味着系统 `ssh` 命令无法通过 `sshpass` 获得密码。

### 问题点 2：`GetExecutorForNode` 签名缺少 Key 和 Password 参数

[executor_factory.go:29-30](file:///Volumes/ORICO2T/Users/sinvigil/Programming/owl/go-owl/internal/ssh/executor_factory.go#L29-L30)

```go
func (f *NodeExecutorFactory) GetExecutorForNode(
    nodeID, nodeAddress string, nodePort int, nodeUser string,
) (NodeExecutor, error) {
```

### 问题点 3：`ConnectionPool.Get` 调用时丢弃了 `SSHKey` 和 `SSHPassword`

[connection_pool.go:55-60](file:///Volumes/ORICO2T/Users/sinvigil/Programming/owl/go-owl/internal/ssh/connection_pool.go#L55-L60)

```go
executor, err := p.factory.GetExecutorForNode(
    nodeInfo.ID,
    nodeInfo.Address,
    nodeInfo.Port,
    nodeInfo.User,
    // ⚠️ nodeInfo.SSHKey 和 nodeInfo.SSHPassword 没有传
)
```

## 修复方案

有两种实现路径可以选择：

---

### 方案 A：基于现有 `os/exec` 架构（使用系统 `ssh` + `sshpass`）

**原理**：继续使用系统 `ssh` 二进制，引入 `sshpass` 来传递密码。

#### 修改 1：`ConnectionInfo` 增加 `Password` 字段

[connection_manager.go](file:///Volumes/ORICO2T/Users/sinvigil/Programming/owl/go-owl/internal/ssh/connection_manager.go#L9-L16)

```go
type ConnectionInfo struct {
    User      string
    Address   string
    Port      int
    KeyFile   string
    Password  string    // ← 新增
    UseConfig bool
}
```

#### 修改 2：`ResolveConnection` 增加 KeyFile 和 Password 参数

```go
func ResolveConnection(nodeID, nodeAddress string, nodePort int, nodeUser, nodeKeyFile, nodePassword string, sshConfigPath string) (*ConnectionInfo, error) {
    info := &ConnectionInfo{
        Address:  nodeAddress,
        Port:     nodePort,
        User:     nodeUser,
        KeyFile:  nodeKeyFile,     // ← 从参数传入
        Password: nodePassword,    // ← 从参数传入
    }
    // 如果节点没有配置密钥，才尝试从 SSH config 读取
    if nodeKeyFile == "" {
        // ... 原有的 SSH config 查找逻辑
    }
    return info, nil
}
```

#### 修改 3：`GetExecutorForNode` 增加 KeyFile 和 Password 参数

```go
func (f *NodeExecutorFactory) GetExecutorForNode(
    nodeID, nodeAddress string, nodePort int, nodeUser, nodeKeyFile, nodePassword string,
) (NodeExecutor, error) {
```

#### 修改 4：`BuildSSHCommand` 增加密码认证支持

在 `ConnectionInfo.BuildSSHCommand` 中，如果有 `KeyFile` 存在时添加 `-i` 参数：

```go
func (ci *ConnectionInfo) BuildSSHCommand(command string) []string {
    // ... 现有参数 ...
    if ci.KeyFile != "" {
        args = append(args, "-i", ci.KeyFile)
    }
    // 密码不在这里处理，由上层包装 sshpass
    // ...
}
```

#### 修改 5：`RemoteNodeExecutorWithInfo.Execute` 增加 sshpass 包装

当有密码时，自动生成 sshpass 前缀：

```go
func (e *RemoteNodeExecutorWithInfo) Execute(command string, timeout time.Duration) (int, string, error) {
    cmdLine := "ssh"
    args := e.connInfo.BuildSSHCommand(command)
    
    if e.connInfo.Password != "" {
        // 使用 sshpass 传递密码
        cmdLine = "sshpass"
        args = append([]string{"-p", e.connInfo.Password, "ssh"}, args...)
    }
    // ...
}
```

**方案 A 的缺点：**
- 依赖系统安装 `sshpass`（Linux 上需要 `apt install sshpass` / `yum install sshpass`）
- `sshpass` 在 macOS 上不自带，需要 `brew install hudochenkov/sshpass/sshpass`
- 密码会出现在进程命令行中（有安全风险，可通过 `SSH_ASKPASS` 缓解）

---

### 方案 B：迁移到 `golang.org/x/crypto/ssh` 原生 Go 实现（推荐）

**原理**：放弃系统 `ssh` 二进制方式，全面迁移到 `golang.org/x/crypto/ssh`，这是项目已经存在的依赖（session 模块和 check 模块都在用），能原生支持密钥和密码认证。

#### 修改 1：`ConnectionInfo` 增加 SSH 认证所需的字段

```go
type ConnectionInfo struct {
    User      string
    Address   string
    Port      int
    KeyFile   string
    KeyData   []byte    // ← 密钥内容（预加载）
    Password  string    // ← 密码
    UseConfig bool
}
```

#### 修改 2：新增 `NativeNodeExecutor`——基于 crypto/ssh 的原生执行器

在 `internal/ssh/` 下新增 `native_executor.go`：

```go
type NativeNodeExecutor struct {
    connInfo *ConnectionInfo
}

func (e *NativeNodeExecutor) Execute(command string, timeout time.Duration) (int, string, error) {
    return e.executeWithTimeout(command, timeout)
}

func (e *NativeNodeExecutor) ExecuteWithConfig(command string, config *TimeoutConfig) (int, string, error) {
    timeout := config.ConnectTimeout + config.CommandTimeout
    return e.executeWithTimeout(command, timeout)
}

func (e *NativeNodeExecutor) executeWithTimeout(command string, timeout time.Duration) (int, string, error) {
    addr := fmt.Sprintf("%s:%d", e.connInfo.Address, e.connInfo.Port)

    config := &ssh.ClientConfig{
        User:            e.connInfo.GetUser(),
        HostKeyCallback: ssh.InsecureIgnoreHostKey(),
        Timeout:         timeout,
    }

    // 认证策略：先试密钥，再试密码
    var auths []ssh.AuthMethod

    if e.connInfo.KeyFile != "" {
        signer, err := e.parseKeyFile()
        if err == nil {
            auths = append(auths, ssh.PublicKeys(signer))
        }
    }

    if e.connInfo.Password != "" {
        auths = append(auths, ssh.Password(e.connInfo.Password))
    }

    // 如果没有配置任何认证方式，使用默认密钥
    if len(auths) == 0 {
        signers := e.tryDefaultKeys()
        if len(signers) > 0 {
            auths = append(auths, ssh.PublicKeys(signers...))
        }
    }

    if len(auths) == 0 {
        return -1, "", fmt.Errorf("没有可用的认证方式：请配置 SSH 密钥或密码")
    }

    config.Auth = auths

    client, err := ssh.Dial("tcp", addr, config)
    if err != nil {
        return -1, "", fmt.Errorf("SSH 连接失败: %w", err)
    }
    defer client.Close()

    // 创建会话并执行命令
    session, err := client.NewSession()
    if err != nil {
        return -1, "", fmt.Errorf("创建 SSH 会话失败: %w", err)
    }
    defer session.Close()

    var stdout, stderr bytes.Buffer
    session.Stdout = &stdout
    session.Stderr = &stderr

    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()

    done := make(chan error, 1)
    go func() {
        done <- session.Run(command)
    }()

    select {
    case err := <-done:
        output := stdout.String()
        if stderr.Len() > 0 {
            output += "\n" + stderr.String()
        }
        if err != nil {
            if exitErr, ok := err.(*ssh.ExitError); ok {
                return exitErr.ExitStatus(), output, nil
            }
            return -1, output, err
        }
        return 0, output, nil
    case <-ctx.Done():
        session.Signal(ssh.SIGTERM)
        return -1, "", fmt.Errorf("命令执行超时")
    }
}
```

#### 修改 3：`NodeExecutorFactory.GetExecutorForNode` 返回原生执行器

```go
func (f *NodeExecutorFactory) GetExecutorForNode(
    nodeID, nodeAddress string, nodePort int, nodeUser, nodeKeyFile, nodePassword string,
) (NodeExecutor, error) {
    connInfo, err := ResolveConnection(nodeID, nodeAddress, nodePort, nodeUser, nodeKeyFile, nodePassword, f.sshConfigPath)
    if err != nil {
        return nil, err
    }

    if isLocalNode(nodeAddress) {
        return &LocalNodeExecutor{}, nil
    }

    // 使用原生 SSH 执行器
    return &NativeNodeExecutor{
        connInfo: connInfo,
    }, nil
}
```

#### 修改 4：`ConnectionPool.Get` 传递 SSHKey 和 Password

```go
executor, err := p.factory.GetExecutorForNode(
    nodeInfo.ID,
    nodeInfo.Address,
    nodeInfo.Port,
    nodeInfo.User,
    nodeInfo.SSHKey,      // ← 新增
    nodeInfo.SSHPassword, // ← 新增
)
```

#### 修改 5：保留 `RemoteNodeExecutorWithInfo` 作为兼容选项

现有的基于系统 `ssh` 的执行器可以保留，通过 `--use-system-ssh` 标记选择。默认使用原生 Go 执行器。

## 方案对比

| 维度 | 方案 A（sshpass） | 方案 B（crypto/ssh） |
|------|-------------------|---------------------|
| 额外依赖 | 需安装 `sshpass` | 无（已依赖 crypto/ssh） |
| 跨平台一致性 | 差（sshpass 在各系统上安装方式不同） | **好**（纯 Go，全平台一致） |
| 密码安全 | 差（密码出现在进程命令行，ps 可见） | **好**（密码只在内存中） |
| 错误信息质量 | 依赖解析 ssh stderr 文本 | **好**（原生 Go 错误类型） |
| 认证回退 | 需要手动实现两次调用 | **好**（`ssh.AuthMethod` 切片按顺序尝试） |
| 密钥路径解析 | 需手动处理 `~/.ssh/*` | 可自动尝试 `~/.ssh/id_rsa`, `~/.ssh/id_ed25519` |
| SSH config 支持 | 通过系统 ssh 自动支持 | 需要手动解析（已有 ConfigManager 可用） |
| 已有 Session 模块 | 不相关 | **可直接复用 pattern**（internal/session 已有完整实践） |
| 改造成本 | 中（修改 5 个文件） | 中高（新增 1 文件 + 修改 5 文件） |
| 可维护性 | 低 | **高** |

**推荐方案：B**。原因：
1. `golang.org/x/crypto/ssh` 已经是依赖项
2. session 模块和新的 check 模块都已经在用，有成熟 pattern
3. 避免 `sshpass` 依赖和密码泄露问题
4. 跨平台行为完全一致，解决当前 macOS 通 Linux 不通的问题

## 迁移策略（推荐）

```
Phase 1: 并行实现
  - 新增 NativeNodeExecutor（基于 crypto/ssh）
  - 保持 RemoteNodeExecutorWithInfo 不动
  - GetExecutorForNode 默返回 NativeNodeExecutor
  - 通过环境变量 OWL_USE_SYSTEM_SSH=true 可回退到系统 ssh

Phase 2: 参数贯通
  - ConnectionInfo 增加 Password 字段
  - ConnectionPool.Get 传递 SSHKey 和 SSHPassword
  - ResolveConnection 接受 key/password 参数

Phase 3: 清理（可选）
  - 如果 NativeNodeExecutor 运行稳定
  - 移除 RemoteNodeExecutorWithInfo 和相关代码
  - 移除 RemoteNodeExecutor（remote_executor.go）
```

## 关键修复点汇总

| 文件 | 当前问题 | 修复内容 |
|------|---------|---------|
| `internal/ssh/connection_manager.go` | `ConnectionInfo` 无 `Password`，`ResolveConnection` 不接收 key/password | 增加 `Password`，改签名 |
| `internal/ssh/executor_factory.go` | `GetExecutorForNode` 签名缺少 key/password | 改签名，返回 `NativeNodeExecutor` |
| `internal/ssh/connection_pool.go` | `Get()` 没传 `SSHKey`/`SSHPassword` | 补齐参数传递 |
| **`internal/ssh/native_executor.go`** | **不存在** | **新增文件：基于 crypto/ssh 的执行器** |
| `internal/control/command/executor_v2.go` | 依赖 `pool.Get` 返回的执行器 | 无需改动（接口兼容） |
