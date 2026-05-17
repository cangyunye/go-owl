# owl 操作架构整改方案

## 1. 问题分析

### 1.1 当前设计问题

根据用户反馈，当前 owl 存在以下设计缺陷：

| 问题 | 描述 | 影响 |
|------|------|------|
| 文件传输需要先连接会话 | `owl file upload` 前必须执行 `owl session attach` | 用户体验差，步骤繁琐 |
| 会话管理与操作耦合 | 文件传输和命令执行都依赖会话状态 | 架构复杂，维护困难 |
| 无自动连接机制 | 执行命令时如果没有会话，需要手动连接 | 操作流程不流畅 |

### 1.2 问题根源

当前设计将节点连接与会话管理强耦合：

```
┌─────────────────────────────────────────────────────────────┐
│                    当前架构                                │
├─────────────────────────────────────────────────────────────┤
│  用户操作                系统流程                          │
│  ──────────────────────────────────────────────────────────│
│  添加节点 ──→ 连接会话 ──→ 文件传输/命令执行              │
│                            ↑                              │
│                   必须先建立会话                          │
└─────────────────────────────────────────────────────────────┘
```

---

## 2. 整改目标

### 2.1 设计原则

1. **文件传输无需会话**：`owl file upload/download` 直接读取节点信息进行传输
2. **命令执行自动连接**：`owl exec` 如果没有会话，自动建立临时连接
3. **会话解耦**：文件传输与会话管理解耦，各独立运行

### 2.2 整改后架构

```
┌─────────────────────────────────────────────────────────────┐
│                    整改后架构                              │
├─────────────────────────────────────────────────────────────┤
│                                                           │
│  ┌──────────────┐    ┌──────────────┐                     │
│  │   文件传输   │    │   命令执行   │                     │
│  │ (file ops)   │    │  (exec)      │                     │
│  └──────┬───────┘    └──────┬───────┘                     │
│         │                   │                              │
│         │ 直接读取节点       │ 自动连接/复用会话            │
│         ↓                   ↓                              │
│  ┌───────────────────────────────────────┐                │
│  │           节点管理器 (Node Manager)    │                │
│  │     - 存储节点信息                     │                │
│  │     - 维护连接状态                     │                │
│  │     - 提供连接池                      │                │
│  └───────────────────────────────────────┘                │
│                                                           │
└─────────────────────────────────────────────────────────────┘
```

---

## 3. 整改方案

### 3.1 文件传输模块整改

**核心变更**：文件传输不再依赖会话，直接从节点管理器获取节点信息进行传输。

```go
// internal/control/transfer/diffusion_transfer.go

package transfer

import (
    "context"
    "fmt"
    "time"

    "github.com/cangyunye/go-owl/internal/common/model"
    "github.com/cangyunye/go-owl/internal/node"
    "github.com/cangyunye/go-owl/internal/ssh"
)

// TransferManager 文件传输管理器
type TransferManager struct {
    nodeManager   *node.Manager
    executorFactory *ssh.ExecutorFactory
}

// NewTransferManager 创建传输管理器
func NewTransferManager(nodeManager *node.Manager) *TransferManager {
    return &TransferManager{
        nodeManager:   nodeManager,
        executorFactory: ssh.NewExecutorFactory(),
    }
}

// UploadFile 上传文件到指定节点（无需会话）
func (tm *TransferManager) UploadFile(ctx context.Context, nodeIDs []string, localPath, remotePath string) error {
    // 1. 从节点管理器获取节点信息
    nodes, err := tm.nodeManager.GetNodesByIDs(nodeIDs)
    if err != nil {
        return fmt.Errorf("获取节点信息失败: %w", err)
    }

    // 2. 并行上传到所有节点
    errors := make(chan error, len(nodes))
    for _, node := range nodes {
        go func(n model.Node) {
            errors <- tm.uploadToNode(ctx, &n, localPath, remotePath)
        }(node)
    }

    // 3. 收集错误
    var errs []error
    for i := 0; i < len(nodes); i++ {
        if err := <-errors; err != nil {
            errs = append(errs, err)
        }
    }

    if len(errs) > 0 {
        return fmt.Errorf("部分节点上传失败: %v", errs)
    }
    return nil
}

// uploadToNode 上传文件到单个节点
func (tm *TransferManager) uploadToNode(ctx context.Context, node *model.Node, localPath, remotePath string) error {
    // 创建临时 SSH 连接（用完即释放）
    executor, err := tm.executorFactory.Create(node)
    if err != nil {
        return fmt.Errorf("连接节点 %s 失败: %w", node.ID, err)
    }
    defer executor.Close()

    // 执行文件上传
    err = executor.UploadFile(ctx, localPath, remotePath)
    if err != nil {
        return fmt.Errorf("上传失败: %w", err)
    }

    return nil
}

// DownloadFile 从节点下载文件（无需会话）
func (tm *TransferManager) DownloadFile(ctx context.Context, nodeID, remotePath, localPath string) error {
    node, err := tm.nodeManager.GetNodeByID(nodeID)
    if err != nil {
        return fmt.Errorf("获取节点信息失败: %w", err)
    }

    executor, err := tm.executorFactory.Create(node)
    if err != nil {
        return fmt.Errorf("连接节点失败: %w", err)
    }
    defer executor.Close()

    return executor.DownloadFile(ctx, remotePath, localPath)
}
```

