# 测试用节点数据和自然语言查询

这个目录包含了用于测试 `query_nodes` 工具的测试数据和自然语言查询场景。

## 文件说明

- [`test_nodes.go`](test_nodes.go) - 测试用的节点数据集，包含 8 个节点
- [`test_natural_queries.go`](test_natural_queries.go) - 测试用的自然语言查询场景，包含 18 个场景

## 测试节点概述

| 节点名称 | 地址 | 状态 | 分组 | 标签 |
|---------|------|------|------|------|
| web-server-01 | 192.168.1.101 | online | web, prod | env:prod, region:us-west, os:linux |
| web-server-02 | 192.168.1.102 | online | web, prod | env:prod, region:us-east, os:linux |
| db-primary-01 | 192.168.1.201 | online | db, prod | env:prod, region:us-west, os:linux, role:primary |
| db-replica-01 | 192.168.1.202 | offline | db, prod | env:prod, region:us-east, os:linux, role:replica |
| test-node-01 | 192.168.1.301 | online | test | env:test, region:us-west, os:linux |
| cache-node-01 | 192.168.1.401 | online | cache, prod | env:prod, region:us-west, os:linux, type:redis |
| monitoring-01 | 192.168.1.501 | online | monitoring, prod | env:prod, region:us-west, os:linux |
| windows-test-01 | 192.168.1.601 | unknown | test, windows | env:test, region:us-east, os:windows |

## 自然语言查询场景

### 基础查询 (node action)
1. 列出所有节点 - 查询所有 8 个节点
2. 按分组查询 - web组 - 查询 web-server-01, web-server-02
3. 按标签查询 - env=test - 查询 test-node-01, windows-test-01
4. 按状态查询 - online - 查询 6 个在线节点
5. 按状态查询 - offline - 查询 db-replica-01
6. 名称模糊搜索 - web - 查询 web-server-01, web-server-02
7. 名称模糊搜索 - db - 查询 db-primary-01, db-replica-01

### 组合查询 (node action)
8. web组 + online - 查询 web-server-01, web-server-02
9. env=prod + 在线 - 查询 5 个生产环境在线节点
10. 查找特定节点 - web-server-01 - 查询单个节点
11. 查找Windows节点 - 查询 windows-test-01
12. 查找美国东部区域的节点 - 查询 3 个节点
13. JSON格式输出 - 以 JSON 格式列出所有节点
14. 查询db分组节点详情 - 查询 db-primary-01, db-replica-01

### 其他动作 (exec/playbook/file)
15. 执行命令场景 - 在 web-server-01 上执行 uptime
16. 在测试环境执行命令 - 在 test 环境节点上运行 df -h
17. Playbook场景 - 在 web 组节点上安装 nginx
18. 文件传输场景 - 上传文件到缓存节点

## 使用示例

```go
package main

import (
	"fmt"

	"github.com/cangyunye/go-owl/test/testdata"
)

func main() {
	// 访问测试节点数据
	fmt.Println("=== 测试节点 ===")
	for _, node := range testdata.TestNodes {
		fmt.Printf("%s (%s)\n", node.Name, node.Address)
	}

	// 按分组查询
	fmt.Println("\n=== web组节点 ===")
	webNodes := testdata.GetNodesByGroup("web")
	for _, node := range webNodes {
		fmt.Println(node.Name)
	}

	// 按标签查询
	fmt.Println("\n=== env=test节点 ===")
	testNodes := testdata.GetNodesByLabel("env", "test")
	for _, node := range testNodes {
		fmt.Println(node.Name)
	}

	// 访问测试场景
	fmt.Println("\n=== 测试场景 ===")
	testdata.PrintScenarioSummary()
}
```

## 查询逻辑说明

### 当前实现的问题
当前 `query_nodes` 工具的实现有一个逻辑问题：**先精确过滤，再模糊搜索**，这会导致：

```go
// 当前错误的顺序
if group != "" {
    nodes = nodeMgr.GetByGroup(group)  // 先过滤
}
if search != "" {
    // 再在过滤后的结果中搜索
    // 这会导致搜索范围被限制在已过滤的节点内
}
```

### 正确的顺序应该是

**方案1：先搜索，后过滤**
```go
// 如果有搜索条件，先获取所有节点进行搜索
if search != "" {
    nodes = nodeMgr.SearchByName(search)
} else {
    nodes = nodeMgr.List()
}

// 再按精确条件过滤
if group != "" {
    nodes = filterByGroup(nodes, group)
}
if labels != nil {
    nodes = filterByLabels(nodes, labels)
}
```

**方案2：同时处理，取交集**
```go
// 获取所有可能匹配的节点（精确条件 OR 搜索条件）
// 然后取两者的交集
```

### 建议的修复策略
对于同时有精确条件和搜索条件的情况，应该：
1. 如果有精确条件（group/labels/status），先应用这些条件
2. 然后在结果中进行搜索
3. 或者反向：先搜索，后过滤

具体选择哪种方案需要根据用户预期来定。
