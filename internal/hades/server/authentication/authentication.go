// Package authentication implements the AuthenticationService ConnectRPC
// handler. It covers the full credential lifecycle: registration, login,
// logout, email verification, password reset, and password change. Login
// uses bcrypt with configurable cost and enforces account lockout after
// repeated failures. Session tokens are stored as SHA-256 hashes.
package authentication

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/crypto/bcrypt"

	v1 "github.com/alipourhabibi/Hades/api/gen/api/authentication/v1"
	v1connect "github.com/alipourhabibi/Hades/api/gen/api/authentication/v1/authenticationv1connect"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/server/authorization"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/auditlog"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/emailverification"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/passwordreset"
	dbsession "github.com/alipourhabibi/Hades/internal/hades/storage/db/session"
	dbuser "github.com/alipourhabibi/Hades/internal/hades/storage/db/user"
	"github.com/alipourhabibi/Hades/internal/hades/cache"
	utilscrypto "github.com/alipourhabibi/Hades/utils/crypto"
	"github.com/alipourhabibi/Hades/utils/email"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/alipourhabibi/Hades/utils/log"
)

// dummyHash is bcrypt'd "x" used for constant-time comparison when a user does
// not exist, preventing user-enumeration via timing.
var dummyHash, _ = bcryptHash("x", bcrypt.DefaultCost)

type Server struct {
	v1connect.AuthenticationServiceHandler

	logger               *log.LoggerWrapper
	userStorage          dbuser.Storage
	sessionStorage       dbsession.Storage
	emailVerStorage      emailverification.Storage
	passwordResetStorage passwordreset.Storage
	auditLogStorage      auditlog.Storage
	authorizationService *authorization.Server
	uow                  db.UnitOfWork
	cache                cache.Cache
	emailSender          *email.Sender
	authCfg              config.AuthConfig
	registryHost         string
}

func NewServer(deps *server.Dependencies) *Server {
	return &Server{
		logger:               deps.Logger,
		userStorage:          deps.UserDB,
		sessionStorage:       deps.SessionDB,
		emailVerStorage:      deps.EmailVerificationDB,
		passwordResetStorage: deps.PasswordResetDB,
		auditLogStorage:      deps.AuditLogDB,
		authorizationService: deps.Authorization,
		uow:                  deps.UoW,
		cache:                deps.Cache,
		emailSender:          deps.EmailSender,
		authCfg:              deps.AuthConfig,
		registryHost:         deps.RegistryHost,
	}
}

func bcryptHash(password string, cost int) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// extractClientIP returns the best-effort client IP from the request.
// It checks X-Forwarded-For and X-Real-IP (set by reverse proxies) before
// falling back to the direct peer address from the connection.
func extractClientIP(req connect.AnyRequest) string {
	if xff := req.Header().Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
	}
	if xri := req.Header().Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host, _, err := net.SplitHostPort(req.Peer().Addr)
	if err != nil {
		return req.Peer().Addr
	}
	return host
}

