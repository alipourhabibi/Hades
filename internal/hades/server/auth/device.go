package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net"
	"strings"
	"time"

	"connectrpc.com/connect"

	v1 "github.com/alipourhabibi/Hades/api/gen/api/auth/v1"
	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/devicegrant"
	"github.com/alipourhabibi/Hades/utils/connerr"
	utilscrypto "github.com/alipourhabibi/Hades/utils/crypto"
)

const (
	deviceCodeExpiry    = 15 * time.Minute
	pollIntervalSeconds = 5
	// devicePath is appended to the configured registry host to build the URL
	// the user is told to visit. It used to be a hardcoded loopback address
	// over cleartext, which every device-flow client was then instructed to
	// open.
	devicePath = "/device"
)

// verificationURL builds the address the approving user is sent to.
func (s *Server) verificationURL() string {
	host := s.registryHost
	if host == "" {
		// No configured host is a misconfiguration rather than a mode: say so
		// with a relative path instead of inventing a loopback address that is
		// wrong for every caller but the operator's own machine.
		return devicePath
	}
	return "https://" + host + devicePath
}

// deviceTokenScopes are the scopes granted to a PAT minted by the device flow.
// An empty scope list means unrestricted, which is too much for a credential
// created by typing a code into a browser, so the token is limited to what the
// buf CLI actually needs: reading modules and pushing to existing ones.
// Creating or updating modules stays an interactive operation.
var deviceTokenScopes = []string{
	string(constants.ResourceModule) + ":" + string(constants.ActionRead),
	string(constants.ResourceModule) + ":" + string(constants.ActionPush),
}

// generateUserCode returns an eight-character code in XXXX-XXXX form.
//
// The alphabet has 36 characters and a byte has 256 values, so folding a raw
// byte with `% 36` makes the first four letters of the alphabet appear about
// 14% more often than the rest. On a code this short that is entropy given away
// for nothing, so the draw is uniform: crypto/rand.Int over the alphabet
// length has no modulo bias by construction.
func generateUserCode() (string, error) {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var sb strings.Builder
	for i := 0; i < 8; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			return "", err
		}
		sb.WriteByte(chars[n.Int64()])
		if i == 3 {
			sb.WriteByte('-')
		}
	}
	return sb.String(), nil
}

func (s *Server) RequestDeviceCode(ctx context.Context, in *connect.Request[v1.RequestDeviceCodeRequest]) (*connect.Response[v1.RequestDeviceCodeResponse], error) {
	rawDevice, deviceHash, err := utilscrypto.GenerateToken("")
	if err != nil {
		s.logger.Error("failed to generate device code", "error", err, "procedure", "RequestDeviceCode")
		return nil, connerr.Internal("failed to generate device code")
	}
	userCode, err := generateUserCode()
	if err != nil {
		s.logger.Error("failed to generate user code", "error", err, "procedure", "RequestDeviceCode")
		return nil, connerr.Internal("failed to generate user code")
	}

	expiresAt := time.Now().Add(deviceCodeExpiry)
	if _, err := s.deviceGrantDB.Create(ctx, deviceHash, userCode, expiresAt); err != nil {
		s.logger.Error("failed to create device grant", "error", err, "procedure", "RequestDeviceCode")
		return nil, connerr.FromDB(err)
	}

	return &connect.Response[v1.RequestDeviceCodeResponse]{
		Msg: &v1.RequestDeviceCodeResponse{
			DeviceCode:          rawDevice,
			UserCode:            userCode,
			VerificationUrl:     s.verificationURL(),
			ExpiresInSeconds:    int32(deviceCodeExpiry.Seconds()),
			PollIntervalSeconds: pollIntervalSeconds,
		},
	}, nil
}

