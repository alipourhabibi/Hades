package authorization

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect"
	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	sessiondb "github.com/alipourhabibi/Hades/internal/hades/storage/db/session"
	utilscrypto "github.com/alipourhabibi/Hades/utils/crypto"
	"github.com/jackc/pgx/v5"
)

// sessionTouchInterval is the minimum time between two last_activity_at writes
// for the same session. Without it every authenticated request would issue a
// write; with it an idle-timeout window is still refreshed accurately enough.
const sessionTouchInterval = 5 * time.Minute

// noAuthProcedures lists Connect-RPC procedures that are always reachable
// without a valid bearer token, and where no user context is ever set.
var noAuthProcedures = map[string]bool{
	// Auth flow - no session exists yet
	"/hades.api.auth.v1.AuthenticationService/Login":                true,
	"/hades.api.auth.v1.AuthenticationService/Register":             true,
	"/hades.api.auth.v1.AuthenticationService/Signin":               true,
	"/hades.api.auth.v1.AuthenticationService/VerifyEmail":          true,
	"/hades.api.auth.v1.AuthenticationService/RequestPasswordReset": true,
	"/hades.api.auth.v1.AuthenticationService/ResetPassword":        true,
	// Device/OAuth flows - begin before a session exists
	"/hades.api.auth.v1.DeviceService/RequestDeviceCode": true,
	"/hades.api.auth.v1.DeviceService/PollDeviceToken":   true,
	"/hades.api.auth.v1.OAuthService/GetOAuthURL":        true,
	"/hades.api.auth.v1.OAuthService/OAuthCallback":      true,
	// Org data is always public - no user context needed
	"/hades.api.identity.v1.OrgService/GetOrg":         true,
	"/hades.api.identity.v1.OrgService/ListOrgMembers": true,
}

// optionalAuthProcedures lists read-only procedures that serve both public
// and private resources. When no Authorization header is present the request
// proceeds as anonymous (no user in context) and only public resources are
// returned. When a header is present it is validated normally; an invalid
// token still returns Unauthenticated.
var optionalAuthProcedures = map[string]bool{
	// Internal Hades registry reads
	"/hades.api.registry.v1.ModuleService/ListModules":     true,
	"/hades.api.registry.v1.ModuleService/GetModule":       true,
	"/hades.api.registry.v1.CommitService/ListCommits":     true,
	"/hades.api.registry.v1.CommitService/GetCommit":       true,
	"/hades.api.registry.v1.CommitService/GetCommitDiff":   true,
	"/hades.api.registry.v1.CommitService/ListModuleFiles": true,
	"/hades.api.registry.v1.CommitService/GetFileContent":  true,
	"/hades.api.registry.v1.CIService/GetCIRun":            true,
	"/hades.api.registry.v1.SDKService/ListSDKs":           true,
	"/hades.api.identity.v1.UserService/GetUser":           true,
	"/hades.api.identity.v1.UserService/ListUsers":         true,
	"/hades.api.identity.v1.OrgService/ListOrganizations":  true,
	"/hades.api.identity.v1.OrgService/GetUserOrgs":        true,
	// buf.build registry protocol reads (buf CLI: dep update, build, export).
	// Each handler resolves the caller as a possibly-nil user and runs
	// CheckReadAccess per module, so anonymous callers see public modules and
	// private ones come back as NotFound.
	"/buf.registry.module.v1.ModuleService/GetModules":  true,
	"/buf.registry.module.v1.ModuleService/ListModules": true,
	"/buf.registry.module.v1.CommitService/GetCommits":  true,
	"/buf.registry.module.v1.CommitService/ListCommits": true,
	"/buf.registry.module.v1.GraphService/GetGraph":     true,
	"/buf.registry.module.v1.DownloadService/Download":  true,
}

// totpPendingAllowed may be called when totp_verified = false.
// Logout is included so a session stuck at the TOTP prompt can still be ended
// by the user instead of lingering until it expires.
var totpPendingAllowed = map[string]bool{
	"/hades.api.auth.v1.TOTPService/VerifyTOTP":       true,
	"/hades.api.auth.v1.AuthenticationService/Logout": true,
}

