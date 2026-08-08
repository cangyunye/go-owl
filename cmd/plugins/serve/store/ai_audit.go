package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type AIAuditRecord struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	Intent         string    `json:"intent"`
	Tool           string    `json:"tool"`
	ParamsSnapshot string    `json:"params_snapshot"`
	Result         string    `json:"result"`
	TargetType     string    `json:"target_type"`
	TargetIDs      string    `json:"target_ids"`
	PromptText     string    `json:"prompt_text,omitempty"`
	ReplyText      string    `json:"reply_text,omitempty"`
	LLMModel       string    `json:"llm_model"`
	LLMDurationMs  int64     `json:"llm_duration_ms"`
	CreatedAt      time.Time `json:"created_at"`
}

type AIAuditStore struct {
	db *sql.DB
}

func NewAIAuditStore(db *sql.DB) *AIAuditStore {
	return &AIAuditStore{db: db}
}

func (s *AIAuditStore) Init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS ai_audit_log (
			id              TEXT PRIMARY KEY,
			user_id         TEXT NOT NULL,
			intent          TEXT NOT NULL DEFAULT '',
			tool            TEXT NOT NULL DEFAULT '',
			params_snapshot TEXT NOT NULL DEFAULT '{}',
			result          TEXT NOT NULL DEFAULT 'success',
			target_type     TEXT DEFAULT '',
			target_ids      TEXT DEFAULT '[]',
			prompt_text     TEXT DEFAULT '',
			reply_text      TEXT DEFAULT '',
			llm_model       TEXT DEFAULT '',
			llm_duration_ms INTEGER DEFAULT 0,
			created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("create ai_audit_log table: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_ai_audit_user ON ai_audit_log(user_id)
	`)
	if err != nil {
		return fmt.Errorf("create ai_audit_user index: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_ai_audit_time ON ai_audit_log(created_at)
	`)
	return err
}

func (s *AIAuditStore) Create(ctx context.Context, r *AIAuditRecord) error {
	r.ID = uuid.New().String()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO ai_audit_log (id, user_id, intent, tool, params_snapshot, result,
			target_type, target_ids, prompt_text, reply_text, llm_model, llm_duration_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, r.ID, r.UserID, r.Intent, r.Tool, r.ParamsSnapshot, r.Result,
		r.TargetType, r.TargetIDs, r.PromptText, r.ReplyText,
		r.LLMModel, r.LLMDurationMs, r.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert ai audit: %w", err)
	}
	return nil
}

func (s *AIAuditStore) List(ctx context.Context, userID string, offset, limit int) ([]*AIAuditRecord, int, error) {
	where := ""
	args := []interface{}{}
	if userID != "" {
		where = " WHERE user_id = ?"
		args = append(args, userID)
	}

	var total int
	countQuery := "SELECT COUNT(*) FROM ai_audit_log" + where
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count ai audit: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		"SELECT id, user_id, intent, tool, params_snapshot, result, target_type, target_ids, prompt_text, reply_text, llm_model, llm_duration_ms, created_at FROM ai_audit_log"+where+" ORDER BY created_at DESC LIMIT ? OFFSET ?",
		append(args, limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list ai audit: %w", err)
	}
	defer rows.Close()

	var records []*AIAuditRecord
	for rows.Next() {
		r := &AIAuditRecord{}
		var createdAt string
		if err := rows.Scan(&r.ID, &r.UserID, &r.Intent, &r.Tool, &r.ParamsSnapshot, &r.Result,
			&r.TargetType, &r.TargetIDs, &r.PromptText, &r.ReplyText, &r.LLMModel, &r.LLMDurationMs, &createdAt); err != nil {
			return nil, 0, fmt.Errorf("scan ai audit: %w", err)
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		records = append(records, r)
	}
	return records, total, nil
}

func (s *AIAuditStore) Get(ctx context.Context, id string) (*AIAuditRecord, error) {
	r := &AIAuditRecord{}
	var createdAt string
	err := s.db.QueryRowContext(ctx,
		"SELECT id, user_id, intent, tool, params_snapshot, result, target_type, target_ids, prompt_text, reply_text, llm_model, llm_duration_ms, created_at FROM ai_audit_log WHERE id = ?", id).
		Scan(&r.ID, &r.UserID, &r.Intent, &r.Tool, &r.ParamsSnapshot, &r.Result,
			&r.TargetType, &r.TargetIDs, &r.PromptText, &r.ReplyText, &r.LLMModel, &r.LLMDurationMs, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("get ai audit: %w", err)
	}
	r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return r, nil
}