func (s *Server) PollDeviceToken(ctx context.Context, in *connect.Request[v1.PollDeviceTokenRequest]) (*connect.Response[v1.PollDeviceTokenResponse], error) {
	host, _, err := net.SplitHostPort(in.Peer().Addr)
	if err != nil {
		host = in.Peer().Addr
	}
	if err := s.enforceLimit(ctx, fmt.Sprintf("devpoll:ip:%s", host), 20, time.Minute, "PollDeviceToken"); err != nil {
		return nil, err
	}

	deviceHash := utilscrypto.HashToken(in.Msg.DeviceCode)
	grant, err := s.deviceGrantDB.GetByDeviceCodeHash(ctx, deviceHash)
	if err != nil {
		return nil, connerr.NotFound("invalid device code")
	}
	if time.Now().After(grant.ExpiresAt) {
		return nil, connerr.InvalidArgument("device code expired")
	}
	if grant.ApprovedAt == nil || grant.UserID == nil {
		return &connect.Response[v1.PollDeviceTokenResponse]{
			Msg: &v1.PollDeviceTokenResponse{Pending: true},
		}, nil
	}
	if grant.APITokenID != nil {
		// A device code is single-use. Returning the literal string
		// "already_issued" in the token field, which is what this did, hands
		// the client something it will try to authenticate with.
		return nil, connerr.FailedPrecondition("a token has already been issued for this device code")
	}
	fullToken, prefix, tokenHash, err := utilscrypto.GenerateAPIToken()
	if err != nil {
		s.logger.Error("failed to generate token", "error", err, "procedure", "PollDeviceToken")
		return nil, connerr.Internal("failed to generate token")
	}

	// The token insert and the grant update are one unit of work, and the
	// update only matches a grant with no token yet. Two polls that interleave
	// therefore cannot both mint a token: the loser's insert rolls back with
	// the failed update rather than leaving a second live credential behind.
	userID := *grant.UserID
	_, err = s.uow.Do(ctx, func(txCtx context.Context) (interface{}, error) {
		tokenRow, err := s.apiTokenDB.Create(txCtx, userID, "device-flow", prefix, tokenHash, deviceTokenScopes, nil)
		if err != nil {
			return nil, connerr.FromDB(err)
		}
		if err := s.deviceGrantDB.AttachToken(txCtx, grant.ID, tokenRow.ID); err != nil {
			return nil, err
		}
		return nil, nil
	}, 15*time.Second)
	if err != nil {
		if errors.Is(err, devicegrant.ErrTokenAlreadyIssued) {
			return nil, connerr.FailedPrecondition("a token has already been issued for this device code")
		}
		s.logger.Error("failed to issue device flow token", "error", err, "procedure", "PollDeviceToken", "user_id", userID)
		return nil, connerr.FromDB(err)
	}

	if s.auditLogDB != nil {
		_ = s.auditLogDB.Create(ctx, &userID, v1.AuditEventType_AUDIT_EVENT_TYPE_API_TOKEN_CREATED, host, "",
			map[string]any{"source": "device_flow", "scopes": deviceTokenScopes})
	}

	return &connect.Response[v1.PollDeviceTokenResponse]{
		Msg: &v1.PollDeviceTokenResponse{Token: fullToken},
	}, nil
}

func (s *Server) ApproveDeviceGrant(ctx context.Context, in *connect.Request[v1.ApproveDeviceGrantRequest]) (*connect.Response[v1.ApproveDeviceGrantResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "ApproveDeviceGrant")
		return nil, connerr.Unauthenticated("not authenticated")
	}

	// A user code is short and human-readable by design, so the lookup is a
	// guessing target and needs a bound of its own.
	if err := s.enforceLimit(ctx, "devapprove:user:"+user.Id, 10, time.Minute, "ApproveDeviceGrant"); err != nil {
		return nil, err
	}

	grant, err := s.deviceGrantDB.GetByUserCode(ctx, in.Msg.UserCode)
	if err != nil {
		s.logger.Warn("invalid user code", "procedure", "ApproveDeviceGrant", "user_id", user.Id)
		return nil, connerr.NotFound("invalid user code")
	}
	if time.Now().After(grant.ExpiresAt) {
		return nil, connerr.InvalidArgument("device code expired")
	}
	// Approval is once-only. Without this a second caller who learns the user
	// code can re-approve a grant that has already been issued and cause the
	// polling device to mint a token bound to their account instead.
	if grant.ApprovedAt != nil {
		s.logger.Warn("refused re-approval of an approved device grant",
			"procedure", "ApproveDeviceGrant", "user_id", user.Id, "grant_id", grant.ID)
		return nil, connerr.FailedPrecondition("this device code has already been approved")
	}

	if err := s.deviceGrantDB.Approve(ctx, grant.ID, user.Id); err != nil {
		// The predicate on the UPDATE is the real enforcement; the check above
		// only makes the common case a clean error rather than a race.
		if errors.Is(err, devicegrant.ErrAlreadyApproved) {
			return nil, connerr.FailedPrecondition("this device code has already been approved")
		}
		s.logger.Error("failed to approve device grant", "error", err, "procedure", "ApproveDeviceGrant", "user_id", user.Id)
		return nil, connerr.FromDB(err)
	}
	s.logger.Info("device grant approved", "procedure", "ApproveDeviceGrant", "user_id", user.Id)
	if s.auditLogDB != nil {
		_ = s.auditLogDB.Create(ctx, &user.Id, v1.AuditEventType_AUDIT_EVENT_TYPE_API_TOKEN_CREATED, "", "",
			map[string]any{"source": "device_flow_approval", "scopes": deviceTokenScopes})
	}
	return &connect.Response[v1.ApproveDeviceGrantResponse]{Msg: &v1.ApproveDeviceGrantResponse{}}, nil
}
