// Package totpsecret stores AES-256-GCM encrypted TOTP secrets. Enrollment
// is a two-step process: Upsert stores the encrypted secret with enabled=false,
// and Enable flips the flag after the user confirms the first valid code.
// Deleting the row disables TOTP for the account.
package totpsecret

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

type TOTPSecretStorage struct {
	db querier
}

func New(pool *pgxpool.Pool) *TOTPSecretStorage {
	return &TOTPSecretStorage{db: pool}
}

func (s *TOTPSecretStorage) WithTx(tx pgx.Tx) *TOTPSecretStorage {
	return &TOTPSecretStorage{db: tx}
}

type Row struct {
	ID         uuid.UUID
	UserID     string
	SecretEnc  string
	Enabled    bool
	EnrolledAt *time.Time
	CreatedAt  time.Time
}

// Upsert creates or replaces the TOTP secret for a user (enabled=false).
func (s *TOTPSecretStorage) Upsert(ctx context.Context, userID, secretEnc string) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO totp_secrets (user_id, secret_enc, enabled)
		 VALUES ($1, $2, FALSE)
		 ON CONFLICT (user_id) DO UPDATE SET secret_enc = EXCLUDED.secret_enc, enabled = FALSE, enrolled_at = NULL`,
		userID, secretEnc,
	)
	return err
}

func (s *TOTPSecretStorage) GetByUserID(ctx context.Context, userID string) (*Row, error) {
	row := &Row{}
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, secret_enc, enabled, enrolled_at, create_time FROM totp_secrets WHERE user_id = $1`,
		userID,
	).Scan(&row.ID, &row.UserID, &row.SecretEnc, &row.Enabled, &row.EnrolledAt, &row.CreatedAt)
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (s *TOTPSecretStorage) Enable(ctx context.Context, userID string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE totp_secrets SET enabled = TRUE, enrolled_at = NOW() WHERE user_id = $1`,
		userID,
	)
	return err
}

func (s *TOTPSecretStorage) Delete(ctx context.Context, userID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM totp_secrets WHERE user_id = $1`, userID)
	return err
}
