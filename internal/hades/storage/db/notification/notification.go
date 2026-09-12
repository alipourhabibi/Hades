// Package notification provides storage operations for in-app notification records.
package notification

import (
	"context"
	"fmt"

	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	"github.com/alipourhabibi/Hades/utils/connerr"
)

// ErrNotFound is returned when a notification does not exist or does not
// belong to the caller.
var ErrNotFound = fmt.Errorf("notification: %w", connerr.ErrNotFound)

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

	// CreateBatch records the same notification for many users in one
	// statement. A push notifies every member of the owning organisation, and
	// doing that one INSERT at a time put one round trip per member on the push
	// path.
	CreateBatch(ctx context.Context, userIDs []string, notificationType, title, body, resourceID string) error

	// ListForUser returns the user's notifications, newest first, at most limit
	// of them starting at offset. A notification is written on every push to
	// every module the user can see, so this set grows without bound and the
	// bound has to be the caller's.
	ListForUser(ctx context.Context, userID string, limit, offset int) ([]*identityv1.Notification, error)

	// MarkRead marks one notification read. It returns ErrNotFound when no
	// notification with that id belongs to userID, so that marking someone
	// else's notification read is a refusal rather than a silent no-op that
	// reports success.
	MarkRead(ctx context.Context, id, userID string) error
}
