// Package oauthidentity stores OAuth identity links between external provider accounts and Hades users.
package oauthidentity

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Storage is the domain interface for OAuth identity persistence.
type Storage interface {
	Create(ctx context.Context, userID, provider, providerUID, email string) error
	GetByProviderUID(ctx context.Context, provider, providerUID string) (*Row, error)
	GetByUserID(ctx context.Context, userID string) ([]*Row, error)
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteByUserAndProvider(ctx context.Context, userID, provider string) error
}

type Row struct {
	ID          uuid.UUID
	UserID      string
	Provider    string
	ProviderUID string
	Email       string
	CreatedAt   time.Time
}
