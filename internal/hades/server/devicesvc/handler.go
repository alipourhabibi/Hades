// Package devicesvc implements the DeviceService ConnectRPC handler for the
// OAuth 2.0 Device Authorization Grant flow (RFC 8628). The flow lets CLI
// tools obtain API tokens without opening a browser on the device itself.
package devicesvc

import (
	"context"
	"crypto/rand"
	"fmt"
	"net"
	"strings"
	"time"

	"connectrpc.com/connect"

	v1 "github.com/alipourhabibi/Hades/api/gen/api/authentication/v1"
	v1connect "github.com/alipourhabibi/Hades/api/gen/api/authentication/v1/authenticationv1connect"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/apitoken"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/devicegrant"
	utilscrypto "github.com/alipourhabibi/Hades/utils/crypto"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/alipourhabibi/Hades/utils/log"
	"github.com/alipourhabibi/Hades/internal/hades/cache"
	"github.com/google/uuid"
)

const (
	deviceCodeExpiry    = 15 * time.Minute
	pollIntervalSeconds = 5
	verificationURL     = "http://localhost:50051/device"
)

type Handler struct {
	v1connect.DeviceServiceHandler

	logger        *log.LoggerWrapper
	deviceGrantDB devicegrant.Storage
	apiTokenDB    apitoken.Storage
	cache         cache.Cache
}

func NewHandler(deps *server.Dependencies) *Handler {
	return &Handler{
		logger:        deps.Logger,
		deviceGrantDB: deps.DeviceGrantDB,
		apiTokenDB:    deps.APITokenDB,
		cache:         deps.Cache,
	}
}

func generateUserCode() (string, error) {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	var sb strings.Builder
	for i, v := range b {
		sb.WriteByte(chars[int(v)%len(chars)])
		if i == 3 {
			sb.WriteByte('-')
		}
	}
	return sb.String(), nil
}

func (h *Handler) RequestDeviceCode(ctx context.Context, in *connect.Request[v1.RequestDeviceCodeRequest]) (*connect.Response[v1.RequestDeviceCodeResponse], error) {
	rawDevice, deviceHash, err := utilscrypto.GenerateToken()
	if err != nil {
		h.logger.Error("failed to generate device code", "error", err, "procedure", "RequestDeviceCode")
		return nil, connErr.Internal("failed to generate device code")
	}
	userCode, err := generateUserCode()
	if err != nil {
		h.logger.Error("failed to generate user code", "error", err, "procedure", "RequestDeviceCode")
		return nil, connErr.Internal("failed to generate user code")
	}

	expiresAt := time.Now().Add(deviceCodeExpiry)
	if _, err := h.deviceGrantDB.Create(ctx, deviceHash, userCode, expiresAt); err != nil {
		h.logger.Error("failed to create device grant", "error", err, "procedure", "RequestDeviceCode")
		return nil, connErr.FromPgx(err)
	}

	return &connect.Response[v1.RequestDeviceCodeResponse]{
		Msg: &v1.RequestDeviceCodeResponse{
			DeviceCode:          rawDevice,
			UserCode:            userCode,
			VerificationUrl:     verificationURL,
			ExpiresInSeconds:    int32(deviceCodeExpiry.Seconds()),
			PollIntervalSeconds: pollIntervalSeconds,
		},
	}, nil
}

func (h *Handler) PollDeviceToken(ctx context.Context, in *connect.Request[v1.PollDeviceTokenRequest]) (*connect.Response[v1.PollDeviceTokenResponse], error) {
	host, _, err := net.SplitHostPort(in.Peer().Addr)
	if err != nil {
		host = in.Peer().Addr
	}
	if h.cache != nil {
		allowed, err := h.cache.Allow(ctx, fmt.Sprintf("devpoll:ip:%s", host), 20, time.Minute)
		if err == nil && !allowed {
			return nil, connErr.ResourceExhausted("too many requests")
		}
	}

	deviceHash := utilscrypto.HashToken(in.Msg.DeviceCode)
	grant, err := h.deviceGrantDB.GetByDeviceCodeHash(ctx, deviceHash)
	if err != nil {
		return nil, connErr.NotFound("invalid device code")
	}
	if time.Now().After(grant.ExpiresAt) {
		return nil, connErr.InvalidArgument("device code expired")
	}
	if grant.ApprovedAt == nil || grant.UserID == nil {
		return &connect.Response[v1.PollDeviceTokenResponse]{
			Msg: &v1.PollDeviceTokenResponse{Pending: true},
		}, nil
	}
	// Grant was approved - create an API token for the user.
	if grant.APITokenID != nil {
		// Token was already created.
		return &connect.Response[v1.PollDeviceTokenResponse]{
			Msg: &v1.PollDeviceTokenResponse{Token: "already_issued"},
		}, nil
	}
	raw, hash, err := utilscrypto.GenerateToken()
	if err != nil {
		h.logger.Error("failed to generate token", "error", err, "procedure", "PollDeviceToken")
		return nil, connErr.Internal("failed to generate token")
	}
	prefix := fmt.Sprintf("hades1_%s", raw[:8])
	fullToken := prefix + "_" + raw
	tokenID, err := h.apiTokenDB.Create(ctx, *grant.UserID, "device-flow", prefix, hash, nil, nil)
	if err != nil {
		h.logger.Error("failed to create API token for device flow", "error", err, "procedure", "PollDeviceToken")
		return nil, connErr.FromPgx(err)
	}
	if err := h.deviceGrantDB.Approve(ctx, grant.ID, *grant.UserID, &tokenID); err != nil {
		h.logger.Error("failed to approve device grant", "error", err, "procedure", "PollDeviceToken")
		return nil, connErr.FromPgx(err)
	}
	return &connect.Response[v1.PollDeviceTokenResponse]{
		Msg: &v1.PollDeviceTokenResponse{Token: fullToken},
	}, nil
}

func (h *Handler) ApproveDeviceGrant(ctx context.Context, in *connect.Request[v1.ApproveDeviceGrantRequest]) (*connect.Response[v1.ApproveDeviceGrantResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*registryv1.User)
	if !ok {
		h.logger.Error("missing user in context", "procedure", "ApproveDeviceGrant")
		return nil, connErr.Internal("missing user in context")
	}

	grant, err := h.deviceGrantDB.GetByUserCode(ctx, in.Msg.UserCode)
	if err != nil {
		h.logger.Warn("invalid user code", "procedure", "ApproveDeviceGrant", "user_id", user.Id)
		return nil, connErr.NotFound("invalid user code")
	}
	if time.Now().After(grant.ExpiresAt) {
		return nil, connErr.InvalidArgument("device code expired")
	}
	if err := h.deviceGrantDB.Approve(ctx, grant.ID, user.Id, (*uuid.UUID)(nil)); err != nil {
		h.logger.Error("failed to approve device grant", "error", err, "procedure", "ApproveDeviceGrant", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
	}
	h.logger.Info("device grant approved", "procedure", "ApproveDeviceGrant", "user_id", user.Id)
	return &connect.Response[v1.ApproveDeviceGrantResponse]{Msg: &v1.ApproveDeviceGrantResponse{}}, nil
}
