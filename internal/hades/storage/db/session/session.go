// Package session provides session storage for user sessions.
package session

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Storage is the domain interface for session persistence.
// Sessions are non-rotating: a single token_hash is valid until idle/absolute expiry
// or explicit revocation. Token rotation (old_token_hash grace window) is intentionally
// absent to keep auth logic simple and auditable.
type Storage interface {
	Create(ctx context.Context, userId, authModule string, expiresAt time.Time) (string, error)
	CreateWithToken(ctx context.Context, userID, authModule, tokenHash, ipAddress, userAgent string, idleExpires, absoluteExpires time.Time) (string, error)
	GetByTokenHash(ctx context.Context, hash string) (*SessionRow, error)
	GetByID(ctx context.Context, id uuid.UUID) (*SessionRow, error)
	ListByUserID(ctx context.Context, userID string) ([]*SessionRow, error)
	// Touch records activity on a session: it sets last_activity_at to now and
	// slides the idle expiry forward. The token itself is never changed.
	Touch(ctx context.Context, id string, idleExpires time.Time) error
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
}
