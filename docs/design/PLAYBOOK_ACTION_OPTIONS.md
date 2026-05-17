# Playbook Action 超时和重试配置设计文档

## 1. 概述

### 1.1 设计目标

- 超时和重试配置作为 **全局默认值**
- Playbook 脚本中的每个 action（如 ssh、sftp、rsync）可以单独配置超时和重试
- 如果 action 未配置，则使用全局默认值
- CLI 参数保留作为全局默认值设置

### 1.2 与 Ansible 的对比

```yaml
# Ansible 风格
- name: Deploy application
  hosts: all
  tasks:
    - name: Run deploy script
      command: /opt/deploy.sh
      timeout: 600          # 任务级超时
      retries: 3           # 任务级重试
      delay: 5              # 重试间隔
      register: deploy_result
```

## 2. 设计方案

### 2.1 全局默认值配置

#### 2.1.1 CLI 全局参数（保持）

```bash
# 全局默认值设置
owl playbook run site.yml \
  --default-connect-timeout=10s \
  --default-command-timeout=5m \
  --default-retry=3 \
  --default-retry-interval=1s
```

#### 2.1.2 Playbook 变量配置

```yaml
# playbook.yml
- name: Deploy Application
  hosts: all
  
  # 全局默认配置
  timeout:
    connect: 10s
    command: 5m
  retry:
    max: 3
    interval: 1s
    max_interval: 30s
  
  pre_tasks:
    - name: Prepare environment
      action: shell apt update
      
  tasks:
    - name: Deploy application
      action: ssh
        cmd: /opt/deploy.sh
        timeout: 10m      # 覆盖全局默认值
        retry: 5          # 覆盖全局默认值
        
    - name: Copy config file
      action: copy
        src: ./config.yml
        dest: /etc/app/config.yml
        # 使用全局默认值
        
    - name: Quick health check
      action: shell curl localhost:8080/health
      timeout: 30s       # 快速任务，超时短
      retry: 1           # 只重试一次
```

### 2.2 Action 配置结构

#### 2.2.1 Action 配置选项

```go
// ActionOptions Action 执行选项
type ActionOptions struct {
    // Timeout 连接超时
    Timeout *TimeoutOption
    
    // Retry 重试配置
    Retry *RetryOption
}

// TimeoutOption 超时配置选项
type TimeoutOption struct {
    // Connect 连接超时（仅 SSH/SFTP 有效）
    Connect time.Duration
    
    // Command 命令执行超时
    Command time.Duration
}

// RetryOption 重试配置选项
type RetryOption struct {
    // Max 最大重试次数
    Max int
    
    // Interval 初始重试间隔
    Interval time.Duration
    
    // MaxInterval 最大重试间隔
    MaxInterval time.Duration
}
```

#### 2.2.2 Action 配置解析

```go
// 支持的 action 配置格式

// 格式 1: 简写（仅命令）
action: shell "echo hello"

// 格式 2: 详细配置（map）
action:
  shell: "echo hello"
  timeout: 30s
  retry: 3

// 格式 3: 完整配置
action:
  ssh:
    cmd: "systemctl restart nginx"
    timeout:
      connect: 5s
      command: 60s
    retry:
      max: 3
      interval: 2s
      max_interval: 30s
```

### 2.3 全局默认值传递

#### 2.3.1 Playbook 全局配置

```go
// ParsedPlaybook 添加全局配置
type ParsedPlaybook struct {
    Name        string
    Hosts       string
    Variables   map[string]interface{}
    
    // 全局超时配置
    DefaultTimeout *TimeoutOption
    
    // 全局重试配置
    DefaultRetry *RetryOption
    
    PreTasks    []*ParsedTask
    Tasks       []*ParsedTask
    PostTasks   []*ParsedTask
}
```

#### 2.3.2 Task 配置解析

```go
// ParsedTask 添加配置选项
type ParsedTask struct {
    Name     string
    Action   string
    Args     map[string]interface{}
    Options  *ActionOptions   // 单个 task 的配置
    Condition *Condition
    Loop     *Loop
}
```

### 2.4 执行器集成

#### 2.4.1 执行流程

```
Playbook 解析
    │
    ├─► 解析全局默认配置 (DefaultTimeout, DefaultRetry)
    │
    ├─► 解析每个 Task 的 ActionOptions
    │       │
    │       └─► 合并：Task Options + 全局默认值
    │
    └─► 执行时使用合并后的配置
```

#### 2.4.2 配置合并逻辑

