package store

import (
	"context"
	"database/sql"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
)

type UserStore struct {
	db *sql.DB
}

func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) Init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS web_users (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			username     TEXT NOT NULL UNIQUE,
			password     TEXT NOT NULL,
			role         TEXT NOT NULL DEFAULT 'viewer',
			display_name TEXT DEFAULT '',
			created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

func (s *UserStore) Create(ctx context.Context, user *model.User) error {
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO web_users (username, password, role, display_name) VALUES (?, ?, ?, ?)`,
		user.Username, user.PasswordHash, user.Role, user.DisplayName)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	user.ID = id
	return nil
}

func (s *UserStore) FindByID(ctx context.Context, id int64) (*model.User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, username, password, role, display_name FROM web_users WHERE id = ?`,
		id)
	user := &model.User{}
	err := row.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.DisplayName)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (s *UserStore) FindByUsername(ctx context.Context, username string) (*model.User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, username, password, role, display_name FROM web_users WHERE username = ?`,
		username)
	user := &model.User{}
	err := row.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.DisplayName)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (s *UserStore) List(ctx context.Context) ([]*model.User, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, username, password, role, display_name FROM web_users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*model.User
	for rows.Next() {
		user := &model.User{}
		if err := rows.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.DisplayName); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *UserStore) ListPaged(ctx context.Context, keyword string, page, pageSize int) ([]*model.User, int, error) {
	where := ""
	args := []interface{}{}
	if keyword != "" {
		like := "%" + keyword + "%"
		where = " WHERE username LIKE ? OR display_name LIKE ?"
		args = append(args, like, like)
	}

	var total int
	countQuery := `SELECT COUNT(*) FROM web_users` + where
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	query := `SELECT id, username, password, role, display_name FROM web_users` + where + ` ORDER BY id LIMIT ? OFFSET ?`
	queryArgs := append(args, pageSize, offset)
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var users []*model.User
	for rows.Next() {
		user := &model.User{}
		if err := rows.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.DisplayName); err != nil {
			return nil, 0, err
		}
		users = append(users, user)
	}
	return users, total, rows.Err()
}

func (s *UserStore) Update(ctx context.Context, user *model.User) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE web_users SET username = ?, password = ?, role = ?, display_name = ? WHERE id = ?`,
		user.Username, user.PasswordHash, user.Role, user.DisplayName, user.ID)
	return err
}

func (s *UserStore) Delete(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM web_users WHERE id = ?`, id)
	return err
}

func (s *UserStore) Count(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM web_users`).Scan(&count)
	return count, err
}
