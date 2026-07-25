// Package backupcode stores hashed TOTP backup codes.
package backupcode

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Storage is the domain interface for backup code persistence.
type Storage interface {
	CreateBatch(ctx context.Context, userID string, codeHashes []string) error
	GetUnused(ctx context.Context, userID, codeHash string) (*Row, error)
	ListByUserID(ctx context.Context, userID string) ([]*Row, error)
	MarkUsed(ctx context.Context, id uuid.UUID) error
	DeleteAllForUser(ctx context.Context, userID string) error
}

type Row struct {
	ID        uuid.UUID
	UserID    string
	CodeHash  string
	UsedAt    *time.Time
	CreatedAt time.Time
}
