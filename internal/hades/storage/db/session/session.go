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
//
// Every method that addresses a session by id takes a uuid.UUID rather than a
// string. SQLite generates ids as 32 hex characters with no hyphens while
// uuid.UUID.String() produces the hyphenated form, so a string-typed id could
// be passed either normalised or raw, and a method that forgot to normalise
// matched zero rows and reported success. Revoke did exactly that: a user was
// told a session they believed compromised had been revoked, and it had not.
// A uuid.UUID parameter cannot be passed un-normalised, because normalisation
// happens once, inside each backend.
type Storage interface {
	Create(ctx context.Context, userId, authModule string, expiresAt time.Time) (uuid.UUID, error)
	CreateWithToken(ctx context.Context, userID, authModule, tokenHash, ipAddress, userAgent string, idleExpires, absoluteExpires time.Time) (uuid.UUID, error)
	GetByTokenHash(ctx context.Context, hash string) (*SessionRow, error)
	GetByID(ctx context.Context, id uuid.UUID) (*SessionRow, error)
	// ListByUserID returns the user's live sessions, most recently active
	// first, capped at 500. The cap exists so the method cannot return an
	// unbounded result set; one account holding five hundred live sessions is
	// already an anomaly worth seeing rather than paging through.
	ListByUserID(ctx context.Context, userID string) ([]*SessionRow, error)
	// Touch records activity on a session: it sets last_activity_at to now and
	// slides the idle expiry forward. The token itself is never changed.
	Touch(ctx context.Context, id uuid.UUID, idleExpires time.Time) error
	Revoke(ctx context.Context, id uuid.UUID) error
	// RevokeAllForUser revokes every live session for userID except exceptID.
	// Pass uuid.Nil to revoke all of them.
	RevokeAllForUser(ctx context.Context, userID string, exceptID uuid.UUID) error
	MarkTOTPVerified(ctx context.Context, id uuid.UUID) error
}

// SessionRow holds the full session row.
type SessionRow struct {
	ID                uuid.UUID
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