// emailUnverifiedAllowed may be called before email_verified_at is set.
var emailUnverifiedAllowed = map[string]bool{
	"/hades.api.auth.v1.AuthenticationService/ResendVerificationEmail": true,
	"/hades.api.auth.v1.AuthenticationService/Logout":                  true,
}

// sessionOnlyProcedures require an interactive session (hds_sess_ token).
// PATs are rejected because these operations affect account security state and
// PATs are higher-risk credentials (CI logs, committed scripts, etc.).
//
// The credential-management procedures are listed here specifically to stop a
// leaked PAT from minting a fresh PAT with wider scopes, which would survive
// revocation of the original.
var sessionOnlyProcedures = map[string]bool{
	"/hades.api.auth.v1.AuthenticationService/ChangePassword":  true,
	"/hades.api.auth.v1.AuthenticationService/Logout":          true,
	"/hades.api.auth.v1.TOTPService/BeginEnrollTOTP":           true,
	"/hades.api.auth.v1.TOTPService/ConfirmEnrollTOTP":         true,
	"/hades.api.auth.v1.TOTPService/DisableTOTP":               true,
	"/hades.api.auth.v1.TOTPService/VerifyTOTP":                true,
	"/hades.api.auth.v1.TOTPService/ListBackupCodes":           true,
	"/hades.api.auth.v1.TOTPService/RegenerateBackupCodes":     true,
	"/hades.api.auth.v1.SessionService/ListSessions":           true,
	"/hades.api.auth.v1.SessionService/RevokeSession":          true,
	"/hades.api.auth.v1.SessionService/RevokeAllOtherSessions": true,
	"/hades.api.auth.v1.APITokenService/CreateAPIToken":        true,
	"/hades.api.auth.v1.APITokenService/ListAPITokens":         true,
	"/hades.api.auth.v1.APITokenService/RevokeAPIToken":        true,
	// Approving a device grant issues a fresh PAT, so a PAT must not be able to
	// drive it: that would be an indirect way to mint a new credential.
	"/hades.api.auth.v1.DeviceService/ApproveDeviceGrant": true,
	"/hades.api.auth.v1.OAuthService/ListLinkedProviders": true,
	"/hades.api.auth.v1.OAuthService/UnlinkProvider":      true,
	"/hades.api.auth.v1.AuditService/ListAuditLog":        true,
}

// patOnlyProcedures require a PAT (hades1_ token).
// These are buf CLI protocol endpoints that no browser UI ever calls (the web
// frontend uses the hades.api.registry.v1 services instead). Rejecting session
// tokens here prevents a stolen browser session from being used to push proto
// files or to impersonate the CLI login handshake.
//
// Only writes and the login handshake are listed. The buf read procedures stay
// in optionalAuthProcedures: they enforce module visibility per module in the
// handler, so anonymous `buf dep update` against public modules keeps working.
var patOnlyProcedures = map[string]bool{
	"/buf.registry.module.v1.UploadService/Upload":             true,
	"/buf.alpha.registry.v1alpha1.AuthnService/GetCurrentUser": true,
}

// isNotFound reports whether err is a "row not found" from the PostgreSQL
// (pgx.ErrNoRows) or SQLite (sql.ErrNoRows) driver.
func isNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows)
}

func (s *Server) NewAuthorizationInterceptor() connect.UnaryInterceptorFunc {
	interceptor := func(next connect.UnaryFunc) connect.UnaryFunc {
		return connect.UnaryFunc(func(
			ctx context.Context,
			req connect.AnyRequest,
		) (connect.AnyResponse, error) {
			procedure := req.Spec().Procedure

			s.logger.Debug("auth interceptor", "procedure", procedure)

			if noAuthProcedures[procedure] {
				return next(ctx, req)
			}

			authHeader := req.Header().Get("Authorization")

			// Optional-auth: no header means anonymous (public resources only).
			// A header IS present must be valid; an invalid token is still rejected.
			if optionalAuthProcedures[procedure] && authHeader == "" {
				return next(ctx, req)
			}

			if authHeader == "" {
				s.logger.Debug("auth interceptor: no authorization header", "procedure", procedure)
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authorization header is required"))
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				s.logger.Debug("auth interceptor: invalid authorization header format", "procedure", procedure)
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authorization header is invalid"))
			}
			rawToken := strings.TrimSpace(parts[1])

			switch {
			case strings.HasPrefix(rawToken, utilscrypto.SessionTokenPrefix):
				if patOnlyProcedures[procedure] {
					return nil, connect.NewError(connect.CodePermissionDenied,
						errors.New("this operation requires an API token, not an interactive session"))
				}
				return s.authenticateSession(ctx, req, next, rawToken, procedure)

			case strings.HasPrefix(rawToken, utilscrypto.APITokenPrefix):
				if sessionOnlyProcedures[procedure] {
					return nil, connect.NewError(connect.CodePermissionDenied,
						errors.New("this operation requires an interactive session, not an API token"))
				}
				return s.authenticateAPIToken(ctx, req, next, rawToken, procedure)

			default:
				s.logger.Debug("auth interceptor: unrecognised token format", "procedure", procedure)
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid token format"))
			}
		})
	}
	return connect.UnaryInterceptorFunc(interceptor)
}

