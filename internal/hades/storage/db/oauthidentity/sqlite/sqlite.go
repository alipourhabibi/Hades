package sqlite

import (
	"context"
	"database/sql"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/oauthidentity"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqltypes"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqlutil"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/google/uuid"
)

// SQLiteOAuthIdentityStorage implements oauthidentity.Storage using database/sql with SQLite.
type SQLiteOAuthIdentityStorage struct {
	db *sql.DB
}

func NewOAuthIdentity(db *sql.DB) *SQLiteOAuthIdentityStorage {
	return &SQLiteOAuthIdentityStorage{db: db}
}

func (s *SQLiteOAuthIdentityStorage) q(ctx context.Context) txkeys.SQLQuerier {
	if tx, ok := txkeys.SQLTxFromContext(ctx); ok {
		return tx
	}
	return s.db
}

func (s *SQLiteOAuthIdentityStorage) Create(ctx context.Context, userID, provider, providerUID, email string) error {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	userID = sqlutil.ID(userID)
	_, err := s.q(ctx).ExecContext(ctx,
		`INSERT INTO oauth_identities (user_id, provider, provider_uid, email) VALUES (?, ?, ?, ?)`,
		userID, provider, providerUID, email)
	return err
}

func (s *SQLiteOAuthIdentityStorage) GetByProviderUID(ctx context.Context, provider, providerUID string) (*oauthidentity.Row, error) {
	row := &oauthidentity.Row{}
	var createdAt sqltypes.Time
	err := s.q(ctx).QueryRowContext(ctx,
		`SELECT id, user_id, provider, provider_uid, COALESCE(email,''), create_time FROM oauth_identities WHERE provider = ? AND provider_uid = ?`,
		provider, providerUID,
	).Scan(&row.ID, &row.UserID, &row.Provider, &row.ProviderUID, &row.Email, &createdAt)
	if err != nil {
		return nil, err
	}
	row.CreatedAt = createdAt.V
	row.UserID = sqlutil.Canonical(row.UserID)
	return row, nil
}

func (s *SQLiteOAuthIdentityStorage) GetByUserID(ctx context.Context, userID string) ([]*oauthidentity.Row, error) {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	userID = sqlutil.ID(userID)
	rows, err := s.q(ctx).QueryContext(ctx,
		`SELECT id, user_id, provider, provider_uid, COALESCE(email,''), create_time FROM oauth_identities WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*oauthidentity.Row
	for rows.Next() {
		row := &oauthidentity.Row{}
		var createdAt sqltypes.Time
		if err := rows.Scan(&row.ID, &row.UserID, &row.Provider, &row.ProviderUID, &row.Email, &createdAt); err != nil {
			return nil, err
		}
		row.CreatedAt = createdAt.V
		row.UserID = sqlutil.Canonical(row.UserID)
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *SQLiteOAuthIdentityStorage) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := s.q(ctx).ExecContext(ctx, `DELETE FROM oauth_identities WHERE id = ?`, sqlutil.UUID(id))
	return err
}

func (s *SQLiteOAuthIdentityStorage) DeleteByUserAndProvider(ctx context.Context, userID, provider string) error {
	// SQLite stores identifiers without hyphens; see sqlutil.ID.
	userID = sqlutil.ID(userID)
	_, err := s.q(ctx).ExecContext(ctx,
		`DELETE FROM oauth_identities WHERE user_id = ? AND provider = ?`, userID, provider)
	return err
}

var _ oauthidentity.Storage = (*SQLiteOAuthIdentityStorage)(nil)
