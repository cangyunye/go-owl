# Linux Go语言分布式运维工具 - 实现方案

## 1. 项目概述

本项目是一个基于Go语言开发的Linux端分布式运维工具，提供节点管理、批量命令执行、脚本执行、剧本流程执行以及文件传输与获取等核心功能。其中，文件传输采用自扩散（P2P-like）方案，优化大文件批量传输效率。

## 2. 核心功能模块

### 2.1 节点管理模块
- 节点注册与认证
- 节点状态监控（在线/离线、CPU、内存、磁盘）
- 节点分组管理
- 节点标签系统
- 节点元数据存储

### 2.2 批量命令执行模块
- 并行命令执行
- 执行超时控制
- 实时输出捕获
- 执行结果汇总与展示
- 错误处理与重试机制

### 2.3 批量Shell脚本传输与执行模块
- 脚本上传到控制节点
- 脚本分发到目标节点
- 脚本执行权限设置
- 执行环境变量配置
- 执行日志记录

### 2.4 剧本流程执行模块（类似Ansible Playbook）
- YAML/JSON格式剧本定义
- 任务编排与顺序控制
- 条件执行（when语句）
- 循环执行（with_items）
- 变量与模板系统
- 任务钩子（pre_tasks/post_tasks）
- 错误处理策略（ignore_errors/any_errors_fatal）

### 2.5 文件传输与获取模块（核心自扩散方案）
- 文件上传（控制节点 → 目标节点）
- 文件下载（目标节点 → 控制节点）
- 目录传输支持
- 断点续传
- 自扩散批量传输（重点功能）

## 3. 自扩散文件传输方案设计

### 3.1 设计目标
- 减少控制节点带宽压力
- 提高大规模节点文件传输效率
- 支持动态调整扩散树结构
- 实时监控传输状态

### 3.2 核心概念

#### 3.2.1 扩散树结构
- **根节点**：控制节点
- **源节点**：已完成文件接收且具备传输能力的节点
- **叶子节点**：最终接收节点

#### 3.2.2 关键参数
- `k`：每个源节点最多同时传输给的子节点数量（扇出系数）
- `max_depth`：扩散树最大深度
- `threshold`：启用自扩散的节点数量阈值（小于此值使用传统方式）

### 3.3 传输流程

#### 3.3.1 任务初始化
1. 控制节点接收文件传输请求
2. 检查目标节点数量，判断是否启用自扩散
3. 若启用自扩散，构建初始扩散树
4. 生成唯一传输任务ID

#### 3.3.2 扩散树构建算法
```
函数 build_diffusion_tree(nodes, k):
    创建队列，初始包含控制节点
    tree = {root: [], level: 0}
    current_sources = [control_node]
    remaining_nodes = nodes
    
    while remaining_nodes 不为空:
        next_sources = []
        for source in current_sources:
            children = 取 remaining_nodes 的前k个节点
            tree[source] = children
            next_sources.extend(children)
            remaining_nodes = remaining_nodes[k:]
            if remaining_nodes 为空:
                break
        current_sources = next_sources
    
    return tree
```

#### 3.3.3 传输执行流程
1. 控制节点向第一层源节点发送文件
2. 每个源节点接收完成后，自动作为新的传输源
3. 源节点向分配给自己的子节点发送文件
4. 递归执行，直到所有节点接收完成
5. 所有节点向控制节点汇报传输状态

#### 3.3.4 子任务分配
- 控制节点为每个源节点生成子任务
- 子任务包含：
  - 子任务ID
  - 父任务ID
  - 目标节点列表
  - 文件元数据（大小、哈希、路径）
  - 传输超时时间
  - 重试策略

### 3.4 状态监控与报告

#### 3.4.1 传输状态定义
- `pending`：等待传输
- `transferring`：传输中
- `completed`：传输完成
- `failed`：传输失败
- `retrying`：重试中

#### 3.4.2 状态汇报机制
- 每个节点每5秒向控制节点汇报进度
- 传输完成/失败立即汇报
- 支持查询任意节点的传输状态

