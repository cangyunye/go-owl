package model

// UserCommand 用户级快捷命令:用户拥有、命名过的命令模板(见 CONTEXT.md Shortcut Command)。
type UserCommand struct {
	ID       int64  `json:"id"`
	UserID   int64  `json:"user_id"`
	Name     string `json:"name"`
	Command  string `json:"command"`
	Position int    `json:"position"`
}
