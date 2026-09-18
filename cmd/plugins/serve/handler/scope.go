package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"strings"
)

// NodeScope 节点/分组范围授权：用户只能操作授权内的节点。
// 空（nil 或两组皆空）= 不限；admin 角色恒不限。
type NodeScope struct {
	Groups []string `json:"groups"`
	Nodes  []string `json:"nodes"`
}

// ParseNodeScope 解析存储的 scope JSON；空/全空/非法 = nil（不限）。
func ParseNodeScope(raw string) *NodeScope {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var ns NodeScope
	if err := json.Unmarshal([]byte(raw), &ns); err != nil {
		return nil
	}
	if len(ns.Groups) == 0 && len(ns.Nodes) == 0 {
		return nil
	}
	return &ns
}

// IsUnrestricted 是否不限范围。
func (ns *NodeScope) IsUnrestricted() bool {
	return ns == nil || (len(ns.Groups) == 0 && len(ns.Nodes) == 0)
}

// contains 判断 scope 是否覆盖目标（节点 ID 白名单 或 分组交集）。
func (ns *NodeScope) contains(nodeID string, groups []string) bool {
	if ns == nil {
		return true
	}
	for _, id := range ns.Nodes {
		if id == nodeID {
			return true
		}
	}
	for _, g := range groups {
		for _, allowed := range ns.Groups {
			if g == allowed {
				return true
			}
		}
	}
	return false
}

// ScopeChecker 按用户查询节点范围并过滤节点集合（页面与 AI 执行入口统一收口）。
type ScopeChecker struct {
	db *sql.DB
}

func NewScopeChecker(db *sql.DB) *ScopeChecker {
	return &ScopeChecker{db: db}
}

// userScope 返回用户名对应的 scope；admin/无 scope/查询失败 = nil（不限）。
func (s *ScopeChecker) userScope(ctx context.Context, username string) *NodeScope {
	if s == nil || s.db == nil || username == "" {
		return nil
	}
	var role, scopeRaw string
	err := s.db.QueryRowContext(ctx,
		`SELECT role, COALESCE(node_scope, '') FROM web_users WHERE username = ?`, username).Scan(&role, &scopeRaw)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("scope: 查询用户范围失败 user=%s: %v", username, err)
		}
		return nil
	}
	if role == "admin" {
		return nil
	}
	return ParseNodeScope(scopeRaw)
}

// FilterNodeIDs 过滤节点 ID 集合：保留节点 ID 白名单内或分组命中授权范围的节点。
func (s *ScopeChecker) FilterNodeIDs(ctx context.Context, username string, nodeIDs []string) []string {
	scope := s.userScope(ctx, username)
	if scope.IsUnrestricted() || len(nodeIDs) == 0 {
		return nodeIDs
	}
	allowed := map[string]bool{}
	// 一次查询取候选节点的分组
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(nodeIDs)), ",")
	args := make([]interface{}, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, COALESCE(groups, '[]') FROM nodes WHERE id IN (`+placeholders+`)`, args...)
	if err != nil {
		log.Printf("scope: 查询节点分组失败: %v", err)
		return nodeIDs // 查询失败不误伤（保守放行，与中间件容错风格一致）
	}
	defer rows.Close()
	for rows.Next() {
		var id, groupsJSON string
		if err := rows.Scan(&id, &groupsJSON); err != nil {
			continue
		}
		var groups []string
		_ = json.Unmarshal([]byte(groupsJSON), &groups)
		if scope.contains(id, groups) {
			allowed[id] = true
		}
	}
	out := make([]string, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		if allowed[id] {
			out = append(out, id)
		}
	}
	return out
}

// FilterNodeRows 过滤节点行（nodeID → 分组 的已解析形态，供 AI 查询复用）。
func (s *ScopeChecker) FilterNodeRows(ctx context.Context, username string, rows []NodeRowRef) []NodeRowRef {
	scope := s.userScope(ctx, username)
	if scope.IsUnrestricted() || len(rows) == 0 {
		return rows
	}
	out := make([]NodeRowRef, 0, len(rows))
	for _, r := range rows {
		if scope.contains(r.ID, r.Groups) {
			out = append(out, r)
		}
	}
	return out
}

// NodeRowRef 是节点行的最小过滤单元。
type NodeRowRef struct {
	ID     string
	Groups []string
}
