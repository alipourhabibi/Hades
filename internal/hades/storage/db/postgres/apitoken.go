package postgres

import (
	"context"
	"time"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/apitoken"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/google/uuid"
	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type APITokenStorage struct {
	pool *pgxpool.Pool
}

func NewAPIToken(pool *pgxpool.Pool) *APITokenStorage {
	return &APITokenStorage{pool: pool}
}

func (s *APITokenStorage) q(ctx context.Context) txkeys.PgxQuerier {
	if tx, ok := txkeys.PgxTxFromContext(ctx); ok {
		return tx
	}
	return s.pool
}

func (s *APITokenStorage) Create(ctx context.Context, userID, name, prefix, tokenHash string, scopes []string, expiresAt *time.Time) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.q(ctx).QueryRow(ctx,
		`INSERT INTO api_tokens (user_id, name, prefix, token_hash, scopes, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		userID, name, prefix, tokenHash, scopes, expiresAt,
	).Scan(&id)
	return id, err
}

const apiTokenColumns = `id, user_id, name, prefix, token_hash, COALESCE(scopes, '{}'),
		        expires_at, last_used_at, revoked_at, create_time`

func scanAPITokenRow(r pgx.Row) (*apitoken.Row, error) {
	row := &apitoken.Row{}
	err := r.Scan(&row.ID, &row.UserID, &row.Name, &row.Prefix, &row.TokenHash, &row.Scopes,
		&row.ExpiresAt, &row.LastUsedAt, &row.RevokedAt, &row.CreatedAt)
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (s *APITokenStorage) GetByTokenHash(ctx context.Context, tokenHash string) (*apitoken.Row, error) {
	return scanAPITokenRow(s.q(ctx).QueryRow(ctx, `SELECT `+apiTokenColumns+` FROM api_tokens WHERE token_hash = $1`, tokenHash))
}

func (s *APITokenStorage) GetByID(ctx context.Context, id uuid.UUID) (*apitoken.Row, error) {
	return scanAPITokenRow(s.q(ctx).QueryRow(ctx, `SELECT `+apiTokenColumns+` FROM api_tokens WHERE id = $1`, id))
}

func (s *APITokenStorage) ListByUserID(ctx context.Context, userID string) ([]*apitoken.Row, error) {
	rows, err := s.q(ctx).Query(ctx,
		`SELECT `+apiTokenColumns+` FROM api_tokens WHERE user_id = $1 AND revoked_at IS NULL ORDER BY create_time DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*apitoken.Row
	for rows.Next() {
		row := &apitoken.Row{}
		if err := rows.Scan(&row.ID, &row.UserID, &row.Name, &row.Prefix, &row.TokenHash, &row.Scopes,
			&row.ExpiresAt, &row.LastUsedAt, &row.RevokedAt, &row.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *APITokenStorage) Revoke(ctx context.Context, id uuid.UUID) error {
	_, err := s.q(ctx).Exec(ctx, `UPDATE api_tokens SET revoked_at = NOW() WHERE id = $1`, id)
	return err
}

func (s *APITokenStorage) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	_, err := s.q(ctx).Exec(ctx, `UPDATE api_tokens SET last_used_at = NOW() WHERE id = $1`, id)
	return err
}

var _ apitoken.Storage = (*APITokenStorage)(nil)
