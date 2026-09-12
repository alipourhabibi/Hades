package auth

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/alipourhabibi/Hades/api/gen/api/auth/v1"
	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	utilscrypto "github.com/alipourhabibi/Hades/utils/crypto"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/google/uuid"
)

func (s *Server) ListSessions(ctx context.Context, in *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "ListSessions")
		return nil, connErr.Unauthenticated("not authenticated")
	}

	rawToken, _ := ctx.Value(constants.ContextKeyAuthorization).(string)
	currentID := ""
	if rawToken != "" {
		tokenHash := utilscrypto.HashToken(rawToken)
		if sess, err := s.sessionStorage.GetByTokenHash(ctx, tokenHash); err == nil {
			currentID = sess.ID
		}
	}

	rows, err := s.sessionStorage.ListByUserID(ctx, user.Id)
	if err != nil {
		s.logger.Error("failed to list sessions", "error", err, "procedure", "ListSessions", "user_id", user.Id)
		return nil, connErr.FromDB(err)
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

func (s *Server) RevokeSession(ctx context.Context, in *connect.Request[v1.RevokeSessionRequest]) (*connect.Response[v1.RevokeSessionResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "RevokeSession")
		return nil, connErr.Unauthenticated("not authenticated")
	}

	sessionID, err := uuid.Parse(in.Msg.SessionId)
	if err != nil {
		s.logger.Warn("invalid session ID", "procedure", "RevokeSession", "user_id", user.Id, "session_id", in.Msg.SessionId)
		return nil, connErr.InvalidArgument("invalid session ID")
	}

	sess, err := s.sessionStorage.GetByID(ctx, sessionID)
	if err != nil || sess.UserID != user.Id {
		s.logger.Warn("session not found or not owned by user", "procedure", "RevokeSession", "user_id", user.Id, "session_id", in.Msg.SessionId)
		return nil, connErr.NotFound("session not found")
	}

	if err := s.sessionStorage.Revoke(ctx, in.Msg.SessionId); err != nil {
		s.logger.Error("failed to revoke session", "error", err, "procedure", "RevokeSession", "user_id", user.Id, "session_id", in.Msg.SessionId)
		return nil, connErr.FromDB(err)
	}
	if s.auditLogDB != nil {
		_ = s.auditLogDB.Create(ctx, &user.Id, v1.AuditEventType_AUDIT_EVENT_TYPE_SESSION_REVOKED, "", "", map[string]any{"session_id": in.Msg.SessionId})
	}

	s.logger.Info("session revoked", "procedure", "RevokeSession", "user_id", user.Id, "session_id", in.Msg.SessionId)
	return &connect.Response[v1.RevokeSessionResponse]{Msg: &v1.RevokeSessionResponse{}}, nil
}

func (s *Server) RevokeAllOtherSessions(ctx context.Context, in *connect.Request[v1.RevokeAllOtherSessionsRequest]) (*connect.Response[v1.RevokeAllOtherSessionsResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "RevokeAllOtherSessions")
		return nil, connErr.Unauthenticated("not authenticated")
	}

	rawToken, _ := ctx.Value(constants.ContextKeyAuthorization).(string)
	currentID := ""
	if rawToken != "" {
		tokenHash := utilscrypto.HashToken(rawToken)
		if sess, err := s.sessionStorage.GetByTokenHash(ctx, tokenHash); err == nil {
			currentID = sess.ID
		}
	}

	if err := s.sessionStorage.RevokeAllForUser(ctx, user.Id, currentID); err != nil {
		s.logger.Error("failed to revoke all other sessions", "error", err, "procedure", "RevokeAllOtherSessions", "user_id", user.Id)
		return nil, connErr.FromDB(err)
	}
	if s.auditLogDB != nil {
		_ = s.auditLogDB.Create(ctx, &user.Id, v1.AuditEventType_AUDIT_EVENT_TYPE_SESSION_REVOKED, "", "", map[string]any{"scope": "all_other"})
	}

	s.logger.Info("all other sessions revoked", "procedure", "RevokeAllOtherSessions", "user_id", user.Id)
	return &connect.Response[v1.RevokeAllOtherSessionsResponse]{Msg: &v1.RevokeAllOtherSessionsResponse{}}, nil
}
