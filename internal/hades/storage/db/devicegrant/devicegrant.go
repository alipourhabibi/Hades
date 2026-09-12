// Package devicegrant stores device authorization grant records.
package devicegrant

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrAlreadyApproved is returned by Approve when the grant already carries an
// approval. Approval is once-only: a user code is short and human-readable by
// design, so a second caller who learns one must not be able to re-point an
// issued grant at their own account.
var ErrAlreadyApproved = errors.New("devicegrant: grant is already approved")

// ErrTokenAlreadyIssued is returned by AttachToken when the grant already has a
// token. It is the outcome two interleaved polls race for, and exactly one of
// them wins.
var ErrTokenAlreadyIssued = errors.New("devicegrant: a token has already been issued for this grant")

// Storage is the domain interface for device grant persistence.
type Storage interface {
	Create(ctx context.Context, deviceCodeHash, userCode string, expiresAt time.Time) (uuid.UUID, error)
	GetByDeviceCodeHash(ctx context.Context, deviceCodeHash string) (*Row, error)
	GetByUserCode(ctx context.Context, userCode string) (*Row, error)

	// Approve binds the grant to a user. The update is conditional on the
	// grant not already being approved, so the check is enforced by the
	// database rather than by a read-then-write that two callers can both pass.
	// Returns ErrAlreadyApproved when no unapproved row matched.
	Approve(ctx context.Context, id uuid.UUID, userID string) error

	// AttachToken records the API token minted for an approved grant. The
	// update is conditional on no token being attached yet; returns
	// ErrTokenAlreadyIssued when one already is.
	AttachToken(ctx context.Context, id uuid.UUID, apiTokenID uuid.UUID) error
}

type Row struct {
	ID             uuid.UUID
	DeviceCodeHash string
	UserCode       string
	UserID         *string
	APITokenID     *uuid.UUID
	ApprovedAt     *time.Time
	ExpiresAt      time.Time
	CreatedAt      time.Time
}
