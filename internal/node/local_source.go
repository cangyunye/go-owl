package node

import (
	"fmt"
	"sync"
)

type LocalSource struct {
	nodes map[string]*LocalNode
	mu    sync.RWMutex
}

type LocalNode struct {
	ID          string
	Name        string
	Address     string
	Port        int
	User        string
	Groups      []string
	Labels      map[string]string
	SSHKey      string
	SSHPassword string
}

func NewLocalSource() *LocalSource {
	return &LocalSource{
		nodes: make(map[string]*LocalNode),
	}
}

func (s *LocalSource) GetNode(idOrName string) (*LocalNode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if node, ok := s.nodes[idOrName]; ok {
		return node, nil
	}

	for _, node := range s.nodes {
		if node.Name == idOrName {
			return node, nil
		}
	}

	return nil, fmt.Errorf("本地节点未找到: %s", idOrName)
}

func (s *LocalSource) ListNodes(opts *ListOptions) ([]*LocalNode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	nodes := make([]*LocalNode, 0, len(s.nodes))
	for _, node := range s.nodes {
		if opts != nil {
			if opts.Name != "" && node.Name != opts.Name {
				continue
			}
			if opts.Group != "" && !contains(node.Groups, opts.Group) {
				continue
			}
			if opts.Label != "" {
				found := false
				for k, v := range node.Labels {
					if k+"="+v == opts.Label || k == opts.Label {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

func (s *LocalSource) AddNode(node *LocalNode) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if node.ID == "" {
		node.ID = node.Name
	}
	s.nodes[node.ID] = node
	if node.Name != "" && node.Name != node.ID {
		s.nodes[node.Name] = node
	}

	return nil
}

func (s *LocalSource) RemoveNode(idOrName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	node, ok := s.nodes[idOrName]
	if !ok {
		return fmt.Errorf("节点不存在: %s", idOrName)
	}

	delete(s.nodes, node.ID)
	if node.Name != "" {
		delete(s.nodes, node.Name)
	}

	return nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
