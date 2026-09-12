package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/passwordreset"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqltypes"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqlutil"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/google/uuid"
)

// SQLitePasswordResetStorage implements passwordreset.Storage using database/sql with SQLite.
type SQLitePasswordResetStorage struct {
	db *sql.DB
}

func NewPasswordReset(db *sql.DB) *SQLitePasswordResetStorage {
	return &SQLitePasswordResetStorage{db: db}
}

func (s *SQLitePasswordResetStorage) q(ctx context.Context) txkeys.SQLQuerier {
	if tx, ok := txkeys.SQLTxFromContext(ctx); ok {
		return tx
	}
	return s.db
}

func (s *SQLitePasswordResetStorage) Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	userID = sqlutil.ID(userID)
	_, err := s.q(ctx).ExecContext(ctx,
		`INSERT INTO password_resets (user_id, token_hash, expires_at) VALUES (?, ?, ?)`,
		userID, tokenHash, expiresAt)
	return err
}

func (s *SQLitePasswordResetStorage) GetByTokenHash(ctx context.Context, tokenHash string) (*passwordreset.Row, error) {
	row := &passwordreset.Row{}
	var expiresAt sqltypes.Time
	var usedAt sqltypes.NullTime
	err := s.q(ctx).QueryRowContext(ctx,
		`SELECT id, user_id, token_hash, expires_at, used_at FROM password_resets WHERE token_hash = ?`, tokenHash,
	).Scan(&row.ID, &row.UserID, &row.TokenHash, &expiresAt, &usedAt)
	if err != nil {
		return nil, err
	}
	row.ExpiresAt = expiresAt.V
	row.UserID = sqlutil.Canonical(row.UserID)
	row.UsedAt = usedAt.Ptr()
	return row, nil
}

// InvalidateForUser retires every outstanding reset token for the user.
func (s *SQLitePasswordResetStorage) InvalidateForUser(ctx context.Context, userID string) error {
	_, err := s.q(ctx).ExecContext(ctx,
		`UPDATE password_resets SET used_at = datetime('now') WHERE user_id = ? AND used_at IS NULL`,
		sqlutil.ID(userID))
	return err
}

func (s *SQLitePasswordResetStorage) MarkUsed(ctx context.Context, id uuid.UUID) error {
	_, err := s.q(ctx).ExecContext(ctx,
		`UPDATE password_resets SET used_at = datetime('now') WHERE id = ?`, sqlutil.UUID(id))
	return err
}

var _ passwordreset.Storage = (*SQLitePasswordResetStorage)(nil)
