// Package common CLI 通用工具包
package common

import (
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
	Nodes  []string         // 节点ID列表
	Groups []string         // 分组列表
	Labels []string         // 标签列表
	Status model.NodeStatus // 节点状态
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
