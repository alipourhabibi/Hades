// Package notification provides storage operations for in-app notification records.
package notification

import (
	"context"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
)

// Storage is the domain interface for notification persistence.
type Storage interface {
	ListForUser(ctx context.Context, userID string) ([]*registryv1.Notification, error)
	MarkRead(ctx context.Context, id, userID string) error
}
