package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/google/uuid"
)

type TransferRecordStatus string

const (
	TransferPending        TransferRecordStatus = "pending"
	TransferRunning        TransferRecordStatus = "running"
	TransferPartialSuccess TransferRecordStatus = "partial_success"
	TransferCompleted      TransferRecordStatus = "completed"
	TransferFailed         TransferRecordStatus = "failed"
	TransferCancelled      TransferRecordStatus = "cancelled"
)

type TransferRecord struct {
	ID            string               `json:"id"`
	FileSource    string               `json:"file_source"`
	DestPath      string               `json:"dest_path"`
	Direction     string               `json:"direction"`
	Status        TransferRecordStatus `json:"status"`
	NodeCount     int                  `json:"node_count"`
	SuccessCount  int                  `json:"success_count"`
	FailedCount   int                  `json:"failed_count"`
	CreatedAt     time.Time            `json:"created_at"`
	UpdatedAt     time.Time            `json:"updated_at"`
	CompletedAt   *time.Time           `json:"completed_at,omitempty"`
}

type TransferRecordStore struct {
	db *sql.DB
}

func NewTransferRecordStore(db *sql.DB) *TransferRecordStore {
	return &TransferRecordStore{db: db}
}

func (s *TransferRecordStore) Init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS transfer_records (
			id TEXT PRIMARY KEY,
			file_source TEXT NOT NULL,
			dest_path TEXT NOT NULL,
			direction TEXT NOT NULL DEFAULT 'push',
			status TEXT NOT NULL DEFAULT 'pending',
			node_count INTEGER DEFAULT 0,
			success_count INTEGER DEFAULT 0,
			failed_count INTEGER DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			completed_at TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `ALTER TABLE tasks ADD COLUMN record_id TEXT DEFAULT ''`)
	if err != nil && !isColumnExists(err) {
		return err
	}
	return nil
}

func (s *TransferRecordStore) Create(ctx context.Context, fileSource, destPath, direction string) (*TransferRecord, error) {
	rec := &TransferRecord{
		ID:        uuid.New().String(),
		FileSource: fileSource,
		DestPath:  destPath,
		Direction: direction,
		Status:    TransferPending,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO transfer_records (id, file_source, dest_path, direction, status, node_count, success_count, failed_count, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 0, 0, 0, ?, ?)`,
		rec.ID, rec.FileSource, rec.DestPath, rec.Direction, rec.Status, rec.CreatedAt, rec.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return rec, nil
}

func (s *TransferRecordStore) Get(ctx context.Context, id string) (*TransferRecord, error) {
	rec := &TransferRecord{}
	var completedAt sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT id, file_source, dest_path, direction, status, node_count, success_count, failed_count, created_at, updated_at, completed_at FROM transfer_records WHERE id = ?`, id).
		Scan(&rec.ID, &rec.FileSource, &rec.DestPath, &rec.Direction, &rec.Status, &rec.NodeCount, &rec.SuccessCount, &rec.FailedCount, &rec.CreatedAt, &rec.UpdatedAt, &completedAt)
	if err != nil {
		return nil, err
	}
	if completedAt.Valid {
		rec.CompletedAt = &completedAt.Time
	}
	return rec, nil
}

func (s *TransferRecordStore) List(ctx context.Context, limit, offset int) ([]*TransferRecord, int, error) {
	var total int
	s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM transfer_records`).Scan(&total)

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, file_source, dest_path, direction, status, node_count, success_count, failed_count, created_at, updated_at, completed_at
		FROM transfer_records ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	records := make([]*TransferRecord, 0)
	for rows.Next() {
		rec := &TransferRecord{}
		var completedAt sql.NullTime
		if err := rows.Scan(&rec.ID, &rec.FileSource, &rec.DestPath, &rec.Direction, &rec.Status, &rec.NodeCount, &rec.SuccessCount, &rec.FailedCount, &rec.CreatedAt, &rec.UpdatedAt, &completedAt); err != nil {
			continue
		}
		if completedAt.Valid {
			rec.CompletedAt = &completedAt.Time
		}
		records = append(records, rec)
	}
	return records, total, nil
}

// MarkRunning 仅在仍处于 pending 时置为 running,避免覆盖此前由
// UpdateNodeResult 已推导出的终态(如全部节点解析失败直接 failed)。
func (s *TransferRecordStore) MarkRunning(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE transfer_records SET status = ?, updated_at = ? WHERE id = ? AND status = 'pending'`,
		TransferRunning, now, id)
	return err
}

// UpdateNodeResult 原子地累加成功/失败计数并刷新聚合状态。
// 不能先 SELECT 再 UPDATE：并发传输(parallel)时多个 goroutine 会读到同一个旧值,
// 各自 +1 后互相覆盖导致丢增量,聚合状态永远停在 running。这里用单条 UPDATE
// 在同一语句内完成计数与状态推导,SQLite 串行化写操作后即为原子。
func (s *TransferRecordStore) UpdateNodeResult(ctx context.Context, id string, success bool) error {
	now := time.Now().UTC()
	inc := 0
	if success {
		inc = 1
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE transfer_records SET
			success_count = success_count + ?,
			failed_count = failed_count + ?,
			status = CASE
				WHEN success_count + ? >= node_count THEN 'completed'
				WHEN failed_count + ? >= node_count THEN 'failed'
				WHEN failed_count + ? > 0 THEN 'partial_success'
				ELSE 'running'
			END,
			completed_at = CASE
				WHEN success_count + ? >= node_count
				  OR failed_count + ? >= node_count THEN ?
				ELSE completed_at
			END,
			updated_at = ?
		WHERE id = ?`,
		inc, 1-inc, inc, 1-inc, 1-inc, inc, 1-inc, now, now, id)
	return err
}

func isColumnExists(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate column name: record_id")
}

func (s *TransferRecordStore) SetNodeCount(ctx context.Context, id string, count int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE transfer_records SET node_count = ? WHERE id = ?`, count, id)
	return err
}