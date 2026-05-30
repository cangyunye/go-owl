# owl node 路由提示词拆分计划

## 背景

根据 exec 和 playbook 路由拆分的模式，将 node 路由也按子命令拆分为独立路由。

## node 子命令分析

从 NODE.md 文档，owl node 有以下子命令：

| 子命令 | 功能 | 用途 |
|--------|------|------|
| `node_list` | 列出节点 | 列出所有已注册的节点 |
| `node_add` | 添加节点 | 添加新节点 |
| `node_update` | 更新节点 | 更新节点信息 |
| `node_remove` | 删除节点 | 删除节点 |
| `node_status` | 查看状态 | 查看节点连接状态 |
| `node_groups` | 管理分组 | 管理节点分组 |
| `node_labels` | 管理标签 | 管理节点标签 |
| `node_import` | 导入节点 | 从文件导入节点 |
| `node_ping` | Ping 检查 | ICMP Ping 检查可达性 |
| `node_check` | SSH 检查 | SSH 连接测试并更新状态 |

## 实施步骤

### 步骤 1: 更新 RouterPrompt

在 RouterPrompt 中添加 node 子命令的区分：

```
【节点管理】
node_list     - 列出节点（查看有哪些节点）
node_add      - 添加节点（添加新节点到系统）
node_update   - 更新节点（修改节点信息）
node_remove   - 删除节点（从系统移除节点）
node_status   - 查看状态（查看节点连接状态）
node_groups   - 管理分组（添加/移除分组）
node_labels   - 管理标签（添加/移除标签）
node_import   - 导入节点（从文件导入节点）
node_ping     - Ping 检查（ICMP 检查可达性）
node_check    - SSH 检查（SSH 连接测试）
```

判断规则：
- 提到"列出节点"、"查看节点"、"有哪些节点" → node_list
- 提到"添加节点"、"新增节点" → node_add
- 提到"更新节点"、"修改节点" → node_update
- 提到"删除节点"、"移除节点" → node_remove
- 提到"节点状态"、"连接状态" → node_status
- 提到"分组"、"组" → node_groups
- 提到"标签"、"label" → node_labels
- 提到"导入节点"、"import" → node_import
- 提到"ping"、"可达性" → node_ping
- 提到"ssh check"、"连接测试" → node_check
- 只说"node"但未明确 → node_list（默认）

### 步骤 2: 创建 node 子命令提示词

为以下子命令创建独立提示词：
1. NodeListSystemPrompt - 列出节点
2. NodeAddSystemPrompt - 添加节点
3. NodeUpdateSystemPrompt - 更新节点
4. NodeRemoveSystemPrompt - 删除节点
5. NodeStatusSystemPrompt - 查看状态
6. NodeGroupsSystemPrompt - 管理分组
7. NodeLabelsSystemPrompt - 管理标签
8. NodeImportSystemPrompt - 导入节点
9. NodePingSystemPrompt - Ping 检查
10. NodeCheckSystemPrompt - SSH 检查

### 步骤 3: 更新 agent.go

1. 更新 groupPrompts 映射
2. 注册新工具（如果需要）
3. 添加 node 兼容逻辑

## 文件修改清单

| 文件 | 修改内容 |
|------|---------|
| `internal/ai/prompts/prompts.go` | 1. 更新 RouterPrompt<br>2. 新增 NodeXXXSystemPrompt (10个) |
| `internal/ai/agent.go` | 1. 更新 groupPrompts 映射<br>2. 添加 node 兼容逻辑 |