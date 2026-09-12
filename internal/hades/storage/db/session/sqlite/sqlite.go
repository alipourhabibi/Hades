package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/session"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqltypes"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqlutil"
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

func (s *SQLiteSessionStorage) Create(ctx context.Context, userId, authModule string, expiresAt time.Time) (uuid.UUID, error) {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	userId = sqlutil.ID(userId)
	var id string
	err := s.q(ctx).QueryRowContext(ctx,
		`INSERT INTO sessions (user_id, auth_module, expires_at) VALUES (?, ?, ?) RETURNING id`,
		userId, authModule, expiresAt,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(id)
}

func (s *SQLiteSessionStorage) CreateWithToken(ctx context.Context, userID, authModule, tokenHash, ipAddress, userAgent string, idleExpires, absoluteExpires time.Time) (uuid.UUID, error) {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	userID = sqlutil.ID(userID)
	var id string
	err := s.q(ctx).QueryRowContext(ctx, `
INSERT INTO sessions (
  user_id, auth_module, expires_at,
  token_hash, ip_address, user_agent,
  last_activity_at, absolute_expires_at
) VALUES (?, ?, ?, ?, ?, ?, datetime('now'), ?) RETURNING id`,
		userID, authModule, idleExpires, tokenHash, ipAddress, userAgent, absoluteExpires,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(id)
}

const sqliteSessionCols = `
id, user_id, auth_module,
COALESCE(token_hash,''), COALESCE(ip_address,''), COALESCE(user_agent,''),
create_time, COALESCE(last_activity_at, create_time),
COALESCE(absolute_expires_at, expires_at), expires_at,
revoked_at,
COALESCE(totp_verified, 0)
FROM sessions`

func scanSQLiteSession(row *sql.Row) (*session.SessionRow, error) {
	r := &session.SessionRow{}
	var createdAt, lastActivityAt, absExpiresAt, idleExpiresAt sqltypes.Time
	var revokedAt sqltypes.NullTime
	var totpVerified int
	err := row.Scan(
		&r.ID, &r.UserID, &r.AuthModule,
		&r.TokenHash, &r.IPAddress, &r.UserAgent,
		&createdAt, &lastActivityAt,
		&absExpiresAt, &idleExpiresAt,
		&revokedAt, &totpVerified,
	)
	if err != nil {
		return nil, err
	}
	// UserID is canonicalised on the way out, like every other id scanned from
	// SQLite. ID is a uuid.UUID and is parsed, which accepts the dashless form,
	// so it always looked right; UserID is a plain string and escaped
	// un-normalised. The handler's ownership check compares it against the
	// user id the API supplied, which is hyphenated, so the comparison was
	// always unequal: RevokeSession found the row, decided it belonged to
	// someone else, and answered 404. A user could not sign a device out on the
	// default backend, and the session stayed live for up to its full lifetime.
	r.UserID = sqlutil.Canonical(r.UserID)
	r.CreatedAt = createdAt.V
	r.LastActivityAt = lastActivityAt.V
	r.AbsoluteExpiresAt = absExpiresAt.V
	r.IdleExpiresAt = idleExpiresAt.V
	r.RevokedAt = revokedAt.Ptr()
	r.TOTPVerified = totpVerified != 0
	return r, nil
}

func (s *SQLiteSessionStorage) GetByTokenHash(ctx context.Context, hash string) (*session.SessionRow, error) {
	return scanSQLiteSession(s.q(ctx).QueryRowContext(ctx, `SELECT `+sqliteSessionCols+` WHERE token_hash = ?`, hash))
}

func (s *SQLiteSessionStorage) GetByID(ctx context.Context, id uuid.UUID) (*session.SessionRow, error) {
	return scanSQLiteSession(s.q(ctx).QueryRowContext(ctx, `SELECT `+sqliteSessionCols+` WHERE id = ?`, sqlutil.UUID(id)))
}

// Every id comparison below goes through sqlutil.UUID. SQLite ids are 32 hex
// characters with no hyphens; uuid.UUID.String() is hyphenated, so an
// un-normalised parameter matches nothing and the UPDATE silently affects zero
// rows while Exec still reports success.
func (s *SQLiteSessionStorage) Touch(ctx context.Context, id uuid.UUID, idleExpires time.Time) error {
	_, err := s.q(ctx).ExecContext(ctx,
		`UPDATE sessions SET last_activity_at = datetime('now'), expires_at = ? WHERE id = ? AND revoked_at IS NULL`,
		idleExpires, sqlutil.UUID(id))
	return err
}

func (s *SQLiteSessionStorage) Revoke(ctx context.Context, id uuid.UUID) error {
	_, err := s.q(ctx).ExecContext(ctx, `UPDATE sessions SET revoked_at = datetime('now') WHERE id = ?`, sqlutil.UUID(id))
	return err
}

func (s *SQLiteSessionStorage) RevokeAllForUser(ctx context.Context, userID string, exceptID uuid.UUID) error {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	userID = sqlutil.ID(userID)
	_, err := s.q(ctx).ExecContext(ctx,
		`UPDATE sessions SET revoked_at = datetime('now') WHERE user_id = ? AND id != ? AND revoked_at IS NULL`,
		userID, sqlutil.UUID(exceptID))
	return err
}

func (s *SQLiteSessionStorage) ListByUserID(ctx context.Context, userID string) ([]*session.SessionRow, error) {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	userID = sqlutil.ID(userID)
	// datetime() on both sides, not a bare column comparison.
	//
	// The driver stores a time.Time as RFC3339 with a numeric offset
	// ("2026-08-07T16:55:49.22+04:00") while datetime('now') yields
	// "2026-08-07 13:55:49". SQLite compares those as strings, and 'T' sorts
	// above ' ', so the bare form was true for every row whatever the actual
	// time: expired sessions stayed in the listing forever. datetime() parses
	// both spellings and normalises them to UTC.
	rows, err := s.q(ctx).QueryContext(ctx, `SELECT `+sqliteSessionCols+`
WHERE user_id = ? AND revoked_at IS NULL AND datetime(expires_at) > datetime('now')
ORDER BY last_activity_at DESC
LIMIT 500`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*session.SessionRow
	for rows.Next() {
		r := &session.SessionRow{}
		var createdAt, lastActivityAt, absExpiresAt, idleExpiresAt sqltypes.Time
		var revokedAt sqltypes.NullTime
		var totpVerified int
		if err := rows.Scan(
			&r.ID, &r.UserID, &r.AuthModule,
			&r.TokenHash, &r.IPAddress, &r.UserAgent,
			&createdAt, &lastActivityAt,
			&absExpiresAt, &idleExpiresAt,
			&revokedAt, &totpVerified,
		); err != nil {
			return nil, err
		}
		// Canonical here too; see scanSQLiteSession.
		r.UserID = sqlutil.Canonical(r.UserID)
		r.CreatedAt = createdAt.V
		r.LastActivityAt = lastActivityAt.V
		r.AbsoluteExpiresAt = absExpiresAt.V
		r.IdleExpiresAt = idleExpiresAt.V
		r.RevokedAt = revokedAt.Ptr()
		r.TOTPVerified = totpVerified != 0
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *SQLiteSessionStorage) MarkTOTPVerified(ctx context.Context, id uuid.UUID) error {
	_, err := s.q(ctx).ExecContext(ctx, `UPDATE sessions SET totp_verified = 1 WHERE id = ?`, sqlutil.UUID(id))
	return err
}

var _ session.Storage = (*SQLiteSessionStorage)(nil)
