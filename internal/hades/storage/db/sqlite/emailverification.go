package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/emailverification"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqltypes"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/google/uuid"
)

// SQLiteEmailVerificationStorage implements emailverification.Storage using database/sql with SQLite.
type SQLiteEmailVerificationStorage struct {
	db *sql.DB
}

func NewEmailVerification(db *sql.DB) *SQLiteEmailVerificationStorage {
	return &SQLiteEmailVerificationStorage{db: db}
}

func (s *SQLiteEmailVerificationStorage) q(ctx context.Context) txkeys.SQLQuerier {
	if tx, ok := txkeys.SQLTxFromContext(ctx); ok {
		return tx
	}
	return s.db
}

func (s *SQLiteEmailVerificationStorage) Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := s.q(ctx).ExecContext(ctx,
		`INSERT INTO email_verifications (user_id, token_hash, expires_at) VALUES (?, ?, ?)`,
		userID, tokenHash, expiresAt)
	return err
}

func (s *SQLiteEmailVerificationStorage) GetByTokenHash(ctx context.Context, tokenHash string) (*emailverification.Row, error) {
	row := &emailverification.Row{}
	var expiresAt sqltypes.Time
	var usedAt sqltypes.NullTime
	err := s.q(ctx).QueryRowContext(ctx,
		`SELECT id, user_id, token_hash, expires_at, used_at FROM email_verifications WHERE token_hash = ?`, tokenHash,
	).Scan(&row.ID, &row.UserID, &row.TokenHash, &expiresAt, &usedAt)
	if err != nil {
		return nil, err
	}
	row.ExpiresAt = expiresAt.V
	row.UsedAt = usedAt.Ptr()
	return row, nil
}

func (s *SQLiteEmailVerificationStorage) MarkUsed(ctx context.Context, id uuid.UUID) error {
	_, err := s.q(ctx).ExecContext(ctx,
		`UPDATE email_verifications SET used_at = datetime('now') WHERE id = ?`, id.String())
	return err
}

var _ emailverification.Storage = (*SQLiteEmailVerificationStorage)(nil)
