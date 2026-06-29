package postgres

import (
	"context"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/backupcode"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// backupCodeBatchQuerier is satisfied by both *pgxpool.Pool and pgx.Tx.
type backupCodeBatchQuerier interface {
	txkeys.PgxQuerier
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

type BackupCodeStorage struct {
	pool *pgxpool.Pool
}

func NewBackupCode(pool *pgxpool.Pool) *BackupCodeStorage {
	return &BackupCodeStorage{pool: pool}
}

func (s *BackupCodeStorage) q(ctx context.Context) backupCodeBatchQuerier {
	if tx, ok := txkeys.PgxTxFromContext(ctx); ok {
		return tx
	}
	return s.pool
}

func (s *BackupCodeStorage) CreateBatch(ctx context.Context, userID string, codeHashes []string) error {
	batch := &pgx.Batch{}
	for _, h := range codeHashes {
		batch.Queue(
			`INSERT INTO totp_backup_codes (user_id, code_hash) VALUES ($1, $2)`,
			userID, h,
		)
	}
	results := s.q(ctx).SendBatch(ctx, batch)
	defer results.Close()
	for range codeHashes {
		if _, err := results.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (s *BackupCodeStorage) GetUnused(ctx context.Context, userID, codeHash string) (*backupcode.Row, error) {
	row := &backupcode.Row{}
	err := s.q(ctx).QueryRow(ctx,
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

func (s *BackupCodeStorage) ListByUserID(ctx context.Context, userID string) ([]*backupcode.Row, error) {
	rows, err := s.q(ctx).Query(ctx,
		`SELECT id, user_id, code_hash, used_at, create_time
		 FROM totp_backup_codes WHERE user_id = $1 ORDER BY create_time`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*backupcode.Row
	for rows.Next() {
		row := &backupcode.Row{}
		if err := rows.Scan(&row.ID, &row.UserID, &row.CodeHash, &row.UsedAt, &row.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *BackupCodeStorage) MarkUsed(ctx context.Context, id uuid.UUID) error {
	_, err := s.q(ctx).Exec(ctx,
		`UPDATE totp_backup_codes SET used_at = NOW() WHERE id = $1`, id)
	return err
}

func (s *BackupCodeStorage) DeleteAllForUser(ctx context.Context, userID string) error {
	_, err := s.q(ctx).Exec(ctx, `DELETE FROM totp_backup_codes WHERE user_id = $1`, userID)
	return err
}

var _ backupcode.Storage = (*BackupCodeStorage)(nil)