#### 3.4.3 全局状态聚合
控制节点维护全局传输状态表：
```go
type TransferStatus struct {
    TaskID      string
    FileMeta    FileMetadata
    Tree        DiffusionTree
    NodeStatus  map[string]NodeTransferStatus
    StartTime   time.Time
    EndTime     time.Time
    OverallStatus string
}

type NodeTransferStatus struct {
    NodeID      string
    ParentID    string
    Children    []string
    Status      string
    Progress    float64 // 0.0 - 1.0
    StartTime   time.Time
    EndTime     time.Time
    Error       string
}
```

### 3.5 容错机制

#### 3.5.1 节点故障处理
- 若源节点传输失败，将其子节点重新分配给其他可用源节点
- 动态调整扩散树结构
- 记录失败日志，支持手动重试

#### 3.5.2 传输重试
- 单个文件块传输失败重试3次
- 整文件传输失败重试2次
- 指数退避重试策略

#### 3.5.3 数据完整性校验
- 使用SHA-256校验文件完整性
- 传输完成后自动校验
- 校验失败自动重试

## 4. 系统架构设计

### 4.1 整体架构

```
┌─────────────────────────────────────────────────────────────┐
│                         控制节点 (Control Node)              │
├─────────────────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │  CLI接口     │  │  API接口     │  │  WebUI(可选) │      │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘      │
│         │                 │                 │               │
│  ┌──────▼─────────────────▼─────────────────▼───────┐      │
│  │              任务调度引擎                        │      │
│  ├─────────────────────────────────────────────────┤      │
│  │  节点管理器  │  命令执行器  │  剧本执行器  │      │
│  ├─────────────────────────────────────────────────┤      │
│  │              文件传输引擎                        │      │
│  │  (含自扩散调度器)                               │      │
│  └─────────────────────────────────────────────────┘      │
│         │                 │                 │               │
│  ┌──────▼─────────────────▼─────────────────▼───────┐      │
│  │              通信层 (gRPC/TCP)                   │      │
│  └─────────────────────────────────────────────────┘      │
└─────────────────────────────────────────────────────────────┘
                          │
                          │ gRPC/TCP
                          │
        ┌─────────────────┼─────────────────┐
        │                 │                 │
   ┌────▼────┐      ┌────▼────┐      ┌────▼────┐
   │ 节点1   │      │ 节点2   │      │ 节点N   │
   └─────────┘      └─────────┘      └─────────┘
   (Agent)          (Agent)          (Agent)
```

### 4.2 核心组件

#### 4.2.1 控制节点组件
- **Task Scheduler**：任务调度与分发
- **Node Manager**：节点生命周期管理
- **Command Executor**：批量命令执行
- **Playbook Engine**：剧本流程解析与执行
- **File Transfer Engine**：文件传输协调（含自扩散调度）
- **Status Monitor**：全局状态监控与聚合

#### 4.2.2 节点Agent组件
- **Agent Core**：Agent主进程
- **Command Runner**：本地命令执行
- **Script Executor**：脚本执行环境
- **File Transfer Agent**：文件收发（含作为源节点的传输能力）
- **Heartbeat**：心跳保持

### 4.3 通信协议
- 使用gRPC作为主要通信协议
- 支持双向流式传输（用于实时日志、进度汇报）
- 文件传输支持自定义TCP协议（优化传输效率）

## 5. 数据模型设计

### 5.1 节点模型
```go
type Node struct {
    ID          string            `json:"id"`
    Name        string            `json:"name"`
    Address     string            `json:"address"`
    Port        int               `json:"port"`
    Status      string            `json:"status"` // online/offline
    Groups      []string          `json:"groups"`
    Labels      map[string]string `json:"labels"`
    Metadata    map[string]string `json:"metadata"`
    CreatedAt   time.Time         `json:"created_at"`
    UpdatedAt   time.Time         `json:"updated_at"`
}
```

### 5.2 任务模型
```go
type Task struct {
    ID          string            `json:"id"`
    Type        string            `json:"type"` // command/script/playbook/file_transfer
    Targets     []string          `json:"targets"`
    Payload     interface{}       `json:"payload"`
    Status      string            `json:"status"`
    CreatedAt   time.Time         `json:"created_at"`
    UpdatedAt   time.Time         `json:"updated_at"`
}
```

