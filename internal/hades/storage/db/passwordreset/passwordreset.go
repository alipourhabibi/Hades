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
}

type Row struct {
	ID        uuid.UUID
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
}
