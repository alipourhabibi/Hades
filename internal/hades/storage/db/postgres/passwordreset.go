package postgres

import (
	"context"
	"time"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/passwordreset"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PasswordResetStorage struct {
	pool *pgxpool.Pool
}

func NewPasswordReset(pool *pgxpool.Pool) *PasswordResetStorage {
	return &PasswordResetStorage{pool: pool}
}

func (s *PasswordResetStorage) q(ctx context.Context) txkeys.PgxQuerier {
	if tx, ok := txkeys.PgxTxFromContext(ctx); ok {
		return tx
	}
	return s.pool
}

func (s *PasswordResetStorage) Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := s.q(ctx).Exec(ctx,
		`INSERT INTO password_resets (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		userID, tokenHash, expiresAt,
	)
	return err
}

func (s *PasswordResetStorage) GetByTokenHash(ctx context.Context, tokenHash string) (*passwordreset.Row, error) {
	row := &passwordreset.Row{}
	err := s.q(ctx).QueryRow(ctx,
		`SELECT id, user_id, token_hash, expires_at, used_at FROM password_resets WHERE token_hash = $1`,
		tokenHash,
	).Scan(&row.ID, &row.UserID, &row.TokenHash, &row.ExpiresAt, &row.UsedAt)
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (s *PasswordResetStorage) MarkUsed(ctx context.Context, id uuid.UUID) error {
	_, err := s.q(ctx).Exec(ctx,
		`UPDATE password_resets SET used_at = NOW() WHERE id = $1`, id,
	)
	return err
}

var _ passwordreset.Storage = (*PasswordResetStorage)(nil)
