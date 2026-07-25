package postgres

import (
	"context"
	"time"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/emailverification"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type EmailVerificationStorage struct {
	pool *pgxpool.Pool
}

func NewEmailVerification(pool *pgxpool.Pool) *EmailVerificationStorage {
	return &EmailVerificationStorage{pool: pool}
}

func (s *EmailVerificationStorage) q(ctx context.Context) txkeys.PgxQuerier {
	if tx, ok := txkeys.PgxTxFromContext(ctx); ok {
		return tx
	}
	return s.pool
}

func (s *EmailVerificationStorage) Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := s.q(ctx).Exec(ctx,
		`INSERT INTO email_verifications (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		userID, tokenHash, expiresAt,
	)
	return err
}

func (s *EmailVerificationStorage) GetByTokenHash(ctx context.Context, tokenHash string) (*emailverification.Row, error) {
	row := &emailverification.Row{}
	err := s.q(ctx).QueryRow(ctx,
		`SELECT id, user_id, token_hash, expires_at, used_at FROM email_verifications WHERE token_hash = $1`,
		tokenHash,
	).Scan(&row.ID, &row.UserID, &row.TokenHash, &row.ExpiresAt, &row.UsedAt)
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (s *EmailVerificationStorage) MarkUsed(ctx context.Context, id uuid.UUID) error {
	_, err := s.q(ctx).Exec(ctx,
		`UPDATE email_verifications SET used_at = NOW() WHERE id = $1`, id,
	)
	return err
}

var _ emailverification.Storage = (*EmailVerificationStorage)(nil)
