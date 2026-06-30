package postgres

import (
	"context"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/totpsecret"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TOTPSecretStorage struct {
	pool *pgxpool.Pool
}

func NewTOTPSecret(pool *pgxpool.Pool) *TOTPSecretStorage {
	return &TOTPSecretStorage{pool: pool}
}

func (s *TOTPSecretStorage) q(ctx context.Context) txkeys.PgxQuerier {
	if tx, ok := txkeys.PgxTxFromContext(ctx); ok {
		return tx
	}
	return s.pool
}

func (s *TOTPSecretStorage) Upsert(ctx context.Context, userID, secretEnc string) error {
	_, err := s.q(ctx).Exec(ctx,
		`INSERT INTO totp_secrets (user_id, secret_enc, enabled)
		 VALUES ($1, $2, FALSE)
		 ON CONFLICT (user_id) DO UPDATE SET secret_enc = EXCLUDED.secret_enc, enabled = FALSE, enrolled_at = NULL`,
		userID, secretEnc,
	)
	return err
}

func (s *TOTPSecretStorage) GetByUserID(ctx context.Context, userID string) (*totpsecret.Row, error) {
	row := &totpsecret.Row{}
	err := s.q(ctx).QueryRow(ctx,
		`SELECT id, user_id, secret_enc, enabled, enrolled_at, create_time FROM totp_secrets WHERE user_id = $1`,
		userID,
	).Scan(&row.ID, &row.UserID, &row.SecretEnc, &row.Enabled, &row.EnrolledAt, &row.CreatedAt)
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (s *TOTPSecretStorage) Enable(ctx context.Context, userID string) error {
	_, err := s.q(ctx).Exec(ctx,
		`UPDATE totp_secrets SET enabled = TRUE, enrolled_at = NOW() WHERE user_id = $1`, userID)
	return err
}

func (s *TOTPSecretStorage) Delete(ctx context.Context, userID string) error {
	_, err := s.q(ctx).Exec(ctx, `DELETE FROM totp_secrets WHERE user_id = $1`, userID)
	return err
}

var _ totpsecret.Storage = (*TOTPSecretStorage)(nil)
