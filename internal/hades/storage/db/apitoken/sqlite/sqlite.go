package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/apitoken"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqltypes"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqlutil"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/google/uuid"
)

// SQLiteAPITokenStorage implements apitoken.Storage using database/sql with SQLite.
type SQLiteAPITokenStorage struct {
	db *sql.DB
}

func NewAPIToken(db *sql.DB) *SQLiteAPITokenStorage {
	return &SQLiteAPITokenStorage{db: db}
}

func (s *SQLiteAPITokenStorage) q(ctx context.Context) txkeys.SQLQuerier {
	if tx, ok := txkeys.SQLTxFromContext(ctx); ok {
		return tx
	}
	return s.db
}

// Scopes are stored as a JSON array, not a comma-joined string.
//
// PostgreSQL stores them in a TEXT[]. A comma-joined string cannot represent a
// scope containing a comma, so the same value round-tripped differently
// depending on the backend. JSON is the smallest encoding that agrees with the
// array semantics on the other side.
func encodeScopes(scopes []string) (string, error) {
	if len(scopes) == 0 {
		return "", nil
	}
	b, err := json.Marshal(scopes)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// decodeScopes reads either encoding: JSON for rows written since the change,
// and the legacy comma-joined form for rows written before it.
func decodeScopes(raw string) []string {
	if raw == "" {
		return nil
	}
	if raw[0] == '[' {
		var out []string
		if err := json.Unmarshal([]byte(raw), &out); err == nil {
			return out
		}
	}
	return strings.Split(raw, ",")
}

func (s *SQLiteAPITokenStorage) Create(ctx context.Context, userID, name, prefix, tokenHash string, scopes []string, expiresAt *time.Time) (*apitoken.Row, error) {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	userID = sqlutil.ID(userID)
	scopeStr, err := encodeScopes(scopes)
	if err != nil {
		return nil, err
	}
	return scanSQLiteAPITokenRow(s.q(ctx).QueryRowContext(ctx,
		`INSERT INTO api_tokens (user_id, name, prefix, token_hash, scopes, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?) RETURNING `+sqliteAPITokenCols,
		userID, name, prefix, tokenHash, scopeStr, expiresAt,
	))
}

func scanSQLiteAPITokenRow(row *sql.Row) (*apitoken.Row, error) {
	r := &apitoken.Row{}
	var scopeStr string
	var expiresAt, lastUsedAt, revokedAt sqltypes.NullTime
	var createdAt sqltypes.Time
	err := row.Scan(&r.ID, &r.UserID, &r.Name, &r.Prefix, &r.TokenHash, &scopeStr,
		&expiresAt, &lastUsedAt, &revokedAt, &createdAt)
	if err != nil {
		return nil, err
	}
	r.Scopes = decodeScopes(scopeStr)
	r.ExpiresAt = expiresAt.Ptr()
	r.LastUsedAt = lastUsedAt.Ptr()
	r.RevokedAt = revokedAt.Ptr()
	r.CreatedAt = createdAt.V
	r.UserID = sqlutil.Canonical(r.UserID)
	return r, nil
}

// #nosec G101 -- a column list, not a credential; see the PostgreSQL implementation.
const sqliteAPITokenCols = `id, user_id, name, prefix, token_hash, COALESCE(scopes,''), expires_at, last_used_at, revoked_at, create_time`

func (s *SQLiteAPITokenStorage) GetByTokenHash(ctx context.Context, tokenHash string) (*apitoken.Row, error) {
	return scanSQLiteAPITokenRow(s.q(ctx).QueryRowContext(ctx,
		`SELECT `+sqliteAPITokenCols+` FROM api_tokens WHERE token_hash = ?`, tokenHash))
}

func (s *SQLiteAPITokenStorage) GetByID(ctx context.Context, id uuid.UUID) (*apitoken.Row, error) {
	return scanSQLiteAPITokenRow(s.q(ctx).QueryRowContext(ctx,
		`SELECT `+sqliteAPITokenCols+` FROM api_tokens WHERE id = ?`, sqlutil.UUID(id)))
}

func (s *SQLiteAPITokenStorage) ListByUserID(ctx context.Context, userID string, limit, offset int) ([]*apitoken.Row, error) {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	userID = sqlutil.ID(userID)
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := s.q(ctx).QueryContext(ctx,
		// Revoked tokens are included; see the PostgreSQL implementation.
		`SELECT `+sqliteAPITokenCols+` FROM api_tokens WHERE user_id = ? ORDER BY create_time DESC LIMIT ? OFFSET ?`,
		userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*apitoken.Row
	for rows.Next() {
		r := &apitoken.Row{}
		var scopeStr string
		var expiresAt, lastUsedAt, revokedAt sqltypes.NullTime
		var createdAt sqltypes.Time
		if err := rows.Scan(&r.ID, &r.UserID, &r.Name, &r.Prefix, &r.TokenHash, &scopeStr,
			&expiresAt, &lastUsedAt, &revokedAt, &createdAt); err != nil {
			return nil, err
		}
		r.Scopes = decodeScopes(scopeStr)
		r.ExpiresAt = expiresAt.Ptr()
		r.LastUsedAt = lastUsedAt.Ptr()
		r.RevokedAt = revokedAt.Ptr()
		r.CreatedAt = createdAt.V
		r.UserID = sqlutil.Canonical(r.UserID)
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *SQLiteAPITokenStorage) Revoke(ctx context.Context, id uuid.UUID) error {
	_, err := s.q(ctx).ExecContext(ctx,
		`UPDATE api_tokens SET revoked_at = datetime('now') WHERE id = ?`, sqlutil.UUID(id))
	return err
}

func (s *SQLiteAPITokenStorage) RevokeByOwner(ctx context.Context, id uuid.UUID, userID string) error {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	userID = sqlutil.ID(userID)
	res, err := s.q(ctx).ExecContext(ctx,
		`UPDATE api_tokens SET revoked_at = datetime('now') WHERE id = ? AND user_id = ? AND revoked_at IS NULL`,
		sqlutil.UUID(id), userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return apitoken.ErrNotFound
	}
	return nil
}

func (s *SQLiteAPITokenStorage) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	_, err := s.q(ctx).ExecContext(ctx,
		`UPDATE api_tokens SET last_used_at = datetime('now') WHERE id = ?`, sqlutil.UUID(id))
	return err
}

var _ apitoken.Storage = (*SQLiteAPITokenStorage)(nil)
