package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/alipourhabibi/Hades/api/gen/api/auth/v1"
	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/apitoken"
	utilscrypto "github.com/alipourhabibi/Hades/utils/crypto"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/google/uuid"
)

const (
	// maxAPITokensPerUser caps how many live tokens one account may hold, so a
	// compromised session cannot mint credentials without bound.
	maxAPITokensPerUser = 50
	// maxAPITokenLifetime caps the requested expiry. A credential that never
	// expires is the one most likely to outlive the access it was granted for.
	maxAPITokenLifetime = 365 * 24 * time.Hour
)

// validateScopes checks a requested scope list.
//
// An empty list is rejected: empty means unrestricted in the enforcement path
// (see authorization.scopeCovers), so a client that simply omits the field
// would otherwise receive a full-authority credential by accident.
func validateScopes(scopes []string) error {
	if len(scopes) == 0 {
		return connErr.InvalidArgument("at least one scope is required; an empty scope list would grant unrestricted access")
	}
	seen := make(map[string]struct{}, len(scopes))
	for _, sc := range scopes {
		if !constants.IsKnownScope(sc) {
			return connErr.InvalidArgument(fmt.Sprintf("unknown scope %q", sc))
		}
		if _, dup := seen[sc]; dup {
			return connErr.InvalidArgument(fmt.Sprintf("duplicate scope %q", sc))
		}
		seen[sc] = struct{}{}
	}
	return nil
}

func (s *Server) CreateAPIToken(ctx context.Context, in *connect.Request[v1.CreateAPITokenRequest]) (*connect.Response[v1.CreateAPITokenResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "CreateAPIToken")
		return nil, connErr.Unauthenticated("not authenticated")
	}

	if err := validateScopes(in.Msg.Scopes); err != nil {
		return nil, err
	}

	var expiresAt *time.Time
	if in.Msg.ExpiresAt != nil {
		t := in.Msg.ExpiresAt.AsTime()
		if !t.After(time.Now()) {
			return nil, connErr.InvalidArgument("expiry must be in the future")
		}
		if t.After(time.Now().Add(maxAPITokenLifetime)) {
			return nil, connErr.InvalidArgument("expiry exceeds the maximum token lifetime")
		}
		expiresAt = &t
	}

	// Count live tokens before minting another one.
	existing, err := s.apiTokenDB.ListByUserID(ctx, user.Id, maxAPITokensPerUser+1, 0)
	if err != nil {
		s.logger.Error("failed to count API tokens", "error", err, "procedure", "CreateAPIToken", "user_id", user.Id)
		return nil, connErr.FromDB(err)
	}
	live := 0
	now := time.Now()
	for _, row := range existing {
		if apiTokenStatus(row, now) == v1.APITokenStatus_API_TOKEN_STATUS_ACTIVE {
			live++
		}
	}
	if live >= maxAPITokensPerUser {
		return nil, connErr.ResourceExhausted("maximum number of active API tokens reached; revoke one first")
	}

	fullToken, prefix, tokenHash, err := utilscrypto.GenerateAPIToken()
	if err != nil {
		s.logger.Error("failed to generate token", "error", err, "procedure", "CreateAPIToken", "user_id", user.Id)
		return nil, connErr.Internal("failed to generate token")
	}

	row, err := s.apiTokenDB.Create(ctx, user.Id, in.Msg.Name, prefix, tokenHash, in.Msg.Scopes, expiresAt)
	if err != nil {
		s.logger.Error("failed to create API token", "error", err, "procedure", "CreateAPIToken", "user_id", user.Id)
		return nil, connErr.FromDB(err)
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

	pageSize, offset := server.Page(in.Msg.PageSize, in.Msg.PageToken)

	rows, err := s.apiTokenDB.ListByUserID(ctx, user.Id, pageSize, offset)
	if err != nil {
		s.logger.Error("failed to list API tokens", "error", err, "procedure", "ListAPITokens", "user_id", user.Id)
		return nil, connErr.FromDB(err)
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

	nextPageToken := server.NextPageToken(len(rows), pageSize, offset)

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
		return nil, connErr.FromDB(err)
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
