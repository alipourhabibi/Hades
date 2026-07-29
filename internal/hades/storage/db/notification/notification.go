// Package notification provides storage operations for in-app notification records.
package notification

import (
	"context"

	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
)

// Storage is the domain interface for notification persistence.
type Storage interface {
	ListForUser(ctx context.Context, userID string) ([]*identityv1.Notification, error)
	MarkRead(ctx context.Context, id, userID string) error
}
