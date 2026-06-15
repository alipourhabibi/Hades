// Package session provides PostgreSQL storage for user sessions, including
// token-based lookup, idle/absolute expiry tracking, and token rotation
// with a grace window so in-flight requests survive rotation.
package session

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

// SessionStorage handles all session CRUD against PostgreSQL.
// It implements the querier interface so it can operate inside
// a UnitOfWork transaction via WithTx.
type SessionStorage struct {
	db querier
}

// New creates a SessionStorage backed by the given connection pool.
func New(pool *pgxpool.Pool) *SessionStorage {
	return &SessionStorage{
		db: pool,
	}
}

// WithTx returns a copy of SessionStorage bound to the given transaction.
func (s *SessionStorage) WithTx(tx pgx.Tx) *SessionStorage {
	return &SessionStorage{db: tx}
}

// Create inserts a minimal session row and returns its ID.
// Prefer CreateWithToken for production paths; this exists for
// legacy/test flows that don't need token-based lookup.
func (s *SessionStorage) Create(
	ctx context.Context,
	userId string,
	authModule string,
	expiresAt time.Time,
) (string, error) {
	query := `
INSERT INTO sessions (
  user_id,
  auth_module,
  expires_at
) VALUES ($1, $2, $3) RETURNING id`

	var id string
	err := s.db.QueryRow(ctx, query,
		userId,
		authModule,
		expiresAt,
	).Scan(&id)
	if err != nil {
		return "", err
	}

	return id, nil
}

// SessionRow holds the full session row as returned by GetByTokenHash /
// ListByUserID.
type SessionRow struct {
	ID                 string
	UserID             string
	AuthModule         string
	TokenHash          string
	IPAddress          string
	UserAgent          string
	CreatedAt          time.Time
	LastActivityAt     time.Time
	AbsoluteExpiresAt  time.Time
	IdleExpiresAt      time.Time
	RevokedAt          *time.Time
	TOTPVerified       bool
	OldTokenHash       string
	OldTokenExpiresAt  *time.Time
}

// CreateWithToken inserts a new session with a hashed token and metadata.
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
	err := s.db.QueryRow(ctx, query,
		userID, authModule, idleExpires,
		tokenHash, ipAddress, userAgent,
		absoluteExpires,
	).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

// GetByTokenHash looks up a session row by the hashed bearer token.
func (s *SessionStorage) GetByTokenHash(ctx context.Context, hash string) (*SessionRow, error) {
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

	row := &SessionRow{}
	err := s.db.QueryRow(ctx, query, hash).Scan(
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

// GetByOldTokenHash looks up a session row by its old_token_hash (grace window).
func (s *SessionStorage) GetByOldTokenHash(ctx context.Context, hash string) (*SessionRow, error) {
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

	row := &SessionRow{}
	err := s.db.QueryRow(ctx, query, hash).Scan(
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

// UpdateActivity rotates the token and bumps last_activity_at.
// The old token hash is stored for the grace period.
func (s *SessionStorage) UpdateActivity(
	ctx context.Context,
	id, newTokenHash, oldTokenHash string,
	oldTokenExpires time.Time,
	newIdleExpires time.Time,
) error {
	query := `
UPDATE sessions SET
  token_hash          = $1,
  old_token_hash      = $2,
  old_token_expires_at = $3,
  last_activity_at    = NOW(),
  expires_at          = $4
WHERE id = $5`
	_, err := s.db.Exec(ctx, query, newTokenHash, oldTokenHash, oldTokenExpires, newIdleExpires, id)
	return err
}

// Revoke sets revoked_at on the session.
func (s *SessionStorage) Revoke(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `UPDATE sessions SET revoked_at = NOW() WHERE id = $1`, id)
	return err
}

// RevokeAllForUser revokes all sessions for userID except the one with exceptID.
func (s *SessionStorage) RevokeAllForUser(ctx context.Context, userID, exceptID string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE sessions SET revoked_at = NOW() WHERE user_id = $1 AND id != $2 AND revoked_at IS NULL`,
		userID, exceptID,
	)
	return err
}

// ListByUserID returns all non-expired sessions for a user.
func (s *SessionStorage) ListByUserID(ctx context.Context, userID string) ([]*SessionRow, error) {
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

	rows, err := s.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*SessionRow
	for rows.Next() {
		row := &SessionRow{}
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

// MarkTOTPVerified sets totp_verified = true on the session.
func (s *SessionStorage) MarkTOTPVerified(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `UPDATE sessions SET totp_verified = TRUE WHERE id = $1`, id)
	return err
}

// GetByID retrieves a session row by its primary key.
func (s *SessionStorage) GetByID(ctx context.Context, id uuid.UUID) (*SessionRow, error) {
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

	row := &SessionRow{}
	err := s.db.QueryRow(ctx, query, id).Scan(
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
