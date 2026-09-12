package auth

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/alipourhabibi/Hades/api/gen/api/auth/v1"
	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
)

func (s *Server) ListAuditLog(ctx context.Context, in *connect.Request[v1.ListAuditLogRequest]) (*connect.Response[v1.ListAuditLogResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "ListAuditLog")
		return nil, connErr.Unauthenticated("not authenticated")
	}

	pageSize, offset := server.Page(in.Msg.PageSize, in.Msg.PageToken)

	rows, err := s.auditLogDB.List(ctx, user.Id, pageSize, offset)
	if err != nil {
		s.logger.Error("failed to list audit log", "error", err, "procedure", "ListAuditLog", "user_id", user.Id)
		return nil, connErr.FromDB(err)
	}

	events := make([]*v1.AuditEvent, 0, len(rows))
	for _, row := range rows {
		ae := &v1.AuditEvent{
			Id:        row.ID.String(),
			EventType: row.EventType,
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

	nextPageToken := server.NextPageToken(len(rows), pageSize, offset)

	return &connect.Response[v1.ListAuditLogResponse]{
		Msg: &v1.ListAuditLogResponse{
			Events:        events,
			NextPageToken: nextPageToken,
		},
	}, nil
}
