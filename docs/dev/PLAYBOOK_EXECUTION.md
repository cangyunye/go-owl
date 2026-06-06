# Playbook 执行引擎开发文档

## 1. 概述

Playbook 是 Owl 的剧本编排系统，采用**流水线式步骤执行**模型，支持多节点并行执行、失败即停、断点续跑等核心能力。

## 2. 核心架构

### 2.1 模块结构

```
internal/
└── control/
    └── playbook/
        ├── executor.go    # 执行引擎
        ├── parser.go      # 剧本解析器
        └── types.go       # 类型定义
cmd/
└── cli/
    └── cmd/
        └── playbook/
            └── run.go     # 命令入口
internal/
└── logfile/
    └── writer.go          # 节点日志写入器
```

### 2.2 核心组件

| 组件 | 文件 | 职责 |
|------|------|------|
| **Executor** | [executor.go](file:///Volumes/ORICO2T/Users/sinvigil/Programming/owl/go-owl/internal/control/playbook/executor.go) | 剧本执行引擎，管理任务流转 |
| **ActionRunner** | [executor.go](file:///Volumes/ORICO2T/Users/sinvigil/Programming/owl/go-owl/internal/control/playbook/executor.go#L106-L169) | 动作执行器，支持 script/command/upload/download |
| **NodeLogWriter** | [writer.go](file:///Volumes/ORICO2T/Users/sinvigil/Programming/owl/go-owl/internal/logfile/writer.go) | 节点日志写入器，按节点隔离日志 |
| **Parser** | [parser.go](file:///Volumes/ORICO2T/Users/sinvigil/Programming/owl/go-owl/internal/control/playbook/parser.go) | YAML 剧本解析器 |

## 3. 执行流程

### 3.1 整体流程

```
用户执行命令
       │
       ▼
┌─────────────────────────────────┐
│ 1. 解析 Playbook YAML          │
│    (parser.go)                  │
└─────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────┐
│ 2. 执行 PreTasks               │
│    失败则终止(除非ignore_errors) │
└─────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────┐
│ 3. 执行 Tasks                  │
│    - 支持 any_errors_fatal     │
│    - 支持 failed_when          │
└─────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────┐
│ 4. 执行 PostTasks              │
│    失败则终止(除非ignore_errors) │
└─────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────┐
│ 5. 记录结果到日志和数据库       │
│    (NodeLogWriter + history)    │
└─────────────────────────────────┘
```

### 3.2 执行引擎核心逻辑

#### 3.2.1 Execute 方法

```go
func (e *playbookExecutor) Execute(playbook *ParsedPlaybook, targets []*model.Node, 
    extraVars map[string]interface{}) (*PlaybookExecution, error)
```

**关键设计点**：

1. **状态追踪**：`PlaybookExecution` 记录整个执行过程的状态
2. **变量合并**：剧本变量 + 外部变量合并到 `exec.Vars`
3. **三段式执行**：PreTasks → Tasks → PostTasks

#### 3.2.2 失败处理策略

| 任务类型 | 默认行为 | fail_continue 模式 | pipeline 模式 |
|----------|----------|-------------------|--------------|
| PreTasks | 失败即终止 | 失败即终止 | 失败即终止 |
| Tasks | 继续执行 | 失败继续，最后汇总 | 失败即终止 |
| PostTasks | 失败即终止 | 失败即终止 | 不允许存在 |

**pipeline 模式核心代码**（executor.go 简化）：

```go
for i := range playbook.Tasks {
    if playbook.ExecutionMode == ExecutionModePipeline && exec.Status == ExecutionStatusFailed {
        break
    }
    results, err := e.executeTaskInternal(exec, mainTask)
    if err != nil && !mainTask.Options.IgnoreErrors {
        if playbook.ExecutionMode == ExecutionModePipeline || mainTask.Options.AnyErrorsFatal {
            exec.Status = ExecutionStatusFailed
            exec.Error = err.Error()
            break  // break 退出循环，落入最终状态判定
        }
    }
    exec.Results[mainTask.Name] = append(exec.Results[mainTask.Name], results...)
}
```

#### 3.2.3 多节点并行执行

当目标节点数 > 1 时，自动启用并行执行（executor.go:591-628）：

```go
if len(exec.TargetNodes) > 1 {
    nodeCount := len(exec.TargetNodes)
    resultsChan := make(chan *TaskResult, nodeCount)
    errChan := make(chan error, nodeCount)
    var wg sync.WaitGroup
    wg.Add(nodeCount)

    for _, target := range exec.TargetNodes {
        go func(nodeID string) {
            defer wg.Done()
            itemResults, err := e.executeTaskForNode(exec, task, nodeID, nil)
            if len(itemResults) > 0 {
                resultsChan <- itemResults[0]
            }
            if err != nil {
                errChan <- err
            }
        }(target.ID)
    }
    // ...
}
```

## 3.3 执行模式

### 3.3.1 模式定义

Playbook 支持两种执行模式：

| 模式 | 说明 |
|------|------|
| `fail_continue`（默认） | 所有任务依次执行，失败不阻断，最后汇总失败状态 |
| `pipeline` | 任一任务失败立即终止后续任务 |

模式在 YAML 中通过 `execution_mode` 字段配置，在 `parser.go` 的 `Parse()` 中解析为 `ExecutionMode` 类型。

### 3.3.2 pipeline 模式校验

`validatePlaybook()` 在解析时对 pipeline 模式做三项校验：
1. **PostTasks 必须为空** — pipeline 的语义是"任一失败即终止全部后续"
2. **不允许 `ignore_errors: true`** — pipeline 下所有任务都是 fatal 的
3. **不允许 `any_errors_fatal: true`** — pipeline 本身就是全线 fatal，无需重复声明

### 3.3.3 pipeline 模式终止逻辑

当 `execution_mode: pipeline` 且任务执行错误时：

1. `executeTaskInternal()` 检测 `exec.Playbook.ExecutionMode == ExecutionModePipeline`，将错误向上传播
2. `Execute()` 中 Tasks 循环检查到错误，设置 `exec.Status = ExecutionStatusFailed` 并 `break`
3. PostTasks 不会执行（因 pipeline 模式下 validate 已确保 PostTasks 为空）

### 3.3.4 多节点并行

在多节点并行执行时，pipeline 模式下任一节点失败即终止该任务的所有节点分发，错误向上传播到 Tasks 循环。

### 3.3.5 断点续跑

通过 `--resume` CLI 标志启用：
- 查询 `operations` 表，匹配 `playbook_path` 且 `status = 'failed'` 的最新记录
- 读取 `current_task_phase` 和 `current_task_index`
- 在 `Execute()` 中跳过已完成的阶段和任务索引
- checkpoint 通过回调函数写入历史数据库

## 4. 任务选项

### 4.1 选项定义

```go
type TaskOptions struct {
    IgnoreErrors   bool   // 忽略错误，继续执行
    AnyErrorsFatal bool   // 任何错误都导致剧本终止
    Tags           []string
    Register       string // 注册结果变量名
    ChangedWhen    string // 自定义 changed 判断
    FailedWhen     string // 自定义失败判断
}
```

### 4.2 选项行为矩阵

| 选项 | 值 | 行为 |
|------|----|------|
| `ignore_errors` | `true` | 任务失败不影响后续执行 |
| `ignore_errors` | `false` (默认) | 任务失败可能终止剧本 |
| `any_errors_fatal` | `true` | 任何节点失败立即终止整个剧本 |
| `any_errors_fatal` | `false` (默认) | 单个节点失败不影响其他节点 |
| `failed_when` | 表达式 | 自定义失败条件判断 |

### 4.3 failed_when 机制

允许用户自定义失败判断逻辑（executor.go:653-660）：

```go
if err != nil && task.Options.FailedWhen != "" {
    evaluator := NewConditionEvaluator(taskVars)
    failed, _ := evaluator.Evaluate(&Condition{Expression: task.Options.FailedWhen})
    if !failed {
        result.ExitCode = 0
        err = nil
    }
}
```

**使用示例**：

```yaml
tasks:
  - name: 检查服务状态
    action: command
    args:
      cmd: systemctl is-active nginx
    options:
      failed_when: "{{ result.exit_code != 0 }}"
```

## 5. 日志机制

### 5.1 节点隔离日志

每个节点有独立的日志文件，路径规则：

```
~/.owl/logs/nodes/<nodeID>.log
```

**环境变量配置**：
- `OWL_LOG_DIR`：自定义日志目录

### 5.2 日志写入器

**NodeLogWriter** 核心设计（writer.go:27-50）：

```go
type NodeLogWriter struct {
    logDir string
    mu     sync.Mutex              // 保护 locks map
    locks  map[string]*sync.Mutex  // 每个节点独立锁
}
```

**并发安全保障**：
- 不同节点可并行写入
- 同一节点串行写入（避免日志乱序）

### 5.3 日志条目格式

```
──────────────────────────────────────────────────────────────────────
[2024-01-15 10:30:45] TASK: deploy-app
COMMAND: bash /tmp/install.sh
EXIT CODE: 0
DURATION: 15s
ERROR: connection refused
OUTPUT:
Install completed successfully
──────────────────────────────────────────────────────────────────────
```

### 5.4 日志记录时机

在 [run.go](file:///Volumes/ORICO2T/Users/sinvigil/Programming/owl/go-owl/cmd/cli/cmd/playbook/run.go) 中，每个任务执行完成后立即记录：

```go
nodeLogWriter.AppendEntry(
    result.NodeID,      // 节点ID
    taskName,          // 任务名称
    result.Action,     // 动作类型
    result.ExitCode,   // 退出码
    result.Output,     // 命令输出
    errorMsg,          // 错误信息
    duration           // 耗时
)
```

## 6. 结果汇总

### 6.1 状态判断

**执行完成后**（executor.go:538-553）：

```go
if exec.Status == ExecutionStatusRunning {
    hasFailure := false
    for _, results := range exec.Results {
        for _, result := range results {
            if result.ExitCode != 0 && result.Error != nil {
                hasFailure = true
                break
            }
        }
    }
    if hasFailure {
        exec.Status = ExecutionStatusFailed
    } else {
        exec.Status = ExecutionStatusCompleted
    }
}
```

### 6.2 终端输出

**三种状态输出**（run.go:398-414）：

| 状态 | 图标 | 输出格式 |
|------|------|----------|
| 成功 | ✅ | `✅ [节点名] 任务名 成功` |
| 失败 | ❌ | `❌ [节点名] 任务名 失败: 错误信息` |
| 非零退出码 | ⚠️ | `⚠️ [节点名] 任务名 退出码 N` |

### 6.3 退出码

```go
if failed > 0 {
    os.Exit(1)  // 有失败任务，非零退出
}
// 否则正常退出 (exit 0)
```

## 7. 数据库记录

### 7.1 操作记录

```go
history.RecordOperation(&history.Operation{
    TaskID:    taskID,
    OpType:    "playbook",
    Command:   string(meta),
    Targets:   targetNodeIDs,
    Status:    finalStatus,  // completed / failed / partial_failure
    CreatedAt: startTime,
})
```

### 7.2 命令执行记录

```go
history.RecordCommandExecution(&history.CommandExecution{
    TaskID:     taskID,
    NodeID:     result.NodeID,
    Command:    taskName,
    ExitCode:   result.ExitCode,
    Stdout:     truncateStr(result.Output, 4096),
    Stderr:     errorMsg,
    DurationMs: duration.Milliseconds(),
    Success:    result.ExitCode == 0,
    CreatedAt:  time.Now(),
})
```

## 8. 扩展能力

### 8.1 Action 类型

| Action | 支持 | 说明 |
|--------|------|------|
| `command` / `cmd` / `shell` | ✅ | 执行命令 |
| `script` | ✅ | 执行脚本文件 |
| `upload` | ✅ | 上传文件 |
| `download` | ✅ | 下载文件 |

### 8.2 重试机制

支持配置重试策略（executor.go:419-454）：

```go
func (r *defaultActionRunner) executeWithRetry(nodeID, cmd string, 
    timeout time.Duration, retryConfig *command.RetryConfig) (*task.TaskResult, error)
```

**重试参数**：
- `max_retries`：最大重试次数
- `initial_interval`：初始重试间隔
- `max_interval`：最大重试间隔（指数退避）

## 9. 设计原则

### 9.1 核心设计理念

1. **状态独立追踪**：每个节点的执行状态独立，单个节点失败不影响其他节点
2. **失败可控**：通过 `ignore_errors` 和 `any_errors_fatal` 精细控制失败行为
3. **可观测性**：完整的日志记录 + 数据库持久化，支持问题追溯
4. **幂等性**：支持断点续跑（设计阶段）

### 9.2 安全约束

- **Dry-run 预览**：生产环境强制要求先预览再执行
- **节点数量限制**：超过阈值时警告
- **变更检测**：对比上次执行的差异

## 10. 典型场景示例

### 10.1 关键步骤失败即停

```yaml
tasks:
  - name: 数据库迁移
    action: command
    args:
      cmd: ./migrate.sh
    options:
      any_errors_fatal: true  # 此步骤失败，剧本立即终止

  - name: 部署应用
    action: script
    args:
      script: deploy.sh
```

### 10.2 非关键步骤忽略错误

```yaml
tasks:
  - name: 清理临时文件
    action: command
    args:
      cmd: rm -rf /tmp/*
    options:
      ignore_errors: true  # 清理失败不影响后续执行

  - name: 启动服务
    action: command
    args:
      cmd: systemctl start nginx
```

### 10.3 自定义失败条件

```yaml
tasks:
  - name: 检查API状态
    action: command
    args:
      cmd: curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/health
    options:
      failed_when: "{{ result.exit_code != 0 or result.output != '200' }}"
```

## 11. 代码优化建议

### 11.1 当前实现的改进空间

1. **超时控制**：当前支持全局超时，但建议增加任务级别超时配置
2. **断点续跑**：设计文档中有此功能，但当前实现未完全支持
3. **条件判断**：`when` 条件支持，但建议增强表达式语法

### 11.2 性能优化

- 日志写入采用节点级锁，避免全局锁竞争
- 多节点并行执行，提高吞吐量
- 结果收集使用 channel，避免阻塞

---

**文档版本**: v1.0  
**更新时间**: 2026-06-01  
**适用版本**: go-owl main branch