### 5.3 剧本模型
```go
type Playbook struct {
    Name        string            `json:"name"`
    Hosts       []string          `json:"hosts"`
    Vars        map[string]interface{} `json:"vars"`
    Tasks       []Task            `json:"tasks"`
    PreTasks    []Task            `json:"pre_tasks"`
    PostTasks   []Task            `json:"post_tasks"`
}

type PlaybookTask struct {
    Name        string            `json:"name"`
    Action      string            `json:"action"`
    Args        map[string]interface{} `json:"args"`
    When        string            `json:"when"`
    WithItems   []interface{}     `json:"with_items"`
    IgnoreErrors bool             `json:"ignore_errors"`
}
```

## 6. 技术栈选型

| 组件 | 技术选型 | 说明 |
|------|----------|------|
| 开发语言 | Go 1.21+ | 并发性能好，部署简单 |
| 通信协议 | gRPC + Protocol Buffers | 高性能，强类型 |
| 数据存储 | SQLite/MySQL | 节点信息、任务记录 |
| 配置管理 | Viper | 支持多格式配置 |
| 日志 | Zap | 高性能日志 |
| CLI | Cobra | 命令行界面 |
| 剧本解析 | YAML.v3 | 解析YAML格式剧本 |
| 文件校验 | SHA-256 | 数据完整性校验 |

## 7. 项目目录结构

```
go-distributed-tool/
├── cmd/
│   ├── control/          # 控制节点主程序
│   └── agent/            # 节点Agent主程序
├── internal/
│   ├── control/          # 控制节点逻辑
│   │   ├── node/         # 节点管理
│   │   ├── task/         # 任务调度
│   │   ├── command/      # 命令执行
│   │   ├── playbook/     # 剧本执行
│   │   └── transfer/     # 文件传输（含自扩散）
│   ├── agent/            # Agent逻辑
│   │   ├── command/      # 命令执行
│   │   ├── script/       # 脚本执行
│   │   └── transfer/     # 文件收发
│   ├── proto/            # Protocol Buffers定义
│   └── common/           # 公共组件
├── pkg/                  # 可导出的库
├── api/                  # API定义
├── scripts/              # 辅助脚本
├── configs/              # 配置文件
├── test/                 # 测试文件
├── go.mod
├── go.sum
└── README.md
```

## 8. 核心API设计

### 8.1 gRPC服务定义

#### 8.1.1 控制节点服务
```protobuf
service ControlService {
    // 节点管理
    rpc RegisterNode(RegisterNodeRequest) returns (RegisterNodeResponse);
    rpc ListNodes(ListNodesRequest) returns (ListNodesResponse);
    
    // 任务执行
    rpc ExecuteCommand(ExecuteCommandRequest) returns (ExecuteCommandResponse);
    rpc ExecuteScript(ExecuteScriptRequest) returns (ExecuteScriptResponse);
    rpc ExecutePlaybook(ExecutePlaybookRequest) returns (ExecutePlaybookResponse);
    
    // 文件传输
    rpc TransferFile(TransferFileRequest) returns (TransferFileResponse);
    rpc GetTransferStatus(GetTransferStatusRequest) returns (GetTransferStatusResponse);
    
    // 流式输出
    rpc StreamTaskOutput(StreamTaskOutputRequest) returns (stream TaskOutput);
}
```

#### 8.1.2 Agent服务
```protobuf
service AgentService {
    // 心跳
    rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
    
    // 命令执行
    rpc RunCommand(RunCommandRequest) returns (RunCommandResponse);
    
    // 文件传输
    rpc ReceiveFile(stream FileChunk) returns (ReceiveFileResponse);
    rpc SendFile(SendFileRequest) returns (stream FileChunk);
    
    // 自扩散传输
    rpc StartDiffusionTransfer(StartDiffusionTransferRequest) returns (StartDiffusionTransferResponse);
}
```

## 9. 实现方法与测试策略

### 9.1 开发原则

1. **测试驱动开发（TDD）**：每个功能模块先编写测试用例，再实现功能
2. **模块独立性**：每个模块应可独立编译、测试
3. **渐进式实现**：按模块顺序开发，每完成一个模块需通过单元测试
4. **可测试性设计**：代码设计需考虑可测试性，使用接口和依赖注入

### 9.2 模块实现顺序

