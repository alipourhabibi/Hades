// Package gitalyoplog provides an append-only log of Gitaly operations.
// Each row is written (auto-committed) before a Gitaly RPC fires, then updated
// to 'completed' / 'failed' / 'rolled_back' after the outcome is known.
// A background cleanup job uses this table to compensate stale 'pending' rows
// left by server crashes between the Gitaly call and the DB transaction commit.
package gitalyoplog

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Status values for the operation log rows.
const (
	StatusPending    = "pending"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
	StatusRolledBack = "rolled_back"
)

// OpType values for the operation log rows.
const (
	OpCreateModule = "create_module"
	OpCommitFiles  = "commit_files"
)

// GitalyOpLogStorage persists Gitaly operation metadata.
// It always uses the pool directly (never inside a UoW transaction) so that
// 'pending' rows are immediately committed and visible to the cleanup job.
type GitalyOpLogStorage struct {
	pool *pgxpool.Pool
}

// New returns a GitalyOpLogStorage backed by the given connection pool.
func New(pool *pgxpool.Pool) *GitalyOpLogStorage {
	return &GitalyOpLogStorage{pool: pool}
}

// CreatePending inserts a new row in 'pending' state and returns its ID.
// This write is auto-committed - it is NOT part of any caller transaction.
func (s *GitalyOpLogStorage) CreatePending(ctx context.Context, opType, moduleName, userID string) (uuid.UUID, error) {
	id := uuid.New()
	const q = `
		INSERT INTO gitaly_operation_log (id, operation_type, status, module_name, user_id)
		VALUES ($1, $2, $3, $4, $5)`
	_, err := s.pool.Exec(ctx, q, id, opType, StatusPending, moduleName, userID)
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

// UpdateStatus sets the status, commit_hash, and error_reason for a log row.
// Pass an empty string for commitHash or errorReason when not applicable.
// This write is auto-committed - it is NOT part of any caller transaction.
func (s *GitalyOpLogStorage) UpdateStatus(ctx context.Context, id uuid.UUID, status, commitHash, errorReason string) error {
	const q = `
		UPDATE gitaly_operation_log
		SET status = $2, commit_hash = NULLIF($3, ''), error_reason = NULLIF($4, ''), update_time = $5
		WHERE id = $1`
	_, err := s.pool.Exec(ctx, q, id, status, commitHash, errorReason, time.Now())
	return err
}