### 3.2 命令执行模块整改

**核心变更**：命令执行自动管理连接生命周期。

```go
// internal/control/command/executor.go

package command

import (
    "context"
    "fmt"
    "sync"
    "time"

    "github.com/cangyunye/go-owl/internal/common/model"
    "github.com/cangyunye/go-owl/internal/node"
    "github.com/cangyunye/go-owl/internal/session"
    "github.com/cangyunye/go-owl/internal/ssh"
)

// CommandExecutor 命令执行器
type CommandExecutor struct {
    nodeManager    *node.Manager
    sessionManager *session.Manager
    executorFactory *ssh.ExecutorFactory
    sessionCache   sync.Map // nodeID -> executor
}

// NewCommandExecutor 创建命令执行器
func NewCommandExecutor(nodeManager *node.Manager, sessionManager *session.Manager) *CommandExecutor {
    return &CommandExecutor{
        nodeManager:    nodeManager,
        sessionManager: sessionManager,
        executorFactory: ssh.NewExecutorFactory(),
    }
}

// ExecuteCommand 执行命令（自动管理连接）
func (ce *CommandExecutor) ExecuteCommand(ctx context.Context, nodeIDs []string, command string) ([]CommandResult, error) {
    results := make([]CommandResult, 0, len(nodeIDs))
    errors := make(chan executeResult, len(nodeIDs))

    for _, nodeID := range nodeIDs {
        go func(id string) {
            result, err := ce.executeOnNode(ctx, id, command)
            errors <- executeResult{nodeID: id, result: result, err: err}
        }(nodeID)
    }

    for i := 0; i < len(nodeIDs); i++ {
        res := <-errors
        if res.err != nil {
            results = append(results, CommandResult{
                NodeID:    res.nodeID,
                Error:     res.err.Error(),
                Success:   false,
            })
        } else {
            results = append(results, *res.result)
        }
    }

    return results, nil
}

func (ce *CommandExecutor) executeOnNode(ctx context.Context, nodeID string, command string) (*CommandResult, error) {
    // 1. 获取节点信息
    node, err := ce.nodeManager.GetNodeByID(nodeID)
    if err != nil {
        return nil, fmt.Errorf("获取节点失败: %w", err)
    }

    // 2. 尝试获取现有会话的执行器
    executor, err := ce.getOrCreateExecutor(node)
    if err != nil {
        return nil, fmt.Errorf("创建执行器失败: %w", err)
    }

    // 3. 执行命令
    output, err := executor.Execute(ctx, command)
    if err != nil {
        // 如果连接断开，尝试重新连接
        executor, retryErr := ce.executorFactory.Create(node)
        if retryErr != nil {
            return nil, fmt.Errorf("连接失败: %w", err)
        }
        output, err = executor.Execute(ctx, command)
    }

    return &CommandResult{
        NodeID:    nodeID,
        Output:    string(output),
        Success:   err == nil,
        Error:     "",
    }, nil
}

// getOrCreateExecutor 获取或创建执行器
func (ce *CommandExecutor) getOrCreateExecutor(node *model.Node) (ssh.Executor, error) {
    // 优先从缓存获取
    if cached, ok := ce.sessionCache.Load(node.ID); ok {
        executor := cached.(ssh.Executor)
        if executor.IsConnected() {
            return executor, nil
        }
        // 连接已断开，移除缓存
        ce.sessionCache.Delete(node.ID)
    }

    // 创建新执行器
    executor, err := ce.executorFactory.Create(node)
    if err != nil {
        return nil, err
    }

    // 缓存执行器
    ce.sessionCache.Store(node.ID, executor)

    return executor, nil
}

type executeResult struct {
    nodeID string
    result *CommandResult
    err    error
}
```

### 3.3 命令行接口整改

**文件传输命令**：移除会话依赖

