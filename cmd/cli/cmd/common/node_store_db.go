package common

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/cangyunye/go-owl/internal/history"
)

var _ NodeStore = (*NodeStoreDB)(nil)

type NodeStoreDB struct {
	db *sql.DB
}

func NewNodeStoreDB(db *sql.DB) *NodeStoreDB {
	return &NodeStoreDB{db: db}
}

func (s *NodeStoreDB) List() ([]*NodeInfo, error) {
	rows, err := s.db.Query(`SELECT id, name, address, port, user, password, ssh_key, status, groups, labels, proxy_jump, created_at, updated_at, last_check_at FROM nodes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*NodeInfo
	for rows.Next() {
		node := &NodeInfo{}
		var groupsJSON, labelsJSON string
		var lastCheckAt sql.NullString
		err := rows.Scan(
			&node.ID, &node.Name, &node.Address, &node.Port,
			&node.User, &node.Password, &node.SSHKey, &node.Status,
			&groupsJSON, &labelsJSON, &node.ProxyJump,
			&node.CreatedAt, &node.UpdatedAt, &lastCheckAt,
		)
		if err != nil {
			return nil, err
		}
		if lastCheckAt.Valid {
			node.LastCheckAt = lastCheckAt.String
		}
		json.Unmarshal([]byte(groupsJSON), &node.Groups)
		json.Unmarshal([]byte(labelsJSON), &node.Labels)
		if node.Groups == nil {
			node.Groups = []string{}
		}
		if node.Labels == nil {
			node.Labels = map[string]string{}
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

func (s *NodeStoreDB) Get(id string) (*NodeInfo, error) {
	node := &NodeInfo{}
	var groupsJSON, labelsJSON string
	var lastCheckAt sql.NullString
	err := s.db.QueryRow(
		`SELECT id, name, address, port, user, password, ssh_key, status, groups, labels, proxy_jump, created_at, updated_at, last_check_at FROM nodes WHERE id = ?`,
		id,
	).Scan(
		&node.ID, &node.Name, &node.Address, &node.Port,
		&node.User, &node.Password, &node.SSHKey, &node.Status,
		&groupsJSON, &labelsJSON, &node.ProxyJump,
		&node.CreatedAt, &node.UpdatedAt, &lastCheckAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("node not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	if lastCheckAt.Valid {
		node.LastCheckAt = lastCheckAt.String
	}
	json.Unmarshal([]byte(groupsJSON), &node.Groups)
	json.Unmarshal([]byte(labelsJSON), &node.Labels)
	if node.Groups == nil {
		node.Groups = []string{}
	}
	if node.Labels == nil {
		node.Labels = map[string]string{}
	}
	return node, nil
}

func (s *NodeStoreDB) Add(node *NodeInfo) error {
	if node.Groups == nil {
		node.Groups = []string{}
	}
	if node.Labels == nil {
		node.Labels = map[string]string{}
	}
	groupsJSON, err := json.Marshal(node.Groups)
	if err != nil {
		return fmt.Errorf("marshal groups: %w", err)
	}
	labelsJSON, err := json.Marshal(node.Labels)
	if err != nil {
		return fmt.Errorf("marshal labels: %w", err)
	}
	now := time.Now().Format(time.RFC3339)
	node.CreatedAt = now
	node.UpdatedAt = now
	var lastCheckAt interface{}
	if node.LastCheckAt == "" {
		lastCheckAt = nil
	} else {
		lastCheckAt = node.LastCheckAt
	}
	_, err = s.db.Exec(
		`INSERT INTO nodes (id, name, address, port, user, password, ssh_key, status, groups, labels, proxy_jump, created_at, updated_at, last_check_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		node.ID, node.Name, node.Address, node.Port,
		node.User, node.Password, node.SSHKey, node.Status,
		string(groupsJSON), string(labelsJSON), node.ProxyJump,
		node.CreatedAt, node.UpdatedAt, lastCheckAt,
	)
	return err
}

func (s *NodeStoreDB) Remove(id string) error {
	result, err := s.db.Exec(`DELETE FROM nodes WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("node not found: %s", id)
	}
	return nil
}

func (s *NodeStoreDB) Update(node *NodeInfo) error {
	groupsJSON, err := json.Marshal(node.Groups)
	if err != nil {
		return fmt.Errorf("marshal groups: %w", err)
	}
	labelsJSON, err := json.Marshal(node.Labels)
	if err != nil {
		return fmt.Errorf("marshal labels: %w", err)
	}
	node.UpdatedAt = time.Now().Format(time.RFC3339)
	result, err := s.db.Exec(
		`UPDATE nodes SET name=?, address=?, port=?, user=?, password=?, ssh_key=?, status=?, groups=?, labels=?, proxy_jump=?, updated_at=? WHERE id=?`,
		node.Name, node.Address, node.Port,
		node.User, node.Password, node.SSHKey, node.Status,
		string(groupsJSON), string(labelsJSON), node.ProxyJump,
		node.UpdatedAt, node.ID,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("node not found: %s", node.ID)
	}
	return nil
}

func (s *NodeStoreDB) Save() error {
	return nil
}

func (s *NodeStoreDB) Load() error {
	return nil
}