// Register creates a new user account.
func (s *Server) Register(ctx context.Context, in *connect.Request[v1.RegisterRequest]) (*connect.Response[v1.RegisterResponse], error) {
	// Normalise username and email to lowercase for case-insensitive handling.
	username := strings.ToLower(in.Msg.Username)
	emailAddr := strings.ToLower(in.Msg.Email)

	if s.cache != nil {
		ip := extractClientIP(in)
		allowed, err := s.cache.Allow(ctx, fmt.Sprintf("register:ip:%s", ip), 3, time.Minute)
		if err == nil && !allowed {
			return nil, connErr.ResourceExhausted("too many requests")
		}
	}

	minLen := s.authCfg.Password.MinLength
	if minLen == 0 {
		minLen = 12
	}
	if len(in.Msg.Password) < minLen {
		return nil, connErr.InvalidArgument(fmt.Sprintf("password must be at least %d characters", minLen))
	}

	_, err := s.userStorage.GetByUsername(ctx, username)
	if err == nil {
		return nil, connErr.AlreadyExists("username already exists")
	}

	cost := s.authCfg.Password.BcryptCost
	if cost == 0 {
		cost = bcrypt.DefaultCost
	}
	hashedPassword, err := bcryptHash(in.Msg.Password, cost)
	if err != nil {
		s.logger.Error("failed to hash password", "error", err, "procedure", "Register")
		return nil, connErr.Internal("failed to hash password")
	}

	var userID string
	_, err = s.uow.Do(ctx, func(ctx context.Context) (interface{}, error) {
		if err := s.userStorage.Create(
			ctx,
			username,
			emailAddr,
			hashedPassword,
			registryv1.UserType_USER_TYPE_USER,
			registryv1.UserState_USER_STATE_ACTIVE,
			in.Msg.Description,
			"",
		); err != nil {
			return nil, connErr.FromPgx(err)
		}
		user, err := s.userStorage.GetByUsername(ctx, username)
		if err != nil {
			return nil, connErr.FromPgx(err)
		}
		userID = user.Id
		return nil, s.authorizationService.AddBasicRolesInTx(ctx, username)
	}, 15*time.Second)
	if err != nil {
		return nil, err
	}

	if err := s.authorizationService.ReloadPolicy(); err != nil {
		return nil, err
	}

	// Send verification email.
	if s.emailVerStorage != nil && s.emailSender != nil {
		expiry := s.authCfg.EmailVerification.TokenExpiryHours
		if expiry == 0 {
			expiry = 24
		}
		raw, hash, err := utilscrypto.GenerateToken()
		if err == nil {
			expiresAt := time.Now().Add(time.Duration(expiry) * time.Hour)
			if err := s.emailVerStorage.Create(ctx, userID, hash, expiresAt); err == nil {
				_ = s.emailSender.Send(emailAddr, "Verify your email",
					fmt.Sprintf("Verify your email: https://%s/verify-email/%s", s.registryHost, raw))
			}
		}
	}

	s.logger.Info("user registered", "procedure", "Register", "user_id", userID, "username", username)
	return &connect.Response[v1.RegisterResponse]{Msg: &v1.RegisterResponse{UserId: userID}}, nil
}

// Signin is kept for backwards compatibility; delegates to Register.
func (s *Server) Signin(ctx context.Context, in *connect.Request[v1.SigninRequest]) (*connect.Response[v1.SigninResponse], error) {
	_, err := s.Register(ctx, connect.NewRequest(&v1.RegisterRequest{
		Username:    in.Msg.Username,
		Password:    in.Msg.Password,
		Description: in.Msg.Description,
		Email:       in.Msg.Email,
	}))
	if err != nil {
		return nil, err
	}
	return &connect.Response[v1.SigninResponse]{Msg: &v1.SigninResponse{Status: true}}, nil
}

// Login authenticates a user and issues a session token.
func (s *Server) Login(ctx context.Context, in *connect.Request[v1.LoginRequest]) (*connect.Response[v1.LoginResponse], error) {
	ip := extractClientIP(in)
	ua := in.Msg.UserAgent
	if ua == "" {
		ua = in.Header().Get("User-Agent")
	}

	if s.cache != nil {
		allowed, err := s.cache.Allow(ctx, fmt.Sprintf("login:ip:%s", ip), 10, time.Minute)
		if err == nil && !allowed {
			return nil, connErr.ResourceExhausted("too many requests")
		}
	}

	// Normalise username to lowercase for case-insensitive lookup.
	username := strings.ToLower(in.Msg.Username)

	af, err := s.userStorage.GetAuthFieldsByUsername(ctx, username)
	if err != nil {
		// Always run bcrypt to prevent timing attacks.
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(in.Msg.Password))
		return nil, connErr.Unauthenticated("invalid credentials")
	}

	// Check lockout.
	if af.LockedUntil != nil && time.Now().Before(*af.LockedUntil) {
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(in.Msg.Password))
		if s.auditLogStorage != nil {
			_ = s.auditLogStorage.Create(ctx, &af.ID, "login_failed", ip, ua, map[string]any{"reason": "locked"})
		}
		return nil, connErr.PermissionDenied("account locked")
	}

	// Verify password.
	if err := bcrypt.CompareHashAndPassword([]byte(af.PasswordHash), []byte(in.Msg.Password)); err != nil {
		maxAttempts := s.authCfg.Lockout.MaxAttempts
		if maxAttempts == 0 {
			maxAttempts = 5
		}
		_ = s.userStorage.IncrementFailedLogins(ctx, af.ID)
		newCount := af.FailedLoginCount + 1
		if newCount >= maxAttempts {
			cooldown := s.authCfg.Lockout.CooldownMinutes
			if cooldown == 0 {
				cooldown = 30
			}
			_ = s.userStorage.LockUntil(ctx, af.ID, time.Now().Add(time.Duration(cooldown)*time.Minute))
			if s.auditLogStorage != nil {
				_ = s.auditLogStorage.Create(ctx, &af.ID, "account_locked", ip, ua, nil)
			}
		}
		if s.auditLogStorage != nil {
			_ = s.auditLogStorage.Create(ctx, &af.ID, "login_failed", ip, ua, map[string]any{"attempts": newCount})
		}
		return nil, connErr.Unauthenticated("invalid credentials")
	}

	// Check email verified.
	if af.EmailVerifiedAt == nil {
		return nil, connErr.PermissionDenied("email not verified")
	}

	// Reset failed logins on successful auth.
	_ = s.userStorage.ResetFailedLogins(ctx, af.ID)

	// Issue session token.
	raw, hash, err := utilscrypto.GenerateToken()
	if err != nil {
		s.logger.Error("failed to generate session token", "error", err, "procedure", "Login")
		return nil, connErr.Internal("failed to generate session token")
	}

	idleDays := s.authCfg.Session.IdleTimeoutDays
	if idleDays == 0 {
		idleDays = 7
	}
	absDays := s.authCfg.Session.AbsoluteTimeoutDays
	if absDays == 0 {
		absDays = 14
	}
	idleExpires := time.Now().Add(time.Duration(idleDays) * 24 * time.Hour)
	absExpires := time.Now().Add(time.Duration(absDays) * 24 * time.Hour)

	_, err = s.sessionStorage.CreateWithToken(ctx, af.ID, "session", hash, ip, ua, idleExpires, absExpires)
	if err != nil {
		s.logger.Error("failed to create session", "error", err, "procedure", "Login", "user_id", af.ID)
		return nil, connErr.FromPgx(err)
	}

	if s.auditLogStorage != nil {
		_ = s.auditLogStorage.Create(ctx, &af.ID, "login_success", ip, ua, nil)
	}

	s.logger.Info("user logged in", "procedure", "Login", "user_id", af.ID)
	return &connect.Response[v1.LoginResponse]{
		Msg: &v1.LoginResponse{Token: raw},
	}, nil
}