```go
// cmd/cli/cmd/file/upload.go

package file

import (
    "context"
    "fmt"
    "os"

    "github.com/spf13/cobra"

    "github.com/cangyunye/go-owl/internal/control/transfer"
    "github.com/cangyunye/go-owl/internal/node"
)

func NewUploadCommand() *cobra.Command {
    var nodeIDs []string
    var localPath string
    var remotePath string

    cmd := &cobra.Command{
        Use:   "upload",
        Short: "上传文件到节点",
        Long:  "直接上传文件到指定节点，无需预先建立会话",
        RunE: func(cmd *cobra.Command, args []string) error {
            // 直接获取节点管理器，无需会话
            nodeManager := node.NewManager()
            transferManager := transfer.NewTransferManager(nodeManager)

            ctx := context.Background()
            err := transferManager.UploadFile(ctx, nodeIDs, localPath, remotePath)
            if err != nil {
                return fmt.Errorf("上传失败: %w", err)
            }

            fmt.Println("文件上传成功")
            return nil
        },
    }

    cmd.Flags().StringArrayVarP(&nodeIDs, "nodes", "n", []string{}, "目标节点 ID（多个用逗号分隔）")
    cmd.Flags().StringVarP(&localPath, "local", "l", "", "本地文件路径")
    cmd.Flags().StringVarP(&remotePath, "remote", "r", "", "远程文件路径")

    cmd.MarkFlagRequired("nodes")
    cmd.MarkFlagRequired("local")
    cmd.MarkFlagRequired("remote")

    return cmd
}
```

**命令执行命令**：自动连接

```go
// cmd/cli/cmd/exec/run.go

package exec

import (
    "context"
    "fmt"

    "github.com/spf13/cobra"

    "github.com/cangyunye/go-owl/internal/control/command"
    "github.com/cangyunye/go-owl/internal/node"
    "github.com/cangyunye/go-owl/internal/session"
)

func NewRunCommand() *cobra.Command {
    var nodeIDs []string
    var commandStr string

    cmd := &cobra.Command{
        Use:   "run",
        Short: "在节点上执行命令",
        Long:  "在指定节点上执行命令，自动管理连接",
        RunE: func(cmd *cobra.Command, args []string) error {
            // 初始化组件
            nodeManager := node.NewManager()
            sessionManager := session.NewManager()
            executor := command.NewCommandExecutor(nodeManager, sessionManager)

            ctx := context.Background()
            results, err := executor.ExecuteCommand(ctx, nodeIDs, commandStr)
            if err != nil {
                return fmt.Errorf("执行失败: %w", err)
            }

            // 输出结果
            for _, result := range results {
                fmt.Printf("[%s] ", result.NodeID)
                if result.Success {
                    fmt.Printf("成功:\n%s\n", result.Output)
                } else {
                    fmt.Printf("失败: %s\n", result.Error)
                }
            }

            return nil
        },
    }

    cmd.Flags().StringArrayVarP(&nodeIDs, "nodes", "n", []string{}, "目标节点 ID")
    cmd.Flags().StringVarP(&commandStr, "command", "c", "", "要执行的命令")

    cmd.MarkFlagRequired("nodes")
    cmd.MarkFlagRequired("command")

    return cmd
}
```

---

## 4. 连接池优化

### 4.1 连接池设计

```go
// internal/ssh/connection_pool.go

package ssh

import (
    "sync"
    "time"

    "github.com/cangyunye/go-owl/internal/common/model"
)

// ConnectionPool SSH 连接池
type ConnectionPool struct {
    pool      sync.Map // nodeID -> *poolEntry
    maxIdle   int
    idleTimeout time.Duration
    mu        sync.Mutex
}

type poolEntry struct {
    executor Executor
    lastUsed time.Time
}

func NewConnectionPool(maxIdle int, idleTimeout time.Duration) *ConnectionPool {
    pool := &ConnectionPool{
        maxIdle:   maxIdle,
        idleTimeout: idleTimeout,
    }

    // 启动清理 goroutine
    go pool.cleanup()

    return pool
}

// Get 获取或创建连接
func (p *ConnectionPool) Get(node *model.Node) (Executor, error) {
    if entry, ok := p.pool.Load(node.ID); ok {
        e := entry.(*poolEntry)
        
        // 检查是否超时
        if time.Since(e.lastUsed) < p.idleTimeout {
            e.lastUsed = time.Now()
            return e.executor, nil
        }

        // 超时则关闭并移除
        e.executor.Close()
        p.pool.Delete(node.ID)
    }

    // 创建新连接
    executor, err := NewExecutor(node)
    if err != nil {
        return nil, err
    }

    p.pool.Store(node.ID, &poolEntry{
        executor: executor,
        lastUsed: time.Now(),
    })

    return executor, nil
}

// cleanup 定期清理过期连接
func (p *ConnectionPool) cleanup() {
    ticker := time.NewTicker(p.idleTimeout / 2)
    defer ticker.Stop()

    for range ticker.C {
        p.pool.Range(func(key, value interface{}) bool {
            entry := value.(*poolEntry)
            if time.Since(entry.lastUsed) >= p.idleTimeout {
                entry.executor.Close()
                p.pool.Delete(key)
            }
            return true
        })
    }
}

// Close 关闭所有连接
func (p *ConnectionPool) Close() {
    p.pool.Range(func(key, value interface{}) bool {
        entry := value.(*poolEntry)
        entry.executor.Close()
        p.pool.Delete(key)
        return true
    })
}
```