```go
func mergeActionOptions(taskOpts *ActionOptions, globalOpts *ActionOptions) *ActionOptions {
    result := &ActionOptions{}
    
    // 合并超时配置
    if taskOpts != nil && taskOpts.Timeout != nil {
        result.Timeout = taskOpts.Timeout
    } else if globalOpts != nil && globalOpts.Timeout != nil {
        result.Timeout = globalOpts.Timeout
    } else {
        result.Timeout = &TimeoutOption{
            Connect: 10 * time.Second,
            Command: 5 * time.Minute,
        }
    }
    
    // 合并重试配置
    if taskOpts != nil && taskOpts.Retry != nil {
        result.Retry = taskOpts.Retry
    } else if globalOpts != nil && globalOpts.Retry != nil {
        result.Retry = globalOpts.Retry
    } else {
        result.Retry = nil  // 不重试
    }
    
    return result
}
```

### 2.5 支持的 Action 类型

| Action | 支持超时 | 支持重试 | 说明 |
|--------|----------|----------|------|
| `shell` | ✅ Command | ✅ | 执行 shell 命令 |
| `command` | ✅ Command | ✅ | 执行命令 |
| `ssh` | ✅ Both | ✅ | SSH 远程执行 |
| `sftp` | ✅ Both | ✅ | SFTP 文件传输 |
| `rsync` | ✅ Both | ✅ | Rsync 同步 |
| `copy` | ✅ Both | ✅ | 文件复制 |
| `template` | ✅ Both | ✅ | 模板渲染复制 |
| `service` | ✅ Command | ✅ | 服务管理 |
| `script` | ✅ Command | ✅ | 本地脚本远程执行 |

## 3. 代码改动

### 3.1 新增文件

| 文件 | 描述 |
|------|------|
| `internal/control/playbook/action_options.go` | Action 配置结构定义 |
| `internal/control/playbook/parser.go` | 解析 action 配置 |

### 3.2 修改文件

| 文件 | 改动点 |
|------|--------|
| `internal/control/playbook/parser.go` | 解析全局配置和 task 配置 |
| `internal/control/playbook/executor.go` | 使用 ActionOptions |
| `cmd/cli/cmd/playbook/run.go` | 移除 task 级参数，保留全局默认值参数 |

### 3.3 CLI 参数调整

```go
// run.go 调整后的参数
var (
    pbRunDefaultConnectTimeout  time.Duration  // 全局默认连接超时
    pbRunDefaultCommandTimeout time.Duration  // 全局默认命令超时
    pbRunDefaultRetry         int           // 全局默认重试次数
    pbRunDefaultRetryInterval  time.Duration  // 全局默认重试间隔
    pbRunDefaultRetryMaxInterval time.Duration // 全局默认最大重试间隔
)

// 移除的参数
// --connect-timeout    (不是 task 级参数)
// --command-timeout    (不是 task 级参数)
// --retry             (不是 task 级参数)
```

## 4. Playbook 语法示例

### 4.1 完整示例

```yaml
- name: Production Deployment
  hosts: web_servers
  
  timeout:
    connect: 10s
    command: 5m
  retry:
    max: 3
    interval: 1s
    max_interval: 30s
  
  pre_tasks:
    - name: Check disk space
      action: shell df -h
      timeout: 30s
    
    - name: Stop existing service
      action: service
        name: myapp
        state: stopped
      timeout: 60s
      retry: 2
  
  tasks:
    - name: Deploy application files
      action: copy
        src: ./app/
        dest: /opt/myapp/
        timeout: 10m    # 大文件传输，超时较长
      retry: 3
    
    - name: Run database migration
      action: ssh
        host: db-server
        cmd: /opt/migrate.sh
        timeout:
          connect: 30s
          command: 30m
        retry:
          max: 5
          interval: 10s
    
    - name: Quick health check
      action: shell curl -f http://localhost:8080/health
      timeout: 10s       # 快速检查，短超时
      retry: 1
  
  post_tasks:
    - name: Start service
      action: service
        name: myapp
        state: started
      timeout: 60s
    
    - name: Verify deployment
      action: shell /opt/healthcheck.sh
      timeout: 30s
```

### 4.2 简单示例

```yaml
- name: Simple Deployment
  hosts: all
  
  # 使用默认超时和重试
  tasks:
    - name: Update package cache
      action: shell apt update
      
    - name: Install nginx
      action: shell apt install -y nginx
      retry: 3
```

### 4.3 全局覆盖示例