// Logout revokes the current session.
func (s *Server) Logout(ctx context.Context, in *connect.Request[v1.LogoutRequest]) (*connect.Response[v1.LogoutResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*registryv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "Logout")
		return nil, connErr.Internal("missing user in context")
	}
	rawToken, _ := ctx.Value(constants.ContextKeyAuthorization).(string)
	if rawToken != "" {
		hash := utilscrypto.HashToken(rawToken)
		session, err := s.sessionStorage.GetByTokenHash(ctx, hash)
		if err == nil {
			_ = s.sessionStorage.Revoke(ctx, session.ID)
		}
	}
	if s.auditLogStorage != nil {
		_ = s.auditLogStorage.Create(ctx, &user.Id, "logout", "", "", nil)
	}
	s.logger.Info("user logged out", "procedure", "Logout", "user_id", user.Id)
	return &connect.Response[v1.LogoutResponse]{Msg: &v1.LogoutResponse{}}, nil
}

// VerifyEmail validates an email verification token.
func (s *Server) VerifyEmail(ctx context.Context, in *connect.Request[v1.VerifyEmailRequest]) (*connect.Response[v1.VerifyEmailResponse], error) {
	if s.emailVerStorage == nil {
		return nil, connErr.Internal("email verification not configured")
	}
	hash := utilscrypto.HashToken(in.Msg.Token)
	row, err := s.emailVerStorage.GetByTokenHash(ctx, hash)
	if err != nil {
		return nil, connErr.NotFound("invalid or expired token")
	}
	if row.UsedAt != nil {
		return nil, connErr.InvalidArgument("token already used")
	}
	if time.Now().After(row.ExpiresAt) {
		return nil, connErr.InvalidArgument("token expired")
	}
	if err := s.emailVerStorage.MarkUsed(ctx, row.ID); err != nil {
		s.logger.Error("failed to mark email verification token used", "error", err, "procedure", "VerifyEmail")
		return nil, connErr.FromPgx(err)
	}
	if err := s.userStorage.SetEmailVerified(ctx, row.UserID); err != nil {
		s.logger.Error("failed to set email verified", "error", err, "procedure", "VerifyEmail", "user_id", row.UserID)
		return nil, connErr.FromPgx(err)
	}
	if s.auditLogStorage != nil {
		_ = s.auditLogStorage.Create(ctx, &row.UserID, "email_verified", "", "", nil)
	}
	s.logger.Info("email verified", "procedure", "VerifyEmail", "user_id", row.UserID)
	return &connect.Response[v1.VerifyEmailResponse]{Msg: &v1.VerifyEmailResponse{}}, nil
}