---

## 5. 整改后的操作流程

### 5.1 文件上传流程

```
用户执行: owl file upload --nodes node1,node2 --local /path/to/file --remote /tmp/

┌─────────────────────────────────────────────────────────────────┐
│ 1. 解析命令参数                                                │
│    - nodes: [node1, node2]                                    │
│    - local: /path/to/file                                     │
│    - remote: /tmp/                                            │
├─────────────────────────────────────────────────────────────────┤
│ 2. 从节点管理器获取节点信息                                     │
│    - node1: {ID: node1, Address: 192.168.1.10, ...}         │
│    - node2: {ID: node2, Address: 192.168.1.11, ...}         │
├─────────────────────────────────────────────────────────────────┤
│ 3. 并行创建临时 SSH 连接并上传                                 │
│    - 对每个节点创建独立连接                                    │
│    - 上传完成后自动释放连接                                    │
├─────────────────────────────────────────────────────────────────┤
│ 4. 返回结果                                                   │
│    - 成功/失败状态                                             │
│    - 错误信息（如有）                                          │
└─────────────────────────────────────────────────────────────────┘
```

### 5.2 命令执行流程

```
用户执行: owl exec run --nodes node1 --command "uptime"

┌─────────────────────────────────────────────────────────────────┐
│ 1. 解析命令参数                                                │
│    - nodes: [node1]                                           │
│    - command: uptime                                          │
├─────────────────────────────────────────────────────────────────┤
│ 2. 检查连接池                                                 │
│    ├─ 有缓存连接 ──→ 复用连接                                  │
│    └─ 无缓存连接 ──→ 创建新连接并缓存                          │
├─────────────────────────────────────────────────────────────────┤
│ 3. 执行命令                                                   │
│    - 如果连接断开，自动重连                                    │
│    - 返回命令输出                                              │
├─────────────────────────────────────────────────────────────────┤
│ 4. 保持连接在池中（用于后续操作）                               │
│    - 空闲超时后自动释放                                        │
└─────────────────────────────────────────────────────────────────┘
```

---

## 6. 测试验证

### 6.1 测试用例

| 测试用例 | 步骤 | 预期结果 |
|---------|------|---------|
| TC-FILE-001 | 添加节点后直接执行 upload | 上传成功，无需先连接会话 |
| TC-FILE-002 | 上传到多个节点 | 并行上传，各自独立连接 |
| TC-FILE-003 | 上传到不存在的节点 | 返回明确错误信息 |
| TC-EXEC-001 | 无会话状态下执行命令 | 自动创建连接，执行成功 |
| TC-EXEC-002 | 重复执行命令 | 复用连接，无需重新连接 |
| TC-EXEC-003 | 连接断开后执行命令 | 自动重连并执行 |
| TC-CACHE-001 | 空闲超时测试 | 连接自动释放 |

### 6.2 验证命令

```bash
# 文件传输测试（无需会话）
owl node add mynode --address 192.168.1.10 --user root
owl file upload --nodes mynode --local ./file.txt --remote /tmp/

# 命令执行测试（自动连接）
owl exec run --nodes mynode --command "uptime"
owl exec run --nodes mynode --command "df -h"  # 复用连接

# 查看连接状态
owl session list
```

---

## 7. 向后兼容性

### 7.1 兼容原有会话功能

保留 `owl session attach` 命令，用于交互式会话场景：

```bash
# 交互式会话（仍支持）
owl session attach mynode
# 在会话中可以执行多个命令，保持连接
```

### 7.2 配置迁移

无需配置变更，自动适配新架构。

---

## 8. 总结

### 整改要点

| 变更项 | 整改前 | 整改后 |
|--------|--------|--------|
| 文件传输 | 需要先连接会话 | 直接读取节点信息传输 |
| 命令执行 | 需要先连接会话 | 自动连接/复用连接 |
| 连接管理 | 手动管理 | 连接池自动管理 |
| 用户体验 | 多步骤操作 | 单命令完成 |

### 优势

1. **简化操作流程**：添加节点后即可直接传输文件
2. **自动连接管理**：无需手动管理会话生命周期
3. **连接复用**：提高命令执行效率
4. **资源优化**：空闲连接自动释放