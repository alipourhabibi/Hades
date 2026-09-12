// Package postgres provides the PostgreSQL implementation of totpsecret.Storage.
package postgres

import (
	"context"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/totpsecret"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TOTPSecretStorage implements totpsecret.Storage against PostgreSQL.
type TOTPSecretStorage struct {
	pool *pgxpool.Pool
}

// New creates a TOTPSecretStorage backed by a connection pool.
func New(pool *pgxpool.Pool) *TOTPSecretStorage {
	return &TOTPSecretStorage{pool: pool}
}

var _ totpsecret.Storage = (*TOTPSecretStorage)(nil)

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
		 ON CONFLICT (user_id) DO UPDATE SET secret_enc = EXCLUDED.secret_enc, enabled = FALSE,
		   enrolled_at = NULL, last_used_counter = NULL`,
		userID, secretEnc,
	)
	return err
}

func (s *TOTPSecretStorage) GetByUserID(ctx context.Context, userID string) (*totpsecret.Row, error) {
	row := &totpsecret.Row{}
	err := s.q(ctx).QueryRow(ctx,
		`SELECT id, user_id, secret_enc, enabled, enrolled_at, create_time, last_used_counter
		 FROM totp_secrets WHERE user_id = $1`,
		userID,
	).Scan(&row.ID, &row.UserID, &row.SecretEnc, &row.Enabled, &row.EnrolledAt, &row.CreatedAt, &row.LastUsedCounter)
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (s *TOTPSecretStorage) Enable(ctx context.Context, userID string) error {
	_, err := s.q(ctx).Exec(ctx,
		`UPDATE totp_secrets SET enabled = TRUE, enrolled_at = NOW() WHERE user_id = $1`,
		userID,
	)
	return err
}

func (s *TOTPSecretStorage) ConsumeCounter(ctx context.Context, userID string, counter uint64) error {
	tag, err := s.q(ctx).Exec(ctx,
		`UPDATE totp_secrets SET last_used_counter = $2
		 WHERE user_id = $1 AND (last_used_counter IS NULL OR last_used_counter < $2)`,
		// #nosec G115 -- the counter is unix seconds / 30, which cannot reach the int64 ceiling.
		userID, int64(counter),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return totpsecret.ErrCodeAlreadyUsed
	}
	return nil
}

func (s *TOTPSecretStorage) Delete(ctx context.Context, userID string) error {
	_, err := s.q(ctx).Exec(ctx, `DELETE FROM totp_secrets WHERE user_id = $1`, userID)
	return err
}
