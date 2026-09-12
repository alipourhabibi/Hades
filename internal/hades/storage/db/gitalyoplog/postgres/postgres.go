// Package postgres provides the PostgreSQL implementation of gitalyoplog.Storage.
package postgres

import (
	"context"
	"time"

	"github.com/alipourhabibi/Hades/internal/hades/storage/db/gitalyoplog"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GitalyOpLogStorage persists git operation metadata.
//
// It always uses the pool directly, never a unit-of-work transaction, so that
// 'pending' rows are immediately committed and visible to the compensation
// pass. That is the whole point of the table.
type GitalyOpLogStorage struct {
	pool *pgxpool.Pool
}

// New returns a GitalyOpLogStorage backed by the given connection pool.
func New(pool *pgxpool.Pool) *GitalyOpLogStorage {
	return &GitalyOpLogStorage{pool: pool}
}

var _ gitalyoplog.Storage = (*GitalyOpLogStorage)(nil)

func (s *GitalyOpLogStorage) CreatePending(ctx context.Context, opType, moduleName, userID string) (uuid.UUID, error) {
	id := uuid.New()
	const q = `
		INSERT INTO gitaly_operation_log (id, operation_type, status, module_name, user_id)
		VALUES ($1, $2, $3, $4, $5)`
	if _, err := s.pool.Exec(ctx, q, id, opType, gitalyoplog.StatusPending, moduleName, userID); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func (s *GitalyOpLogStorage) UpdateStatus(ctx context.Context, id uuid.UUID, status, commitHash, errorReason string) error {
	const q = `
		UPDATE gitaly_operation_log
		SET status = $2, commit_hash = NULLIF($3, ''), error_reason = NULLIF($4, ''), update_time = $5
		WHERE id = $1`
	_, err := s.pool.Exec(ctx, q, id, status, commitHash, errorReason, time.Now())
	return err
}
