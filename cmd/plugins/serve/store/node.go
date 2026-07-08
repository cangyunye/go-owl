package store

import (
	"context"
	"database/sql"
	"encoding/json"
)

type NodeStore struct {
	db *sql.DB
}

func NewNodeStore(db *sql.DB) *NodeStore {
	return &NodeStore{db: db}
}

func (s *NodeStore) ListByGroups(ctx context.Context, groups []string) ([]string, error) {
	if len(groups) == 0 {
		return nil, nil
	}

	rows, err := s.db.QueryContext(ctx, `SELECT id, groups FROM nodes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groupSet := make(map[string]bool, len(groups))
	for _, g := range groups {
		groupSet[g] = true
	}

	var ids []string
	for rows.Next() {
		var id, groupsJSON string
		if err := rows.Scan(&id, &groupsJSON); err != nil {
			continue
		}
		var nodeGroups []string
		if err := json.Unmarshal([]byte(groupsJSON), &nodeGroups); err != nil {
			continue
		}
		for _, ng := range nodeGroups {
			if groupSet[ng] {
				ids = append(ids, id)
				break
			}
		}
	}
	return ids, rows.Err()
}
