package node

import (
	"fmt"
	"sync"

	"github.com/cangyunye/go-owl/internal/common/model"
)

type NodeStore interface {
	Get(id string) (*model.Node, bool)
	Set(id string, node *model.Node)
	Delete(id string) bool
	GetAll() []*model.Node
}

type InMemoryNodeStore struct {
	mu    sync.RWMutex
	nodes map[string]*model.Node
}

func NewInMemoryNodeStore() *InMemoryNodeStore {
	return &InMemoryNodeStore{
		nodes: make(map[string]*model.Node),
	}
}

func (s *InMemoryNodeStore) Get(id string) (*model.Node, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	node, ok := s.nodes[id]
	return node, ok
}

func (s *InMemoryNodeStore) Set(id string, node *model.Node) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes[id] = node
}

func (s *InMemoryNodeStore) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.nodes[id]; ok {
		delete(s.nodes, id)
		return true
	}
	return false
}

func (s *InMemoryNodeStore) GetAll() []*model.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	nodes := make([]*model.Node, 0, len(s.nodes))
	for _, node := range s.nodes {
		nodes = append(nodes, node.Clone())
	}
	return nodes
}

type Manager interface {
	Register(node *model.Node) error
	Unregister(id string) error
	GetByID(id string) (*model.Node, error)
	List() []*model.Node
	GetByGroup(group string) []*model.Node
	GetByLabels(labels map[string]string) []*model.Node
	UpdateStatus(id string, status model.NodeStatus) error
	GetOnlineNodes() []*model.Node
	Count() int
}

type manager struct {
	store NodeStore
}

func NewManager(store NodeStore) Manager {
	return &manager{store: store}
}

func (m *manager) Register(node *model.Node) error {
	if err := node.Validate(); err != nil {
		return fmt.Errorf("invalid node: %w", err)
	}

	if _, exists := m.store.Get(node.ID); exists {
		return fmt.Errorf("node with ID '%s' already exists", node.ID)
	}

	node.SetStatus(model.NodeStatusOnline)
	m.store.Set(node.ID, node)
	return nil
}

func (m *manager) Unregister(id string) error {
	if _, exists := m.store.Get(id); !exists {
		return fmt.Errorf("node with ID '%s' not found", id)
	}
	m.store.Delete(id)
	return nil
}

func (m *manager) GetByID(id string) (*model.Node, error) {
	node, exists := m.store.Get(id)
	if !exists {
		return nil, fmt.Errorf("node with ID '%s' not found", id)
	}
	return node.Clone(), nil
}

func (m *manager) List() []*model.Node {
	return m.store.GetAll()
}

func (m *manager) GetByGroup(group string) []*model.Node {
	nodes := m.store.GetAll()
	result := make([]*model.Node, 0)
	for _, node := range nodes {
		if node.HasGroup(group) {
			result = append(result, node)
		}
	}
	return result
}

func (m *manager) GetByLabels(labels map[string]string) []*model.Node {
	nodes := m.store.GetAll()
	result := make([]*model.Node, 0)
	for _, node := range nodes {
		if node.MatchLabels(labels) {
			result = append(result, node)
		}
	}
	return result
}

func (m *manager) UpdateStatus(id string, status model.NodeStatus) error {
	node, exists := m.store.Get(id)
	if !exists {
		return fmt.Errorf("node with ID '%s' not found", id)
	}
	node.SetStatus(status)
	m.store.Set(id, node)
	return nil
}

func (m *manager) GetOnlineNodes() []*model.Node {
	nodes := m.store.GetAll()
	result := make([]*model.Node, 0)
	for _, node := range nodes {
		if node.Status == model.NodeStatusOnline {
			result = append(result, node)
		}
	}
	return result
}

func (m *manager) Count() int {
	return len(m.store.GetAll())
}
