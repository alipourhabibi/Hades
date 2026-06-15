// Package sessionsvc implements the SessionService ConnectRPC handler. It
// lets authenticated users list, revoke, and bulk-revoke their own sessions.
// Ownership is enforced by matching the session's user_id against the caller.
package sessionsvc

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/alipourhabibi/Hades/api/gen/api/authentication/v1"
	v1connect "github.com/alipourhabibi/Hades/api/gen/api/authentication/v1/authenticationv1connect"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/auditlog"
	dbsession "github.com/alipourhabibi/Hades/internal/hades/storage/db/session"
	utilscrypto "github.com/alipourhabibi/Hades/utils/crypto"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/alipourhabibi/Hades/utils/log"
	"github.com/google/uuid"
)

type Handler struct {
	v1connect.SessionServiceHandler

	logger         *log.LoggerWrapper
	sessionStorage *dbsession.SessionStorage
	auditLogDB     *auditlog.AuditLogStorage
}

func NewHandler(deps *server.Dependencies) *Handler {
	return &Handler{
		logger:         deps.Logger,
		sessionStorage: deps.SessionDB,
		auditLogDB:     deps.AuditLogDB,
	}
}

func (h *Handler) ListSessions(ctx context.Context, in *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*registryv1.User)
	if !ok {
		h.logger.Error("missing user in context", "procedure", "ListSessions")
		return nil, connErr.Internal("missing user in context")
	}

	// Determine current session ID from the bearer token.
	rawToken, _ := ctx.Value(constants.ContextKeyAuthorization).(string)
	currentID := ""
	if rawToken != "" {
		tokenHash := utilscrypto.HashToken(rawToken)
		if sess, err := h.sessionStorage.GetByTokenHash(ctx, tokenHash); err == nil {
			currentID = sess.ID
		}
	}

	rows, err := h.sessionStorage.ListByUserID(ctx, user.Id)
	if err != nil {
		h.logger.Error("failed to list sessions", "error", err, "procedure", "ListSessions", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
	}

	sessions := make([]*v1.Session, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, &v1.Session{
			Id:             row.ID,
			IpAddress:      row.IPAddress,
			UserAgent:      row.UserAgent,
			CreatedAt:      timestamppb.New(row.CreatedAt),
			LastActivityAt: timestamppb.New(row.LastActivityAt),
			IsCurrent:      row.ID == currentID,
		})
	}
	return &connect.Response[v1.ListSessionsResponse]{
		Msg: &v1.ListSessionsResponse{Sessions: sessions},
	}, nil
}

func (h *Handler) RevokeSession(ctx context.Context, in *connect.Request[v1.RevokeSessionRequest]) (*connect.Response[v1.RevokeSessionResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*registryv1.User)
	if !ok {
		h.logger.Error("missing user in context", "procedure", "RevokeSession")
		return nil, connErr.Internal("missing user in context")
	}

	sessionID, err := uuid.Parse(in.Msg.SessionId)
	if err != nil {
		h.logger.Warn("invalid session ID", "procedure", "RevokeSession", "user_id", user.Id, "session_id", in.Msg.SessionId)
		return nil, connErr.InvalidArgument("invalid session ID")
	}

	// Verify ownership.
	sess, err := h.sessionStorage.GetByID(ctx, sessionID)
	if err != nil || sess.UserID != user.Id {
		h.logger.Warn("session not found or not owned by user", "procedure", "RevokeSession", "user_id", user.Id, "session_id", in.Msg.SessionId)
		return nil, connErr.NotFound("session not found")
	}

	if err := h.sessionStorage.Revoke(ctx, in.Msg.SessionId); err != nil {
		h.logger.Error("failed to revoke session", "error", err, "procedure", "RevokeSession", "user_id", user.Id, "session_id", in.Msg.SessionId)
		return nil, connErr.FromPgx(err)
	}
	if h.auditLogDB != nil {
		_ = h.auditLogDB.Create(ctx, &user.Id, "session_revoked", "", "", map[string]any{"session_id": in.Msg.SessionId})
	}

	h.logger.Info("session revoked", "procedure", "RevokeSession", "user_id", user.Id, "session_id", in.Msg.SessionId)
	return &connect.Response[v1.RevokeSessionResponse]{Msg: &v1.RevokeSessionResponse{}}, nil
}

func (h *Handler) RevokeAllOtherSessions(ctx context.Context, in *connect.Request[v1.RevokeAllOtherSessionsRequest]) (*connect.Response[v1.RevokeAllOtherSessionsResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*registryv1.User)
	if !ok {
		h.logger.Error("missing user in context", "procedure", "RevokeAllOtherSessions")
		return nil, connErr.Internal("missing user in context")
	}

	rawToken, _ := ctx.Value(constants.ContextKeyAuthorization).(string)
	currentID := ""
	if rawToken != "" {
		tokenHash := utilscrypto.HashToken(rawToken)
		if sess, err := h.sessionStorage.GetByTokenHash(ctx, tokenHash); err == nil {
			currentID = sess.ID
		}
	}

	if err := h.sessionStorage.RevokeAllForUser(ctx, user.Id, currentID); err != nil {
		h.logger.Error("failed to revoke all other sessions", "error", err, "procedure", "RevokeAllOtherSessions", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
	}
	if h.auditLogDB != nil {
		_ = h.auditLogDB.Create(ctx, &user.Id, "session_revoked", "", "", map[string]any{"scope": "all_other"})
	}

	h.logger.Info("all other sessions revoked", "procedure", "RevokeAllOtherSessions", "user_id", user.Id)
	return &connect.Response[v1.RevokeAllOtherSessionsResponse]{Msg: &v1.RevokeAllOtherSessionsResponse{}}, nil
}
