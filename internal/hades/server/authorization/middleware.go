package authorization

import (
	"context"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	utilscrypto "github.com/alipourhabibi/Hades/utils/crypto"
	utilserr "github.com/alipourhabibi/Hades/utils/errors"
)

// noAuthProcedures lists Connect-RPC procedures that are always reachable
// without a valid bearer token, and where no user context is ever set.
var noAuthProcedures = map[string]bool{
	// Auth flow - no session exists yet
	"/hades.api.authentication.v1.AuthenticationService/Login":                true,
	"/hades.api.authentication.v1.AuthenticationService/Register":             true,
	"/hades.api.authentication.v1.AuthenticationService/Signin":               true,
	"/hades.api.authentication.v1.AuthenticationService/VerifyEmail":          true,
	"/hades.api.authentication.v1.AuthenticationService/RequestPasswordReset": true,
	"/hades.api.authentication.v1.AuthenticationService/ResetPassword":        true,
	// Device/OAuth flows - begin before a session exists
	"/hades.api.authentication.v1.DeviceService/RequestDeviceCode": true,
	"/hades.api.authentication.v1.DeviceService/PollDeviceToken":   true,
	"/hades.api.authentication.v1.OAuthService/GetOAuthURL":        true,
	"/hades.api.authentication.v1.OAuthService/OAuthCallback":      true,
	// Org data is always public - no user context needed
	"/hades.api.registry.v1.OrgService/GetOrg":          true,
	"/hades.api.registry.v1.OrgService/ListOrgMembers":  true,
}

// optionalAuthProcedures lists read-only procedures that serve both public
// and private resources. When no Authorization header is present the request
// proceeds as anonymous (no user in context) and only public resources are
// returned. When a header is present it is validated normally; an invalid
// token still returns Unauthenticated.
var optionalAuthProcedures = map[string]bool{
	// Internal Hades registry reads
	"/hades.api.registry.v1.ModuleService/ListModules": true,
	"/hades.api.registry.v1.ModuleService/GetModule":   true,
	"/hades.api.registry.v1.CommitService/ListCommits":        true,
	"/hades.api.registry.v1.CommitService/GetCommit":          true,
	"/hades.api.registry.v1.DiffService/GetCommitDiff":        true,
	"/hades.api.registry.v1.UserService/GetUser":              true,
	"/hades.api.registry.v1.UserService/ListUsers":            true,
	"/hades.api.registry.v1.OrgService/ListOrganizations":     true,
	"/hades.api.registry.v1.OrgService/GetUserOrgs":           true,
	"/hades.api.registry.v1.TreeService/ListModuleFiles":      true,
	"/hades.api.registry.v1.TreeService/GetFileContent":       true,
	"/hades.api.registry.v1.CIService/GetCIRun":        true,
	"/hades.api.registry.v1.SDKService/ListSDKs":       true,
	// buf.build registry protocol reads (used by the buf CLI)
	"/buf.registry.module.v1.ModuleService/GetModules":  true,
	"/buf.registry.module.v1.ModuleService/ListModules": true,
	"/buf.registry.module.v1.CommitService/GetCommits":  true,
	"/buf.registry.module.v1.CommitService/ListCommits": true,
	"/buf.registry.module.v1.GraphService/GetGraph":     true,
	"/buf.registry.module.v1.DownloadService/Download":  true,
}

// totpPendingAllowed may be called when totp_verified = false.
var totpPendingAllowed = map[string]bool{
	"/hades.api.authentication.v1.TOTPService/VerifyTOTP": true,
}

// emailUnverifiedAllowed may be called before email_verified_at is set.
var emailUnverifiedAllowed = map[string]bool{
	"/hades.api.authentication.v1.AuthenticationService/ResendVerificationEmail": true,
}

