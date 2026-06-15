// Package oauthidentity stores OAuth identity links between external provider
// accounts and Hades users. Each row binds a (provider, provider_uid) pair
// to a local user_id so that the same Hades account is returned on
// subsequent OAuth logins with the same provider identity.
package oauthidentity

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

type OAuthIdentityStorage struct {
	db querier
}

func New(pool *pgxpool.Pool) *OAuthIdentityStorage {
	return &OAuthIdentityStorage{db: pool}
}

func (s *OAuthIdentityStorage) WithTx(tx pgx.Tx) *OAuthIdentityStorage {
	return &OAuthIdentityStorage{db: tx}
}

type Row struct {
	ID          uuid.UUID
	UserID      string
	Provider    string
	ProviderUID string
	Email       string
	CreatedAt   time.Time
}

func (s *OAuthIdentityStorage) Create(ctx context.Context, userID, provider, providerUID, email string) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO oauth_identities (user_id, provider, provider_uid, email) VALUES ($1, $2, $3, $4)`,
		userID, provider, providerUID, email,
	)
	return err
}

func (s *OAuthIdentityStorage) GetByProviderUID(ctx context.Context, provider, providerUID string) (*Row, error) {
	row := &Row{}
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, provider, provider_uid, COALESCE(email,''), create_time FROM oauth_identities WHERE provider = $1 AND provider_uid = $2`,
		provider, providerUID,
	).Scan(&row.ID, &row.UserID, &row.Provider, &row.ProviderUID, &row.Email, &row.CreatedAt)
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (s *OAuthIdentityStorage) GetByUserID(ctx context.Context, userID string) ([]*Row, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, user_id, provider, provider_uid, COALESCE(email,''), create_time FROM oauth_identities WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*Row
	for rows.Next() {
		row := &Row{}
		if err := rows.Scan(&row.ID, &row.UserID, &row.Provider, &row.ProviderUID, &row.Email, &row.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *OAuthIdentityStorage) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.Exec(ctx, `DELETE FROM oauth_identities WHERE id = $1`, id)
	return err
}

func (s *OAuthIdentityStorage) DeleteByUserAndProvider(ctx context.Context, userID, provider string) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM oauth_identities WHERE user_id = $1 AND provider = $2`,
		userID, provider,
	)
	return err
}
