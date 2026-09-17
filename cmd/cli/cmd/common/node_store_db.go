package common

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "github.com/cangyunye/go-owl/internal/history"
	"github.com/cangyunye/go-owl/internal/secrets"
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
	rows, err := s.db.Query(`SELECT id, COALESCE(name, ''), COALESCE(address, ''), port, COALESCE(user, ''), COALESCE(password, ''), COALESCE(ssh_key, ''), COALESCE(status, 'unknown'), COALESCE(groups, '[]'), COALESCE(labels, '{}'), COALESCE(proxy_jump, ''), created_at, updated_at, last_check_at FROM nodes`)
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


// decryptCredentials 就地解密节点凭据(存储层经 internal/secrets 加密,
// 无前缀的存量明文原样通过)。
func (s *NodeStoreDB) decryptCredentials(node *NodeInfo) error {
	pw, err := secrets.Decrypt(node.Password)
	if err != nil {
		return err
	}
	key, err := secrets.Decrypt(node.SSHKey)
	if err != nil {
		return err
	}
	node.Password = pw
	node.SSHKey = key
	return nil
}

// ReencryptNodeCredentials 把 nodes 表中的明文凭据就地加密(一次性迁移,
// 不自动执行)。未设置 OWL_ENC_KEY 时返回错误;已加密与空值跳过。
// 返回实际加密的凭据条数(password/ssh_key 分别计数)。
func ReencryptNodeCredentials(db *sql.DB) (int, error) {
	if _, enabled, err := secrets.LoadKey(); err != nil {
		return 0, err
	} else if !enabled {
		return 0, fmt.Errorf("未设置 %s,拒绝执行迁移(加密处于关闭状态)", secrets.KeyEnvName)
	}

	rows, err := db.Query(`SELECT id, COALESCE(password, ''), COALESCE(ssh_key, '') FROM nodes`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type cred struct{ id, col, val string }
	var pending []cred
	for rows.Next() {
		var id, pw, key string
		if err := rows.Scan(&id, &pw, &key); err != nil {
			return 0, err
		}
		if pw != "" && !strings.HasPrefix(pw, secrets.Prefix) {
			pending = append(pending, cred{id, "password", pw})
		}
		if key != "" && !strings.HasPrefix(key, secrets.Prefix) {
			pending = append(pending, cred{id, "ssh_key", key})
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	rows.Close()

	count := 0
	for _, c := range pending {
		enc, err := secrets.Encrypt(c.val)
		if err != nil {
			return count, err
		}
		if _, err := db.Exec(`UPDATE nodes SET `+c.col+` = ? WHERE id = ?`, enc, c.id); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// encryptCredentials 返回加密后的凭据副本(未设 OWL_ENC_KEY 时为明文)。
func encryptCredentials(node *NodeInfo) (string, string, error) {
	pw, err := secrets.Encrypt(node.Password)
	if err != nil {
		return "", "", err
	}
	key, err := secrets.Encrypt(node.SSHKey)
	if err != nil {
		return "", "", err
	}
	return pw, key, nil
}

func (s *NodeStoreDB) List() ([]*NodeInfo, error) {
	s.ensureConsistent()
	nodes, err := s.listInternal()
	if err != nil {
		return nil, err
	}
	for _, n := range nodes {
		if err := s.decryptCredentials(n); err != nil {
			return nil, err
		}
	}
	return nodes, nil
}

func (s *NodeStoreDB) Get(id string) (*NodeInfo, error) {
	s.ensureConsistent()
	node := &NodeInfo{}
	var groupsJSON, labelsJSON string
	var lastCheckAt sql.NullString
	err := s.db.QueryRow(
		`SELECT id, COALESCE(name, ''), COALESCE(address, ''), port, COALESCE(user, ''), COALESCE(password, ''), COALESCE(ssh_key, ''), COALESCE(status, 'unknown'), COALESCE(groups, '[]'), COALESCE(labels, '{}'), COALESCE(proxy_jump, ''), created_at, updated_at, last_check_at FROM nodes WHERE id = ?`,
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
	if err := s.decryptCredentials(node); err != nil {
		return nil, err
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
	encPw, encKey, err := encryptCredentials(node)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO nodes (id, name, address, port, user, password, ssh_key, status, groups, labels, proxy_jump, created_at, updated_at, last_check_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		node.ID, node.Name, node.Address, node.Port,
		node.User, encPw, encKey, node.Status,
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
	encPw, encKey, err := encryptCredentials(node)
	if err != nil {
		return err
	}
	result, err := s.db.Exec(
		`UPDATE nodes SET name=?, address=?, port=?, user=?, password=?, ssh_key=?, status=?, groups=?, labels=?, proxy_jump=?, updated_at=?, last_check_at=? WHERE id=?`,
		node.Name, node.Address, node.Port,
		node.User, encPw, encKey, node.Status,
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
		encPw, encKey, encErr := encryptCredentials(node)
		if encErr != nil {
			return encErr
		}
		node.Password, node.SSHKey = encPw, encKey
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
