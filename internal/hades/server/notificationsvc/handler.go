// Package notificationsvc implements the NotificationService ConnectRPC handler.
// It manages in-app notifications for the authenticated user via:
//   - ListNotifications     - returns all notifications for the caller, newest first.
//   - MarkNotificationRead  - marks a single notification as read.
//
// Ownership is enforced at the storage layer by filtering on user_id, so
// callers can only see and modify their own notifications.
package notificationsvc

import (
	"context"

	"connectrpc.com/connect"

	registrypbv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	registryv1connect "github.com/alipourhabibi/Hades/api/gen/api/registry/v1/registryv1connect"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/notification"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/alipourhabibi/Hades/utils/log"
)

// Handler implements the NotificationService ConnectRPC handler.
type Handler struct {
	registryv1connect.NotificationServiceHandler

	logger              *log.LoggerWrapper
	notificationStorage *notification.NotificationStorage
}

// NewHandler constructs a Handler wired to the notification storage from the
// shared dependency bag.
func NewHandler(deps *server.Dependencies) *Handler {
	return &Handler{
		logger:              deps.Logger,
		notificationStorage: deps.NotificationDB,
	}
}

// ListNotifications returns all notifications for the authenticated user,
// ordered newest first.  Both read and unread notifications are included.
func (h *Handler) ListNotifications(ctx context.Context, in *connect.Request[registrypbv1.ListNotificationsRequest]) (*connect.Response[registrypbv1.ListNotificationsResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*registrypbv1.User)
	if !ok {
		h.logger.Error("missing user in context", "procedure", "ListNotifications")
		return nil, connErr.Internal("missing user in context")
	}

	notifications, err := h.notificationStorage.ListForUser(ctx, user.Id)
	if err != nil {
		h.logger.Error("failed to list notifications", "error", err, "procedure", "ListNotifications", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
	}

	return &connect.Response[registrypbv1.ListNotificationsResponse]{
		Msg: &registrypbv1.ListNotificationsResponse{Notifications: notifications},
	}, nil
}

// MarkNotificationRead marks the notification identified by id as read.
// The notification must belong to the authenticated user; the storage layer
// enforces this via a user_id filter.  Silently succeeds if the notification
// is already marked read.
func (h *Handler) MarkNotificationRead(ctx context.Context, in *connect.Request[registrypbv1.MarkNotificationReadRequest]) (*connect.Response[registrypbv1.MarkNotificationReadResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*registrypbv1.User)
	if !ok {
		h.logger.Error("missing user in context", "procedure", "MarkNotificationRead")
		return nil, connErr.Internal("missing user in context")
	}

	if err := h.notificationStorage.MarkRead(ctx, in.Msg.Id, user.Id); err != nil {
		h.logger.Error("failed to mark notification read", "error", err, "procedure", "MarkNotificationRead", "user_id", user.Id, "notification_id", in.Msg.Id)
		return nil, connErr.FromPgx(err)
	}

	return &connect.Response[registrypbv1.MarkNotificationReadResponse]{
		Msg: &registrypbv1.MarkNotificationReadResponse{},
	}, nil
}
