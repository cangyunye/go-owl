package common

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "github.com/cangyunye/go-owl/internal/history"
	"github.com/cangyunye/go-owl/internal/logger"
)

var _ NodeStore = (*NodeStoreDB)(nil)

type NodeStoreDB struct {
	db        *sql.DB
	checkOnce sync.Once
	// conflictPrompt 控制 ensureConsistent 是否交互式解决节点冲突。
	// 默认开启(保持原有行为);TUI 等读路径不应被交互提示阻塞的场景可关闭。
	conflictPrompt bool
}

func NewNodeStoreDB(db *sql.DB) *NodeStoreDB {
	return &NodeStoreDB{db: db, conflictPrompt: true}
}

// SetConflictPrompt 开关 ensureConsistent 的交互式冲突提示。
// 关闭后冲突检测仅记录警告,不弹交互提示、不阻塞读路径。
func (s *NodeStoreDB) SetConflictPrompt(enabled bool) {
	s.conflictPrompt = enabled
}

func (s *NodeStoreDB) ensureConsistent() {
	s.checkOnce.Do(func() {
		dbNodes, err := s.listInternal()
		if err != nil {
			return
		}
		jsonNodes, err := ReadNodesFromJSON(NodeJSONPath())
		if err != nil || jsonNodes == nil {
			return
		}
		if !s.conflictPrompt {
			// 非交互:仅检测并告警,不弹交互提示、不阻塞读路径(TUI 等场景)
			if conflicts := DetectConflicts(dbNodes, jsonNodes); len(conflicts) > 0 {
				logger.Warn("node data inconsistency detected; reconcile via conflict resolution before exec commands",
					logger.WithField("conflicts", len(conflicts)))
			}
			return
		}
		if err := resolveNodeConflicts(s.db, dbNodes, jsonNodes); err != nil {
			logger.Warn("node data inconsistency detected", logger.WithError(err))
		}
	})
}

func (s *NodeStoreDB) listInternal() ([]*NodeInfo, error) {
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

func (s *NodeStoreDB) List() ([]*NodeInfo, error) {
	s.ensureConsistent()
	return s.listInternal()
}

func (s *NodeStoreDB) Get(id string) (*NodeInfo, error) {
	s.ensureConsistent()
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
	var lastCheckAt interface{}
	if node.LastCheckAt == "" {
		lastCheckAt = nil
	} else {
		lastCheckAt = node.LastCheckAt
	}
	result, err := s.db.Exec(
		`UPDATE nodes SET name=?, address=?, port=?, user=?, password=?, ssh_key=?, status=?, groups=?, labels=?, proxy_jump=?, updated_at=?, last_check_at=? WHERE id=?`,
		node.Name, node.Address, node.Port,
		node.User, node.Password, node.SSHKey, node.Status,
		string(groupsJSON), string(labelsJSON), node.ProxyJump,
		node.UpdatedAt, lastCheckAt, node.ID,
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

func (s *NodeStoreDB) BulkUpsert(nodes []*NodeInfo) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO nodes (id, name, address, port, user, password, ssh_key, status, groups, labels, proxy_jump, created_at, updated_at, last_check_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().Format(time.RFC3339)
	for _, node := range nodes {
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
		if node.CreatedAt == "" {
			node.CreatedAt = now
		}
		if node.UpdatedAt == "" {
			node.UpdatedAt = now
		}
		var lastCheckAt interface{}
		if node.LastCheckAt == "" {
			lastCheckAt = nil
		} else {
			lastCheckAt = node.LastCheckAt
		}
		_, err = stmt.Exec(
			node.ID, node.Name, node.Address, node.Port,
			node.User, node.Password, node.SSHKey, node.Status,
			string(groupsJSON), string(labelsJSON), node.ProxyJump,
			node.CreatedAt, node.UpdatedAt, lastCheckAt,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *NodeStoreDB) Save() error {
	return nil
}

func (s *NodeStoreDB) Load() error {
	return nil
}
