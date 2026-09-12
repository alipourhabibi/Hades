// Package gitalyoplog provides an append-only log of git operations.
//
// Each row is written (auto-committed) before the git write fires, then
// updated to 'completed' / 'failed' / 'rolled_back' once the outcome is known.
// A compensation pass uses this table to reconcile stale 'pending' rows left by
// a crash between the git call and the database commit.
//
// It exists on both backends. It used to be PostgreSQL-only, with callers
// nil-guarding it, which meant the default deployment had no crash
// reconciliation at all and nothing said so.
package gitalyoplog

import (
	"context"

	"github.com/google/uuid"
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

// Storage is the domain interface for the operation log.
//
// Implementations must write outside any caller transaction, so that a
// 'pending' row is committed and visible before the git call it describes.
type Storage interface {
	// CreatePending inserts a row in 'pending' state and returns its ID.
	CreatePending(ctx context.Context, opType, moduleName, userID string) (uuid.UUID, error)

	// UpdateStatus sets the status, commit hash and error reason for a row.
	// Pass an empty string for commitHash or errorReason when not applicable.
	UpdateStatus(ctx context.Context, id uuid.UUID, status, commitHash, errorReason string) error
}
