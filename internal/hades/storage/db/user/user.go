// Package user provides PostgreSQL storage for user accounts, authentication
// fields, and session-based lookups.
package user

import (
	"context"
	"time"

	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
)

// Storage is the domain interface for user persistence.
type Storage interface {
	GetByUsername(ctx context.Context, username string) (*identityv1.User, error)
	GetByID(ctx context.Context, id string) (*identityv1.User, error)
	GetByEmail(ctx context.Context, email string) (*identityv1.User, error)
	GetBySessionId(ctx context.Context, sessionId string) (*identityv1.User, error)
	GetAuthFieldsByUsername(ctx context.Context, username string) (*AuthFields, error)
	GetAuthFieldsByID(ctx context.Context, id string) (*AuthFields, error)
	Create(ctx context.Context, username, email, password string, t identityv1.UserType, status identityv1.UserState, description, url string) error
	List(ctx context.Context, query string) ([]*identityv1.User, error)
	Update(ctx context.Context, userID, description, url string) (*identityv1.User, error)
	IncrementFailedLogins(ctx context.Context, userID string) error
	ResetFailedLogins(ctx context.Context, userID string) error
	LockUntil(ctx context.Context, userID string, until time.Time) error
	SetEmailVerified(ctx context.Context, userID string) error
	UpdatePassword(ctx context.Context, userID, newHash string) error
}

// AuthFields holds fields required during login / password validation.
type AuthFields struct {
	ID               string
	Username         string
	Email            string
	PasswordHash     string
	EmailVerifiedAt  *time.Time
	FailedLoginCount int
	LockedUntil      *time.Time
}
