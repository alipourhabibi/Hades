package postgres

import (
	"context"
	"time"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/devicegrant"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/google/uuid"
	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DeviceGrantStorage struct {
	pool *pgxpool.Pool
}

func NewDeviceGrant(pool *pgxpool.Pool) *DeviceGrantStorage {
	return &DeviceGrantStorage{pool: pool}
}

func (s *DeviceGrantStorage) q(ctx context.Context) txkeys.PgxQuerier {
	if tx, ok := txkeys.PgxTxFromContext(ctx); ok {
		return tx
	}
	return s.pool
}

const deviceGrantColumns = `id, device_code_hash, user_code, user_id, api_token_id, approved_at, expires_at, create_time`

func scanDeviceGrantRow(r pgx.Row) (*devicegrant.Row, error) {
	row := &devicegrant.Row{}
	err := r.Scan(&row.ID, &row.DeviceCodeHash, &row.UserCode, &row.UserID, &row.APITokenID,
		&row.ApprovedAt, &row.ExpiresAt, &row.CreatedAt)
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (s *DeviceGrantStorage) Create(ctx context.Context, deviceCodeHash, userCode string, expiresAt time.Time) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.q(ctx).QueryRow(ctx,
		`INSERT INTO device_grants (device_code_hash, user_code, expires_at) VALUES ($1, $2, $3) RETURNING id`,
		deviceCodeHash, userCode, expiresAt,
	).Scan(&id)
	return id, err
}

func (s *DeviceGrantStorage) GetByDeviceCodeHash(ctx context.Context, deviceCodeHash string) (*devicegrant.Row, error) {
	return scanDeviceGrantRow(s.q(ctx).QueryRow(ctx,
		`SELECT `+deviceGrantColumns+` FROM device_grants WHERE device_code_hash = $1`, deviceCodeHash))
}

func (s *DeviceGrantStorage) GetByUserCode(ctx context.Context, userCode string) (*devicegrant.Row, error) {
	return scanDeviceGrantRow(s.q(ctx).QueryRow(ctx,
		`SELECT `+deviceGrantColumns+` FROM device_grants WHERE user_code = $1`, userCode))
}

func (s *DeviceGrantStorage) Approve(ctx context.Context, id uuid.UUID, userID string, apiTokenID *uuid.UUID) error {
	_, err := s.q(ctx).Exec(ctx,
		`UPDATE device_grants SET user_id = $1, api_token_id = $2, approved_at = NOW() WHERE id = $3`,
		userID, apiTokenID, id,
	)
	return err
}

var _ devicegrant.Storage = (*DeviceGrantStorage)(nil)
