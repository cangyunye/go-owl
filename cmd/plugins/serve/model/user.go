package model

type Role string

const (
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleEditor   Role = "editor"
	RoleViewer   Role = "viewer"
)

type User struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	Role         Role   `json:"role"`
	DisplayName  string `json:"display_name,omitempty"`
	// NodeScope 节点/分组范围授权（JSON: {"groups":[],"nodes":[]}）；
	// 空 = 不限；admin 恒不限。
	NodeScope string `json:"node_scope,omitempty"`
}
