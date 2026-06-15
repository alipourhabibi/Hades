// Package apitokensvc implements the APITokenService ConnectRPC handler.
// It manages personal API tokens (create, list, revoke) for authenticated
// users. Tokens are stored as SHA-256 hashes; the plaintext is returned
// only once at creation time.
package apitokensvc

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/alipourhabibi/Hades/api/gen/api/authentication/v1"
	v1connect "github.com/alipourhabibi/Hades/api/gen/api/authentication/v1/authenticationv1connect"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/apitoken"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/auditlog"
	utilscrypto "github.com/alipourhabibi/Hades/utils/crypto"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/alipourhabibi/Hades/utils/log"
	"github.com/google/uuid"
)

type Handler struct {
	v1connect.APITokenServiceHandler

	logger     *log.LoggerWrapper
	apiTokenDB *apitoken.APITokenStorage
	auditLogDB *auditlog.AuditLogStorage
}

func NewHandler(deps *server.Dependencies) *Handler {
	return &Handler{
		logger:     deps.Logger,
		apiTokenDB: deps.APITokenDB,
		auditLogDB: deps.AuditLogDB,
	}
}

func (h *Handler) CreateAPIToken(ctx context.Context, in *connect.Request[v1.CreateAPITokenRequest]) (*connect.Response[v1.CreateAPITokenResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*registryv1.User)
	if !ok {
		h.logger.Error("missing user in context", "procedure", "CreateAPIToken")
		return nil, connErr.Internal("missing user in context")
	}

	raw, hash, err := utilscrypto.GenerateToken()
	if err != nil {
		h.logger.Error("failed to generate token", "error", err, "procedure", "CreateAPIToken", "user_id", user.Id)
		return nil, connErr.Internal("failed to generate token")
	}

	// Token format: "hades1_{first5chars}_{rawtoken}" - prefix lets users
	// identify Hades tokens at a glance without exposing the full value.
	prefix := fmt.Sprintf("hades1_%s", raw[:5])
	fullToken := prefix + "_" + raw

	var expiresAt *time.Time
	if in.Msg.ExpiresAt != nil {
		t := in.Msg.ExpiresAt.AsTime()
		expiresAt = &t
	}

	id, err := h.apiTokenDB.Create(ctx, user.Id, in.Msg.Name, prefix, hash, in.Msg.Scopes, expiresAt)
	if err != nil {
		h.logger.Error("failed to create API token", "error", err, "procedure", "CreateAPIToken", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
	}

	if h.auditLogDB != nil {
		_ = h.auditLogDB.Create(ctx, &user.Id, "api_token_created", "", "", map[string]any{"token_id": id.String()})
	}

	h.logger.Info("API token created", "procedure", "CreateAPIToken", "user_id", user.Id, "token_id", id.String())
	return &connect.Response[v1.CreateAPITokenResponse]{
		Msg: &v1.CreateAPITokenResponse{
			Id:        id.String(),
			Token:     fullToken,
			Prefix:    prefix,
			CreatedAt: timestamppb.Now(),
		},
	}, nil
}

func (h *Handler) ListAPITokens(ctx context.Context, in *connect.Request[v1.ListAPITokensRequest]) (*connect.Response[v1.ListAPITokensResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*registryv1.User)
	if !ok {
		h.logger.Error("missing user in context", "procedure", "ListAPITokens")
		return nil, connErr.Internal("missing user in context")
	}

	rows, err := h.apiTokenDB.ListByUserID(ctx, user.Id)
	if err != nil {
		h.logger.Error("failed to list API tokens", "error", err, "procedure", "ListAPITokens", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
	}

	tokens := make([]*v1.APIToken, 0, len(rows))
	for _, row := range rows {
		t := &v1.APIToken{
			Id:        row.ID.String(),
			Name:      row.Name,
			Prefix:    row.Prefix,
			Scopes:    row.Scopes,
			CreatedAt: timestamppb.New(row.CreatedAt),
		}
		if row.LastUsedAt != nil {
			t.LastUsedAt = timestamppb.New(*row.LastUsedAt)
		}
		if row.ExpiresAt != nil {
			t.ExpiresAt = timestamppb.New(*row.ExpiresAt)
		}
		tokens = append(tokens, t)
	}
	return &connect.Response[v1.ListAPITokensResponse]{
		Msg: &v1.ListAPITokensResponse{Tokens: tokens},
	}, nil
}

func (h *Handler) RevokeAPIToken(ctx context.Context, in *connect.Request[v1.RevokeAPITokenRequest]) (*connect.Response[v1.RevokeAPITokenResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*registryv1.User)
	if !ok {
		h.logger.Error("missing user in context", "procedure", "RevokeAPIToken")
		return nil, connErr.Internal("missing user in context")
	}

	id, err := uuid.Parse(in.Msg.Id)
	if err != nil {
		h.logger.Warn("invalid token ID", "procedure", "RevokeAPIToken", "user_id", user.Id, "token_id", in.Msg.Id)
		return nil, connErr.InvalidArgument("invalid token ID")
	}

	// Verify ownership.
	row, err := h.apiTokenDB.GetByID(ctx, id)
	if err != nil || row.UserID != user.Id {
		h.logger.Warn("token not found or not owned by user", "procedure", "RevokeAPIToken", "user_id", user.Id, "token_id", in.Msg.Id)
		return nil, connErr.NotFound("token not found")
	}

	if err := h.apiTokenDB.Revoke(ctx, id); err != nil {
		h.logger.Error("failed to revoke API token", "error", err, "procedure", "RevokeAPIToken", "user_id", user.Id, "token_id", in.Msg.Id)
		return nil, connErr.FromPgx(err)
	}
	if h.auditLogDB != nil {
		_ = h.auditLogDB.Create(ctx, &user.Id, "api_token_revoked", "", "", map[string]any{"token_id": in.Msg.Id})
	}

	h.logger.Info("API token revoked", "procedure", "RevokeAPIToken", "user_id", user.Id, "token_id", in.Msg.Id)
	return &connect.Response[v1.RevokeAPITokenResponse]{Msg: &v1.RevokeAPITokenResponse{}}, nil
}
