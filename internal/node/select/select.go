// Package nodeselect 提供 CLI 与 Web 共用的执行目标选择语义。
// 精确匹配（组名完整相等、标签键值相等），不做子串模糊匹配——
// 界面搜索框的模糊查询在各自的列表 API 中，与此无关。
//
// 注：目录名为 select，但 select 是 Go 关键字，不能作为包名，
// 故包名为 nodeselect；导入路径仍为 .../internal/node/select。
package nodeselect

import (
	"context"
	"fmt"
	"strings"
)

type NodeRow struct {
	ID     string
	Name   string
	Groups []string
	Labels map[string]string
	Status string
}

type NodeSource interface {
	List(ctx context.Context) ([]NodeRow, error)
}

type SelectOptions struct {
	NodeIDs []string
	Groups  []string
	Labels  map[string]string
	Status  string
}

func (o SelectOptions) Empty() bool {
	return len(o.NodeIDs) == 0 && len(o.Groups) == 0 && len(o.Labels) == 0 && o.Status == ""
}

type Selector struct {
	source NodeSource
}

func NewSelector(source NodeSource) *Selector {
	return &Selector{source: source}
}

// Select 按 CLI 语义解析执行目标。
// 优先级：NodeIDs > Groups > Labels > Status（多条件并存时取其一，不做交集）。
// 空选项返回全部节点。NodeIDs 中任一 id/name 无法解析则整体报错。
func (s *Selector) Select(ctx context.Context, opts SelectOptions) ([]NodeRow, error) {
	all, err := s.source.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("节点列表获取失败: %w", err)
	}
	switch {
	case opts.Empty():
		return all, nil
	case len(opts.NodeIDs) > 0:
		return selectByIDOrName(all, opts.NodeIDs)
	case len(opts.Groups) > 0:
		return selectByGroups(all, opts.Groups), nil
	case len(opts.Labels) > 0:
		return selectByLabels(all, opts.Labels), nil
	default:
		return selectByStatus(all, opts.Status), nil
	}
}

func selectByIDOrName(all []NodeRow, ids []string) ([]NodeRow, error) {
	byID := make(map[string]NodeRow, len(all))
	byName := make(map[string]NodeRow, len(all))
	for _, n := range all {
		byID[n.ID] = n
		if n.Name != "" {
			byName[n.Name] = n
		}
	}
	var out []NodeRow
	var missing []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if n, ok := byID[id]; ok {
			out = append(out, n)
			continue
		}
		if n, ok := byName[id]; ok {
			out = append(out, n)
			continue
		}
		missing = append(missing, id)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("节点不存在: %v", missing)
	}
	return out, nil
}

func selectByGroups(all []NodeRow, groups []string) []NodeRow {
	want := make(map[string]bool, len(groups))
	for _, g := range groups {
		g = strings.TrimSpace(g)
		if g != "" {
			want[g] = true
		}
	}
	var out []NodeRow
	for _, n := range all {
		for _, g := range n.Groups {
			if want[g] {
				out = append(out, n)
				break
			}
		}
	}
	return out
}

func selectByLabels(all []NodeRow, labels map[string]string) []NodeRow {
	var out []NodeRow
	for _, n := range all {
		match := true
		for k, v := range labels {
			got, ok := n.Labels[k]
			if !ok || (v != "" && got != v) {
				match = false
				break
			}
		}
		if match {
			out = append(out, n)
		}
	}
	return out
}

func selectByStatus(all []NodeRow, status string) []NodeRow {
	var out []NodeRow
	for _, n := range all {
		if n.Status == status {
			out = append(out, n)
		}
	}
	return out
}
