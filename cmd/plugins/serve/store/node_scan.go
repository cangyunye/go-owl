package store

import (
	"encoding/json"

	commonmodel "github.com/cangyunye/go-owl/internal/common/model"
)

// nodeScanner 兼容 *sql.Row 与 *sql.Rows 的最小扫描接口。
type nodeScanner interface {
	Scan(dest ...interface{}) error
}

// NodeRowColumns 是 nodes 表共享扫描的规范列清单（与 ScanNodeRow 的扫描顺序
// 一一对应）。groups/labels 用 COALESCE 归一，覆盖存量库的 NULL 与空值。
const NodeRowColumns = `id, COALESCE(name, ''), COALESCE(address, ''), COALESCE(port, 22), COALESCE(user, ''), COALESCE(status, ''), COALESCE(groups, '[]'), COALESCE(labels, '{}')`

// ParseNodeGroups 解析 nodes.groups JSON 数组列。
// 非法 JSON / 空串 / 类型不匹配一律归一为 nil，调用方无需重复容错。
func ParseNodeGroups(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// ParseNodeLabels 解析 nodes.labels JSON 对象列。
// 非法 JSON / 空串 / 类型不匹配一律归一为 nil。
func ParseNodeLabels(raw string) map[string]string {
	if raw == "" {
		return nil
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// ScanNodeRow 按 NodeRowColumns 顺序扫描一行 nodes 表记录。
// DTO 不同的调用方（AI/选择器/监控）可只复用 ParseNodeGroups/ParseNodeLabels。
func ScanNodeRow(s nodeScanner) (*commonmodel.Node, error) {
	var n commonmodel.Node
	var groupsJSON, labelsJSON string
	if err := s.Scan(&n.ID, &n.Name, &n.Address, &n.Port, &n.User, &n.Status, &groupsJSON, &labelsJSON); err != nil {
		return nil, err
	}
	n.Groups = ParseNodeGroups(groupsJSON)
	n.Labels = ParseNodeLabels(labelsJSON)
	return &n, nil
}
