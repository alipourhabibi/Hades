// Package emailverification stores single-use email verification tokens.
// Tokens are stored as SHA-256 hashes; the plaintext is sent by email and
// never persisted. A token is valid until its expires_at timestamp or until
// it has been consumed via MarkUsed.
package emailverification

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

type EmailVerificationStorage struct {
	db querier
}

func New(pool *pgxpool.Pool) *EmailVerificationStorage {
	return &EmailVerificationStorage{db: pool}
}

func (s *EmailVerificationStorage) WithTx(tx pgx.Tx) *EmailVerificationStorage {
	return &EmailVerificationStorage{db: tx}
}

type Row struct {
	ID        uuid.UUID
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
}

func (s *EmailVerificationStorage) Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO email_verifications (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		userID, tokenHash, expiresAt,
	)
	return err
}

func (s *EmailVerificationStorage) GetByTokenHash(ctx context.Context, tokenHash string) (*Row, error) {
	row := &Row{}
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, token_hash, expires_at, used_at FROM email_verifications WHERE token_hash = $1`,
		tokenHash,
	).Scan(&row.ID, &row.UserID, &row.TokenHash, &row.ExpiresAt, &row.UsedAt)
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (s *EmailVerificationStorage) MarkUsed(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`UPDATE email_verifications SET used_at = NOW() WHERE id = $1`,
		id,
	)
	return err
}
