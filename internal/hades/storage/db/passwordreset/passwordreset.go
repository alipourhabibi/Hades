// Package passwordreset stores single-use password reset tokens. Tokens are
// stored as SHA-256 hashes; the plaintext is sent by email and never
// persisted. A token is valid until its expires_at timestamp or until it
// has been consumed via MarkUsed.
package passwordreset

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

type PasswordResetStorage struct {
	db querier
}

func New(pool *pgxpool.Pool) *PasswordResetStorage {
	return &PasswordResetStorage{db: pool}
}

func (s *PasswordResetStorage) WithTx(tx pgx.Tx) *PasswordResetStorage {
	return &PasswordResetStorage{db: tx}
}

type Row struct {
	ID        uuid.UUID
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
}

func (s *PasswordResetStorage) Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO password_resets (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		userID, tokenHash, expiresAt,
	)
	return err
}

func (s *PasswordResetStorage) GetByTokenHash(ctx context.Context, tokenHash string) (*Row, error) {
	row := &Row{}
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, token_hash, expires_at, used_at FROM password_resets WHERE token_hash = $1`,
		tokenHash,
	).Scan(&row.ID, &row.UserID, &row.TokenHash, &row.ExpiresAt, &row.UsedAt)
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (s *PasswordResetStorage) MarkUsed(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`UPDATE password_resets SET used_at = NOW() WHERE id = $1`,
		id,
	)
	return err
}
