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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// sessionTouchInterval is the minimum time between two last_activity_at writes
// for the same session. Without it every authenticated request would issue a
// write; with it an idle-timeout window is still refreshed accurately enough.
const sessionTouchInterval = 5 * time.Minute

// backgroundWriteTimeout bounds the bookkeeping writes the interceptor makes
// (session touch, token last-used). They are not worth delaying a request for.
const backgroundWriteTimeout = 2 * time.Second

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
	"/hades.api.identity.v1.OrgService/ListOrganizations":  true,
	"/hades.api.identity.v1.OrgService/GetUserOrgs":        true,
	// An organisation record is readable without a credential: the name, the
	// description and the module count are what a public module page already
	// discloses through its owner field.
	//
	// It is optional-auth rather than no-auth. noAuthProcedures short-circuits
	// before any header is parsed, so the handler never saw a user even when a
	// valid token was presented, and redactEmails then blanked the caller's own
	// address out of their own organisation's record.
	//
	// ListOrgMembers is deliberately NOT here. The roster is a map of who works
	// where: combined with ListOrganizations it gave an unauthenticated visitor
	// the org chart and a target list for credential attacks, and every module
	// page exposes an owner name, so no guessing was needed. Requiring a
	// credential does not make it secret, it makes it attributable.
	//
	// If public rosters are wanted, that should be an explicit per-organisation
	// visibility field defaulting to private, not the absence of a check.
	"/hades.api.identity.v1.OrgService/GetOrg": true,
	// buf.build registry protocol reads (buf CLI: dep update, build, export).
	// Each handler resolves the caller as a possibly-nil user and runs
	// CheckReadAccess per module, so anonymous callers see public modules and
	// private ones come back as NotFound.
	//
	// ListModules and ListCommits are deliberately absent: neither buf adapter
	// implements them, so an entry here would describe a method that does not
	// exist.
	"/buf.registry.module.v1.ModuleService/GetModules": true,
	"/buf.registry.module.v1.CommitService/GetCommits": true,
	"/buf.registry.module.v1.GraphService/GetGraph":    true,
	"/buf.registry.module.v1.DownloadService/Download": true,
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

// authInterceptor implements connect.Interceptor.
//
// It is not built with connect.UnaryInterceptorFunc, whose WrapStreamingHandler
// is a documented no-op. Every interceptor in this server used that helper,
// which meant the first streaming RPC added to any registered service would
// have been unauthenticated by default, with nothing to signal it. Here the
// streaming handler is implemented and refuses.
type authInterceptor struct {
	s *Server
}

func (i authInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return i.s.wrapUnary(next)
}

// WrapStreamingHandler refuses streaming calls outright.
//
// Authenticating a stream is a different problem from authenticating a unary
// call and this server has not solved it. Refusing is the honest answer;
// silently serving the stream with no credential check is what the no-op
// default would have done. Remove this once streaming auth exists.
func (i authInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		i.s.logger.Error("auth interceptor: refused a streaming call; streaming authentication is not implemented",
			"procedure", conn.Spec().Procedure)
		return connect.NewError(connect.CodeUnimplemented,
			errors.New("streaming procedures are not supported by this server"))
	}
}

