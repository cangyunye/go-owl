package nodeselect

import (
	"context"

	"github.com/cangyunye/go-owl/internal/node"
)

// ResolverSource 把 CLI 的 NodeResolver（合并 local/API/ssh-config 来源）
// 适配为共享选择器的 NodeSource。
type ResolverSource struct {
	resolver *node.NodeResolver
}

func NewResolverSource(resolver *node.NodeResolver) *ResolverSource {
	return &ResolverSource{resolver: resolver}
}

func (s *ResolverSource) List(ctx context.Context) ([]NodeRow, error) {
	nodes, err := s.resolver.ListNodes(&node.ListOptions{})
	if err != nil {
		return nil, err
	}
	rows := make([]NodeRow, 0, len(nodes))
	for _, n := range nodes {
		rows = append(rows, NodeRow{
			ID:     n.ID,
			Name:   n.Name,
			Groups: n.Groups,
			Labels: n.Labels,
		})
	}
	return rows, nil
}