func (s *Server) authenticateSession(ctx context.Context, req connect.AnyRequest, next connect.UnaryFunc, rawToken, procedure string) (connect.AnyResponse, error) {
	fullUser, err := s.resolveSession(ctx, rawToken, procedure)
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, constants.ContextKeyUser, fullUser)
	ctx = context.WithValue(ctx, constants.ContextKeyAuthorization, rawToken)
	return next(ctx, req)
}

// resolveSession validates a session token and returns its user. procedure is
// used for the email/TOTP exemption lookups and for logging; pass
// externalProcedure from callers that are not Connect procedures.
func (s *Server) resolveSession(ctx context.Context, rawToken, procedure string) (*identityv1.User, error) {
	if s.sessionStorage == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("session store not configured"))
	}
	tokenHash := utilscrypto.HashToken(rawToken)
	sess, err := s.sessionStorage.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		if !isNotFound(err) {
			s.logger.Error("auth interceptor: session store error", "error", err, "procedure", procedure)
			return nil, connect.NewError(connect.CodeUnavailable, errors.New("authentication service unavailable"))
		}
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid token"))
	}
	if err := s.validateSessionChecks(ctx, sess, procedure); err != nil {
		return nil, err
	}
	fullUser, err := s.userStorage.GetByID(ctx, sess.UserID)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("user not found"))
	}
	s.touchSession(sess)
	return fullUser, nil
}

// touchSession slides the idle expiry forward and records activity, at most
// once per sessionTouchInterval. The idle window is derived from the session
// itself (IdleExpiresAt - LastActivityAt), so it keeps whatever length was
// configured when the session was created. The new idle expiry never extends
// past the absolute expiry. Runs in the background: a failed write only costs
// accuracy of the idle timeout, never the request.
func (s *Server) touchSession(sess *sessiondb.SessionRow) {
	now := time.Now()
	if now.Sub(sess.LastActivityAt) < sessionTouchInterval {
		return
	}
	window := sess.IdleExpiresAt.Sub(sess.LastActivityAt)
	if window <= 0 {
		return
	}
	idleExpires := now.Add(window)
	if idleExpires.After(sess.AbsoluteExpiresAt) {
		idleExpires = sess.AbsoluteExpiresAt
	}
	id := sess.ID
	go func() {
		if err := s.sessionStorage.Touch(context.Background(), id, idleExpires); err != nil {
			s.logger.Error("auth interceptor: failed to touch session", "error", err, "session_id", id)
		}
	}()
}

func (s *Server) authenticateAPIToken(ctx context.Context, req connect.AnyRequest, next connect.UnaryFunc, rawToken, procedure string) (connect.AnyResponse, error) {
	fullUser, scopes, err := s.resolveAPIToken(ctx, rawToken, procedure)
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, constants.ContextKeyUser, fullUser)
	ctx = context.WithValue(ctx, constants.ContextKeyAuthorization, rawToken)
	if len(scopes) > 0 {
		ctx = context.WithValue(ctx, constants.ContextKeyTokenScopes, scopes)
	}
	return next(ctx, req)
}

