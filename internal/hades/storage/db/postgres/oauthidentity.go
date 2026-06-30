package postgres

import (
	"context"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/oauthidentity"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OAuthIdentityStorage struct {
	pool *pgxpool.Pool
}

func NewOAuthIdentity(pool *pgxpool.Pool) *OAuthIdentityStorage {
	return &OAuthIdentityStorage{pool: pool}
}

func (s *OAuthIdentityStorage) q(ctx context.Context) txkeys.PgxQuerier {
	if tx, ok := txkeys.PgxTxFromContext(ctx); ok {
		return tx
	}
	return s.pool
}

func (s *OAuthIdentityStorage) Create(ctx context.Context, userID, provider, providerUID, email string) error {
	_, err := s.q(ctx).Exec(ctx,
		`INSERT INTO oauth_identities (user_id, provider, provider_uid, email) VALUES ($1, $2, $3, $4)`,
		userID, provider, providerUID, email,
	)
	return err
}

func (s *OAuthIdentityStorage) GetByProviderUID(ctx context.Context, provider, providerUID string) (*oauthidentity.Row, error) {
	row := &oauthidentity.Row{}
	err := s.q(ctx).QueryRow(ctx,
		`SELECT id, user_id, provider, provider_uid, COALESCE(email,''), create_time FROM oauth_identities WHERE provider = $1 AND provider_uid = $2`,
		provider, providerUID,
	).Scan(&row.ID, &row.UserID, &row.Provider, &row.ProviderUID, &row.Email, &row.CreatedAt)
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (s *OAuthIdentityStorage) GetByUserID(ctx context.Context, userID string) ([]*oauthidentity.Row, error) {
	rows, err := s.q(ctx).Query(ctx,
		`SELECT id, user_id, provider, provider_uid, COALESCE(email,''), create_time FROM oauth_identities WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*oauthidentity.Row
	for rows.Next() {
		row := &oauthidentity.Row{}
		if err := rows.Scan(&row.ID, &row.UserID, &row.Provider, &row.ProviderUID, &row.Email, &row.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *OAuthIdentityStorage) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := s.q(ctx).Exec(ctx, `DELETE FROM oauth_identities WHERE id = $1`, id)
	return err
}

func (s *OAuthIdentityStorage) DeleteByUserAndProvider(ctx context.Context, userID, provider string) error {
	_, err := s.q(ctx).Exec(ctx,
		`DELETE FROM oauth_identities WHERE user_id = $1 AND provider = $2`, userID, provider)
	return err
}

var _ oauthidentity.Storage = (*OAuthIdentityStorage)(nil)
