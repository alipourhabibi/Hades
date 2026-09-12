// Package notification implements the NotificationService ConnectRPC handler.
package notification

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	registrypbv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	registryv1connect "github.com/alipourhabibi/Hades/api/gen/api/identity/v1/identityv1connect"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/notification"
	"github.com/alipourhabibi/Hades/utils/connerr"
	"github.com/alipourhabibi/Hades/utils/log"
)

// Handler implements the NotificationService ConnectRPC handler.
type Handler struct {
	registryv1connect.UnimplementedNotificationServiceHandler

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
		return nil, connerr.Unauthenticated("not authenticated")
	}

	limit, offset := server.Page(in.Msg.PageSize, in.Msg.PageToken)
	notifications, err := h.notificationStorage.ListForUser(ctx, user.Id, limit, offset)
	if err != nil {
		h.logger.Error("failed to list notifications", "error", err, "procedure", "ListNotifications", "user_id", user.Id)
		return nil, connerr.FromDB(err)
	}

	return &connect.Response[registrypbv1.ListNotificationsResponse]{
		Msg: &registrypbv1.ListNotificationsResponse{
			Notifications: notifications,
			NextPageToken: server.NextPageToken(len(notifications), limit, offset),
		},
	}, nil
}

func (h *Handler) MarkNotificationRead(ctx context.Context, in *connect.Request[registrypbv1.MarkNotificationReadRequest]) (*connect.Response[registrypbv1.MarkNotificationReadResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*registrypbv1.User)
	if !ok {
		h.logger.Error("missing user in context", "procedure", "MarkNotificationRead")
		return nil, connerr.Unauthenticated("not authenticated")
	}

	if err := h.notificationStorage.MarkRead(ctx, in.Msg.Id, user.Id); err != nil {
		// A notification that does not exist, or belongs to someone else, or
		// was already read, is reported as success on purpose: returning
		// NotFound here would confirm which notification ids exist. The
		// storage layer distinguishes the cases so a caller that needs to can;
		// this handler deliberately does not.
		if errors.Is(err, notification.ErrNotFound) {
			h.logger.Debug("mark-read matched no notification", "procedure", "MarkNotificationRead",
				"user_id", user.Id, "notification_id", in.Msg.Id)
			return &connect.Response[registrypbv1.MarkNotificationReadResponse]{
				Msg: &registrypbv1.MarkNotificationReadResponse{},
			}, nil
		}
		h.logger.Error("failed to mark notification read", "error", err, "procedure", "MarkNotificationRead", "user_id", user.Id, "notification_id", in.Msg.Id)
		return nil, connerr.FromDB(err)
	}

	return &connect.Response[registrypbv1.MarkNotificationReadResponse]{
		Msg: &registrypbv1.MarkNotificationReadResponse{},
	}, nil
}
