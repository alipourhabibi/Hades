package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/devicegrant"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sqltypes"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/txkeys"
	"github.com/google/uuid"
)

// SQLiteDeviceGrantStorage implements devicegrant.Storage using database/sql with SQLite.
type SQLiteDeviceGrantStorage struct {
	db *sql.DB
}

func NewDeviceGrant(db *sql.DB) *SQLiteDeviceGrantStorage {
	return &SQLiteDeviceGrantStorage{db: db}
}

func (s *SQLiteDeviceGrantStorage) q(ctx context.Context) txkeys.SQLQuerier {
	if tx, ok := txkeys.SQLTxFromContext(ctx); ok {
		return tx
	}
	return s.db
}

const sqliteDeviceGrantCols = `id, device_code_hash, user_code, user_id, api_token_id, approved_at, expires_at, create_time`

func scanSQLiteDeviceGrant(row *sql.Row) (*devicegrant.Row, error) {
	r := &devicegrant.Row{}
	var approvedAt sqltypes.NullTime
	var expiresAt, createdAt sqltypes.Time
	err := row.Scan(&r.ID, &r.DeviceCodeHash, &r.UserCode, &r.UserID, &r.APITokenID,
		&approvedAt, &expiresAt, &createdAt)
	if err != nil {
		return nil, err
	}
	r.ApprovedAt = approvedAt.Ptr()
	r.ExpiresAt = expiresAt.V
	r.CreatedAt = createdAt.V
	return r, nil
}

func (s *SQLiteDeviceGrantStorage) Create(ctx context.Context, deviceCodeHash, userCode string, expiresAt time.Time) (uuid.UUID, error) {
	var id string
	err := s.q(ctx).QueryRowContext(ctx,
		`INSERT INTO device_grants (device_code_hash, user_code, expires_at) VALUES (?, ?, ?) RETURNING id`,
		deviceCodeHash, userCode, expiresAt,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(id)
}

func (s *SQLiteDeviceGrantStorage) GetByDeviceCodeHash(ctx context.Context, deviceCodeHash string) (*devicegrant.Row, error) {
	return scanSQLiteDeviceGrant(s.q(ctx).QueryRowContext(ctx,
		`SELECT `+sqliteDeviceGrantCols+` FROM device_grants WHERE device_code_hash = ?`, deviceCodeHash))
}

func (s *SQLiteDeviceGrantStorage) GetByUserCode(ctx context.Context, userCode string) (*devicegrant.Row, error) {
	return scanSQLiteDeviceGrant(s.q(ctx).QueryRowContext(ctx,
		`SELECT `+sqliteDeviceGrantCols+` FROM device_grants WHERE user_code = ?`, userCode))
}

func (s *SQLiteDeviceGrantStorage) Approve(ctx context.Context, id uuid.UUID, userID string, apiTokenID *uuid.UUID) error {
	_, err := s.q(ctx).ExecContext(ctx,
		`UPDATE device_grants SET user_id = ?, api_token_id = ?, approved_at = datetime('now') WHERE id = ?`,
		userID, apiTokenID, id.String())
	return err
}

var _ devicegrant.Storage = (*SQLiteDeviceGrantStorage)(nil)
