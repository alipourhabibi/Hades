// Package apitoken stores personal API tokens.
package apitoken

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is returned by RevokeByOwner when no matching, non-revoked token
// was found for the given (id, userID) pair.
var ErrNotFound = errors.New("token not found")

// Storage is the domain interface for API token persistence.
type Storage interface {
	Create(ctx context.Context, userID, name, prefix, tokenHash string, scopes []string, expiresAt *time.Time) (*Row, error)
	GetByTokenHash(ctx context.Context, tokenHash string) (*Row, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Row, error)
	// ListByUserID returns the user's tokens, revoked ones included, so that
	// APITokenStatus can report REVOKED rather than the row disappearing.
	ListByUserID(ctx context.Context, userID string, limit, offset int) ([]*Row, error)
	Revoke(ctx context.Context, id uuid.UUID) error
	// RevokeByOwner atomically revokes the token only if it is owned by userID.
	// Returns pgx.ErrNoRows if no matching token was found or ownership check fails.
	RevokeByOwner(ctx context.Context, id uuid.UUID, userID string) error
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
