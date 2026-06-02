package testdata

import (
	"time"

	"github.com/cangyunye/go-owl/internal/common/model"
)

// TestNodes 是测试用的节点数据集
var TestNodes = []*model.Node{
	{
		ID:        "node-001",
		Name:      "web-server-01",
		Address:   "192.168.1.101",
		Port:      22,
		User:      "admin",
		Status:    model.NodeStatusOnline,
		Groups:    []string{"web", "prod"},
		Labels:    map[string]string{"env": "prod", "region": "us-west", "os": "linux"},
		Metadata:  map[string]string{"created_by": "sysadmin"},
		CreatedAt: time.Now().Add(-24 * time.Hour * 30),
		UpdatedAt: time.Now().Add(-24 * time.Hour),
	},
	{
		ID:        "node-002",
		Name:      "web-server-02",
		Address:   "192.168.1.102",
		Port:      22,
		User:      "admin",
		Status:    model.NodeStatusOnline,
		Groups:    []string{"web", "prod"},
		Labels:    map[string]string{"env": "prod", "region": "us-east", "os": "linux"},
		Metadata:  map[string]string{"created_by": "sysadmin"},
		CreatedAt: time.Now().Add(-24 * time.Hour * 25),
		UpdatedAt: time.Now().Add(-24 * time.Hour * 2),
	},
	{
		ID:        "node-003",
		Name:      "db-primary-01",
		Address:   "192.168.1.201",
		Port:      22,
		User:      "dbadmin",
		Status:    model.NodeStatusOnline,
		Groups:    []string{"db", "prod"},
		Labels:    map[string]string{"env": "prod", "region": "us-west", "os": "linux", "role": "primary"},
		Metadata:  map[string]string{"created_by": "dbadmin"},
		CreatedAt: time.Now().Add(-24 * time.Hour * 60),
		UpdatedAt: time.Now().Add(-24 * time.Hour * 3),
	},
	{
		ID:        "node-004",
		Name:      "db-replica-01",
		Address:   "192.168.1.202",
		Port:      22,
		User:      "dbadmin",
		Status:    model.NodeStatusOffline,
		Groups:    []string{"db", "prod"},
		Labels:    map[string]string{"env": "prod", "region": "us-east", "os": "linux", "role": "replica"},
		Metadata:  map[string]string{"created_by": "dbadmin"},
		CreatedAt: time.Now().Add(-24 * time.Hour * 45),
		UpdatedAt: time.Now().Add(-24 * time.Hour * 10),
	},
	{
		ID:        "node-005",
		Name:      "test-node-01",
		Address:   "192.168.1.301",
		Port:      22,
		User:      "devuser",
		Status:    model.NodeStatusOnline,
		Groups:    []string{"test"},
		Labels:    map[string]string{"env": "test", "region": "us-west", "os": "linux"},
		Metadata:  map[string]string{"created_by": "devuser"},
		CreatedAt: time.Now().Add(-24 * time.Hour * 15),
		UpdatedAt: time.Now().Add(-24 * time.Hour * 5),
	},
	{
		ID:        "node-006",
		Name:      "cache-node-01",
		Address:   "192.168.1.401",
		Port:      22,
		User:      "cacheadmin",
		Status:    model.NodeStatusOnline,
		Groups:    []string{"cache", "prod"},
		Labels:    map[string]string{"env": "prod", "region": "us-west", "os": "linux", "type": "redis"},
		Metadata:  map[string]string{"created_by": "cacheadmin"},
		CreatedAt: time.Now().Add(-24 * time.Hour * 20),
		UpdatedAt: time.Now().Add(-24 * time.Hour * 4),
	},
	{
		ID:        "node-007",
		Name:      "monitoring-01",
		Address:   "192.168.1.501",
		Port:      22,
		User:      "monitor",
		Status:    model.NodeStatusOnline,
		Groups:    []string{"monitoring", "prod"},
		Labels:    map[string]string{"env": "prod", "region": "us-west", "os": "linux"},
		Metadata:  map[string]string{"created_by": "monitor"},
		CreatedAt: time.Now().Add(-24 * time.Hour * 50),
		UpdatedAt: time.Now().Add(-24 * time.Hour * 1),
	},
	{
		ID:        "node-008",
		Name:      "windows-test-01",
		Address:   "192.168.1.601",
		Port:      5985,
		User:      "testuser",
		Status:    model.NodeStatusUnknown,
		Groups:    []string{"test", "windows"},
		Labels:    map[string]string{"env": "test", "region": "us-east", "os": "windows"},
		Metadata:  map[string]string{"created_by": "testuser"},
		CreatedAt: time.Now().Add(-24 * time.Hour * 10),
		UpdatedAt: time.Now().Add(-24 * time.Hour * 7),
	},
}

// GetTestNodeByName 通过名称查找测试节点
func GetTestNodeByName(name string) *model.Node {
	for _, node := range TestNodes {
		if node.Name == name {
			return node
		}
	}
	return nil
}

// GetNodesByGroup 通过分组获取节点
func GetNodesByGroup(group string) []*model.Node {
	result := make([]*model.Node, 0)
	for _, node := range TestNodes {
		for _, g := range node.Groups {
			if g == group {
				result = append(result, node)
				break
			}
		}
	}
	return result
}

// GetNodesByLabel 通过标签获取节点
func GetNodesByLabel(key, value string) []*model.Node {
	result := make([]*model.Node, 0)
	for _, node := range TestNodes {
		if val, ok := node.Labels[key]; ok && val == value {
			result = append(result, node)
		}
	}
	return result
}

// GetNodesByStatus 通过状态获取节点
func GetNodesByStatus(status model.NodeStatus) []*model.Node {
	result := make([]*model.Node, 0)
	for _, node := range TestNodes {
		if node.Status == status {
			result = append(result, node)
		}
	}
	return result
}
