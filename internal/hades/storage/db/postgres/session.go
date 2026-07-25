package postgres

import (
	"context"
	"time"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/session"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SessionStorage handles all session CRUD against PostgreSQL.
type SessionStorage struct {
	pool *pgxpool.Pool
}

func NewSession(pool *pgxpool.Pool) *SessionStorage {
	return &SessionStorage{pool: pool}
}

func (s *SessionStorage) q(ctx context.Context) txkeys.PgxQuerier {
	if tx, ok := txkeys.PgxTxFromContext(ctx); ok {
		return tx
	}
	return s.pool
}

func (s *SessionStorage) Create(ctx context.Context, userId, authModule string, expiresAt time.Time) (string, error) {
	query := `
INSERT INTO sessions (user_id, auth_module, expires_at)
VALUES ($1, $2, $3) RETURNING id`
	var id string
	err := s.q(ctx).QueryRow(ctx, query, userId, authModule, expiresAt).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *SessionStorage) CreateWithToken(ctx context.Context, userID, authModule, tokenHash, ipAddress, userAgent string, idleExpires, absoluteExpires time.Time) (string, error) {
	query := `
INSERT INTO sessions (
  user_id, auth_module, expires_at,
  token_hash, ip_address, user_agent,
  last_activity_at, absolute_expires_at
) VALUES ($1, $2, $3, $4, $5, $6, NOW(), $7)
RETURNING id`
	var id string
	err := s.q(ctx).QueryRow(ctx, query, userID, authModule, idleExpires, tokenHash, ipAddress, userAgent, absoluteExpires).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

const sessionSelectColumns = `
SELECT
  id, user_id, auth_module,
  COALESCE(token_hash,''), COALESCE(ip_address,''), COALESCE(user_agent,''),
  create_time, COALESCE(last_activity_at, create_time),
  COALESCE(absolute_expires_at, expires_at), expires_at,
  revoked_at,
  COALESCE(totp_verified, FALSE),
  COALESCE(old_token_hash,''), old_token_expires_at
FROM sessions`

func scanSessionRow(row pgx.Row) (*session.SessionRow, error) {
	r := &session.SessionRow{}
	err := row.Scan(
		&r.ID, &r.UserID, &r.AuthModule,
		&r.TokenHash, &r.IPAddress, &r.UserAgent,
		&r.CreatedAt, &r.LastActivityAt,
		&r.AbsoluteExpiresAt, &r.IdleExpiresAt,
		&r.RevokedAt,
		&r.TOTPVerified,
		&r.OldTokenHash, &r.OldTokenExpiresAt,
	)
	return r, err
}

func (s *SessionStorage) GetByTokenHash(ctx context.Context, hash string) (*session.SessionRow, error) {
	return scanSessionRow(s.q(ctx).QueryRow(ctx, sessionSelectColumns+` WHERE token_hash = $1`, hash))
}

func (s *SessionStorage) GetByOldTokenHash(ctx context.Context, hash string) (*session.SessionRow, error) {
	return scanSessionRow(s.q(ctx).QueryRow(ctx, sessionSelectColumns+` WHERE old_token_hash = $1 AND old_token_expires_at > NOW()`, hash))
}

func (s *SessionStorage) GetByID(ctx context.Context, id uuid.UUID) (*session.SessionRow, error) {
	return scanSessionRow(s.q(ctx).QueryRow(ctx, sessionSelectColumns+` WHERE id = $1`, id))
}

func (s *SessionStorage) UpdateActivity(ctx context.Context, id, newTokenHash, oldTokenHash string, oldTokenExpires, newIdleExpires time.Time) error {
	query := `
UPDATE sessions SET
  token_hash           = $1,
  old_token_hash       = $2,
  old_token_expires_at = $3,
  last_activity_at     = NOW(),
  expires_at           = $4
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
	rows, err := s.q(ctx).Query(ctx, sessionSelectColumns+`
WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > NOW()
ORDER BY last_activity_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*session.SessionRow
	for rows.Next() {
		r := &session.SessionRow{}
		if err := rows.Scan(
			&r.ID, &r.UserID, &r.AuthModule,
			&r.TokenHash, &r.IPAddress, &r.UserAgent,
			&r.CreatedAt, &r.LastActivityAt,
			&r.AbsoluteExpiresAt, &r.IdleExpiresAt,
			&r.RevokedAt,
			&r.TOTPVerified,
			&r.OldTokenHash, &r.OldTokenExpiresAt,
		); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *SessionStorage) MarkTOTPVerified(ctx context.Context, id string) error {
	_, err := s.q(ctx).Exec(ctx, `UPDATE sessions SET totp_verified = TRUE WHERE id = $1`, id)
	return err
}

var _ session.Storage = (*SessionStorage)(nil)
