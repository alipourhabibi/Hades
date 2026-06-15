// Package backupcode stores hashed TOTP backup codes. Each code is a
// one-time-use credential; GetUnused + MarkUsed is the consumption path.
// DeleteAllForUser clears the table before a new batch is generated so
// the user always has exactly backupCodeCount valid codes.
package backupcode

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

type BackupCodeStorage struct {
	db querier
}

func New(pool *pgxpool.Pool) *BackupCodeStorage {
	return &BackupCodeStorage{db: pool}
}

func (s *BackupCodeStorage) WithTx(tx pgx.Tx) *BackupCodeStorage {
	return &BackupCodeStorage{db: tx}
}

type Row struct {
	ID        uuid.UUID
	UserID    string
	CodeHash  string
	UsedAt    *time.Time
	CreatedAt time.Time
}

func (s *BackupCodeStorage) CreateBatch(ctx context.Context, userID string, codeHashes []string) error {
	batch := &pgx.Batch{}
	for _, h := range codeHashes {
		batch.Queue(
			`INSERT INTO totp_backup_codes (user_id, code_hash) VALUES ($1, $2)`,
			userID, h,
		)
	}
	results := s.db.SendBatch(ctx, batch)
	defer results.Close()
	for range codeHashes {
		if _, err := results.Exec(); err != nil {
			return err
		}
	}
	return nil
}

// GetUnused returns the first unused backup code row matching codeHash.
func (s *BackupCodeStorage) GetUnused(ctx context.Context, userID, codeHash string) (*Row, error) {
	row := &Row{}
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, code_hash, used_at, create_time
		 FROM totp_backup_codes WHERE user_id = $1 AND code_hash = $2 AND used_at IS NULL
		 LIMIT 1`,
		userID, codeHash,
	).Scan(&row.ID, &row.UserID, &row.CodeHash, &row.UsedAt, &row.CreatedAt)
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (s *BackupCodeStorage) ListByUserID(ctx context.Context, userID string) ([]*Row, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, user_id, code_hash, used_at, create_time
		 FROM totp_backup_codes WHERE user_id = $1 ORDER BY create_time`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*Row
	for rows.Next() {
		row := &Row{}
		if err := rows.Scan(&row.ID, &row.UserID, &row.CodeHash, &row.UsedAt, &row.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *BackupCodeStorage) MarkUsed(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`UPDATE totp_backup_codes SET used_at = NOW() WHERE id = $1`,
		id,
	)
	return err
}

func (s *BackupCodeStorage) DeleteAllForUser(ctx context.Context, userID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM totp_backup_codes WHERE user_id = $1`, userID)
	return err
}
