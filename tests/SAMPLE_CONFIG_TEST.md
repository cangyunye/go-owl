# 示例节点配置分离测试文档

## 概述

本文档记录了将示例节点测试数据从硬编码方式改为配置文件方式的变更。

## 变更内容

### 1. 新增文件: `/workspace/go-owl/cmd/cli/cmd/common/sample_config.go`

此文件负责管理示例节点配置文件的读取和写入：

- **DefaultSampleNodes**: 保存默认的 JSON 格式示例节点数据
- **loadSampleNodes()**: 从 `~/.owl/sample_nodes.json` 加载示例节点
- **saveDefaultSampleNodes()**: 创建默认配置文件
- **SaveSampleNodes()**: 保存节点数据到配置文件

### 2. 修改文件: `/workspace/go-owl/cmd/cli/cmd/common/node.go`

- **initSampleData()**: 改为从配置文件加载示例节点，而不是硬编码
- 添加了错误处理：配置文件加载失败时输出警告信息

## 配置文件位置

配置文件位于：`~/.owl/sample_nodes.json`

## 默认配置文件内容

首次运行程序时，如果配置文件不存在，会自动创建以下默认配置：

```json
{
  "_comment": "示例节点配置文件 - 可以根据需要修改或扩展这些节点",
  "nodes": [
    {
      "id": "node1",
      "name": "web-server-1",
      "address": "192.168.1.10",
      "port": 8080,
      "user": "root",
      "status": "online",
      "groups": ["web", "production"],
      "labels": {"env": "prod", "region": "us-east"},
      "created_at": "",
      "updated_at": ""
    },
    {
      "id": "node2",
      "name": "web-server-2",
      "address": "192.168.1.11",
      "port": 8080,
      "user": "root",
      "status": "online",
      "groups": ["web", "production"],
      "labels": {"env": "prod", "region": "us-west"},
      "created_at": "",
      "updated_at": ""
    },
    {
      "id": "node3",
      "name": "db-server-1",
      "address": "192.168.1.20",
      "port": 8080,
      "user": "root",
      "status": "online",
      "groups": ["database"],
      "labels": {"env": "prod", "type": "mysql"},
      "created_at": "",
      "updated_at": ""
    },
    {
      "id": "node4",
      "name": "cache-server-1",
      "address": "192.168.1.30",
      "port": 8080,
      "user": "root",
      "status": "offline",
      "groups": ["cache"],
      "labels": {"env": "staging"},
      "created_at": "",
      "updated_at": ""
    }
  ]
}
```

## 使用说明

### 自动创建配置

当程序首次运行且 `~/.owl/nodes.json` 不存在时：

1. 程序会尝试加载 `~/.owl/sample_nodes.json`
2. 如果配置文件不存在，会自动创建默认配置
3. 示例节点会被加载到内存存储中

### 手动修改配置

用户可以直接编辑 `~/.owl/sample_nodes.json` 文件：

1. 修改现有节点信息
2. 添加新的节点
3. 删除不需要的节点
4. 下次运行程序时会加载修改后的配置

### 配置文件格式要求

- 必须是有效的 JSON 格式
- 节点数组的键名必须是 `nodes`
- 每个节点对象必须包含 `id` 字段（作为唯一标识）

## 错误处理

如果配置文件格式错误或无法读取：

- 程序会输出警告信息到 stderr
- 不会影响程序的其他功能
- 程序会继续运行，但不会加载示例节点

## 测试命令

```bash
# 构建项目
cd /workspace/go-owl && go build -o owl ./cmd/cli/

# 运行测试
./owl node list

# 查看生成的配置文件
cat ~/.owl/sample_nodes.json
```

## 验证结果

- ✅ 项目成功构建
- ✅ 示例节点数据从配置文件加载
- ✅ 自动创建默认配置文件（如果不存在）
- ✅ 错误处理机制正常工作
