package store

import (
	"context"
	"database/sql"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
)

// CommandStore 管理用户级快捷命令(user_commands 表)。
type CommandStore struct {
	db *sql.DB
}

func NewCommandStore(db *sql.DB) *CommandStore {
	return &CommandStore{db: db}
}

func (s *CommandStore) Init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS user_commands (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id    INTEGER NOT NULL REFERENCES web_users(id) ON DELETE CASCADE,
			name       TEXT NOT NULL,
			command    TEXT NOT NULL,
			position   INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_user_commands_user ON user_commands(user_id, position);
	`)
	return err
}

func (s *CommandStore) ListByUser(ctx context.Context, userID int64) ([]*model.UserCommand, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, name, command, position FROM user_commands WHERE user_id = ? ORDER BY position, id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*model.UserCommand, 0)
	for rows.Next() {
		cmd := &model.UserCommand{}
		if err := rows.Scan(&cmd.ID, &cmd.UserID, &cmd.Name, &cmd.Command, &cmd.Position); err != nil {
			return nil, err
		}
		list = append(list, cmd)
	}
	return list, rows.Err()
}

func (s *CommandStore) Create(ctx context.Context, cmd *model.UserCommand) error {
	var max int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(position), -1) FROM user_commands WHERE user_id = ?`, cmd.UserID).Scan(&max)
	cmd.Position = max + 1

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO user_commands (user_id, name, command, position) VALUES (?, ?, ?, ?)`,
		cmd.UserID, cmd.Name, cmd.Command, cmd.Position)
	if err != nil {
		return err
	}
	cmd.ID, err = result.LastInsertId()
	return err
}

func (s *CommandStore) Update(ctx context.Context, cmd *model.UserCommand) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE user_commands SET name = ?, command = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ?`,
		cmd.Name, cmd.Command, cmd.ID, cmd.UserID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *CommandStore) Delete(ctx context.Context, id, userID int64) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM user_commands WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *CommandStore) Reorder(ctx context.Context, userID int64, orderedIDs []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, id := range orderedIDs {
		if _, err := tx.ExecContext(ctx,
			`UPDATE user_commands SET position = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ?`,
			i, id, userID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *CommandStore) CountByUser(ctx context.Context, userID int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_commands WHERE user_id = ?`, userID).Scan(&n)
	return n, err
}
