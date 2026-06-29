// Package apitoken stores personal API tokens.
package apitoken

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Storage is the domain interface for API token persistence.
type Storage interface {
	Create(ctx context.Context, userID, name, prefix, tokenHash string, scopes []string, expiresAt *time.Time) (uuid.UUID, error)
	GetByTokenHash(ctx context.Context, tokenHash string) (*Row, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Row, error)
	ListByUserID(ctx context.Context, userID string) ([]*Row, error)
	Revoke(ctx context.Context, id uuid.UUID) error
	UpdateLastUsed(ctx context.Context, id uuid.UUID) error
}

type Row struct {
	ID         uuid.UUID
	UserID     string
	Name       string
	Prefix     string
	TokenHash  string
	Scopes     []string
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}
