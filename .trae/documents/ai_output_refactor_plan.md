# AI 输出重构计划

## 问题分析

目前 AI 模块的输出是重新模仿了实际子命令的输出格式，存在以下问题：

1. **维护成本高**：每次修改 owl 子命令的输出格式，需要同步修改 AI 模块
2. **不一致风险**：容易出现输出不一致的问题
3. **重复代码**：格式化逻辑重复实现

## 两种方案对比

### 方案 A：通过 os/exec 直接调用子命令（推荐）

**优点**：
- **极度简单**：不需要重构任何代码，直接调用
- **输出 100% 一致**：确保 AI 输出与实际子命令完全一致
- **自动同步**：任何子命令的更新都会自动反映在 AI 输出中
- **维护成本最低**

**缺点**：
- 性能略低（需要启动新进程），但对于 AI 场景可接受
- 只能获取文本输出，不好处理结构化数据

**适用场景**：所有需要输出与实际子命令一致的工具

### 方案 B：重构输出格式化函数

**优点**：
- 更好的程序控制
- 性能更好
- 可以获取结构化数据

**缺点**：
- 需要大量重构工作
- 仍然有代码重复
- exec 相关的逻辑非常复杂，难以提取复用

## 推荐方案：方案 A（os/exec）

对于大多数工具，特别是 node list, exec run, exec script，直接通过 os/exec 调用 owl 子命令是最简单有效的方案。

## 实施计划（方案 A）

### 阶段一：重构 QueryNodes 和 QueryDatabase 工具

**目标**：让这些工具直接调用 `owl node list` 子命令

**文件**：`internal/ai/tools.go`

**变更点**：

1. 修改 `QueryNodesTool.Execute`：
   - 构建 `owl node list` 命令
   - 添加对应的参数（如 --group, --label, --status, --search 等）
   - 执行命令并捕获输出
   - 返回输出

2. 同样修改 `QueryDatabaseTool.Execute`

### 阶段二：重构 ExecRun 和 ExecScript 工具

**目标**：让这些工具直接调用 `owl exec run` 和 `owl exec script`

**变更点**：

1. 修改 `ExecRunTool.Execute`：
   - 构建 `owl exec run <command> <args>` 命令
   - 处理 nodes/group/label/status 等筛选参数
   - 执行命令并捕获输出
   - 返回输出

2. 修改 `ExecScriptTool.Execute`：
   - 构建 `owl exec script <script-file> <args>` 命令
   - 执行命令并捕获输出
   - 返回输出

### 阶段三：更新系统提示词

**目标**：调整提示词，让 AI 直接返回命令输出，不要再格式化

**文件**：`internal/ai/prompts/prompts.go`

### 阶段四：测试验证

1. 验证所有功能正常工作
2. 确保 AI 输出与实际子命令完全一致

## 修改文件列表

1. `internal/ai/tools.go` - 重构所有工具使用 os/exec
2. `internal/ai/prompts/prompts.go` - 更新系统提示词
3. `internal/ai/agent.go` - 保持不变（响应处理逻辑已正常）

## 回滚计划

如果方案 A 有问题，可以退回到方案 B。

