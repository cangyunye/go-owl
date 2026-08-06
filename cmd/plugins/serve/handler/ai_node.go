package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"

	ai2 "github.com/cangyunye/go-owl/internal/ai"
)

// dbNodeStoreAdapter implements ai2.NodeStoreAdapter backed by the serve nodes
// table, so the AI agent sees the real node inventory in its prompts and in
// the local rule-based fallback.
type dbNodeStoreAdapter struct {
	db *sql.DB
}

const nodeSelectColumns = `id, COALESCE(name,''), COALESCE(address,''), COALESCE(port,22), COALESCE(status,'unknown'), COALESCE(groups,'[]'), COALESCE(labels,'{}'), COALESCE(created_at,''), COALESCE(updated_at,'')`

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanNodeInfo(s rowScanner) (*ai2.NodeInfoAdapter, error) {
	var n ai2.NodeInfoAdapter
	var groupsJSON, labelsJSON string
	if err := s.Scan(&n.ID, &n.Name, &n.Address, &n.Port, &n.Status, &groupsJSON, &labelsJSON, &n.CreatedAt, &n.UpdatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(groupsJSON), &n.Groups); err != nil {
		n.Groups = nil
	}
	if err := json.Unmarshal([]byte(labelsJSON), &n.Labels); err != nil {
		n.Labels = nil
	}
	return &n, nil
}

func (a *dbNodeStoreAdapter) List() ([]*ai2.NodeInfoAdapter, error) {
	rows, err := a.db.Query(`SELECT ` + nodeSelectColumns + ` FROM nodes`)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	defer rows.Close()

	var out []*ai2.NodeInfoAdapter
	for rows.Next() {
		n, err := scanNodeInfo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (a *dbNodeStoreAdapter) Get(id string) (*ai2.NodeInfoAdapter, error) {
	row := a.db.QueryRow(`SELECT `+nodeSelectColumns+` FROM nodes WHERE id = ?`, id)
	n, err := scanNodeInfo(row)
	if err != nil {
		return nil, err
	}
	return n, nil
}

func (a *dbNodeStoreAdapter) Add(*ai2.NodeInfoAdapter) error {
	return fmt.Errorf("add node via AI chat not supported")
}

func (a *dbNodeStoreAdapter) Remove(string) error {
	return fmt.Errorf("remove node via AI chat not supported")
}

func (a *dbNodeStoreAdapter) Update(*ai2.NodeInfoAdapter) error {
	return fmt.Errorf("update node via AI chat not supported")
}

func (a *dbNodeStoreAdapter) Save() error { return nil }

func (a *dbNodeStoreAdapter) Load() error { return nil }
