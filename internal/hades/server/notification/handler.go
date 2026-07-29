// Package notification implements the NotificationService ConnectRPC handler.
package notification

import (
	"context"

	"connectrpc.com/connect"

	registrypbv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	registryv1connect "github.com/alipourhabibi/Hades/api/gen/api/identity/v1/identityv1connect"
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
	notificationStorage notification.Storage
}

func NewHandler(deps *server.Dependencies) *Handler {
	return &Handler{
		logger:              deps.Logger,
		notificationStorage: deps.NotificationDB,
	}
}

func (h *Handler) ListNotifications(ctx context.Context, in *connect.Request[registrypbv1.ListNotificationsRequest]) (*connect.Response[registrypbv1.ListNotificationsResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*registrypbv1.User)
	if !ok {
		h.logger.Error("missing user in context", "procedure", "ListNotifications")
		return nil, connErr.Unauthenticated("not authenticated")
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

func (h *Handler) MarkNotificationRead(ctx context.Context, in *connect.Request[registrypbv1.MarkNotificationReadRequest]) (*connect.Response[registrypbv1.MarkNotificationReadResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*registrypbv1.User)
	if !ok {
		h.logger.Error("missing user in context", "procedure", "MarkNotificationRead")
		return nil, connErr.Unauthenticated("not authenticated")
	}

	if err := h.notificationStorage.MarkRead(ctx, in.Msg.Id, user.Id); err != nil {
		h.logger.Error("failed to mark notification read", "error", err, "procedure", "MarkNotificationRead", "user_id", user.Id, "notification_id", in.Msg.Id)
		return nil, connErr.FromPgx(err)
	}

	return &connect.Response[registrypbv1.MarkNotificationReadResponse]{
		Msg: &registrypbv1.MarkNotificationReadResponse{},
	}, nil
}