```yaml
- name: Long Running Tasks
  hosts: all
  
  timeout:
    command: 1h        # 所有任务 1 小时超时
  retry:
    max: 5            # 所有任务最多重试 5 次
  
  tasks:
    - name: Build project
      action: shell make build
      # 继承全局配置: 1h 超时, 5 次重试
    
    - name: Quick status check
      action: shell systemctl status myapp
      timeout: 30s     # 覆盖为短超时
      retry: 0         # 覆盖为不重试
```

## 5. YAML 解析实现

### 5.1 Task 配置解析

```go
func (p *Parser) parseTaskAction(taskMap map[string]interface{}) (string, *ActionOptions, error) {
    options := &ActionOptions{}
    
    for actionName, actionValue := range taskMap {
        if actionName == "name" || actionName == "when" || actionName == "loop" {
            continue
        }
        
        switch v := actionValue.(type) {
        case string:
            // action: shell "echo hello"
            return actionName, nil, nil
            
        case map[string]interface{}:
            // action:
            //   shell: "echo hello"
            //   timeout: 30s
            //   retry: 3
            cmd, opts, err := p.parseActionConfig(actionName, v)
            if err != nil {
                return "", nil, err
            }
            options = opts
            return cmd, options, nil
            
        default:
            return "", nil, fmt.Errorf("unsupported action format: %v", actionValue)
        }
    }
    
    return "", nil, fmt.Errorf("action not found in task")
}

func (p *Parser) parseActionConfig(actionName string, config map[string]interface{}) (string, *ActionOptions, error) {
    options := &ActionOptions{}
    var command string
    
    for key, value := range config {
        switch key {
        case "cmd", "command", "script":
            command = fmt.Sprintf("%v", value)
            
        case "timeout":
            if dur, ok := value.(string); ok {
                options.Timeout = &TimeoutOption{
                    Command: parseDuration(dur),
                }
            } else if m, ok := value.(map[string]interface{}); ok {
                options.Timeout = &TimeoutOption{
                    Connect: parseDuration(getString(m, "connect", "10s")),
                    Command: parseDuration(getString(m, "command", "5m")),
                }
            }
            
        case "retry":
            if n, ok := value.(int); ok {
                options.Retry = &RetryOption{Max: n}
            } else if m, ok := value.(map[string]interface{}); ok {
                options.Retry = &RetryOption{
                    Max:        getInt(m, "max", 3),
                    Interval:   parseDuration(getString(m, "interval", "1s")),
                    MaxInterval: parseDuration(getString(m, "max_interval", "30s")),
                }
            }
        }
    }
    
    return command, options, nil
}
```

## 6. 实施计划

### 6.1 阶段划分

| 阶段 | 任务 | 预计工作量 |
|------|------|------------|
| 阶段 1 | 创建 ActionOptions 结构定义 | 0.5 天 |
| 阶段 2 | 修改 Parser 解析 action 配置 | 1.5 天 |
| 阶段 3 | 修改 Executor 使用 ActionOptions | 1 天 |
| 阶段 4 | 调整 CLI 参数（移除 task 级参数） | 0.5 天 |
| 阶段 5 | 添加单元测试 | 1 天 |
| 阶段 6 | 集成测试 | 1 天 |

**总预计工作量：约 5.5 个工作日**

## 7. 向后兼容性

- 现有的 playbook 脚本无需修改
- 未配置超时/重试的 action 使用默认值
- CLI 全局参数保持向后兼容

## 8. 实现状态

### 8.1 已完成

- ✅ `action_options.go` - ActionOptions、TimeoutOption、RetryOption、PlaybookDefaults 结构定义
- ✅ `action_options_test.go` - 单元测试
- ✅ `parser.go` - 添加 TimeoutConfigYAML、RetryConfigYAML 结构，解析 task 级配置
- ✅ `parser_test.go` - 添加 parseTimeout、parseRetry 单元测试
- ✅ `executor.go` - ActionRunner 接口添加 actionOpts 参数，使用 MergeActionOptions 合并配置
- ✅ `playbook/run.go` - CLI 参数已调整为全局默认格式

### 8.2 文件清单

| 文件 | 状态 |
|------|------|
| `internal/control/playbook/action_options.go` | ✅ |
| `internal/control/playbook/action_options_test.go` | ✅ |
| `internal/control/playbook/parser.go` | ✅ |
| `internal/control/playbook/parser_test.go` | ✅ |
| `internal/control/playbook/executor.go` | ✅ |
| `cmd/cli/cmd/playbook/run.go` | ✅ |

## 9. 文档更新

- [ ] `docs/usage/PLAYBOOK.md` - 添加 action 配置说明
- [ ] `docs/syntax/PLAYBOOK_SYNTAX.md` - 添加超时和重试配置语法