// ResendVerificationEmail sends a new verification email.
// Returns an error if the email is already verified.
func (s *Server) ResendVerificationEmail(ctx context.Context, in *connect.Request[v1.ResendVerificationEmailRequest]) (*connect.Response[v1.ResendVerificationEmailResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*registryv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "ResendVerificationEmail")
		return nil, connErr.Internal("missing user in context")
	}

	// Guard: do not allow re-sending if email is already verified.
	af, err := s.userStorage.GetAuthFieldsByUsername(ctx, user.Username)
	if err == nil && af.EmailVerifiedAt != nil {
		return nil, connErr.InvalidArgument("email is already verified")
	}

	if s.cache != nil {
		allowed, err := s.cache.Allow(ctx, fmt.Sprintf("emailresend:user:%s", user.Id), 3, 10*time.Minute)
		if err == nil && !allowed {
			return nil, connErr.ResourceExhausted("too many requests")
		}
	}
	expiry := s.authCfg.EmailVerification.TokenExpiryHours
	if expiry == 0 {
		expiry = 24
	}
	raw, hash, err := utilscrypto.GenerateToken()
	if err != nil {
		s.logger.Error("failed to generate verification token", "error", err, "procedure", "ResendVerificationEmail", "user_id", user.Id)
		return nil, connErr.Internal("failed to generate verification token")
	}
	expiresAt := time.Now().Add(time.Duration(expiry) * time.Hour)
	if s.emailVerStorage != nil {
		_ = s.emailVerStorage.Create(ctx, user.Id, hash, expiresAt)
	}
	if s.emailSender != nil {
		_ = s.emailSender.Send(user.Email, "Verify your email",
			fmt.Sprintf("Verify your email: https://%s/verify-email/%s", s.registryHost, raw))
	}
	return &connect.Response[v1.ResendVerificationEmailResponse]{Msg: &v1.ResendVerificationEmailResponse{}}, nil
}

// RequestPasswordReset initiates a password reset flow.
func (s *Server) RequestPasswordReset(ctx context.Context, in *connect.Request[v1.RequestPasswordResetRequest]) (*connect.Response[v1.RequestPasswordResetResponse], error) {
	ip := extractClientIP(in)
	if s.cache != nil {
		allowed, err := s.cache.Allow(ctx, fmt.Sprintf("pwreset:ip:%s", ip), 3, time.Minute)
		if err == nil && !allowed {
			return nil, connErr.ResourceExhausted("too many requests")
		}
	}
	// Always return the same response regardless of whether the email exists.
	user, err := s.userStorage.GetByEmail(ctx, strings.ToLower(in.Msg.Email))
	if err == nil && s.passwordResetStorage != nil && s.emailSender != nil {
		expiry := s.authCfg.PasswordReset.TokenExpiryHours
		if expiry == 0 {
			expiry = 1
		}
		raw, hash, tokenErr := utilscrypto.GenerateToken()
		if tokenErr == nil {
			expiresAt := time.Now().Add(time.Duration(expiry) * time.Hour)
			if createErr := s.passwordResetStorage.Create(ctx, user.Id, hash, expiresAt); createErr == nil {
				_ = s.emailSender.Send(in.Msg.Email, "Reset your password",
					fmt.Sprintf("Your password reset token: %s (expires in %d hour(s))", raw, expiry))
			}
		}
		if s.auditLogStorage != nil {
			_ = s.auditLogStorage.Create(ctx, &user.Id, "password_reset_requested", ip, "", nil)
		}
	}
	return &connect.Response[v1.RequestPasswordResetResponse]{Msg: &v1.RequestPasswordResetResponse{}}, nil
}

