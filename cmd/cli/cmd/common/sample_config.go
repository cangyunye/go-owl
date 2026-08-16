package common

import (
	"path/filepath"
)

// GetSampleConfigFile 获取示例节点配置文件路径
func GetSampleConfigFile() string {
	return filepath.Join(GetConfigDir(), "sample_nodes.json")
}
