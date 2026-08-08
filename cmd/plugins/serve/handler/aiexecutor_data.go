package handler

import (
	"context"
	"encoding/json"
	"fmt"
)

// nodeRow is the structured representation of a nodes-table row used by the AI
// data layer; renderers (text/markdown) turn it into display output.
type nodeRow struct {
	ID      string
	Name    string
	Address string
	Port    int
	User    string
	Status  string
	Groups  []string
	Labels  map[string]string
}

// queryNodeRows is the shared data layer for query_nodes / query_database: it
// runs the SQL and returns structured rows, leaving presentation to renderers.
func (e *WebExecutor) queryNodeRows(ctx context.Context, group, status, search string) ([]nodeRow, error) {
	query := "SELECT id, name, address, port, user, status, COALESCE(groups, '[]'), COALESCE(labels, '{}') FROM nodes WHERE 1=1"
	args := []interface{}{}

	if group != "" {
		query += " AND groups LIKE ?"
		args = append(args, "%\""+group+"\"%")
	}
	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}
	if search != "" {
		query += " AND (name LIKE ? OR address LIKE ?)"
		args = append(args, "%"+search+"%", "%"+search+"%")
	}

	rows, err := e.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query node rows: %w", err)
	}
	defer rows.Close()

	var out []nodeRow
	for rows.Next() {
		var r nodeRow
		var groupsJSON, labelsJSON string
		if err := rows.Scan(&r.ID, &r.Name, &r.Address, &r.Port, &r.User, &r.Status, &groupsJSON, &labelsJSON); err != nil {
			continue
		}
		if err := json.Unmarshal([]byte(groupsJSON), &r.Groups); err != nil {
			r.Groups = nil
		}
		if err := json.Unmarshal([]byte(labelsJSON), &r.Labels); err != nil {
			r.Labels = nil
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
