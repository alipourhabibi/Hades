// Package apitoken stores personal API tokens. Token values are stored as
// SHA-256 hashes; the plaintext is returned only at creation time. Tokens
// support optional expiry and scopes, and are soft-deleted via Revoke
// (setting revoked_at) rather than hard-deleted so that audit traces are
// preserved.
package apitoken

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

type APITokenStorage struct {
	db querier
}

func New(pool *pgxpool.Pool) *APITokenStorage {
	return &APITokenStorage{db: pool}
}

func (s *APITokenStorage) WithTx(tx pgx.Tx) *APITokenStorage {
	return &APITokenStorage{db: tx}
}

type Row struct {
	ID         uuid.UUID
	UserID     string
	Name       string
	Prefix     string
	TokenHash  string
	Scopes     []string
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

func (s *APITokenStorage) Create(
	ctx context.Context,
	userID, name, prefix, tokenHash string,
	scopes []string,
	expiresAt *time.Time,
) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.db.QueryRow(ctx,
		`INSERT INTO api_tokens (user_id, name, prefix, token_hash, scopes, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		userID, name, prefix, tokenHash, scopes, expiresAt,
	).Scan(&id)
	return id, err
}

func (s *APITokenStorage) GetByTokenHash(ctx context.Context, tokenHash string) (*Row, error) {
	row := &Row{}
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, name, prefix, token_hash, COALESCE(scopes, '{}'),
		        expires_at, last_used_at, revoked_at, create_time
		 FROM api_tokens WHERE token_hash = $1`,
		tokenHash,
	).Scan(&row.ID, &row.UserID, &row.Name, &row.Prefix, &row.TokenHash, &row.Scopes,
		&row.ExpiresAt, &row.LastUsedAt, &row.RevokedAt, &row.CreatedAt)
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (s *APITokenStorage) GetByID(ctx context.Context, id uuid.UUID) (*Row, error) {
	row := &Row{}
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, name, prefix, token_hash, COALESCE(scopes, '{}'),
		        expires_at, last_used_at, revoked_at, create_time
		 FROM api_tokens WHERE id = $1`,
		id,
	).Scan(&row.ID, &row.UserID, &row.Name, &row.Prefix, &row.TokenHash, &row.Scopes,
		&row.ExpiresAt, &row.LastUsedAt, &row.RevokedAt, &row.CreatedAt)
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (s *APITokenStorage) ListByUserID(ctx context.Context, userID string) ([]*Row, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, user_id, name, prefix, token_hash, COALESCE(scopes, '{}'),
		        expires_at, last_used_at, revoked_at, create_time
		 FROM api_tokens WHERE user_id = $1 AND revoked_at IS NULL
		 ORDER BY create_time DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*Row
	for rows.Next() {
		row := &Row{}
		if err := rows.Scan(&row.ID, &row.UserID, &row.Name, &row.Prefix, &row.TokenHash, &row.Scopes,
			&row.ExpiresAt, &row.LastUsedAt, &row.RevokedAt, &row.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *APITokenStorage) Revoke(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.Exec(ctx, `UPDATE api_tokens SET revoked_at = NOW() WHERE id = $1`, id)
	return err
}

func (s *APITokenStorage) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.Exec(ctx, `UPDATE api_tokens SET last_used_at = NOW() WHERE id = $1`, id)
	return err
}
