// Package auditsvc implements the AuditService ConnectRPC handler.
// It exposes paginated audit log access for the authenticated user via
// ListAuditLog. Pagination uses a cursor-style page token (integer offset).
package auditsvc

import (
	"context"
	"strconv"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/alipourhabibi/Hades/api/gen/api/authentication/v1"
	v1connect "github.com/alipourhabibi/Hades/api/gen/api/authentication/v1/authenticationv1connect"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/auditlog"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/alipourhabibi/Hades/utils/log"
)

type Handler struct {
	v1connect.AuditServiceHandler

	logger     *log.LoggerWrapper
	auditLogDB *auditlog.AuditLogStorage
}

func NewHandler(deps *server.Dependencies) *Handler {
	return &Handler{
		logger:     deps.Logger,
		auditLogDB: deps.AuditLogDB,
	}
}

func (h *Handler) ListAuditLog(ctx context.Context, in *connect.Request[v1.ListAuditLogRequest]) (*connect.Response[v1.ListAuditLogResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*registryv1.User)
	if !ok {
		h.logger.Error("missing user in context", "procedure", "ListAuditLog")
		return nil, connErr.Internal("missing user in context")
	}

	pageSize := int(in.Msg.PageSize)
	if pageSize <= 0 {
		pageSize = 50
	}
	offset := 0
	if in.Msg.PageToken != "" {
		if n, err := strconv.Atoi(in.Msg.PageToken); err == nil {
			offset = n
		}
	}

	rows, err := h.auditLogDB.List(ctx, user.Id, pageSize, offset)
	if err != nil {
		h.logger.Error("failed to list audit log", "error", err, "procedure", "ListAuditLog", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
	}

	events := make([]*v1.AuditEvent, 0, len(rows))
	for _, row := range rows {
		ae := &v1.AuditEvent{
			Id:        row.ID.String(),
			EventType: row.Event,
			IpAddress: row.IPAddress,
			UserAgent: row.UserAgent,
			CreatedAt: timestamppb.New(row.CreatedAt),
		}
		if row.Metadata != nil {
			if s, err := structpb.NewStruct(row.Metadata); err == nil {
				ae.Metadata = s
			}
		}
		events = append(events, ae)
	}

	nextPageToken := ""
	if len(rows) == pageSize {
		nextPageToken = strconv.Itoa(offset + pageSize)
	}

	return &connect.Response[v1.ListAuditLogResponse]{
		Msg: &v1.ListAuditLogResponse{
			Events:        events,
			NextPageToken: nextPageToken,
		},
	}, nil
}
