package sqlite

import (
	"context"
	"database/sql"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqltypes"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/totpsecret"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
)

// SQLiteTOTPSecretStorage implements totpsecret.Storage using database/sql with SQLite.
type SQLiteTOTPSecretStorage struct {
	db *sql.DB
}

func NewTOTPSecret(db *sql.DB) *SQLiteTOTPSecretStorage {
	return &SQLiteTOTPSecretStorage{db: db}
}

func (s *SQLiteTOTPSecretStorage) q(ctx context.Context) txkeys.SQLQuerier {
	if tx, ok := txkeys.SQLTxFromContext(ctx); ok {
		return tx
	}
	return s.db
}

func (s *SQLiteTOTPSecretStorage) Upsert(ctx context.Context, userID, secretEnc string) error {
	_, err := s.q(ctx).ExecContext(ctx,
		`INSERT INTO totp_secrets (user_id, secret_enc, enabled) VALUES (?, ?, 0)
		 ON CONFLICT(user_id) DO UPDATE SET secret_enc = excluded.secret_enc, enabled = 0, enrolled_at = NULL`,
		userID, secretEnc)
	return err
}

func (s *SQLiteTOTPSecretStorage) GetByUserID(ctx context.Context, userID string) (*totpsecret.Row, error) {
	row := &totpsecret.Row{}
	var enrolledAt sqltypes.NullTime
	var createdAt sqltypes.Time
	err := s.q(ctx).QueryRowContext(ctx,
		`SELECT id, user_id, secret_enc, enabled, enrolled_at, create_time FROM totp_secrets WHERE user_id = ?`, userID,
	).Scan(&row.ID, &row.UserID, &row.SecretEnc, &row.Enabled, &enrolledAt, &createdAt)
	if err != nil {
		return nil, err
	}
	row.EnrolledAt = enrolledAt.Ptr()
	row.CreatedAt = createdAt.V
	return row, nil
}

func (s *SQLiteTOTPSecretStorage) Enable(ctx context.Context, userID string) error {
	_, err := s.q(ctx).ExecContext(ctx,
		`UPDATE totp_secrets SET enabled = 1, enrolled_at = datetime('now') WHERE user_id = ?`, userID)
	return err
}

func (s *SQLiteTOTPSecretStorage) Delete(ctx context.Context, userID string) error {
	_, err := s.q(ctx).ExecContext(ctx, `DELETE FROM totp_secrets WHERE user_id = ?`, userID)
	return err
}

var _ totpsecret.Storage = (*SQLiteTOTPSecretStorage)(nil)
