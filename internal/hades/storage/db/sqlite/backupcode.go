package sqlite

import (
	"context"
	"database/sql"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/backupcode"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqltypes"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/google/uuid"
)

// SQLiteBackupCodeStorage implements backupcode.Storage using database/sql with SQLite.
type SQLiteBackupCodeStorage struct {
	db *sql.DB
}

func NewBackupCode(db *sql.DB) *SQLiteBackupCodeStorage {
	return &SQLiteBackupCodeStorage{db: db}
}

func (s *SQLiteBackupCodeStorage) q(ctx context.Context) txkeys.SQLQuerier {
	if tx, ok := txkeys.SQLTxFromContext(ctx); ok {
		return tx
	}
	return s.db
}

func (s *SQLiteBackupCodeStorage) CreateBatch(ctx context.Context, userID string, codeHashes []string) error {
	for _, h := range codeHashes {
		_, err := s.q(ctx).ExecContext(ctx,
			`INSERT INTO totp_backup_codes (user_id, code_hash) VALUES (?, ?)`, userID, h)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteBackupCodeStorage) GetUnused(ctx context.Context, userID, codeHash string) (*backupcode.Row, error) {
	row := &backupcode.Row{}
	var usedAt sqltypes.NullTime
	var createdAt sqltypes.Time
	err := s.q(ctx).QueryRowContext(ctx,
		`SELECT id, user_id, code_hash, used_at, create_time
		 FROM totp_backup_codes WHERE user_id = ? AND code_hash = ? AND used_at IS NULL LIMIT 1`,
		userID, codeHash,
	).Scan(&row.ID, &row.UserID, &row.CodeHash, &usedAt, &createdAt)
	if err != nil {
		return nil, err
	}
	row.UsedAt = usedAt.Ptr()
	row.CreatedAt = createdAt.V
	return row, nil
}

func (s *SQLiteBackupCodeStorage) ListByUserID(ctx context.Context, userID string) ([]*backupcode.Row, error) {
	rows, err := s.q(ctx).QueryContext(ctx,
		`SELECT id, user_id, code_hash, used_at, create_time FROM totp_backup_codes WHERE user_id = ? ORDER BY create_time`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*backupcode.Row
	for rows.Next() {
		row := &backupcode.Row{}
		var usedAt sqltypes.NullTime
		var createdAt sqltypes.Time
		if err := rows.Scan(&row.ID, &row.UserID, &row.CodeHash, &usedAt, &createdAt); err != nil {
			return nil, err
		}
		row.UsedAt = usedAt.Ptr()
		row.CreatedAt = createdAt.V
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *SQLiteBackupCodeStorage) MarkUsed(ctx context.Context, id uuid.UUID) error {
	_, err := s.q(ctx).ExecContext(ctx,
		`UPDATE totp_backup_codes SET used_at = datetime('now') WHERE id = ?`, id.String())
	return err
}

func (s *SQLiteBackupCodeStorage) DeleteAllForUser(ctx context.Context, userID string) error {
	_, err := s.q(ctx).ExecContext(ctx, `DELETE FROM totp_backup_codes WHERE user_id = ?`, userID)
	return err
}

var _ backupcode.Storage = (*SQLiteBackupCodeStorage)(nil)
