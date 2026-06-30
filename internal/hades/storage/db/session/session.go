// Package session provides PostgreSQL storage for user sessions.
package session

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Storage is the domain interface for session persistence.
type Storage interface {
	Create(ctx context.Context, userId, authModule string, expiresAt time.Time) (string, error)
	CreateWithToken(ctx context.Context, userID, authModule, tokenHash, ipAddress, userAgent string, idleExpires, absoluteExpires time.Time) (string, error)
	GetByTokenHash(ctx context.Context, hash string) (*SessionRow, error)
	GetByOldTokenHash(ctx context.Context, hash string) (*SessionRow, error)
	GetByID(ctx context.Context, id uuid.UUID) (*SessionRow, error)
	ListByUserID(ctx context.Context, userID string) ([]*SessionRow, error)
	UpdateActivity(ctx context.Context, id, newTokenHash, oldTokenHash string, oldTokenExpires, newIdleExpires time.Time) error
	Revoke(ctx context.Context, id string) error
	RevokeAllForUser(ctx context.Context, userID, exceptID string) error
	MarkTOTPVerified(ctx context.Context, id string) error
}

// SessionRow holds the full session row.
type SessionRow struct {
	ID                string
	UserID            string
	AuthModule        string
	TokenHash         string
	IPAddress         string
	UserAgent         string
	CreatedAt         time.Time
	LastActivityAt    time.Time
	AbsoluteExpiresAt time.Time
	IdleExpiresAt     time.Time
	RevokedAt         *time.Time
	TOTPVerified      bool
	OldTokenHash      string
	OldTokenExpiresAt *time.Time
}
