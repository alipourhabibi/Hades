// Package totpsecret stores AES-256-GCM encrypted TOTP secrets.
package totpsecret

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Storage is the domain interface for TOTP secret persistence.
type Storage interface {
	Upsert(ctx context.Context, userID, secretEnc string) error
	GetByUserID(ctx context.Context, userID string) (*Row, error)
	Enable(ctx context.Context, userID string) error
	Delete(ctx context.Context, userID string) error
}

type Row struct {
	ID         uuid.UUID
	UserID     string
	SecretEnc  string
	Enabled    bool
	EnrolledAt *time.Time
	CreatedAt  time.Time
}
