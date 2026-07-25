// Package devicegrant stores device authorization grant records.
package devicegrant

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Storage is the domain interface for device grant persistence.
type Storage interface {
	Create(ctx context.Context, deviceCodeHash, userCode string, expiresAt time.Time) (uuid.UUID, error)
	GetByDeviceCodeHash(ctx context.Context, deviceCodeHash string) (*Row, error)
	GetByUserCode(ctx context.Context, userCode string) (*Row, error)
	Approve(ctx context.Context, id uuid.UUID, userID string, apiTokenID *uuid.UUID) error
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
