package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	nodeselect "github.com/cangyunye/go-owl/internal/node/select"
)

// dbNodeSource 从 serve 的 nodes 表加载节点，供共享选择器精确过滤。
// 界面搜索框的模糊查询在 node.go 的列表 API，与此无关。
type dbNodeSource struct {
	db *sql.DB
}

func (s *dbNodeSource) List(ctx context.Context) ([]nodeselect.NodeRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, COALESCE(name,''), COALESCE(groups,'[]'), COALESCE(labels,'{}'), COALESCE(status,'') FROM nodes`)
	if err != nil {
		return nil, fmt.Errorf("查询节点表失败: %w", err)
	}
	defer rows.Close()

	var out []nodeselect.NodeRow
	for rows.Next() {
		var r nodeselect.NodeRow
		var groupsJSON, labelsJSON string
		if err := rows.Scan(&r.ID, &r.Name, &groupsJSON, &labelsJSON, &r.Status); err != nil {
			return nil, fmt.Errorf("读取节点行失败: %w", err)
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
