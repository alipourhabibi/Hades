// Package totpsecret stores AES-256-GCM encrypted TOTP secrets.
package totpsecret

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrCodeAlreadyUsed is returned by ConsumeCounter when the time step has
// already been accepted for this account.
var ErrCodeAlreadyUsed = errors.New("totpsecret: this code has already been used")

// Storage is the domain interface for TOTP secret persistence.
type Storage interface {
	Upsert(ctx context.Context, userID, secretEnc string) error
	GetByUserID(ctx context.Context, userID string) (*Row, error)
	Enable(ctx context.Context, userID string) error
	Delete(ctx context.Context, userID string) error

	// ConsumeCounter records that the given TOTP time step has been accepted,
	// and refuses a step at or below the highest already recorded.
	//
	// The check and the write are one conditional UPDATE, so two concurrent
	// presentations of the same code cannot both succeed. Returns
	// ErrCodeAlreadyUsed when the step was already consumed.
	ConsumeCounter(ctx context.Context, userID string, counter uint64) error
}

type Row struct {
	ID         uuid.UUID
	UserID     string
	SecretEnc  string
	Enabled    bool
	EnrolledAt *time.Time
	CreatedAt  time.Time
	// LastUsedCounter is the highest TOTP time step accepted for this account,
	// or nil when none has been.
	LastUsedCounter *int64
}
