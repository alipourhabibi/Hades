// Package notification provides storage operations for in-app notification records.
package notification

import (
	"context"

	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
)

// Event types carried in Notification.type. They are the machine-readable half
// of a notification; title and body are the human half.
const (
	// TypeCommitPushed is raised on every module owner and org member other
	// than the person who pushed.
	TypeCommitPushed = "commit.pushed"
	// TypeSDKSucceeded is raised on the author of the commit an SDK job was
	// generated from.
	TypeSDKSucceeded = "sdk.succeeded"
	// TypeSDKFailed is raised on the same recipient when generation fails.
	TypeSDKFailed = "sdk.failed"
)

// Storage is the domain interface for notification persistence.
type Storage interface {
	// Create records one notification for one user. resourceID is the id of
	// whatever the notification is about (a commit id, an SDK job id) and may
	// be empty.
	Create(ctx context.Context, userID, notificationType, title, body, resourceID string) error
	ListForUser(ctx context.Context, userID string) ([]*identityv1.Notification, error)
	MarkRead(ctx context.Context, id, userID string) error
}
