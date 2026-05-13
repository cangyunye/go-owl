package history

import (
	"encoding/json"
	"time"
)

// Session 会话记录
type Session struct {
	ID           string     `json:"id"`
	Mode         string     `json:"mode"`
	NodeIDs      []string   `json:"node_ids"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	ClosedAt     *time.Time `json:"closed_at,omitempty"`
	CommandCount int        `json:"command_count"`
	SuccessCount int        `json:"success_count"`
	ErrorCount   int        `json:"error_count"`
}

// SessionCommand 会话命令记录
type SessionCommand struct {
	ID         int64      `json:"id"`
	SessionID  string     `json:"session_id"`
	Command    string     `json:"command"`
	NodeID     string     `json:"node_id"`
	Targets    []string   `json:"targets,omitempty"`
	ExitCode   int        `json:"exit_code"`
	Output     string     `json:"output,omitempty"`
	Error      string     `json:"error,omitempty"`
	Duration   int64      `json:"duration_ms"`
	ExecutedAt time.Time  `json:"executed_at"`
}

// RecordSession 记录会话开始
func RecordSession(session *Session) error {
	if GetGlobalDB() == nil {
		return nil
	}

	nodeIDsJSON, err := json.Marshal(session.NodeIDs)
	if err != nil {
		return err
	}

	_, err = GetGlobalDB().Connection().Exec(`
		INSERT INTO sessions (id, mode, node_ids, status, created_at, command_count, success_count, error_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, session.ID, session.Mode, nodeIDsJSON, session.Status, session.CreatedAt, session.CommandCount, session.SuccessCount, session.ErrorCount)

	return err
}

// UpdateSession 更新会话信息
func UpdateSession(session *Session) error {
	if GetGlobalDB() == nil {
		return nil
	}

	var closedAt interface{}
	if session.ClosedAt != nil {
		closedAt = session.ClosedAt
	}

	_, err := GetGlobalDB().Connection().Exec(`
		UPDATE sessions SET status = ?, closed_at = ?, command_count = ?, success_count = ?, error_count = ?
		WHERE id = ?
	`, session.Status, closedAt, session.CommandCount, session.SuccessCount, session.ErrorCount, session.ID)

	return err
}

// RecordSessionCommand 记录会话命令
func RecordSessionCommand(cmd *SessionCommand) error {
	if GetGlobalDB() == nil {
		return nil
	}

	targetsJSON, err := json.Marshal(cmd.Targets)
	if err != nil {
		targetsJSON = nil
	}

	_, err = GetGlobalDB().Connection().Exec(`
		INSERT INTO session_commands (session_id, command, targets, executed_at)
		VALUES (?, ?, ?, ?)
	`, cmd.SessionID, cmd.Command, targetsJSON, cmd.ExecutedAt)

	return err
}

// GetSession 根据 ID 获取会话
func GetSession(sessionID string) (*Session, error) {
	if GetGlobalDB() == nil {
		return nil, nil
	}

	row := GetGlobalDB().Connection().QueryRow(`
		SELECT id, mode, node_ids, status, created_at, closed_at, command_count, success_count, error_count
		FROM sessions WHERE id = ?
	`, sessionID)

	var s Session
	var nodeIDsJSON []byte
	var closedAt *time.Time

	err := row.Scan(&s.ID, &s.Mode, &nodeIDsJSON, &s.Status, &s.CreatedAt, &closedAt, &s.CommandCount, &s.SuccessCount, &s.ErrorCount)
	if err != nil {
		return nil, err
	}

	json.Unmarshal(nodeIDsJSON, &s.NodeIDs)
	s.ClosedAt = closedAt

	return &s, nil
}

// QuerySessions 查询会话列表
func QuerySessions(limit int) ([]*Session, error) {
	if GetGlobalDB() == nil {
		return nil, nil
	}

	query := `SELECT id, mode, node_ids, status, created_at, closed_at, command_count, success_count, error_count
		FROM sessions ORDER BY created_at DESC LIMIT ?`

	rows, err := GetGlobalDB().Connection().Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		var s Session
		var nodeIDsJSON []byte
		var closedAt *time.Time

		err := rows.Scan(&s.ID, &s.Mode, &nodeIDsJSON, &s.Status, &s.CreatedAt, &closedAt, &s.CommandCount, &s.SuccessCount, &s.ErrorCount)
		if err != nil {
			continue
		}

		json.Unmarshal(nodeIDsJSON, &s.NodeIDs)
		s.ClosedAt = closedAt
		sessions = append(sessions, &s)
	}

	return sessions, nil
}

// QuerySessionCommands 查询会话命令
func QuerySessionCommands(sessionID string, nodeID string, since time.Duration, limit int) ([]*SessionCommand, error) {
	if GetGlobalDB() == nil {
		return nil, nil
	}

	query := `SELECT id, session_id, command, targets, executed_at FROM session_commands WHERE 1=1`
	args := []interface{}{}

	if sessionID != "" {
		query += ` AND session_id = ?`
		args = append(args, sessionID)
	}

	if since > 0 {
		sinceTime := time.Now().Add(-since)
		query += ` AND executed_at >= ?`
		args = append(args, sinceTime)
	}

	query += ` ORDER BY executed_at DESC`

	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := GetGlobalDB().Connection().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var commands []*SessionCommand
	for rows.Next() {
		var cmd SessionCommand
		var targetsJSON []byte

		err := rows.Scan(&cmd.ID, &cmd.SessionID, &cmd.Command, &targetsJSON, &cmd.ExecutedAt)
		if err != nil {
			continue
		}

		if targetsJSON != nil {
			json.Unmarshal(targetsJSON, &cmd.Targets)
		}
		commands = append(commands, &cmd)
	}

	return commands, nil
}
