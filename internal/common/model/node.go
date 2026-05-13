package model

import (
	"encoding/json"
	"fmt"
	"time"
)

type NodeStatus string

const (
	NodeStatusOnline  NodeStatus = "online"
	NodeStatusOffline NodeStatus = "offline"
	NodeStatusUnknown NodeStatus = "unknown"
)

type Node struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Address   string            `json:"address"`
	Port      int               `json:"port"`
	User      string            `json:"user"`
	Status    NodeStatus        `json:"status"`
	Groups    []string          `json:"groups"`
	Labels    map[string]string `json:"labels"`
	Metadata  map[string]string `json:"metadata"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

func NewNode(id, name, address string, port int, user string) *Node {
	now := time.Now()
	return &Node{
		ID:        id,
		Name:      name,
		Address:   address,
		Port:      port,
		User:      user,
		Status:    NodeStatusOffline,
		Groups:    make([]string, 0),
		Labels:    make(map[string]string),
		Metadata:  make(map[string]string),
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (n *Node) Validate() error {
	if n.ID == "" {
		return fmt.Errorf("node ID is required")
	}
	if n.Name == "" {
		return fmt.Errorf("node name is required")
	}
	if n.Address == "" {
		return fmt.Errorf("node address is required")
	}
	if n.Port <= 0 || n.Port > 65535 {
		return fmt.Errorf("node port must be between 1 and 65535")
	}
	return nil
}

func (n *Node) SetStatus(status NodeStatus) {
	n.Status = status
	n.UpdatedAt = time.Now()
}

func (n *Node) AddGroup(group string) {
	for _, g := range n.Groups {
		if g == group {
			return
		}
	}
	n.Groups = append(n.Groups, group)
	n.UpdatedAt = time.Now()
}

func (n *Node) RemoveGroup(group string) {
	newGroups := make([]string, 0)
	for _, g := range n.Groups {
		if g != group {
			newGroups = append(newGroups, g)
		}
	}
	n.Groups = newGroups
	n.UpdatedAt = time.Now()
}

func (n *Node) SetLabel(key, value string) {
	n.Labels[key] = value
	n.UpdatedAt = time.Now()
}

func (n *Node) RemoveLabel(key string) {
	delete(n.Labels, key)
	n.UpdatedAt = time.Now()
}

func (n *Node) SetMetadata(key, value string) {
	n.Metadata[key] = value
	n.UpdatedAt = time.Now()
}

func (n *Node) HasGroup(group string) bool {
	for _, g := range n.Groups {
		if g == group {
			return true
		}
	}
	return false
}

func (n *Node) HasLabel(key, value string) bool {
	v, ok := n.Labels[key]
	return ok && v == value
}

func (n *Node) MatchLabels(labels map[string]string) bool {
	for key, value := range labels {
		if !n.HasLabel(key, value) {
			return false
		}
	}
	return true
}

func (n *Node) JSONSerialize() ([]byte, error) {
	return json.Marshal(n)
}

func (n *Node) JSONDeserialize(data []byte) error {
	return json.Unmarshal(data, n)
}

func (n *Node) Clone() *Node {
	clone := &Node{
		ID:        n.ID,
		Name:      n.Name,
		Address:   n.Address,
		Port:      n.Port,
		User:      n.User,
		Status:    n.Status,
		Groups:    make([]string, len(n.Groups)),
		Labels:    make(map[string]string),
		Metadata:  make(map[string]string),
		CreatedAt: n.CreatedAt,
		UpdatedAt: n.UpdatedAt,
	}
	copy(clone.Groups, n.Groups)
	for k, v := range n.Labels {
		clone.Labels[k] = v
	}
	for k, v := range n.Metadata {
		clone.Metadata[k] = v
	}
	return clone
}
