# Owl 核心模块开发文档

## 1. AI 助手模块

### 1.1 概述

AI 助手模块提供自然语言交互界面，让用户可以通过对话式操作来使用 owl 的各项功能，包括节点管理、命令执行、剧本执行和文件传输等。

### 1.2 核心架构

```
internal/ai/
├── agent.go              # Agent 核心逻辑
├── tools.go              # 工具定义和实现
├── config.go             # 配置管理
├── prompts/              # 提示词模块
│   └── prompts.go        # 提示词定义
└── llm/                  # LLM 客户端
    ├── interface.go      # LLM 接口定义
    ├── openai.go         # OpenAI 客户端
    └── anthropic.go      # Anthropic 客户端
```

### 1.3 核心组件

| 组件 | 文件 | 职责 |
|------|------|------|
| **Agent** | [agent.go](file:///Volumes/ORICO2T/Users/sinvigil/Programming/owl/go-owl/internal/ai/agent.go) | AI 交互核心协调器 |
| **Tools** | [tools.go](file:///Volumes/ORICO2T/Users/sinvigil/Programming/owl/go-owl/internal/ai/tools.go) | 封装各项功能的工具集 |
| **Config** | [config.go](file:///Volumes/ORICO2T/Users/sinvigil/Programming/owl/go-owl/internal/ai/config.go) | AI 配置管理 |

### 1.4 工具实现方式

**当前实现（方案 A）**：直接调用 owl 子命令

```go
func (t *QueryNodesTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
    cmdArgs := []string{"node", "list"}
    output, err := runOwlCommand(ctx, cmdArgs)
    if err != nil {
        return t.fallbackExecute(ctx, args)
    }
    return output, nil
}
```

**未来优化（方案 B）**：直接调用公共逻辑，避免进程启动开销

### 1.5 支持的 Provider

| Provider | 基础 URL | 默认模型 |
|----------|----------|----------|
| openai | https://api.openai.com/v1 | gpt-4o |
| anthropic | https://api.anthropic.com | claude-3.5-sonnet |
| dashscope | https://dashscope.aliyuncs.com | qwen-turbo |

### 1.6 路由系统

AI 工具路由支持的任务类型：
- `node`: 节点管理
- `exec_run`: 执行命令
- `exec_script`: 执行脚本
- `file`: 文件传输
- `playbook_list`: 列出剧本
- `playbook_run`: 执行剧本

---

## 2. Node 节点管理模块

### 2.1 概述

节点管理模块负责节点的注册、认证、状态监控、分组和标签管理。

### 2.2 核心功能

| 功能 | 说明 |
|------|------|
| 节点注册与认证 | 节点加入集群的身份验证 |
| 状态监控 | 在线/离线状态、CPU、内存、磁盘 |
| 分组管理 | 按业务场景分组节点 |
| 标签系统 | 灵活的节点标记和筛选 |
| 元数据存储 | 节点附加信息管理 |

### 2.3 节点解析器

**NodeResolver** 负责节点的查询和解析：

```go
type NodeResolver interface {
    ListNodes(opts *ListOptions) ([]*ResolvedNode, error)
    Resolve(nodeID string) (*ResolvedNode, error)
    SearchByAddress(pattern string) []*model.Node
}
```

### 2.4 节点模型

```go
type ResolvedNode struct {
    ID      string
    Name    string
    Address string
    Port    int
    User    string
    Status  string
    Groups  []string
    Labels  map[string]string
}
```

### 2.5 节点选择机制

支持多种方式选择目标节点：

| 参数 | 说明 | 示例 |
|------|------|------|
| `--nodes` | 指定节点 ID | `--nodes web-01,web-02` |
| `--group` | 按分组选择 | `--group web` |
| `--label` | 按标签选择 | `--label env=prod` |
| `--status` | 按状态选择 | `--status online` |

### 2.6 节点冲突检测

在执行命令前会检查节点配置冲突：

```go
func CheckNodeConflictsBeforeExec() {
    // 检查 nodes.json 与数据库的一致性
    // 如果存在冲突，提示用户使用 --sync-nodes 同步
}
```

---

## 3. Exec 命令执行模块

### 3.1 概述

Exec 模块提供命令和脚本的批量执行能力，支持并行/串行执行、异步执行、重试机制和安全检查。

### 3.2 命令结构

```
exec/
├── exec.go      # 命令入口
├── run.go       # 命令执行
└── script.go    # 脚本执行
```

### 3.3 核心命令

#### 3.3.1 `owl exec run` - 执行命令

```bash
# 基本用法
owl exec run "uptime" --nodes web-01,web-02

# 按分组执行
owl exec run "df -h" --group web

# 并行/串行模式
owl exec run "sleep 5" --parallel   # 默认
owl exec run "sleep 5" --serial     # 串行执行

# 重试机制
owl exec run "curl api.example.com" --retry 3 --retry-interval 2s

# 异步执行
owl exec run "long-running.sh" --async
```

**支持的参数**：

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--nodes` | 目标节点 | 全部节点 |
| `--group` | 目标分组 | - |
| `--label` | 目标标签 | - |
| `--status` | 节点状态筛选 | - |
| `--parallel` | 并行执行 | true |
| `--serial` | 串行执行 | false |
| `--retry` | 重试次数 | 3 |
| `--async` | 异步执行 | false |
| `--timeout` | 执行超时 | 60s |

#### 3.3.2 `owl exec script` - 执行脚本

```bash
# 执行本地脚本
owl exec script deploy.sh --nodes web-01,web-02

# 指定目标目录
owl exec script install.sh --dest /tmp --group web

# 直接内容执行（不留痕迹）
owl exec script init.sh --inline --nodes test-01

# 执行后保留脚本
owl exec script setup.sh --keep --nodes all
```

**支持的参数**：

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--nodes` | 目标节点 | 全部节点 |
| `--group` | 目标分组 | - |
| `--dest` | 目标目录 | /tmp |
| `--args` | 脚本参数 | - |
| `--inline` | 直接内容执行 | false |
| `--keep` | 保留脚本文件 | false |
| `--timeout` | 执行超时 | 5min |

### 3.4 执行流程

#### 3.4.1 命令执行流程

```
用户命令 → 节点解析 → 黑名单检查 → 执行命令 → 记录日志 → 返回结果
```

#### 3.4.2 安全检查机制

**黑名单检查**：在执行前检查命令是否包含危险操作

```go
checker := blacklist.NewChecker(cfg)
result := checker.Check(nodeInfo.User, execmd)
if result.Blocked {
    // 提示用户确认
}
```

**支持的危险命令模式**：
- `rm -rf /`
- 格式化磁盘
- 系统关机命令

### 3.5 执行模式

#### 3.5.1 同步执行（默认）

```go
resultChan := executor.RunStreaming(ctx, targetNodeIDs, execmd, opts)
for result := range resultChan {
    processResult(result)
}
```

#### 3.5.2 异步执行

```go
asyncOpts := &async.AsyncOptions{
    Timeout:      execAsyncTimeout,
    PollInterval: execAsyncPollInterval,
}
tasks, err := executor.RunAsync(ctx, targetNodeIDs, execmd, asyncOpts)
```

#### 3.5.3 并行 vs 串行

```go
isParallel := execParallel && !execSerial
opts := &command.ExecuteOptions{
    Parallel: isParallel,
}
```

### 3.6 重试机制

支持指数退避重试策略：

```go
opts.RetryConfig = &command.RetryConfig{
    MaxRetries:      execRetry,
    InitialInterval: execRetryInterval,
    MaxInterval:     execRetryMaxInterval,
}
```

### 3.7 日志记录

每个命令执行完成后会记录到：
1. **节点日志文件**：`~/.owl/logs/nodes/<nodeID>.log`（完整输出）
2. **SQLite 历史数据库**：`~/.owl/owl.db`（stdout 截断为 4096 字符）

```go
// 节点日志
nodeLogWriter.AppendEntry(result.NodeID, taskID, execmd, 
    result.ExitCode, result.Output, errorMsg, result.Duration)

// 历史记录
history.RecordCommandExecution(&history.CommandExecution{
    TaskID:     taskID,
    NodeID:     result.NodeID,
    Command:    execmd,
    ExitCode:   result.ExitCode,
    Stdout:     truncateOutput(result.Output, 4096),
    DurationMs: result.Duration.Milliseconds(),
    Success:    result.Success,
})
```

### 3.8 输出格式

支持三种输出格式：

| 格式 | 说明 | 示例 |
|------|------|------|
| `simple` | 简洁输出 | `✅ [web-01] 成功` |
| `detail` | 详细输出 | 包含错误类型、建议等 |
| `json` | JSON 格式 | 便于程序处理 |

---

## 4. 跨模块协作

### 4.1 模块依赖关系

```
┌─────────────────────────────────────────────────────────┐
│                      AI Module                          │
│         (自然语言交互入口)                              │
└───────────────────┬───────────────────────────────────┘
                    │ 调用
                    ▼
┌─────────────────────────────────────────────────────────┐
│                    Exec Module                          │
│         (命令/脚本执行核心)                             │
├─────────────────────────────────────────────────────────┤
│  ┌──────────────┐    ┌──────────────┐                  │
│  │  run         │    │  script      │                  │
│  │ 命令执行     │    │ 脚本执行     │                  │
│  └──────┬───────┘    └──────┬───────┘                  │
└─────────┼───────────────────┼──────────────────────────┘
          │                   │
          │ 依赖              │ 依赖
          ▼                   ▼
┌─────────────────────────────────────────────────────────┐
│                    Node Module                          │
│         (节点解析、状态管理)                            │
└─────────────────────────────────────────────────────────┘
```

### 4.2 数据流转

1. **AI → Exec → Node**：
    - AI 解析用户请求
    - 调用 Exec 工具执行命令
    - Exec 通过 NodeResolver 获取目标节点

2. **Exec → Logfile**：
    - 命令执行完成后
    - 写入节点日志文件
    - 记录历史数据库

---

## 5. 配置管理

### 5.1 配置文件路径

```
~/.owl/config.yaml
```

### 5.2 配置结构

```yaml
ai:
  provider: openai
  model: gpt-4o
  api_key: ${OWL_API_KEY}
  base_url: https://api.openai.com/v1
  timeout: 120

exec:
  default_timeout: 60s
  default_retry: 3
  parallel: true

node:
  sync_nodes: false
```

### 5.3 环境变量优先级

1. 环境变量（最高）
2. 命令行参数
3. 配置文件（最低）

---

## 6. 安全特性

### 6.1 危险命令检测

通过黑名单机制防止误操作：
- 支持用户级别的规则配置
- 执行前确认机制
- `--force` 参数跳过检查

### 6.2 日志审计

完整的执行记录：
- 节点级独立日志
- 历史数据库记录
- 支持追溯和审计

### 6.3 超时保护

多层超时控制：
- 连接超时（SSH 连接）
- 命令超时（执行时间）
- 异步任务超时（长时间运行任务）

---

## 7. 扩展能力

### 7.1 新 Provider 支持

```go
// 新增 Provider 需要实现：
1. 实现 LLM 客户端接口
2. 添加到 Providers 映射表
3. 实现模型列表获取函数
```

### 7.2 新工具添加

```go
// 新增 AI 工具需要：
1. 定义工具结构体
2. 实现 Execute 方法
3. 注册到工具列表
4. 添加提示词描述
```

---

**文档版本**: v1.0  
**更新时间**: 2026-06-01  
**适用版本**: go-owl main branch