func (i authInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

// NewAuthorizationInterceptor returns the authentication and authorization
// interceptor for every registered handler.
func (s *Server) NewAuthorizationInterceptor() connect.Interceptor {
	return authInterceptor{s: s}
}

func (s *Server) wrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	{
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

			if err := s.limitBearerAttempts(ctx, req.Peer().Addr, procedure); err != nil {
				return nil, err
			}

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
}

// bearerAttemptLimit and bearerAttemptWindow bound how many bearer credentials
// one peer may present. Each presentation costs a token-hash lookup, and on
// SQLite that lookup had no index until the catch-up migration, so an
// unbounded replay was a cheap way to make the database do work.
const (
	bearerAttemptLimit  = 300
	bearerAttemptWindow = time.Minute
)

// limitBearerAttempts bounds credential presentation per peer.
//
// Unlike the login and TOTP limiters in the auth handlers, this one fails
// OPEN: it sits in front of every authenticated request in the server, so
// refusing on a cache outage would take the whole API down rather than
// degrading one endpoint. The tradeoff is defensible here and not there
// because the secret it guards is a 256-bit token rather than a password or a
// six-digit code: this limit exists to bound work, not to bound guessing.
// See internal/hades/server/auth/ratelimit.go and docs2/adr/010.
func (s *Server) limitBearerAttempts(ctx context.Context, peer, procedure string) error {
	if s.cache == nil {
		return nil
	}
	allowed, err := s.cache.Allow(ctx, "bearer:peer:"+peer, bearerAttemptLimit, bearerAttemptWindow)
	if err != nil {
		s.logger.Error("auth interceptor: bearer limiter unavailable, allowing request",
			"error", err, "procedure", procedure)
		return nil
	}
	if !allowed {
		s.logger.Warn("auth interceptor: bearer attempt limit exceeded", "procedure", procedure, "peer", peer)
		return connect.NewError(connect.CodeResourceExhausted, errors.New("too many requests"))
	}
	return nil
}

func (s *Server) authenticateSession(ctx context.Context, req connect.AnyRequest, next connect.UnaryFunc, rawToken, procedure string) (connect.AnyResponse, error) {
	fullUser, err := s.resolveSession(ctx, rawToken, procedure)
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, constants.ContextKeyUser, fullUser)
	ctx = context.WithValue(ctx, constants.ContextKeyAuthorization, rawToken)
	// An interactive session carries no scope restriction. Stating it here is
	// the point: the previous code left the value unset, and the scope check
	// read "absent" as "unrestricted", so any future path that lost the value
	// would have been granted full authority by the same rule.
	ctx = context.WithValue(ctx, constants.ContextKeyTokenScopes, UnrestrictedScopes())
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
		// A database failure is not "the user does not exist". Reporting one as
		// the other told every caller their credential was bad during an
		// outage, and did so with a different code than the same failure
		// produces elsewhere in this file.
		if !isNotFound(err) {
			s.logger.Error("auth interceptor: user store error", "error", err, "procedure", procedure)
			return nil, connect.NewError(connect.CodeUnavailable, errors.New("authentication service unavailable"))
		}
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid token"))
	}
	if err := s.checkUserState(fullUser, procedure); err != nil {
		return nil, err
	}
	s.touchSession(ctx, sess)
	return fullUser, nil
}

// touchSession slides the idle expiry forward and records activity, at most
// once per sessionTouchInterval. The idle window is derived from the session
// itself (IdleExpiresAt - LastActivityAt), so it keeps whatever length was
// configured when the session was created. The new idle expiry never extends
// past the absolute expiry. Runs in the background: a failed write only costs
// accuracy of the idle timeout, never the request.
func (s *Server) touchSession(ctx context.Context, sess *sessiondb.SessionRow) {
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
	// Inline with a short timeout rather than a bare goroutine. See
	// recordTokenUse: the goroutine form was unbounded and outlived shutdown,
	// and this write happens at most once per sessionTouchInterval per session
	// so it is not on the hot path in practice.
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), backgroundWriteTimeout)
	defer cancel()
	if err := s.sessionStorage.Touch(writeCtx, sess.ID, idleExpires); err != nil {
		s.logger.Warn("auth interceptor: failed to touch session", "error", err, "session_id", sess.ID)
	}
}

func (s *Server) authenticateAPIToken(ctx context.Context, req connect.AnyRequest, next connect.UnaryFunc, rawToken, procedure string) (connect.AnyResponse, error) {
	fullUser, scopes, err := s.resolveAPIToken(ctx, rawToken, procedure)
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, constants.ContextKeyUser, fullUser)
	ctx = context.WithValue(ctx, constants.ContextKeyAuthorization, rawToken)
	ctx = context.WithValue(ctx, constants.ContextKeyTokenScopes, scopes)
	return next(ctx, req)
}

// scopesFromContext returns the scopes attached by the interceptor.
//
// A context with no scopes at all is an unauthenticated or optional-auth
// request, which the handlers gate on the user being nil; there is no
// credential to restrict, so it is unrestricted here. A context carrying a
// Scopes value always states its own intent.
func scopesFromContext(ctx context.Context) Scopes {
	if sc, ok := ctx.Value(constants.ContextKeyTokenScopes).(Scopes); ok {
		return sc
	}
	return UnrestrictedScopes()
}

