package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/session"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqltypes"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/google/uuid"
)

// SQLiteSessionStorage implements session.Storage using database/sql with SQLite.
type SQLiteSessionStorage struct {
	db *sql.DB
}

func NewSession(db *sql.DB) *SQLiteSessionStorage {
	return &SQLiteSessionStorage{db: db}
}

func (s *SQLiteSessionStorage) q(ctx context.Context) txkeys.SQLQuerier {
	if tx, ok := txkeys.SQLTxFromContext(ctx); ok {
		return tx
	}
	return s.db
}

func (s *SQLiteSessionStorage) Create(ctx context.Context, userId, authModule string, expiresAt time.Time) (string, error) {
	var id string
	err := s.q(ctx).QueryRowContext(ctx,
		`INSERT INTO sessions (user_id, auth_module, expires_at) VALUES (?, ?, ?) RETURNING id`,
		userId, authModule, expiresAt,
	).Scan(&id)
	return id, err
}

func (s *SQLiteSessionStorage) CreateWithToken(ctx context.Context, userID, authModule, tokenHash, ipAddress, userAgent string, idleExpires, absoluteExpires time.Time) (string, error) {
	var id string
	err := s.q(ctx).QueryRowContext(ctx, `
INSERT INTO sessions (
  user_id, auth_module, expires_at,
  token_hash, ip_address, user_agent,
  last_activity_at, absolute_expires_at
) VALUES (?, ?, ?, ?, ?, ?, datetime('now'), ?) RETURNING id`,
		userID, authModule, idleExpires, tokenHash, ipAddress, userAgent, absoluteExpires,
	).Scan(&id)
	return id, err
}

const sqliteSessionCols = `
id, user_id, auth_module,
COALESCE(token_hash,''), COALESCE(ip_address,''), COALESCE(user_agent,''),
create_time, COALESCE(last_activity_at, create_time),
COALESCE(absolute_expires_at, expires_at), expires_at,
revoked_at,
COALESCE(totp_verified, 0),
COALESCE(old_token_hash,''), old_token_expires_at
FROM sessions`

func scanSQLiteSession(row *sql.Row) (*session.SessionRow, error) {
	r := &session.SessionRow{}
	var createdAt, lastActivityAt, absExpiresAt, idleExpiresAt sqltypes.Time
	var revokedAt, oldTokenExpiresAt sqltypes.NullTime
	var totpVerified int
	err := row.Scan(
		&r.ID, &r.UserID, &r.AuthModule,
		&r.TokenHash, &r.IPAddress, &r.UserAgent,
		&createdAt, &lastActivityAt,
		&absExpiresAt, &idleExpiresAt,
		&revokedAt, &totpVerified,
		&r.OldTokenHash, &oldTokenExpiresAt,
	)
	if err != nil {
		return nil, err
	}
	r.CreatedAt = createdAt.V
	r.LastActivityAt = lastActivityAt.V
	r.AbsoluteExpiresAt = absExpiresAt.V
	r.IdleExpiresAt = idleExpiresAt.V
	r.RevokedAt = revokedAt.Ptr()
	r.TOTPVerified = totpVerified != 0
	r.OldTokenExpiresAt = oldTokenExpiresAt.Ptr()
	return r, nil
}

func (s *SQLiteSessionStorage) GetByTokenHash(ctx context.Context, hash string) (*session.SessionRow, error) {
	return scanSQLiteSession(s.q(ctx).QueryRowContext(ctx, `SELECT `+sqliteSessionCols+` WHERE token_hash = ?`, hash))
}

func (s *SQLiteSessionStorage) GetByOldTokenHash(ctx context.Context, hash string) (*session.SessionRow, error) {
	return scanSQLiteSession(s.q(ctx).QueryRowContext(ctx, `SELECT `+sqliteSessionCols+` WHERE old_token_hash = ? AND old_token_expires_at > datetime('now')`, hash))
}

func (s *SQLiteSessionStorage) GetByID(ctx context.Context, id uuid.UUID) (*session.SessionRow, error) {
	return scanSQLiteSession(s.q(ctx).QueryRowContext(ctx, `SELECT `+sqliteSessionCols+` WHERE id = ?`, id.String()))
}

func (s *SQLiteSessionStorage) UpdateActivity(ctx context.Context, id, newTokenHash, oldTokenHash string, oldTokenExpires, newIdleExpires time.Time) error {
	_, err := s.q(ctx).ExecContext(ctx, `
UPDATE sessions SET
  token_hash           = ?,
  old_token_hash       = ?,
  old_token_expires_at = ?,
  last_activity_at     = datetime('now'),
  expires_at           = ?
WHERE id = ?`, newTokenHash, oldTokenHash, oldTokenExpires, newIdleExpires, id)
	return err
}

func (s *SQLiteSessionStorage) Revoke(ctx context.Context, id string) error {
	_, err := s.q(ctx).ExecContext(ctx, `UPDATE sessions SET revoked_at = datetime('now') WHERE id = ?`, id)
	return err
}

func (s *SQLiteSessionStorage) RevokeAllForUser(ctx context.Context, userID, exceptID string) error {
	_, err := s.q(ctx).ExecContext(ctx,
		`UPDATE sessions SET revoked_at = datetime('now') WHERE user_id = ? AND id != ? AND revoked_at IS NULL`,
		userID, exceptID)
	return err
}

func (s *SQLiteSessionStorage) ListByUserID(ctx context.Context, userID string) ([]*session.SessionRow, error) {
	rows, err := s.q(ctx).QueryContext(ctx, `SELECT `+sqliteSessionCols+` WHERE user_id = ? AND revoked_at IS NULL AND expires_at > datetime('now') ORDER BY last_activity_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*session.SessionRow
	for rows.Next() {
		r := &session.SessionRow{}
		var createdAt, lastActivityAt, absExpiresAt, idleExpiresAt sqltypes.Time
		var revokedAt, oldTokenExpiresAt sqltypes.NullTime
		var totpVerified int
		if err := rows.Scan(
			&r.ID, &r.UserID, &r.AuthModule,
			&r.TokenHash, &r.IPAddress, &r.UserAgent,
			&createdAt, &lastActivityAt,
			&absExpiresAt, &idleExpiresAt,
			&revokedAt, &totpVerified,
			&r.OldTokenHash, &oldTokenExpiresAt,
		); err != nil {
			return nil, err
		}
		r.CreatedAt = createdAt.V
		r.LastActivityAt = lastActivityAt.V
		r.AbsoluteExpiresAt = absExpiresAt.V
		r.IdleExpiresAt = idleExpiresAt.V
		r.RevokedAt = revokedAt.Ptr()
		r.TOTPVerified = totpVerified != 0
		r.OldTokenExpiresAt = oldTokenExpiresAt.Ptr()
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *SQLiteSessionStorage) MarkTOTPVerified(ctx context.Context, id string) error {
	_, err := s.q(ctx).ExecContext(ctx, `UPDATE sessions SET totp_verified = 1 WHERE id = ?`, id)
	return err
}

var _ session.Storage = (*SQLiteSessionStorage)(nil)
