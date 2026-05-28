package node

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cangyunye/go-owl/internal/history"
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
	Status      string
	ProxyJump   string
	CreatedAt   string
	UpdatedAt   string
}

func NewLocalSource() (*LocalSource, error) {
	s := &LocalSource{
		nodes: make(map[string]*LocalNode),
	}
	if err := s.loadNodes(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *LocalSource) loadNodes() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("获取用户主目录失败: %w", err)
	}

	merged := make(map[string]*LocalNode)

	db := history.GetGlobalDB()
	if db != nil {
		conn := db.Connection()
		if conn != nil {
			rows, err := conn.Query(
				`SELECT id, name, address, port, user, password, ssh_key, status, groups, labels, proxy_jump, created_at, updated_at FROM nodes`)
			if err != nil {
				logger.Warn("Failed to query nodes from database",
					logger.WithOperation("node_load"),
					logger.WithField("error", err.Error()))
			} else {
				defer rows.Close()
				for rows.Next() {
					var id, name, address, user, password, sshKey, status, groupsStr, labelsStr, proxyJump string
					var port int
					var createdAt, updatedAt time.Time

					err := rows.Scan(&id, &name, &address, &port, &user, &password, &sshKey, &status, &groupsStr, &labelsStr, &proxyJump, &createdAt, &updatedAt)
					if err != nil {
						logger.Warn("Failed to scan node row from database",
							logger.WithOperation("node_load"),
							logger.WithField("error", err.Error()))
						continue
					}

					var groups []string
					var labels map[string]string
					json.Unmarshal([]byte(groupsStr), &groups)
					json.Unmarshal([]byte(labelsStr), &labels)

					node := &LocalNode{
						ID:          id,
						Name:        name,
						Address:     address,
						Port:        port,
						User:        user,
						SSHKey:      sshKey,
						SSHPassword: password,
						Status:      status,
						ProxyJump:   proxyJump,
						CreatedAt:   createdAt.Format(time.RFC3339),
						UpdatedAt:   updatedAt.Format(time.RFC3339),
						Groups:      groups,
						Labels:      labels,
					}
					merged[id] = node
					if name != "" && name != id {
						merged[name] = node
					}
				}
				if err := rows.Err(); err != nil {
					logger.Warn("Error iterating database node rows",
						logger.WithOperation("node_load"),
						logger.WithField("error", err.Error()))
				}
				logger.Info("Loaded nodes from database",
					logger.WithOperation("node_load"),
					logger.WithField("count", countDistinctNodes(merged)))
			}
		}
	}

	dataFile := filepath.Join(home, ".owl", "nodes.json")
	data, err := os.ReadFile(dataFile)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Debug("Nodes file does not exist, using database nodes only",
				logger.WithOperation("node_load"),
				logger.WithField("file", dataFile))
		} else {
			logger.Warn("Failed to read nodes file",
				logger.WithOperation("node_load"),
				logger.WithField("file", dataFile),
				logger.WithField("error", err.Error()))
		}
	} else {
		var jsonNodes []*struct {
			ID        string            `json:"id"`
			Name      string            `json:"name"`
			Address   string            `json:"address"`
			Port      int               `json:"port"`
			User      string            `json:"user"`
			Password  string            `json:"password,omitempty"`
			SSHKey    string            `json:"ssh_key,omitempty"`
			Status    string            `json:"status,omitempty"`
			ProxyJump string            `json:"proxy_jump,omitempty"`
			CreatedAt string            `json:"created_at,omitempty"`
			UpdatedAt string            `json:"updated_at,omitempty"`
			Groups    []string          `json:"groups"`
			Labels    map[string]string `json:"labels"`
		}
		if err := json.Unmarshal(data, &jsonNodes); err != nil {
			logger.Warn("Failed to parse nodes file, using database nodes only",
				logger.WithOperation("node_load"),
				logger.WithField("file", dataFile),
				logger.WithField("error", err.Error()))
		} else {
			for _, n := range jsonNodes {
				if existing, ok := merged[n.ID]; ok {
					if n.Name != "" {
						existing.Name = n.Name
					}
					if n.Address != "" {
						existing.Address = n.Address
					}
					if n.Port != 0 {
						existing.Port = n.Port
					}
					if n.User != "" {
						existing.User = n.User
					}
					if n.Password != "" {
						existing.SSHPassword = n.Password
					}
					if n.SSHKey != "" {
						existing.SSHKey = n.SSHKey
					}
					if n.Status != "" {
						existing.Status = n.Status
					}
					if n.ProxyJump != "" {
						existing.ProxyJump = n.ProxyJump
					}
					if n.CreatedAt != "" {
						existing.CreatedAt = n.CreatedAt
					}
					if n.UpdatedAt != "" {
						existing.UpdatedAt = n.UpdatedAt
					}
					if len(n.Groups) > 0 {
						existing.Groups = n.Groups
					}
					if len(n.Labels) > 0 {
						existing.Labels = n.Labels
					}
				} else {
					merged[n.ID] = &LocalNode{
						ID:          n.ID,
						Name:        n.Name,
						Address:     n.Address,
						Port:        n.Port,
						User:        n.User,
						SSHKey:      n.SSHKey,
						SSHPassword: n.Password,
						Status:      n.Status,
						ProxyJump:   n.ProxyJump,
						CreatedAt:   n.CreatedAt,
						UpdatedAt:   n.UpdatedAt,
						Groups:      n.Groups,
						Labels:      n.Labels,
					}
				}
				if n.Name != "" && n.Name != n.ID {
					merged[n.Name] = merged[n.ID]
				}
			}
			logger.Info("Loaded nodes from file (overlay)",
				logger.WithOperation("node_load"),
				logger.WithField("file_count", len(jsonNodes)))
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes = merged

	logger.Info("Total nodes loaded",
		logger.WithOperation("node_load"),
		logger.WithField("total_count", countDistinctNodes(merged)))

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

func countDistinctNodes(merged map[string]*LocalNode) int {
	seen := make(map[string]bool)
	for _, node := range merged {
		if !seen[node.ID] {
			seen[node.ID] = true
		}
	}
	return len(seen)
}
