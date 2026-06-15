// Package devicegrant stores device authorization grant records for the
// OAuth 2.0 Device Authorization Grant flow (RFC 8628). Each row tracks a
// pending or approved device code exchange, including the user-facing code
// shown on the CLI and the resulting API token once the user approves.
package devicegrant

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

type DeviceGrantStorage struct {
	db querier
}

func New(pool *pgxpool.Pool) *DeviceGrantStorage {
	return &DeviceGrantStorage{db: pool}
}

func (s *DeviceGrantStorage) WithTx(tx pgx.Tx) *DeviceGrantStorage {
	return &DeviceGrantStorage{db: tx}
}

type Row struct {
	ID              uuid.UUID
	DeviceCodeHash  string
	UserCode        string
	UserID          *string
	APITokenID      *uuid.UUID
	ApprovedAt      *time.Time
	ExpiresAt       time.Time
	CreatedAt       time.Time
}

func (s *DeviceGrantStorage) Create(ctx context.Context, deviceCodeHash, userCode string, expiresAt time.Time) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.db.QueryRow(ctx,
		`INSERT INTO device_grants (device_code_hash, user_code, expires_at)
		 VALUES ($1, $2, $3) RETURNING id`,
		deviceCodeHash, userCode, expiresAt,
	).Scan(&id)
	return id, err
}

func (s *DeviceGrantStorage) GetByDeviceCodeHash(ctx context.Context, deviceCodeHash string) (*Row, error) {
	row := &Row{}
	err := s.db.QueryRow(ctx,
		`SELECT id, device_code_hash, user_code, user_id, api_token_id, approved_at, expires_at, create_time
		 FROM device_grants WHERE device_code_hash = $1`,
		deviceCodeHash,
	).Scan(&row.ID, &row.DeviceCodeHash, &row.UserCode, &row.UserID, &row.APITokenID,
		&row.ApprovedAt, &row.ExpiresAt, &row.CreatedAt)
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (s *DeviceGrantStorage) GetByUserCode(ctx context.Context, userCode string) (*Row, error) {
	row := &Row{}
	err := s.db.QueryRow(ctx,
		`SELECT id, device_code_hash, user_code, user_id, api_token_id, approved_at, expires_at, create_time
		 FROM device_grants WHERE user_code = $1`,
		userCode,
	).Scan(&row.ID, &row.DeviceCodeHash, &row.UserCode, &row.UserID, &row.APITokenID,
		&row.ApprovedAt, &row.ExpiresAt, &row.CreatedAt)
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (s *DeviceGrantStorage) Approve(ctx context.Context, id uuid.UUID, userID string, apiTokenID *uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`UPDATE device_grants SET user_id = $1, api_token_id = $2, approved_at = NOW() WHERE id = $3`,
		userID, apiTokenID, id,
	)
	return err
}
