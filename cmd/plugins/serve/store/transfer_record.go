package store

import (
	"context"
	"database/sql"
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

func (s *TransferRecordStore) MarkRunning(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE transfer_records SET status = ?, updated_at = ? WHERE id = ?`,
		TransferRunning, now, id)
	return err
}

func (s *TransferRecordStore) UpdateNodeResult(ctx context.Context, id string, success bool) error {
	now := time.Now().UTC()

	var nodeCount, successCount, failedCount int
	err := s.db.QueryRowContext(ctx,
		`SELECT node_count, success_count, failed_count FROM transfer_records WHERE id = ?`, id).
		Scan(&nodeCount, &successCount, &failedCount)
	if err != nil {
		return err
	}

	if success {
		successCount++
	} else {
		failedCount++
	}

	var status TransferRecordStatus
	if successCount >= nodeCount {
		status = TransferCompleted
	} else if failedCount > 0 {
		status = TransferPartialSuccess
		if failedCount >= nodeCount {
			status = TransferFailed
		}
	} else {
		status = TransferRunning
	}

	var completedAt interface{}
	if status == TransferCompleted || status == TransferFailed {
		completedAt = now
	} else {
		completedAt = nil
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE transfer_records SET success_count = ?, failed_count = ?, status = ?, updated_at = ?, completed_at = ? WHERE id = ?`,
		successCount, failedCount, status, now, completedAt, id)
	return err
}

func isColumnExists(err error) bool {
	return err != nil && (err.Error() == "duplicate column name: record_id")
}

func (s *TransferRecordStore) SetNodeCount(ctx context.Context, id string, count int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE transfer_records SET node_count = ? WHERE id = ?`, count, id)
	return err
}