| 序号 | 模块 | 优先级 | 依赖关系 | 验收标准 |
|------|------|--------|----------|----------|
| 1 | 项目基础框架 | 高 | 无 | 项目可编译运行 |
| 2 | 节点数据模型 | 高 | 无 | 数据模型可序列化 |
| 3 | 节点管理器 | 高 | 节点模型 | CRUD操作测试通过 |
| 4 | 任务调度器 | 高 | 节点管理 | 任务分发测试通过 |
| 5 | 命令执行器 | 中 | 任务调度 | 命令执行结果正确 |
| 6 | 脚本传输执行 | 中 | 命令执行 | 脚本传输执行成功 |
| 7 | 基础文件传输 | 中 | 无 | 文件完整传输 |
| 8 | 扩散树构建 | 高 | 节点管理 | 树结构正确 |
| 9 | 自扩散传输 | 高 | 扩散树、文件传输 | 多节点扩散成功 |
| 10 | 剧本解析器 | 中 | 无 | YAML解析正确 |
| 11 | 剧本执行引擎 | 低 | 剧本解析、命令执行 | 剧本流程正确执行 |

### 9.3 测试策略

#### 9.3.1 单元测试规范

```bash
# 测试文件命名规范
模块_test.go

# 测试函数命名规范
Test<被测函数名>_<场景描述>

# 示例
TestNodeManager_CreateNode
TestNodeManager_CreateNode_WithDuplicateID
TestDiffusionTree_Build_WithKValue
```

#### 9.3.2 测试覆盖率要求

| 模块 | 最低覆盖率 | 说明 |
|------|-----------|------|
| 数据模型 | 90% | 所有字段序列化/反序列化 |
| 核心逻辑 | 80% | 业务逻辑代码 |
| 工具函数 | 90% | 辅助函数 |
| 协议处理 | 70% | gRPC消息处理 |

#### 9.3.3 Mock策略

- 使用 `gomock` 生成接口Mock
- 使用 `testify` 的 mock 子包
- 避免测试依赖外部服务

```bash
# 安装测试依赖
go install github.com/golang/mock/mockgen@latest
go get github.com/stretchr/testify
```

### 9.4 每个模块的测试用例模板

#### 模块1：项目基础框架
**测试用例**：
- [ ] `TestConfig_Load`：配置文件加载测试
- [ ] `TestLogger_Init`：日志初始化测试
- [ ] `TestProject_Build`：项目编译测试

#### 模块2：节点数据模型
**测试用例**：
- [ ] `TestNode_New`：节点创建测试
- [ ] `TestNode_JSONSerialize`：节点JSON序列化/反序列化测试
- [ ] `TestNode_Validation`：节点数据校验测试

#### 模块3：节点管理器
**测试用例**：
- [ ] `TestNodeManager_Register`：节点注册测试
- [ ] `TestNodeManager_Unregister`：节点注销测试
- [ ] `TestNodeManager_List`：节点列表查询测试
- [ ] `TestNodeManager_GetByID`：按ID获取节点测试
- [ ] `TestNodeManager_GetByGroup`：按分组获取节点测试
- [ ] `TestNodeManager_GetByLabels`：按标签获取节点测试
- [ ] `TestNodeManager_UpdateStatus`：节点状态更新测试

#### 模块4：任务调度器
**测试用例**：
- [ ] `TestTaskScheduler_Create`：任务创建测试
- [ ] `TestTaskScheduler_Dispatch`：任务分发测试
- [ ] `TestTaskScheduler_Cancel`：任务取消测试
- [ ] `TestTaskScheduler_GetStatus`：任务状态查询测试
- [ ] `TestTaskScheduler_Parallelism`：并发调度测试

#### 模块5：命令执行器
**测试用例**：
- [ ] `TestCommandExecutor_Execute`：命令执行测试
- [ ] `TestCommandExecutor_Timeout`：命令超时测试
- [ ] `TestCommandExecutor_Output`：命令输出捕获测试
- [ ] `TestCommandExecutor_Error`：命令错误处理测试

#### 模块6：脚本传输执行
**测试用例**：
- [ ] `TestScriptTransfer_Upload`：脚本上传测试
- [ ] `TestScriptTransfer_Download`：脚本下载测试
- [ ] `TestScriptExecutor_Execute`：脚本执行测试
- [ ] `TestScriptExecutor_Env`：环境变量测试

