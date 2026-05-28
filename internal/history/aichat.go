package history

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type AiChat struct {
	ID          string `json:"id"`
	SessionID   string `json:"session_id"`
	Step        string `json:"step"`
	Role        string `json:"role"`
	Prompt      string `json:"prompt,omitempty"`
	Input       string `json:"input,omitempty"`
	Output      string `json:"output,omitempty"`
	ToolCalls   string `json:"tool_calls,omitempty"`
	ToolResults string `json:"tool_results,omitempty"`
	DurationMs  int64  `json:"duration_ms"`
	Error       string `json:"error,omitempty"`
	Metadata    string `json:"metadata,omitempty"`
	CreatedAt   string `json:"created_at"`
}

func RecordAiChat(db *sql.DB, chat *AiChat) error {
	if db == nil {
		return nil
	}
	if chat.ID == "" {
		chat.ID = uuid.New().String()
	}
	if chat.CreatedAt == "" {
		chat.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	query := `INSERT INTO aichat (id, session_id, step, role, prompt, input, output, tool_calls, tool_results, duration_ms, error, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := db.Exec(query,
		chat.ID, chat.SessionID, chat.Step, chat.Role,
		chat.Prompt, chat.Input, chat.Output,
		chat.ToolCalls, chat.ToolResults,
		chat.DurationMs, chat.Error, chat.Metadata, chat.CreatedAt,
	)
	return err
}

type AiChatSession struct {
	SessionID  string `json:"session_id"`
	FirstInput string `json:"first_input"`
	ToolName   string `json:"tool_name"`
	StepCount  int    `json:"step_count"`
	DurationMs int64  `json:"duration_ms"`
	StartTime  string `json:"start_time"`
}

func QueryAiChatSessions(db *sql.DB, sessionID string, limit int) ([]AiChatSession, error) {
	if db == nil {
		return nil, nil
	}

	where := ""
	args := []interface{}{}
	if sessionID != "" {
		where = "WHERE a.session_id = ?"
		args = append(args, sessionID)
	}

	query := fmt.Sprintf(`
		SELECT
			a.session_id,
			(SELECT input FROM aichat WHERE session_id = a.session_id AND role = 'user' ORDER BY created_at ASC LIMIT 1) as first_input,
			(SELECT tool_calls FROM aichat WHERE session_id = a.session_id AND tool_calls IS NOT NULL AND tool_calls != '' ORDER BY created_at ASC LIMIT 1) as tool_name,
			COUNT(*) as step_count,
			COALESCE(SUM(a.duration_ms), 0) as duration_ms,
			MIN(a.created_at) as start_time
		FROM aichat a
		%s
		GROUP BY a.session_id
		ORDER BY start_time DESC
		LIMIT ?`, where)

	args = append(args, limit)
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []AiChatSession
	for rows.Next() {
		var s AiChatSession
		var toolName sql.NullString
		err := rows.Scan(&s.SessionID, &s.FirstInput, &toolName, &s.StepCount, &s.DurationMs, &s.StartTime)
		if err != nil {
			return nil, err
		}
		if toolName.Valid && toolName.String != "" {
			if idx := strings.Index(toolName.String, `"name":"`); idx != -1 {
				start := idx + 8
				if end := strings.Index(toolName.String[start:], `"`); end != -1 {
					s.ToolName = toolName.String[start : start+end]
				}
			}
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func QueryAiChatSteps(db *sql.DB, sessionID string) ([]AiChat, error) {
	if db == nil {
		return nil, nil
	}

	rows, err := db.Query(
		`SELECT id, session_id, step, role, output, tool_calls, tool_results, duration_ms, error, created_at
		FROM aichat WHERE session_id = ? ORDER BY created_at ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var steps []AiChat
	for rows.Next() {
		var s AiChat
		err := rows.Scan(&s.ID, &s.SessionID, &s.Step, &s.Role, &s.Output, &s.ToolCalls, &s.ToolResults, &s.DurationMs, &s.Error, &s.CreatedAt)
		if err != nil {
			return nil, err
		}
		steps = append(steps, s)
	}
	return steps, nil
}

func CleanAiChat(db *sql.DB, days int) (int64, error) {
	if db == nil {
		return 0, nil
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format(time.RFC3339)
	result, err := db.Exec(`DELETE FROM aichat WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func RecordAiChatGlobal(chat *AiChat) error {
	db := GetGlobalDB()
	if db == nil {
		return nil
	}
	return RecordAiChat(db.Connection(), chat)
}

func QueryAiChatSessionsGlobal(sessionID string, limit int) ([]AiChatSession, error) {
	db := GetGlobalDB()
	if db == nil {
		return nil, nil
	}
	return QueryAiChatSessions(db.Connection(), sessionID, limit)
}

func QueryAiChatStepsGlobal(sessionID string) ([]AiChat, error) {
	db := GetGlobalDB()
	if db == nil {
		return nil, nil
	}
	return QueryAiChatSteps(db.Connection(), sessionID)
}

func CleanAiChatGlobal(days int) (int64, error) {
	db := GetGlobalDB()
	if db == nil {
		return 0, nil
	}
	return CleanAiChat(db.Connection(), days)
}
