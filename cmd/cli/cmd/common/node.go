package common

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
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
	ID        string            `json:"id" yaml:"id"`
	Name      string            `json:"name" yaml:"name"`
	Address   string            `json:"address" yaml:"address"`
	Port      int               `json:"port" yaml:"port"`
	User      string            `json:"user" yaml:"user"`
	Password  string            `json:"password,omitempty" yaml:"password,omitempty"`
	SSHKey    string            `json:"ssh_key,omitempty" yaml:"ssh_key,omitempty"`
	Status    string            `json:"status" yaml:"status"`
	Groups    []string          `json:"groups" yaml:"groups"`
	Labels    map[string]string `json:"labels" yaml:"labels"`
	ProxyJump string            `json:"proxy_jump,omitempty" yaml:"proxy_jump,omitempty"`
	CreatedAt string            `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	UpdatedAt string            `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
}

// InMemoryNodeStore 内存节点存储（支持文件持久化）
type InMemoryNodeStore struct {
	nodes map[string]*NodeInfo
	sync.RWMutex
	dataFile string
}

// 全局单例存储
var globalStore *InMemoryNodeStore

// getConfigDir 获取配置目录
func getConfigDir() string {
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
	dataFile := filepath.Join(getConfigDir(), "nodes.json")
	globalStore = &InMemoryNodeStore{
		nodes:    make(map[string]*NodeInfo),
		dataFile: dataFile,
	}
	// 尝试加载数据文件
	if err := globalStore.Load(); err != nil {
		// 加载失败，初始化示例数据
		globalStore.initSampleData()
	}
}

// NewInMemoryNodeStore 创建内存节点存储
func NewInMemoryNodeStore() *InMemoryNodeStore {
	dataFile := filepath.Join(getConfigDir(), "nodes.json")
	store := &InMemoryNodeStore{
		nodes:    make(map[string]*NodeInfo),
		dataFile: dataFile,
	}
	// 注意：不调用 initSampleData，只用于测试
	return store
}

func (s *InMemoryNodeStore) initSampleData() {
	s.Lock()
	defer s.Unlock()
	s.nodes["node1"] = &NodeInfo{
		ID:      "node1",
		Name:    "web-server-1",
		Address: "192.168.1.10",
		Port:    8080,
		Status:  "online",
		Groups:  []string{"web", "production"},
		Labels:  map[string]string{"env": "prod", "region": "us-east"},
	}
	s.nodes["node2"] = &NodeInfo{
		ID:      "node2",
		Name:    "web-server-2",
		Address: "192.168.1.11",
		Port:    8080,
		Status:  "online",
		Groups:  []string{"web", "production"},
		Labels:  map[string]string{"env": "prod", "region": "us-west"},
	}
	s.nodes["node3"] = &NodeInfo{
		ID:      "node3",
		Name:    "db-server-1",
		Address: "192.168.1.20",
		Port:    8080,
		Status:  "online",
		Groups:  []string{"database"},
		Labels:  map[string]string{"env": "prod", "type": "mysql"},
	}
	s.nodes["node4"] = &NodeInfo{
		ID:      "node4",
		Name:    "cache-server-1",
		Address: "192.168.1.30",
		Port:    8080,
		Status:  "offline",
		Groups:  []string{"cache"},
		Labels:  map[string]string{"env": "staging"},
	}
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

// Node manager commands

var (
	addName     string
	addAddress  string
	addPort     int
	addUser     string
	addPassword string
	addSSHKey   string
	addGroups   string
	addLabels   []string
)

// RunAddNode 添加节点
func RunAddNode(args []string) {
	nodeID := args[0]
	store := GetNodeStore().(*InMemoryNodeStore)

	// 检查节点是否已存在
	if _, err := store.Get(nodeID); err == nil {
		fmt.Fprintf(os.Stderr, "Error: node already exists: %s\n", nodeID)
		os.Exit(1)
	}

	// 解析分组
	groups := []string{}
	if addGroups != "" {
		for _, g := range splitAndTrim(addGroups, ",") {
			if g != "" {
				groups = append(groups, g)
			}
		}
	}

	// 解析标签
	labels := make(map[string]string)
	for _, label := range addLabels {
		parts := splitAndTrim(label, "=")
		if len(parts) == 2 {
			labels[parts[0]] = parts[1]
		}
	}

	// 创建节点
	now := time.Now().Format(time.RFC3339)
	node := &NodeInfo{
		ID:        nodeID,
		Name:      addName,
		Address:   addAddress,
		Port:      addPort,
		User:      addUser,
		Password:  addPassword,
		SSHKey:    addSSHKey,
		Status:    "offline",
		Groups:    groups,
		Labels:    labels,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// 保存节点
	if err := store.Add(node); err != nil {
		fmt.Fprintf(os.Stderr, "Error adding node: %v\n", err)
		os.Exit(1)
	}

	// 持久化到文件
	store.Save()

	fmt.Printf("Node '%s' added successfully\n", nodeID)
	fmt.Printf("  Name:    %s\n", node.Name)
	fmt.Printf("  Address: %s:%d\n", node.Address, node.Port)
}

// RunRemoveNode 删除节点
func RunRemoveNode(args []string) {
	store := GetNodeStore().(*InMemoryNodeStore)
	success := 0
	failed := 0

	for _, nodeID := range args {
		if err := store.Remove(nodeID); err != nil {
			fmt.Printf("Failed to remove node '%s': %v\n", nodeID, err)
			failed++
		} else {
			fmt.Printf("Node '%s' removed successfully\n", nodeID)
			success++
		}
	}

	// 持久化到文件
	if success > 0 {
		store.Save()
	}

	fmt.Printf("\nRemoved: %d nodes, Failed: %d\n", success, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

// Helper functions
func splitAndTrim(s string, sep string) []string {
	parts := make([]string, 0)
	for _, p := range splitStr(s, sep) {
		if trimmed := trimStr(p); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

func splitStr(s string, sep string) []string {
	result := make([]string, 0)
	start := 0
	for i := 0; i < len(s); i++ {
		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	result = append(result, s[start:])
	return result
}

func trimStr(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for _, s := range strs[1:] {
		result += sep + s
	}
	return result
}
