package ai

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	// SQLite 驱动：与 internal/history 一致（modernc 纯 Go 实现）
	_ "modernc.org/sqlite"
)

// SQLiteSessionStore 基于 SQLite 的会话持久化。
// CLI 与 serve 共享 ~/.owl/owl.db 时，两宿主的会话同库共存、互不干扰
// （以 host 列区分宿主，session_id 含用户命名空间）。
type SQLiteSessionStore struct {
	db *sql.DB
}

// NewSQLiteSessionStore 打开会话存储并确保表结构就绪（幂等）。
func NewSQLiteSessionStore(db *sql.DB) (*SQLiteSessionStore, error) {
	s := &SQLiteSessionStore{db: db}
	if err := s.EnsureSchema(); err != nil {
		return nil, err
	}
	return s, nil
}

// EnsureSchema 建表（幂等，可安全重复执行）。
func (s *SQLiteSessionStore) EnsureSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS ai_sessions (
			session_id TEXT NOT NULL,
			host       TEXT NOT NULL DEFAULT '',
			title      TEXT NOT NULL DEFAULT '',
			state      TEXT NOT NULL DEFAULT '{}',
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (session_id, host)
		)
	`)
	if err != nil {
		return fmt.Errorf("create ai_sessions table: %w", err)
	}
	_, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_ai_sessions_host ON ai_sessions(host, updated_at)`)
	if err != nil {
		return fmt.Errorf("create ai_sessions index: %w", err)
	}
	return nil
}

func (s *SQLiteSessionStore) Save(rec *SessionRecord) error {
	stateJSON, err := json.Marshal(rec.State)
	if err != nil {
		return fmt.Errorf("marshal session state: %w", err)
	}
	now := time.Now().UTC()
	_, err = s.db.Exec(`
		INSERT INTO ai_sessions (session_id, host, title, state, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id, host) DO UPDATE SET
			title = excluded.title,
			state = excluded.state,
			updated_at = excluded.updated_at
	`, rec.SessionID, rec.Host, rec.Title, string(stateJSON), now, now)
	if err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return nil
}

func (s *SQLiteSessionStore) Load(sessionID, host string) (*SessionRecord, error) {
	var rec SessionRecord
	var stateJSON string
	var createdAt, updatedAt sql.NullString
	err := s.db.QueryRow(`
		SELECT session_id, host, title, state, created_at, updated_at
		FROM ai_sessions WHERE session_id = ? AND host = ?
	`, sessionID, host).Scan(&rec.SessionID, &rec.Host, &rec.Title, &stateJSON, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found: %s@%s", sessionID, host)
	}
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}

	rec.State = &SessionState{}
	if err := json.Unmarshal([]byte(stateJSON), rec.State); err != nil {
		return nil, fmt.Errorf("unmarshal session state: %w", err)
	}
	if t, err := time.Parse(time.RFC3339, createdAt.String); err == nil {
		rec.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, updatedAt.String); err == nil {
		rec.UpdatedAt = t
	}
	return &rec, nil
}

func (s *SQLiteSessionStore) List(host string, limit int) ([]SessionMeta, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT session_id, host, title, updated_at
		FROM ai_sessions WHERE host = ?
		ORDER BY updated_at DESC LIMIT ?
	`, host, limit)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var metas []SessionMeta
	for rows.Next() {
		var m SessionMeta
		var updatedAt sql.NullString
		if err := rows.Scan(&m.SessionID, &m.Host, &m.Title, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan session meta: %w", err)
		}
		if t, err := time.Parse(time.RFC3339, updatedAt.String); err == nil {
			m.UpdatedAt = t
		}
		metas = append(metas, m)
	}
	return metas, rows.Err()
}

func (s *SQLiteSessionStore) Delete(sessionID, host string) error {
	_, err := s.db.Exec(`DELETE FROM ai_sessions WHERE session_id = ? AND host = ?`, sessionID, host)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}
