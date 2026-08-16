package common

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// NodeStore 节点存储接口
type NodeStore interface {
	List() ([]*NodeInfo, error)
	Get(id string) (*NodeInfo, error)
	Add(node *NodeInfo) error
	Remove(id string) error
	Update(node *NodeInfo) error
	Save() error
	Load() error
}

// NodeInfo 节点信息
type NodeInfo struct {
	ID         string            `json:"id" yaml:"id"`
	Name       string            `json:"name" yaml:"name"`
	Address    string            `json:"address" yaml:"address"`
	Port       int               `json:"port" yaml:"port"`
	User       string            `json:"user" yaml:"user"`
	Password   string            `json:"password,omitempty" yaml:"password,omitempty"`
	SSHKey     string            `json:"ssh_key,omitempty" yaml:"ssh_key,omitempty"`
	Status     string            `json:"status" yaml:"status"`
	Groups     []string          `json:"groups" yaml:"groups"`
	Labels     map[string]string `json:"labels" yaml:"labels"`
	ProxyJump  string            `json:"proxy_jump,omitempty" yaml:"proxy_jump,omitempty"`
	CreatedAt  string            `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	UpdatedAt  string            `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
	LastCheckAt string           `json:"last_check_at,omitempty" yaml:"last_check_at,omitempty"`
}

// InMemoryNodeStore 内存节点存储（支持文件持久化）
type InMemoryNodeStore struct {
	nodes map[string]*NodeInfo
	sync.RWMutex
	dataFile string
}

// 全局单例存储
var globalStore NodeStore

// GetConfigDir 获取配置目录
func GetConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp"
	}
	dir := filepath.Join(home, ".owl")
	os.MkdirAll(dir, 0755)
	return dir
}

// init 初始化全局存储
func init() {
	dataFile := filepath.Join(GetConfigDir(), "nodes.json")
	store := &InMemoryNodeStore{
		nodes:    make(map[string]*NodeInfo),
		dataFile: dataFile,
	}
	// 尝试加载数据文件
	if err := store.Load(); err != nil {
		// 加载失败，初始化示例数据
		store.initSampleData()
	}
	globalStore = store
}

// NewInMemoryNodeStoreAt 创建内存节点存储(数据文件路径自定义,测试用)
func NewInMemoryNodeStoreAt(dataFile string) *InMemoryNodeStore {
	store := &InMemoryNodeStore{
		nodes:    make(map[string]*NodeInfo),
		dataFile: dataFile,
	}
	return store
}

func (s *InMemoryNodeStore) initSampleData() {
	// 空实现，不再自动加载示例节点
	// 示例节点将通过 'owl node sample' 命令生成
	// 如果没有节点，直接使用空映射即可
}

// Load 从文件加载数据
func (s *InMemoryNodeStore) Load() error {
	s.Lock()
	defer s.Unlock()

	data, err := os.ReadFile(s.dataFile)
	if err != nil {
		return err
	}

	var nodes []*NodeInfo
	if err := json.Unmarshal(data, &nodes); err != nil {
		return err
	}

	s.nodes = make(map[string]*NodeInfo)
	for _, n := range nodes {
		s.nodes[n.ID] = n
	}
	return nil
}

// Save 保存数据到文件
func (s *InMemoryNodeStore) Save() error {
	s.Lock()
	defer s.Unlock()

	nodes := make([]*NodeInfo, 0, len(s.nodes))
	for _, n := range s.nodes {
		nodes = append(nodes, n)
	}

	data, err := json.MarshalIndent(nodes, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.dataFile, data, 0644)
}

func (s *InMemoryNodeStore) List() ([]*NodeInfo, error) {
	s.RLock()
	defer s.RUnlock()
	nodes := make([]*NodeInfo, 0, len(s.nodes))
	for _, n := range s.nodes {
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func (s *InMemoryNodeStore) Get(id string) (*NodeInfo, error) {
	s.RLock()
	defer s.RUnlock()
	node, ok := s.nodes[id]
	if !ok {
		return nil, fmt.Errorf("node not found: %s", id)
	}
	return node, nil
}

func (s *InMemoryNodeStore) Add(node *NodeInfo) error {
	s.Lock()
	defer s.Unlock()
	if _, ok := s.nodes[node.ID]; ok {
		return fmt.Errorf("node already exists: %s", node.ID)
	}
	s.nodes[node.ID] = node
	return nil
}

func (s *InMemoryNodeStore) Remove(id string) error {
	s.Lock()
	defer s.Unlock()
	if _, ok := s.nodes[id]; !ok {
		return fmt.Errorf("node not found: %s", id)
	}
	delete(s.nodes, id)
	return nil
}

func (s *InMemoryNodeStore) Update(node *NodeInfo) error {
	s.Lock()
	defer s.Unlock()
	if _, ok := s.nodes[node.ID]; !ok {
		return fmt.Errorf("node not found: %s", node.ID)
	}
	s.nodes[node.ID] = node
	return nil
}

// GetNodeStore 获取全局节点存储
func GetNodeStore() NodeStore {
	return globalStore
}

// InitNodeStoreFromDB 从数据库初始化节点存储
func InitNodeStoreFromDB(db *sql.DB) {
	globalStore = NewNodeStoreDB(db)
}
