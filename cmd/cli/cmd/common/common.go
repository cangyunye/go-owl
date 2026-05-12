// Package common CLI 通用工具包
package common

import (
	"fmt"
	"strings"

	"github.com/cangyunye/go-owl/internal/common/model"
)

// OutputFormat 输出格式
type OutputFormat string

const (
	OutputFormatTable OutputFormat = "table"
	OutputFormatJSON  OutputFormat = "json"
	OutputFormatYAML  OutputFormat = "yaml"
)

// NodeSelector 节点选择器
type NodeSelector struct {
	Nodes   []string // 节点ID列表
	Groups  []string // 分组列表
	Labels  []string // 标签列表
	Status  model.NodeStatus // 节点状态
}

// ParseLabels 解析标签字符串 "key=value,key2=value2"
func ParseLabels(labelStrs []string) (map[string]string, error) {
	labels := make(map[string]string)
	for _, str := range labelStrs {
		parts := strings.SplitN(str, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid label format: %s (expected key=value)", str)
		}
		labels[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return labels, nil
}

// ParseNodeList 解析节点列表 "node1,node2,node3"
func ParseNodeList(nodesStr string) []string {
	if nodesStr == "" {
		return nil
	}
	nodes := strings.Split(nodesStr, ",")
	result := make([]string, 0, len(nodes))
	for _, n := range nodes {
		n = strings.TrimSpace(n)
		if n != "" {
			result = append(result, n)
		}
	}
	return result
}

// ParseGroupList 解析分组列表
func ParseGroupList(groupsStr string) []string {
	return ParseNodeList(groupsStr)
}