// resolveAPIToken validates a PAT and returns its user and declared scopes.
// An empty scope slice means unrestricted.
func (s *Server) resolveAPIToken(ctx context.Context, rawToken, procedure string) (*identityv1.User, []string, error) {
	if s.apiTokenStorage == nil {
		return nil, nil, connect.NewError(connect.CodeUnauthenticated, errors.New("API token store not configured"))
	}
	tokenHash := utilscrypto.HashToken(rawToken)
	apiTok, err := s.apiTokenStorage.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		if !isNotFound(err) {
			s.logger.Error("auth interceptor: API token store error", "error", err, "procedure", procedure)
			return nil, nil, connect.NewError(connect.CodeUnavailable, errors.New("authentication service unavailable"))
		}
		return nil, nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid token"))
	}
	if apiTok.RevokedAt != nil {
		return nil, nil, connect.NewError(connect.CodeUnauthenticated, errors.New("API token revoked"))
	}
	if apiTok.ExpiresAt != nil && time.Now().After(*apiTok.ExpiresAt) {
		return nil, nil, connect.NewError(connect.CodeUnauthenticated, errors.New("API token expired"))
	}
	go func() { _ = s.apiTokenStorage.UpdateLastUsed(context.Background(), apiTok.ID) }()
	fullUser, err := s.userStorage.GetByID(ctx, apiTok.UserID)
	if err != nil {
		return nil, nil, connect.NewError(connect.CodeUnauthenticated, errors.New("user not found"))
	}
	return fullUser, apiTok.Scopes, nil
}

// externalProcedure is the procedure label used by callers outside the
// Connect interceptor (currently the Go module proxy). It matches no entry in
// the exemption maps, so such callers get the strictest session checks.
const externalProcedure = "(external)"

// UserFromToken validates a raw bearer credential and returns the user it
// belongs to plus the scopes it carries (empty means unrestricted). It applies
// exactly the same rules as the interceptor: prefix routing, revocation and
// expiry checks, and for sessions the email-verified and TOTP gates.
//
// It exists for routes that cannot go through the Connect interceptor, such as
// the Go module proxy, so they do not grow a second, divergent auth path.
func (s *Server) UserFromToken(ctx context.Context, rawToken string) (*identityv1.User, []string, error) {
	switch {
	case strings.HasPrefix(rawToken, utilscrypto.SessionTokenPrefix):
		user, err := s.resolveSession(ctx, rawToken, externalProcedure)
		return user, nil, err
	case strings.HasPrefix(rawToken, utilscrypto.APITokenPrefix):
		return s.resolveAPIToken(ctx, rawToken, externalProcedure)
	default:
		return nil, nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid token format"))
	}
}

// ScopesAllow reports whether scopes permit resource_type:action.
// An empty slice means unrestricted.
func ScopesAllow(scopes []string, resourceType, action string) bool {
	return scopeCovers(scopes, resourceType, action)
}

// validateSessionChecks runs all security checks for a session token. Fails
// closed: any DB error during status lookup rejects the request rather than
// allowing it through.
func (s *Server) validateSessionChecks(ctx context.Context, sess *sessiondb.SessionRow, procedure string) error {
	if sess.RevokedAt != nil {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("session revoked"))
	}
	if time.Now().After(sess.IdleExpiresAt) || time.Now().After(sess.AbsoluteExpiresAt) {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("session expired"))
	}
	af, err := s.userStorage.GetAuthFieldsByID(ctx, sess.UserID)
	if err != nil {
		if !isNotFound(err) {
			s.logger.Error("auth interceptor: user store error", "error", err, "procedure", procedure)
			return connect.NewError(connect.CodeUnavailable, errors.New("authentication service unavailable"))
		}
		return connect.NewError(connect.CodeUnauthenticated, errors.New("could not verify account status"))
	}
	if af.EmailVerifiedAt == nil && !emailUnverifiedAllowed[procedure] {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("email not verified"))
	}
	if !sess.TOTPVerified && !totpPendingAllowed[procedure] && s.totpSecretDB != nil {
		tsRow, tsErr := s.totpSecretDB.GetByUserID(ctx, sess.UserID)
		if tsErr != nil {
			if !isNotFound(tsErr) {
				s.logger.Error("auth interceptor: TOTP store error", "error", tsErr, "procedure", procedure)
				return connect.NewError(connect.CodeUnavailable, errors.New("authentication service unavailable"))
			}
			// No TOTP record: user has not configured TOTP; allow through.
		} else if tsRow.Enabled {
			return connect.NewError(connect.CodeUnauthenticated, errors.New("TOTP verification required"))
		}
	}
	return nil
}
