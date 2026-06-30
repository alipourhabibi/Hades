package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/apitoken"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqltypes"
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

func (s *SQLiteAPITokenStorage) Create(ctx context.Context, userID, name, prefix, tokenHash string, scopes []string, expiresAt *time.Time) (uuid.UUID, error) {
	scopeStr := strings.Join(scopes, ",")
	var id string
	err := s.q(ctx).QueryRowContext(ctx,
		`INSERT INTO api_tokens (user_id, name, prefix, token_hash, scopes, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?) RETURNING id`,
		userID, name, prefix, tokenHash, scopeStr, expiresAt,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(id)
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
	if scopeStr != "" {
		r.Scopes = strings.Split(scopeStr, ",")
	}
	r.ExpiresAt = expiresAt.Ptr()
	r.LastUsedAt = lastUsedAt.Ptr()
	r.RevokedAt = revokedAt.Ptr()
	r.CreatedAt = createdAt.V
	return r, nil
}

const sqliteAPITokenCols = `id, user_id, name, prefix, token_hash, COALESCE(scopes,''), expires_at, last_used_at, revoked_at, create_time`

func (s *SQLiteAPITokenStorage) GetByTokenHash(ctx context.Context, tokenHash string) (*apitoken.Row, error) {
	return scanSQLiteAPITokenRow(s.q(ctx).QueryRowContext(ctx,
		`SELECT `+sqliteAPITokenCols+` FROM api_tokens WHERE token_hash = ?`, tokenHash))
}

func (s *SQLiteAPITokenStorage) GetByID(ctx context.Context, id uuid.UUID) (*apitoken.Row, error) {
	return scanSQLiteAPITokenRow(s.q(ctx).QueryRowContext(ctx,
		`SELECT `+sqliteAPITokenCols+` FROM api_tokens WHERE id = ?`, id.String()))
}

func (s *SQLiteAPITokenStorage) ListByUserID(ctx context.Context, userID string) ([]*apitoken.Row, error) {
	rows, err := s.q(ctx).QueryContext(ctx,
		`SELECT `+sqliteAPITokenCols+` FROM api_tokens WHERE user_id = ? AND revoked_at IS NULL ORDER BY create_time DESC`, userID)
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
		if scopeStr != "" {
			r.Scopes = strings.Split(scopeStr, ",")
		}
		r.ExpiresAt = expiresAt.Ptr()
		r.LastUsedAt = lastUsedAt.Ptr()
		r.RevokedAt = revokedAt.Ptr()
		r.CreatedAt = createdAt.V
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *SQLiteAPITokenStorage) Revoke(ctx context.Context, id uuid.UUID) error {
	_, err := s.q(ctx).ExecContext(ctx,
		`UPDATE api_tokens SET revoked_at = datetime('now') WHERE id = ?`, id.String())
	return err
}

func (s *SQLiteAPITokenStorage) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	_, err := s.q(ctx).ExecContext(ctx,
		`UPDATE api_tokens SET last_used_at = datetime('now') WHERE id = ?`, id.String())
	return err
}

var _ apitoken.Storage = (*SQLiteAPITokenStorage)(nil)
