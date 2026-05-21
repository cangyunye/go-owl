package node

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/cangyunye/go-owl/internal/logger"
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

func NewLocalSource() (*LocalSource, error) {
	s := &LocalSource{
		nodes: make(map[string]*LocalNode),
	}
	if err := s.loadFromFile(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *LocalSource) loadFromFile() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("获取用户主目录失败: %w", err)
	}
	dataFile := filepath.Join(home, ".owl", "nodes.json")

	data, err := os.ReadFile(dataFile)
	if err != nil {
		// 如果文件不存在，这是正常的，返回空列表
		if os.IsNotExist(err) {
			logger.Debug("Nodes file does not exist, starting with empty node list",
				logger.WithOperation("node_load"),
				logger.WithField("file", dataFile))
			return nil
		}
		// 其他读取错误才返回错误
		return fmt.Errorf("读取节点文件 %s 失败: %w", dataFile, err)
	}

	var nodes []*struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Address  string `json:"address"`
		Port     int    `json:"port"`
		User     string `json:"user"`
		Password string `json:"password,omitempty"`
		SSHKey   string `json:"ssh_key,omitempty"`
		Groups   []string `json:"groups"`
		Labels   map[string]string `json:"labels"`
	}
	if err := json.Unmarshal(data, &nodes); err != nil {
		return fmt.Errorf("解析节点文件 %s 失败: %w", dataFile, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, n := range nodes {
		localNode := &LocalNode{
			ID:          n.ID,
			Name:        n.Name,
			Address:     n.Address,
			Port:        n.Port,
			User:        n.User,
			Groups:      n.Groups,
			Labels:      n.Labels,
			SSHKey:      n.SSHKey,
			SSHPassword: n.Password,
		}
		s.nodes[n.ID] = localNode
		if n.Name != "" && n.Name != n.ID {
			s.nodes[n.Name] = localNode
		}
	}
	logger.Info("Loaded nodes from file", 
		logger.WithOperation("node_load"),
		logger.WithField("count", len(nodes)))
	
	return nil
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

	seen := make(map[string]bool)
	nodes := make([]*LocalNode, 0, len(s.nodes))
	for _, node := range s.nodes {
		if seen[node.ID] {
			continue
		}
		seen[node.ID] = true
		
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
	logger.Info("Node added", 
		logger.WithOperation("node_add"),
		logger.WithField("node_id", node.ID),
		logger.WithField("node_name", node.Name))
	return nil
}

func (s *LocalSource) RemoveNode(idOrName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	node, ok := s.nodes[idOrName]
	if !ok {
		logger.Warn("Attempted to remove non-existent node", 
			logger.WithOperation("node_remove"),
			logger.WithField("node_id", idOrName))
		return fmt.Errorf("node not found: %s", idOrName)
	}

	delete(s.nodes, node.ID)
	if node.Name != "" {
		delete(s.nodes, node.Name)
	}
	logger.Info("Node removed", 
		logger.WithOperation("node_remove"),
		logger.WithField("node_id", node.ID))
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
