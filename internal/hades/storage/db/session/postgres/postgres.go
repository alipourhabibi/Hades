// Package postgres provides the PostgreSQL implementation of session.Storage.
package postgres

import (
	"context"
	"time"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/session"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SessionStorage implements session.Storage against PostgreSQL.
type SessionStorage struct {
	pool *pgxpool.Pool
}

// New creates a SessionStorage backed by a connection pool.
func New(pool *pgxpool.Pool) *SessionStorage {
	return &SessionStorage{pool: pool}
}

var _ session.Storage = (*SessionStorage)(nil)

func (s *SessionStorage) q(ctx context.Context) txkeys.PgxQuerier {
	if tx, ok := txkeys.PgxTxFromContext(ctx); ok {
		return tx
	}
	return s.pool
}

func (s *SessionStorage) Create(ctx context.Context, userId, authModule string, expiresAt time.Time) (string, error) {
	query := `
INSERT INTO sessions (
  user_id,
  auth_module,
  expires_at
) VALUES ($1, $2, $3) RETURNING id`

	var id string
	err := s.q(ctx).QueryRow(ctx, query, userId, authModule, expiresAt).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *SessionStorage) CreateWithToken(
	ctx context.Context,
	userID, authModule, tokenHash, ipAddress, userAgent string,
	idleExpires, absoluteExpires time.Time,
) (string, error) {
	query := `
INSERT INTO sessions (
  user_id, auth_module, expires_at,
  token_hash, ip_address, user_agent,
  last_activity_at, absolute_expires_at
) VALUES ($1, $2, $3, $4, $5, $6, NOW(), $7)
RETURNING id`

	var id string
	err := s.q(ctx).QueryRow(ctx, query,
		userID, authModule, idleExpires,
		tokenHash, ipAddress, userAgent,
		absoluteExpires,
	).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *SessionStorage) GetByTokenHash(ctx context.Context, hash string) (*session.SessionRow, error) {
	query := `
SELECT
  id, user_id, auth_module,
  COALESCE(token_hash,''), COALESCE(ip_address,''), COALESCE(user_agent,''),
  create_time, COALESCE(last_activity_at, create_time),
  COALESCE(absolute_expires_at, expires_at), expires_at,
  revoked_at,
  COALESCE(totp_verified, FALSE),
  COALESCE(old_token_hash,''), old_token_expires_at
FROM sessions
WHERE token_hash = $1`

	row := &session.SessionRow{}
	err := s.q(ctx).QueryRow(ctx, query, hash).Scan(
		&row.ID, &row.UserID, &row.AuthModule,
		&row.TokenHash, &row.IPAddress, &row.UserAgent,
		&row.CreatedAt, &row.LastActivityAt,
		&row.AbsoluteExpiresAt, &row.IdleExpiresAt,
		&row.RevokedAt,
		&row.TOTPVerified,
		&row.OldTokenHash, &row.OldTokenExpiresAt,
	)
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (s *SessionStorage) GetByOldTokenHash(ctx context.Context, hash string) (*session.SessionRow, error) {
	query := `
SELECT
  id, user_id, auth_module,
  COALESCE(token_hash,''), COALESCE(ip_address,''), COALESCE(user_agent,''),
  create_time, COALESCE(last_activity_at, create_time),
  COALESCE(absolute_expires_at, expires_at), expires_at,
  revoked_at,
  COALESCE(totp_verified, FALSE),
  COALESCE(old_token_hash,''), old_token_expires_at
FROM sessions
WHERE old_token_hash = $1 AND old_token_expires_at > NOW()`

	row := &session.SessionRow{}
	err := s.q(ctx).QueryRow(ctx, query, hash).Scan(
		&row.ID, &row.UserID, &row.AuthModule,
		&row.TokenHash, &row.IPAddress, &row.UserAgent,
		&row.CreatedAt, &row.LastActivityAt,
		&row.AbsoluteExpiresAt, &row.IdleExpiresAt,
		&row.RevokedAt,
		&row.TOTPVerified,
		&row.OldTokenHash, &row.OldTokenExpiresAt,
	)
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (s *SessionStorage) UpdateActivity(
	ctx context.Context,
	id, newTokenHash, oldTokenHash string,
	oldTokenExpires, newIdleExpires time.Time,
) error {
	query := `
UPDATE sessions SET
  token_hash          = $1,
  old_token_hash      = $2,
  old_token_expires_at = $3,
  last_activity_at    = NOW(),
  expires_at          = $4
WHERE id = $5`
	_, err := s.q(ctx).Exec(ctx, query, newTokenHash, oldTokenHash, oldTokenExpires, newIdleExpires, id)
	return err
}

func (s *SessionStorage) Revoke(ctx context.Context, id string) error {
	_, err := s.q(ctx).Exec(ctx, `UPDATE sessions SET revoked_at = NOW() WHERE id = $1`, id)
	return err
}

func (s *SessionStorage) RevokeAllForUser(ctx context.Context, userID, exceptID string) error {
	_, err := s.q(ctx).Exec(ctx,
		`UPDATE sessions SET revoked_at = NOW() WHERE user_id = $1 AND id != $2 AND revoked_at IS NULL`,
		userID, exceptID,
	)
	return err
}

func (s *SessionStorage) ListByUserID(ctx context.Context, userID string) ([]*session.SessionRow, error) {
	query := `
SELECT
  id, user_id, auth_module,
  COALESCE(token_hash,''), COALESCE(ip_address,''), COALESCE(user_agent,''),
  create_time, COALESCE(last_activity_at, create_time),
  COALESCE(absolute_expires_at, expires_at), expires_at,
  revoked_at,
  COALESCE(totp_verified, FALSE),
  COALESCE(old_token_hash,''), old_token_expires_at
FROM sessions
WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > NOW()
ORDER BY last_activity_at DESC`

	rows, err := s.q(ctx).Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*session.SessionRow
	for rows.Next() {
		row := &session.SessionRow{}
		if err := rows.Scan(
			&row.ID, &row.UserID, &row.AuthModule,
			&row.TokenHash, &row.IPAddress, &row.UserAgent,
			&row.CreatedAt, &row.LastActivityAt,
			&row.AbsoluteExpiresAt, &row.IdleExpiresAt,
			&row.RevokedAt,
			&row.TOTPVerified,
			&row.OldTokenHash, &row.OldTokenExpiresAt,
		); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *SessionStorage) MarkTOTPVerified(ctx context.Context, id string) error {
	_, err := s.q(ctx).Exec(ctx, `UPDATE sessions SET totp_verified = TRUE WHERE id = $1`, id)
	return err
}

func (s *SessionStorage) GetByID(ctx context.Context, id uuid.UUID) (*session.SessionRow, error) {
	query := `
SELECT
  id, user_id, auth_module,
  COALESCE(token_hash,''), COALESCE(ip_address,''), COALESCE(user_agent,''),
  create_time, COALESCE(last_activity_at, create_time),
  COALESCE(absolute_expires_at, expires_at), expires_at,
  revoked_at,
  COALESCE(totp_verified, FALSE),
  COALESCE(old_token_hash,''), old_token_expires_at
FROM sessions
WHERE id = $1`

	row := &session.SessionRow{}
	err := s.q(ctx).QueryRow(ctx, query, id).Scan(
		&row.ID, &row.UserID, &row.AuthModule,
		&row.TokenHash, &row.IPAddress, &row.UserAgent,
		&row.CreatedAt, &row.LastActivityAt,
		&row.AbsoluteExpiresAt, &row.IdleExpiresAt,
		&row.RevokedAt,
		&row.TOTPVerified,
		&row.OldTokenHash, &row.OldTokenExpiresAt,
	)
	if err != nil {
		return nil, err
	}
	return row, nil
}
