package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// AI 高危操作审批单状态
const (
	AIApprovalPending  = "pending"
	AIApprovalExecuted = "executed"
	AIApprovalFailed   = "failed"
	AIApprovalRejected = "rejected"
)

// AIApproval 是 AI 聊天中高危写操作的持久审批单：
// 确认门挂起时创建，聊天框确认或从审批入口批准后执行。
type AIApproval struct {
	ID            string `json:"id"`
	SessionKey    string `json:"session_key"`
	UserID        string `json:"user_id"`
	Username      string `json:"username"`
	ToolName      string `json:"tool_name"`
	ArgumentsJSON string `json:"arguments_json"`
	Summary       string `json:"summary"`
	Status        string `json:"status"`
	Result        string `json:"result,omitempty"`
	DecidedBy     string `json:"decided_by,omitempty"`
	RequestedAt   int64  `json:"requested_at"`
	DecidedAt     int64  `json:"decided_at,omitempty"`
}

// AIApprovalStore 审批单存储。
type AIApprovalStore struct {
	db *sql.DB
}

func NewAIApprovalStore(db *sql.DB) *AIApprovalStore {
	return &AIApprovalStore{db: db}
}

// Init 建表（幂等）。
func (s *AIApprovalStore) Init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS ai_pending_approvals (
			id             TEXT PRIMARY KEY,
			session_key    TEXT NOT NULL DEFAULT '',
			user_id        TEXT NOT NULL DEFAULT '',
			username       TEXT NOT NULL DEFAULT '',
			tool_name      TEXT NOT NULL DEFAULT '',
			arguments_json TEXT NOT NULL DEFAULT '{}',
			summary        TEXT NOT NULL DEFAULT '',
			status         TEXT NOT NULL DEFAULT 'pending',
			result         TEXT NOT NULL DEFAULT '',
			decided_by     TEXT NOT NULL DEFAULT '',
			requested_at   INTEGER NOT NULL DEFAULT 0,
			decided_at     INTEGER NOT NULL DEFAULT 0
		)`)
	if err != nil {
		return fmt.Errorf("create ai_pending_approvals: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_ai_approvals_status ON ai_pending_approvals(status, requested_at)`)
	return err
}

// CreateIfAbsent 创建审批单；同 session 同工具同参数且仍 pending 时幂等返回既有单。
func (s *AIApprovalStore) CreateIfAbsent(ctx context.Context, rec *AIApproval) (*AIApproval, bool, error) {
	existing, err := s.findPendingDuplicate(ctx, rec.SessionKey, rec.ToolName, rec.ArgumentsJSON)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		return existing, false, nil
	}
	rec.ID = uuid.New().String()
	rec.Status = AIApprovalPending
	if rec.RequestedAt == 0 {
		rec.RequestedAt = time.Now().Unix()
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO ai_pending_approvals
			(id, session_key, user_id, username, tool_name, arguments_json, summary, status, result, decided_by, requested_at, decided_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, '', '', ?, 0)`,
		rec.ID, rec.SessionKey, rec.UserID, rec.Username, rec.ToolName, rec.ArgumentsJSON, rec.Summary, rec.Status, rec.RequestedAt)
	if err != nil {
		return nil, false, fmt.Errorf("create ai approval: %w", err)
	}
	return rec, true, nil
}

func (s *AIApprovalStore) findPendingDuplicate(ctx context.Context, sessionKey, toolName, argsJSON string) (*AIApproval, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, session_key, user_id, username, tool_name, arguments_json, summary, status, result, decided_by, requested_at, decided_at
		FROM ai_pending_approvals
		WHERE session_key = ? AND tool_name = ? AND arguments_json = ? AND status = 'pending'
		LIMIT 1`, sessionKey, toolName, argsJSON)
	var rec AIApproval
	err := row.Scan(&rec.ID, &rec.SessionKey, &rec.UserID, &rec.Username, &rec.ToolName, &rec.ArgumentsJSON,
		&rec.Summary, &rec.Status, &rec.Result, &rec.DecidedBy, &rec.RequestedAt, &rec.DecidedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// Get 读取审批单。
func (s *AIApprovalStore) Get(ctx context.Context, id string) (*AIApproval, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, session_key, user_id, username, tool_name, arguments_json, summary, status, result, decided_by, requested_at, decided_at
		FROM ai_pending_approvals WHERE id = ?`, id)
	var rec AIApproval
	err := row.Scan(&rec.ID, &rec.SessionKey, &rec.UserID, &rec.Username, &rec.ToolName, &rec.ArgumentsJSON,
		&rec.Summary, &rec.Status, &rec.Result, &rec.DecidedBy, &rec.RequestedAt, &rec.DecidedAt)
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// List 列出审批单：admin 可见全部；普通用户仅本人。status 为空=全部。
func (s *AIApprovalStore) List(ctx context.Context, userID string, isAdmin bool, status string, limit int) ([]*AIApproval, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query := `SELECT id, session_key, user_id, username, tool_name, arguments_json, summary, status, result, decided_by, requested_at, decided_at
		FROM ai_pending_approvals WHERE 1=1`
	args := []interface{}{}
	if !isAdmin {
		query += " AND user_id = ?"
		args = append(args, userID)
	}
	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}
	query += " ORDER BY requested_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AIApproval
	for rows.Next() {
		var rec AIApproval
		if err := rows.Scan(&rec.ID, &rec.SessionKey, &rec.UserID, &rec.Username, &rec.ToolName, &rec.ArgumentsJSON,
			&rec.Summary, &rec.Status, &rec.Result, &rec.DecidedBy, &rec.RequestedAt, &rec.DecidedAt); err != nil {
			continue
		}
		out = append(out, &rec)
	}
	return out, rows.Err()
}

// Decide 置审批结论（仅 pending 单可流转，防重复执行）。
func (s *AIApprovalStore) Decide(ctx context.Context, id, status, decidedBy, result string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE ai_pending_approvals SET status = ?, decided_by = ?, result = ?, decided_at = ?
		WHERE id = ? AND status = 'pending'`, status, decidedBy, result, time.Now().Unix(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("审批单不存在或已处理")
	}
	return nil
}

// CloseBySession 用户聊天框确认/取消后关闭该会话的 pending 单（幂等）。
func (s *AIApprovalStore) CloseBySession(ctx context.Context, sessionKey, status, result string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE ai_pending_approvals SET status = ?, result = ?, decided_by = 'session', decided_at = ?
		WHERE session_key = ? AND status = 'pending'`,
		status, result, time.Now().Unix(), sessionKey)
	return err
}