#### 模块7：基础文件传输
**测试用例**：
- [ ] `TestFileTransfer_SmallFile`：小文件传输测试（<1MB）
- [ ] `TestFileTransfer_LargeFile`：大文件传输测试（>100MB）
- [ ] `TestFileTransfer_Integrity`：文件完整性校验测试
- [ ] `TestFileTransfer_Resume`：断点续传测试
- [ ] `TestFileTransfer_Directory`：目录传输测试

#### 模块8：扩散树构建
**测试用例**：
- [ ] `TestDiffusionTree_Build`：基础扩散树构建测试
- [ ] `TestDiffusionTree_Build_WithK`：指定k值的扩散树构建测试
- [ ] `TestDiffusionTree_Build_WithDepth`：限制深度的扩散树构建测试
- [ ] `TestDiffusionTree_Unequal`：不均匀节点分布测试
- [ ] `TestDiffusionTree_SingleNode`：单节点场景测试
- [ ] `TestDiffusionTree_Empty`：空节点列表测试

#### 模块9：自扩散传输
**测试用例**：
- [ ] `TestDiffusionTransfer_Init`：扩散传输初始化测试
- [ ] `TestDiffusionTransfer_SubTask`：子任务生成测试
- [ ] `TestDiffusionTransfer_Status`：状态汇报测试
- [ ] `TestDiffusionTransfer_Progress`：进度计算测试
- [ ] `TestDiffusionTransfer_Failure`：节点故障处理测试
- [ ] `TestDiffusionTransfer_Reassign`：子节点重分配测试
- [ ] `TestDiffusionTransfer_Recovery`：传输恢复测试

#### 模块10：剧本解析器
**测试用例**：
- [ ] `TestPlaybookParser_Parse`：基础剧本解析测试
- [ ] `TestPlaybookParser_Vars`：变量解析测试
- [ ] `TestPlaybookParser_Tasks`：任务列表解析测试
- [ ] `TestPlaybookParser_When`：条件语句解析测试
- [ ] `TestPlaybookParser_WithItems`：循环语句解析测试
- [ ] `TestPlaybookParser_PrePostTasks`：钩子任务解析测试
- [ ] `TestPlaybookParser_Invalid`：无效剧本处理测试

#### 模块11：剧本执行引擎
**测试用例**：
- [ ] `TestPlaybookExecutor_Run`：剧本执行测试
- [ ] `TestPlaybookExecutor_Sequence`：任务顺序执行测试
- [ ] `TestPlaybookExecutor_Condition`：条件执行测试
- [ ] `TestPlaybookExecutor_Loop`：循环执行测试
- [ ] `TestPlaybookExecutor_IgnoreError`：错误忽略测试
- [ ] `TestPlaybookExecutor_FatalError`：致命错误处理测试

## 10. 实现里程碑

### 阶段一：基础框架
- [ ] 项目初始化与目录结构搭建
- [ ] gRPC服务定义与代码生成
- [ ] 节点注册与心跳机制
- [ ] 基础CLI框架

### 阶段二：核心功能
- [ ] 批量命令执行
- [ ] 脚本传输与执行
- [ ] 基础文件传输（点对点）

### 阶段三：自扩散传输
- [ ] 扩散树构建算法
- [ ] 子任务分配与调度
- [ ] 状态监控与报告
- [ ] 容错处理

### 阶段四：剧本系统
- [ ] 剧本YAML解析
- [ ] 任务编排引擎
- [ ] 变量与模板系统
- [ ] 条件与循环执行

### 阶段五：完善与优化
- [ ] WebUI（可选）
- [ ] 性能优化
- [ ] 文档完善
- [ ] 测试覆盖

## 11. 风险与应对

| 风险 | 影响 | 应对措施 |
|------|------|----------|
| 自扩散树构建不合理导致传输效率低 | 高 | 支持动态调整k值，提供多种树构建算法 |
| 网络分区导致扩散树断裂 | 中 | 实现网络分区检测，自动重构扩散树 |
| 大文件传输占用过多内存 | 中 | 使用流式传输，分块处理 |
| 节点故障导致任务失败 | 中 | 完善的重试机制和故障转移策略 |

## 12. 后续扩展方向

- 支持容器化部署
- 集成监控告警系统
- 支持插件系统
- 多租户支持
- 审计日志与合规性
