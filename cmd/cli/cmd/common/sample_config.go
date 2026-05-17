package common

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultSampleNodes 默认示例节点数据
var DefaultSampleNodes = `[
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
]`

// GetSampleConfigFile 获取示例节点配置文件路径
func GetSampleConfigFile() string {
	return filepath.Join(GetConfigDir(), "sample_nodes.json")
}

// loadSampleNodes 从配置文件加载示例节点数据
func loadSampleNodes() ([]*NodeInfo, error) {
	configFile := GetSampleConfigFile()

	// 如果配置文件不存在，返回空切片（不创建默认配置）
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		return []*NodeInfo{}, nil
	}

	// 读取配置文件
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read sample config: %w", err)
	}

	// 尝试解析为节点数组
	var nodes []*NodeInfo
	if err := json.Unmarshal(data, &nodes); err != nil {
		return nil, fmt.Errorf("failed to parse sample config: %w", err)
	}

	return nodes, nil
}

// saveDefaultSampleNodes 保存默认示例节点配置
func saveDefaultSampleNodes(configFile string) error {
	// 确保目录存在
	dir := filepath.Dir(configFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// 直接写入默认数据（保留注释说明）
	content := `{
  "_comment": "示例节点配置文件 - 可以根据需要修改或扩展这些节点",
  "nodes": ` + DefaultSampleNodes + `
}`

	return os.WriteFile(configFile, []byte(content), 0644)
}

// SaveSampleNodes 保存示例节点数据到配置文件
func SaveSampleNodes(nodes []*NodeInfo) error {
	configFile := GetSampleConfigFile()

	data, err := json.MarshalIndent(nodes, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal nodes: %w", err)
	}

	return os.WriteFile(configFile, data, 0644)
}
