// Package passwordreset stores single-use password reset tokens.
package passwordreset

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Storage is the domain interface for password reset persistence.
type Storage interface {
	Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	GetByTokenHash(ctx context.Context, tokenHash string) (*Row, error)
	MarkUsed(ctx context.Context, id uuid.UUID) error

	// InvalidateForUser marks every outstanding reset token for the user as
	// used, so that requesting a new one retires the old ones rather than
	// adding to a growing set of simultaneously valid tokens.
	InvalidateForUser(ctx context.Context, userID string) error
}

type Row struct {
	ID        uuid.UUID
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
}