// resolveAPIToken validates a PAT and returns its user and declared scopes.
func (s *Server) resolveAPIToken(ctx context.Context, rawToken, procedure string) (*identityv1.User, Scopes, error) {
	if s.apiTokenStorage == nil {
		return nil, Scopes{}, connect.NewError(connect.CodeUnauthenticated, errors.New("API token store not configured"))
	}
	tokenHash := utilscrypto.HashToken(rawToken)
	apiTok, err := s.apiTokenStorage.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		if !isNotFound(err) {
			s.logger.Error("auth interceptor: API token store error", "error", err, "procedure", procedure)
			return nil, Scopes{}, connect.NewError(connect.CodeUnavailable, errors.New("authentication service unavailable"))
		}
		return nil, Scopes{}, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid token"))
	}
	if apiTok.RevokedAt != nil {
		return nil, Scopes{}, connect.NewError(connect.CodeUnauthenticated, errors.New("API token revoked"))
	}
	if apiTok.ExpiresAt != nil && time.Now().After(*apiTok.ExpiresAt) {
		return nil, Scopes{}, connect.NewError(connect.CodeUnauthenticated, errors.New("API token expired"))
	}
	fullUser, err := s.userStorage.GetByID(ctx, apiTok.UserID)
	if err != nil {
		if !isNotFound(err) {
			s.logger.Error("auth interceptor: user store error", "error", err, "procedure", procedure)
			return nil, Scopes{}, connect.NewError(connect.CodeUnavailable, errors.New("authentication service unavailable"))
		}
		return nil, Scopes{}, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid token"))
	}
	if err := s.checkUserState(fullUser, procedure); err != nil {
		return nil, Scopes{}, err
	}
	s.recordTokenUse(ctx, apiTok.ID)
	return fullUser, ScopesFromValues(apiTok.Scopes), nil
}

// recordTokenUse updates last_used_at for a personal access token.
//
// It runs inline with a short timeout rather than in a bare goroutine on
// context.Background(). The goroutine version was unbounded (one per
// authenticated request), uncoordinated with shutdown, and outlived
// srv.Shutdown; a write that only records telemetry is not worth that.
// WithoutCancel keeps the write from being cancelled by a client that
// disconnects mid-request, while the timeout keeps it from delaying one.
func (s *Server) recordTokenUse(ctx context.Context, id uuid.UUID) {
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), backgroundWriteTimeout)
	defer cancel()
	if err := s.apiTokenStorage.UpdateLastUsed(writeCtx, id); err != nil {
		s.logger.Warn("auth interceptor: failed to record API token use", "error", err, "token_id", id)
	}
}

// checkUserState rejects credentials belonging to an account that is not
// active. Without it, deactivating or suspending a user had no effect on their
// existing sessions or personal access tokens: they kept authenticating until
// the credential expired on its own.
func (s *Server) checkUserState(u *identityv1.User, procedure string) error {
	switch u.State {
	case identityv1.UserState_USER_STATE_ACTIVE:
		return nil
	case identityv1.UserState_USER_STATE_UNSPECIFIED:
		// Rows written before the column was populated. Treated as active so an
		// upgrade does not lock every existing account out; the migration that
		// backfills it is what removes this case.
		return nil
	default:
		s.logger.Warn("auth interceptor: rejected credential for inactive account",
			"procedure", procedure, "user_id", u.Id, "state", u.State.String())
		return connect.NewError(connect.CodePermissionDenied, errors.New("account is not active"))
	}
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
func (s *Server) UserFromToken(ctx context.Context, rawToken string) (*identityv1.User, Scopes, error) {
	switch {
	case strings.HasPrefix(rawToken, utilscrypto.SessionTokenPrefix):
		user, err := s.resolveSession(ctx, rawToken, externalProcedure)
		if err != nil {
			return nil, Scopes{}, err
		}
		// An interactive session carries no scope restriction, stated rather
		// than implied by a nil slice.
		return user, UnrestrictedScopes(), nil
	case strings.HasPrefix(rawToken, utilscrypto.APITokenPrefix):
		return s.resolveAPIToken(ctx, rawToken, externalProcedure)
	default:
		return nil, Scopes{}, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid token format"))
	}
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
	if !sess.TOTPVerified && !totpPendingAllowed[procedure] {
		// Second-factor enforcement must not depend on whether an optional
		// builder call was made at wire-up time. A nil store used to skip the
		// whole branch, so removing or reordering that call silently disabled
		// 2FA for every session with no startup error and no log line.
		// Server.Validate refuses to start in that state; this is the
		// belt-and-braces half.
		if s.totpSecretDB == nil {
			s.logger.Error("auth interceptor: TOTP store not configured; refusing session", "procedure", procedure)
			return connect.NewError(connect.CodeUnavailable, errors.New("authentication service unavailable"))
		}
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
