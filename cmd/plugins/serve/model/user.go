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
}