// ResetPassword validates the reset token and updates the password.
func (s *Server) ResetPassword(ctx context.Context, in *connect.Request[v1.ResetPasswordRequest]) (*connect.Response[v1.ResetPasswordResponse], error) {
	if s.passwordResetStorage == nil {
		return nil, connErr.Internal("password reset not configured")
	}
	minLen := s.authCfg.Password.MinLength
	if minLen == 0 {
		minLen = 12
	}
	if len(in.Msg.NewPassword) < minLen {
		return nil, connErr.InvalidArgument(fmt.Sprintf("password must be at least %d characters", minLen))
	}
	hash := utilscrypto.HashToken(in.Msg.Token)
	row, err := s.passwordResetStorage.GetByTokenHash(ctx, hash)
	if err != nil {
		return nil, connErr.NotFound("invalid or expired token")
	}
	if row.UsedAt != nil {
		return nil, connErr.InvalidArgument("token already used")
	}
	if time.Now().After(row.ExpiresAt) {
		return nil, connErr.InvalidArgument("token expired")
	}
	cost := s.authCfg.Password.BcryptCost
	if cost == 0 {
		cost = bcrypt.DefaultCost
	}
	newHash, err := bcryptHash(in.Msg.NewPassword, cost)
	if err != nil {
		s.logger.Error("failed to hash new password", "error", err, "procedure", "ResetPassword")
		return nil, connErr.Internal("failed to hash password")
	}
	if err := s.passwordResetStorage.MarkUsed(ctx, row.ID); err != nil {
		s.logger.Error("failed to mark password reset token used", "error", err, "procedure", "ResetPassword")
		return nil, connErr.FromPgx(err)
	}
	if err := s.userStorage.UpdatePassword(ctx, row.UserID, newHash); err != nil {
		s.logger.Error("failed to update password", "error", err, "procedure", "ResetPassword", "user_id", row.UserID)
		return nil, connErr.FromPgx(err)
	}
	// Revoke all sessions on password reset.
	_ = s.sessionStorage.RevokeAllForUser(ctx, row.UserID, "")
	if s.auditLogStorage != nil {
		_ = s.auditLogStorage.Create(ctx, &row.UserID, "password_changed", "", "", nil)
	}
	s.logger.Info("password reset", "procedure", "ResetPassword", "user_id", row.UserID)
	return &connect.Response[v1.ResetPasswordResponse]{Msg: &v1.ResetPasswordResponse{}}, nil
}

// ChangePassword updates the password for the authenticated user.
// Set RevokeOtherSessions=true to sign out all other active sessions.
func (s *Server) ChangePassword(ctx context.Context, in *connect.Request[v1.ChangePasswordRequest]) (*connect.Response[v1.ChangePasswordResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*registryv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "ChangePassword")
		return nil, connErr.Internal("missing user in context")
	}
	af, err := s.userStorage.GetAuthFieldsByUsername(ctx, strings.ToLower(user.Username))
	if err != nil {
		s.logger.Error("failed to get user auth fields", "error", err, "procedure", "ChangePassword", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(af.PasswordHash), []byte(in.Msg.OldPassword)); err != nil {
		return nil, connErr.Unauthenticated("invalid password")
	}
	minLen := s.authCfg.Password.MinLength
	if minLen == 0 {
		minLen = 12
	}
	if len(in.Msg.NewPassword) < minLen {
		return nil, connErr.InvalidArgument(fmt.Sprintf("password must be at least %d characters", minLen))
	}
	cost := s.authCfg.Password.BcryptCost
	if cost == 0 {
		cost = bcrypt.DefaultCost
	}
	newHash, err := bcryptHash(in.Msg.NewPassword, cost)
	if err != nil {
		s.logger.Error("failed to hash new password", "error", err, "procedure", "ChangePassword", "user_id", user.Id)
		return nil, connErr.Internal("failed to hash password")
	}
	if err := s.userStorage.UpdatePassword(ctx, user.Id, newHash); err != nil {
		s.logger.Error("failed to update password", "error", err, "procedure", "ChangePassword", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
	}
	if in.Msg.RevokeOtherSessions {
		// Keep the current session; revoke all others.
		rawToken, _ := ctx.Value(constants.ContextKeyAuthorization).(string)
		currentSessionID := ""
		if rawToken != "" {
			tokenHash := utilscrypto.HashToken(rawToken)
			if session, err := s.sessionStorage.GetByTokenHash(ctx, tokenHash); err == nil {
				currentSessionID = session.ID
			}
		}
		_ = s.sessionStorage.RevokeAllForUser(ctx, user.Id, currentSessionID)
	}
	if s.emailSender != nil {
		_ = s.emailSender.Send(user.Email, "Password changed",
			"Your account password was changed. If this wasn't you, please contact support.")
	}
	if s.auditLogStorage != nil {
		_ = s.auditLogStorage.Create(ctx, &user.Id, "password_changed", "", "", nil)
	}
	s.logger.Info("password changed", "procedure", "ChangePassword", "user_id", user.Id)
	return &connect.Response[v1.ChangePasswordResponse]{Msg: &v1.ChangePasswordResponse{}}, nil
}
