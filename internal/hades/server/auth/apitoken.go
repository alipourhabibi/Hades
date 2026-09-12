package auth

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/alipourhabibi/Hades/api/gen/api/auth/v1"
	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/apitoken"
	utilscrypto "github.com/alipourhabibi/Hades/utils/crypto"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/google/uuid"
)

func (s *Server) CreateAPIToken(ctx context.Context, in *connect.Request[v1.CreateAPITokenRequest]) (*connect.Response[v1.CreateAPITokenResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "CreateAPIToken")
		return nil, connErr.Unauthenticated("not authenticated")
	}

	raw, _, err := utilscrypto.GenerateToken()
	if err != nil {
		s.logger.Error("failed to generate token", "error", err, "procedure", "CreateAPIToken", "user_id", user.Id)
		return nil, connErr.Internal("failed to generate token")
	}

	prefix := fmt.Sprintf("hades1_%s", raw[:5])
	fullToken := prefix + "_" + raw
	tokenHash := utilscrypto.HashToken(fullToken)

	var expiresAt *time.Time
	if in.Msg.ExpiresAt != nil {
		t := in.Msg.ExpiresAt.AsTime()
		expiresAt = &t
	}

	row, err := s.apiTokenDB.Create(ctx, user.Id, in.Msg.Name, prefix, tokenHash, in.Msg.Scopes, expiresAt)
	if err != nil {
		s.logger.Error("failed to create API token", "error", err, "procedure", "CreateAPIToken", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
	}

	if s.auditLogDB != nil {
		_ = s.auditLogDB.Create(ctx, &user.Id, v1.AuditEventType_AUDIT_EVENT_TYPE_API_TOKEN_CREATED, "", "", map[string]any{"token_id": row.ID.String()})
	}

	s.logger.Info("API token created", "procedure", "CreateAPIToken", "user_id", user.Id, "token_id", row.ID.String())
	return &connect.Response[v1.CreateAPITokenResponse]{
		Msg: &v1.CreateAPITokenResponse{
			Id:        row.ID.String(),
			Token:     fullToken,
			Prefix:    prefix,
			CreatedAt: timestamppb.New(row.CreatedAt),
		},
	}, nil
}

func (s *Server) ListAPITokens(ctx context.Context, in *connect.Request[v1.ListAPITokensRequest]) (*connect.Response[v1.ListAPITokensResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "ListAPITokens")
		return nil, connErr.Unauthenticated("not authenticated")
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

	rows, err := s.apiTokenDB.ListByUserID(ctx, user.Id, pageSize, offset)
	if err != nil {
		s.logger.Error("failed to list API tokens", "error", err, "procedure", "ListAPITokens", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
	}

	now := time.Now()
	tokens := make([]*v1.APIToken, 0, len(rows))
	for _, row := range rows {
		t := &v1.APIToken{
			Id:        row.ID.String(),
			Name:      row.Name,
			Prefix:    row.Prefix,
			Scopes:    row.Scopes,
			CreatedAt: timestamppb.New(row.CreatedAt),
			Status:    apiTokenStatus(row, now),
		}
		if row.LastUsedAt != nil {
			t.LastUsedAt = timestamppb.New(*row.LastUsedAt)
		}
		if row.ExpiresAt != nil {
			t.ExpiresAt = timestamppb.New(*row.ExpiresAt)
		}
		tokens = append(tokens, t)
	}

	nextPageToken := ""
	if len(rows) == pageSize {
		nextPageToken = strconv.Itoa(offset + pageSize)
	}

	return &connect.Response[v1.ListAPITokensResponse]{
		Msg: &v1.ListAPITokensResponse{Tokens: tokens, NextPageToken: nextPageToken},
	}, nil
}

func (s *Server) RevokeAPIToken(ctx context.Context, in *connect.Request[v1.RevokeAPITokenRequest]) (*connect.Response[v1.RevokeAPITokenResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "RevokeAPIToken")
		return nil, connErr.Unauthenticated("not authenticated")
	}

	id, err := uuid.Parse(in.Msg.Id)
	if err != nil {
		s.logger.Warn("invalid token ID", "procedure", "RevokeAPIToken", "user_id", user.Id, "token_id", in.Msg.Id)
		return nil, connErr.InvalidArgument("invalid token ID")
	}

	if err := s.apiTokenDB.RevokeByOwner(ctx, id, user.Id); err != nil {
		if errors.Is(err, apitoken.ErrNotFound) {
			return nil, connErr.NotFound("token not found")
		}
		s.logger.Error("failed to revoke API token", "error", err, "procedure", "RevokeAPIToken", "user_id", user.Id, "token_id", in.Msg.Id)
		return nil, connErr.FromPgx(err)
	}
	if s.auditLogDB != nil {
		_ = s.auditLogDB.Create(ctx, &user.Id, v1.AuditEventType_AUDIT_EVENT_TYPE_API_TOKEN_REVOKED, "", "", map[string]any{"token_id": in.Msg.Id})
	}

	s.logger.Info("API token revoked", "procedure", "RevokeAPIToken", "user_id", user.Id, "token_id", in.Msg.Id)
	return &connect.Response[v1.RevokeAPITokenResponse]{Msg: &v1.RevokeAPITokenResponse{}}, nil
}

func apiTokenStatus(row *apitoken.Row, now time.Time) v1.APITokenStatus {
	if row.RevokedAt != nil {
		return v1.APITokenStatus_API_TOKEN_STATUS_REVOKED
	}
	if row.ExpiresAt != nil && now.After(*row.ExpiresAt) {
		return v1.APITokenStatus_API_TOKEN_STATUS_EXPIRED
	}
	return v1.APITokenStatus_API_TOKEN_STATUS_ACTIVE
}