func (s *Server) NewAuthorizationInterceptor() connect.UnaryInterceptorFunc {
	interceptor := func(next connect.UnaryFunc) connect.UnaryFunc {
		return connect.UnaryFunc(func(
			ctx context.Context,
			req connect.AnyRequest,
		) (connect.AnyResponse, error) {
			procedure := req.Spec().Procedure

			// No-auth procedures always pass through without touching the header.
			if noAuthProcedures[procedure] {
				return next(ctx, req)
			}

			authHeader := req.Header().Get("Authorization")

			// Optional-auth procedures: no header → anonymous (no user in context).
			// If a header IS present it must be valid; an invalid token is rejected
			// just as for required-auth procedures.
			if optionalAuthProcedures[procedure] && authHeader == "" {
				return next(ctx, req)
			}

			if authHeader == "" {
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authorization header is required"))
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authorization header is invalid"))
			}
			rawToken := parts[1]
			tokenHash := utilscrypto.HashToken(rawToken)

			// Attempt 1: current session token.
			if s.sessionStorage != nil {
				session, err := s.sessionStorage.GetByTokenHash(ctx, tokenHash)
				if err == nil {
					if session.RevokedAt != nil {
						return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("session revoked"))
					}
					if time.Now().After(session.IdleExpiresAt) {
						return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("session expired"))
					}
					if time.Now().After(session.AbsoluteExpiresAt) {
						return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("session expired"))
					}
					// Check email verified.
					af, afErr := s.userStorage.GetAuthFieldsByID(ctx, session.UserID)
					if afErr == nil && af.EmailVerifiedAt == nil && !emailUnverifiedAllowed[procedure] {
						return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("email not verified"))
					}
					// Check TOTP.
					if !session.TOTPVerified && !totpPendingAllowed[procedure] && s.totpSecretDB != nil {
						tsRow, tsErr := s.totpSecretDB.GetByUserID(ctx, session.UserID)
						if tsErr == nil && tsRow.Enabled {
							return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("TOTP verification required"))
						}
					}
					fullUser, err := s.userStorage.GetByID(ctx, session.UserID)
					if err != nil {
						return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("user not found"))
					}
					ctx = context.WithValue(ctx, constants.ContextKeyUser, fullUser)
					ctx = context.WithValue(ctx, constants.ContextKeyAuthorization, rawToken)
					return next(ctx, req)
				}

				// Attempt 2: old token hash within the rotation grace window.
				oldSession, oldErr := s.sessionStorage.GetByOldTokenHash(ctx, tokenHash)
				if oldErr == nil && oldSession.RevokedAt == nil {
					fullUser, err := s.userStorage.GetByID(ctx, oldSession.UserID)
					if err != nil {
						return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("user not found"))
					}
					ctx = context.WithValue(ctx, constants.ContextKeyUser, fullUser)
					ctx = context.WithValue(ctx, constants.ContextKeyAuthorization, rawToken)
					return next(ctx, req)
				}
			}

			// Attempt 3: long-lived API token.
			if s.apiTokenStorage != nil {
				apiTok, err := s.apiTokenStorage.GetByTokenHash(ctx, tokenHash)
				if err == nil {
					if apiTok.RevokedAt != nil {
						return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("API token revoked"))
					}
					if apiTok.ExpiresAt != nil && time.Now().After(*apiTok.ExpiresAt) {
						return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("API token expired"))
					}
					go func() { _ = s.apiTokenStorage.UpdateLastUsed(context.Background(), apiTok.ID) }()
					fullUser, err := s.userStorage.GetByID(ctx, apiTok.UserID)
					if err != nil {
						return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("user not found"))
					}
					ctx = context.WithValue(ctx, constants.ContextKeyUser, fullUser)
					ctx = context.WithValue(ctx, constants.ContextKeyAuthorization, rawToken)
					return next(ctx, req)
				}
			}

			// Legacy fallback: UUID session token (old behaviour before token hashing).
			user, err := s.UserFromSessionID(ctx, rawToken)
			if err != nil {
				return nil, utilserr.ToConnectError(err)
			}
			ctx = context.WithValue(ctx, constants.ContextKeyUser, user)
			ctx = context.WithValue(ctx, constants.ContextKeyAuthorization, rawToken)
			return next(ctx, req)
		})
	}
	return connect.UnaryInterceptorFunc(interceptor)
}
