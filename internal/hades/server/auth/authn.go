package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/crypto/bcrypt"

	v1 "github.com/alipourhabibi/Hades/api/gen/api/auth/v1"
	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/utils/clientip"
	utilscrypto "github.com/alipourhabibi/Hades/utils/crypto"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
)

// extractClientIP returns the caller's IP address.
//
// Forwarding headers are honoured only when the immediate peer is one of the
// configured trusted proxies; see utils/clientip. With no trusted proxies
// configured (the default) the transport peer address is always used, so a
// client cannot influence rate-limit keys or the audit log by sending
// X-Forwarded-For.
func extractClientIP(req connect.AnyRequest, trusted clientip.TrustedProxies) string {
	return clientip.FromHeaders(
		req.Peer().Addr,
		req.Header().Get("X-Forwarded-For"),
		req.Header().Get("X-Real-IP"),
		trusted,
	)
}

// totpRequired reports whether the user has TOTP enabled, meaning a freshly
// created session cannot be used until VerifyTOTP succeeds. This is advisory
// only, so a lookup failure reports false: the authorization interceptor
// enforces the requirement on every subsequent call regardless of what is
// reported here.
func (s *Server) totpRequired(ctx context.Context, userID string) bool {
	if s.totpSecretDB == nil {
		return false
	}
	row, err := s.totpSecretDB.GetByUserID(ctx, userID)
	if err != nil {
		return false
	}
	return row.Enabled
}

func (s *Server) Register(ctx context.Context, in *connect.Request[v1.RegisterRequest]) (*connect.Response[v1.RegisterResponse], error) {
	username := strings.ToLower(in.Msg.Username)
	emailAddr := strings.ToLower(in.Msg.Email)

	if constants.IsReservedName(username) {
		return nil, connErr.InvalidArgument("username is reserved")
	}
	if emailAddr == "" {
		return nil, connErr.InvalidArgument("email is required")
	}

	if s.cache != nil {
		ip := extractClientIP(in, s.trustedProxies)
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

	cost := s.authCfg.Password.BcryptCost
	if cost == 0 {
		cost = bcrypt.DefaultCost
	}
	hashedPassword, err := bcryptHash(in.Msg.Password, cost)
	if err != nil {
		s.logger.Error("failed to hash password", "error", err, "procedure", "Register")
		return nil, connErr.Internal("failed to hash password")
	}

	// No pre-flight existence check: it races with a concurrent registration of
	// the same name. The unique constraint is the authority, and FromDB maps the
	// violation to AlreadyExists.
	var userID string
	_, err = s.uow.Do(ctx, func(ctx context.Context) (interface{}, error) {
		if err := s.userStorage.Create(
			ctx,
			username,
			emailAddr,
			hashedPassword,
			identityv1.UserType_USER_TYPE_USER,
			identityv1.UserState_USER_STATE_ACTIVE,
			in.Msg.Description,
			"",
		); err != nil {
			mapped := connErr.FromDB(err)
			if connect.CodeOf(mapped) == connect.CodeAlreadyExists {
				s.logger.Warn("registration conflict", "procedure", "Register", "username", username)
				return nil, connErr.AlreadyExists("username or email is already registered")
			}
			s.logger.Error("failed to create user", "error", err, "procedure", "Register", "username", username)
			return nil, mapped
		}
		user, err := s.userStorage.GetByUsername(ctx, username)
		if err != nil {
			s.logger.Error("failed to read back created user", "error", err, "procedure", "Register", "username", username)
			return nil, connErr.FromDB(err)
		}
		userID = user.Id
		return nil, s.authorizationService.AddBasicRoles(ctx, username)
	}, 15*time.Second)
	if err != nil {
		return nil, err
	}

	if s.emailVerStorage != nil && s.emailSender != nil {
		expiry := s.authCfg.EmailVerification.TokenExpiryHours
		if expiry == 0 {
			expiry = 24
		}
		raw, hash, err := utilscrypto.GenerateToken("")
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

// Signin is a legacy alias for Register that reports only success or failure.
//
// Deprecated: call Register instead. It carries identical behaviour but returns
// the created user id. This method exists only so existing clients keep
// working, and it doubles the rate-limited registration surface; remove it once
// no client depends on it.
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

func (s *Server) Login(ctx context.Context, in *connect.Request[v1.LoginRequest]) (*connect.Response[v1.LoginResponse], error) {
	ip := extractClientIP(in, s.trustedProxies)
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

	username := strings.ToLower(in.Msg.Username)

	af, err := s.userStorage.GetAuthFieldsByUsername(ctx, username)
	if err != nil {
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(in.Msg.Password))
		return nil, connErr.Unauthenticated("invalid credentials")
	}

	if af.LockedUntil != nil && time.Now().Before(*af.LockedUntil) {
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(in.Msg.Password))
		if s.auditLogDB != nil {
			_ = s.auditLogDB.Create(ctx, &af.ID, v1.AuditEventType_AUDIT_EVENT_TYPE_LOGIN_FAILED, ip, ua, map[string]any{"reason": "locked"})
		}
		return nil, connErr.PermissionDenied("account locked")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(af.PasswordHash), []byte(in.Msg.Password)); err != nil {
		maxAttempts := s.authCfg.Lockout.MaxAttempts
		if maxAttempts == 0 {
			maxAttempts = 5
		}
		if err := s.userStorage.IncrementFailedLogins(ctx, af.ID); err != nil {
			s.logger.Error("failed to increment failed login count", "error", err, "procedure", "Login", "user_id", af.ID)
		}
		newCount := af.FailedLoginCount + 1
		if newCount >= maxAttempts {
			cooldown := s.authCfg.Lockout.CooldownMinutes
			if cooldown == 0 {
				cooldown = 30
			}
			// Lockout is a security control, not bookkeeping: if the write fails
			// the account is not actually locked, so the caller is told the
			// request failed rather than being handed a silent bypass.
			if err := s.userStorage.LockUntil(ctx, af.ID, time.Now().Add(time.Duration(cooldown)*time.Minute)); err != nil {
				s.logger.Error("failed to lock account", "error", err, "procedure", "Login", "user_id", af.ID)
				return nil, connErr.Unavailable("authentication temporarily unavailable")
			}
			if s.auditLogDB != nil {
				_ = s.auditLogDB.Create(ctx, &af.ID, v1.AuditEventType_AUDIT_EVENT_TYPE_ACCOUNT_LOCKED, ip, ua, nil)
			}
		}
		if s.auditLogDB != nil {
			_ = s.auditLogDB.Create(ctx, &af.ID, v1.AuditEventType_AUDIT_EVENT_TYPE_LOGIN_FAILED, ip, ua, map[string]any{"attempts": newCount})
		}
		return nil, connErr.Unauthenticated("invalid credentials")
	}

	if af.EmailVerifiedAt == nil {
		return nil, connErr.PermissionDenied("email not verified")
	}

	if err := s.userStorage.ResetFailedLogins(ctx, af.ID); err != nil {
		s.logger.Error("failed to reset failed login count", "error", err, "procedure", "Login", "user_id", af.ID)
	}

	fullToken, tokenHash, err := utilscrypto.GenerateToken(utilscrypto.SessionTokenPrefix)
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

	_, err = s.sessionStorage.CreateWithToken(ctx, af.ID, "session", tokenHash, ip, ua, idleExpires, absExpires)
	if err != nil {
		s.logger.Error("failed to create session", "error", err, "procedure", "Login", "user_id", af.ID)
		return nil, connErr.FromDB(err)
	}

	if s.auditLogDB != nil {
		_ = s.auditLogDB.Create(ctx, &af.ID, v1.AuditEventType_AUDIT_EVENT_TYPE_LOGIN_SUCCESS, ip, ua, nil)
	}

	s.logger.Info("user logged in", "procedure", "Login", "user_id", af.ID)
	// The token is returned even when TOTP is pending: VerifyTOTP itself needs an
	// authenticated session to identify the caller, and the interceptor rejects
	// the token for every other procedure until the second factor is confirmed.
	return &connect.Response[v1.LoginResponse]{
		Msg: &v1.LoginResponse{
			Token:       fullToken,
			PendingTotp: s.totpRequired(ctx, af.ID),
		},
	}, nil
}

func (s *Server) Logout(ctx context.Context, in *connect.Request[v1.LogoutRequest]) (*connect.Response[v1.LogoutResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "Logout")
		return nil, connErr.Unauthenticated("not authenticated")
	}
	rawToken, _ := ctx.Value(constants.ContextKeyAuthorization).(string)
	if rawToken != "" {
		hash := utilscrypto.HashToken(rawToken)
		session, err := s.sessionStorage.GetByTokenHash(ctx, hash)
		if err == nil {
			_ = s.sessionStorage.Revoke(ctx, session.ID)
		}
	}
	if s.auditLogDB != nil {
		_ = s.auditLogDB.Create(ctx, &user.Id, v1.AuditEventType_AUDIT_EVENT_TYPE_LOGOUT, "", "", nil)
	}
	s.logger.Info("user logged out", "procedure", "Logout", "user_id", user.Id)
	return &connect.Response[v1.LogoutResponse]{Msg: &v1.LogoutResponse{}}, nil
}

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
	// Both writes go in one transaction: consuming the token without recording
	// the verification would burn the user's only link.
	if _, err := s.uow.Do(ctx, func(txCtx context.Context) (interface{}, error) {
		if err := s.emailVerStorage.MarkUsed(txCtx, row.ID); err != nil {
			s.logger.Error("failed to mark email verification token used", "error", err, "procedure", "VerifyEmail")
			return nil, connErr.FromDB(err)
		}
		if err := s.userStorage.SetEmailVerified(txCtx, row.UserID); err != nil {
			s.logger.Error("failed to set email verified", "error", err, "procedure", "VerifyEmail", "user_id", row.UserID)
			return nil, connErr.FromDB(err)
		}
		return nil, nil
	}, 15*time.Second); err != nil {
		return nil, err
	}
	if s.auditLogDB != nil {
		_ = s.auditLogDB.Create(ctx, &row.UserID, v1.AuditEventType_AUDIT_EVENT_TYPE_EMAIL_VERIFIED, "", "", nil)
	}
	s.logger.Info("email verified", "procedure", "VerifyEmail", "user_id", row.UserID)
	return &connect.Response[v1.VerifyEmailResponse]{Msg: &v1.VerifyEmailResponse{}}, nil
}

func (s *Server) ResendVerificationEmail(ctx context.Context, in *connect.Request[v1.ResendVerificationEmailRequest]) (*connect.Response[v1.ResendVerificationEmailResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "ResendVerificationEmail")
		return nil, connErr.Unauthenticated("not authenticated")
	}

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
	raw, hash, err := utilscrypto.GenerateToken("")
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

func (s *Server) RequestPasswordReset(ctx context.Context, in *connect.Request[v1.RequestPasswordResetRequest]) (*connect.Response[v1.RequestPasswordResetResponse], error) {
	ip := extractClientIP(in, s.trustedProxies)
	if s.cache != nil {
		allowed, err := s.cache.Allow(ctx, fmt.Sprintf("pwreset:ip:%s", ip), 3, time.Minute)
		if err == nil && !allowed {
			return nil, connErr.ResourceExhausted("too many requests")
		}
	}
	user, err := s.userStorage.GetByEmail(ctx, strings.ToLower(in.Msg.Email))
	if err == nil && s.passwordResetStorage != nil && s.emailSender != nil {
		expiry := s.authCfg.PasswordReset.TokenExpiryHours
		if expiry == 0 {
			expiry = 1
		}
		raw, hash, tokenErr := utilscrypto.GenerateToken("")
		if tokenErr == nil {
			expiresAt := time.Now().Add(time.Duration(expiry) * time.Hour)
			if createErr := s.passwordResetStorage.Create(ctx, user.Id, hash, expiresAt); createErr == nil {
				_ = s.emailSender.Send(in.Msg.Email, "Reset your password",
					fmt.Sprintf("Your password reset token: %s (expires in %d hour(s))", raw, expiry))
			}
		}
		if s.auditLogDB != nil {
			_ = s.auditLogDB.Create(ctx, &user.Id, v1.AuditEventType_AUDIT_EVENT_TYPE_PASSWORD_RESET, ip, "", nil)
		}
	}
	return &connect.Response[v1.RequestPasswordResetResponse]{Msg: &v1.RequestPasswordResetResponse{}}, nil
}

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
	// Consuming the token and changing the password must succeed or fail
	// together, otherwise a failed update leaves the user with a spent token and
	// the old password. Clearing the lockout counters is part of the same unit:
	// a user who resets precisely because they were locked out must be able to
	// log in afterwards.
	if _, err := s.uow.Do(ctx, func(txCtx context.Context) (interface{}, error) {
		if err := s.passwordResetStorage.MarkUsed(txCtx, row.ID); err != nil {
			s.logger.Error("failed to mark password reset token used", "error", err, "procedure", "ResetPassword")
			return nil, connErr.FromDB(err)
		}
		if err := s.userStorage.UpdatePassword(txCtx, row.UserID, newHash); err != nil {
			s.logger.Error("failed to update password", "error", err, "procedure", "ResetPassword", "user_id", row.UserID)
			return nil, connErr.FromDB(err)
		}
		if err := s.userStorage.ResetFailedLogins(txCtx, row.UserID); err != nil {
			s.logger.Error("failed to clear lockout state", "error", err, "procedure", "ResetPassword", "user_id", row.UserID)
			return nil, connErr.FromDB(err)
		}
		return nil, nil
	}, 15*time.Second); err != nil {
		return nil, err
	}

	// Every existing session must die with the old password. A failure here is
	// not cosmetic: it would leave an attacker's session alive after the
	// legitimate owner reset their password.
	if err := s.sessionStorage.RevokeAllForUser(ctx, row.UserID, ""); err != nil {
		s.logger.Error("failed to revoke sessions after password reset", "error", err, "procedure", "ResetPassword", "user_id", row.UserID)
		return nil, connErr.Unavailable("password was changed but existing sessions could not be revoked; revoke them manually")
	}
	if s.auditLogDB != nil {
		_ = s.auditLogDB.Create(ctx, &row.UserID, v1.AuditEventType_AUDIT_EVENT_TYPE_PASSWORD_CHANGED, "", "", nil)
	}
	s.logger.Info("password reset", "procedure", "ResetPassword", "user_id", row.UserID)
	return &connect.Response[v1.ResetPasswordResponse]{Msg: &v1.ResetPasswordResponse{}}, nil
}

func (s *Server) ChangePassword(ctx context.Context, in *connect.Request[v1.ChangePasswordRequest]) (*connect.Response[v1.ChangePasswordResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "ChangePassword")
		return nil, connErr.Unauthenticated("not authenticated")
	}
	af, err := s.userStorage.GetAuthFieldsByUsername(ctx, strings.ToLower(user.Username))
	if err != nil {
		s.logger.Error("failed to get user auth fields", "error", err, "procedure", "ChangePassword", "user_id", user.Id)
		return nil, connErr.FromDB(err)
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
		return nil, connErr.FromDB(err)
	}
	if in.Msg.RevokeOtherSessions {
		rawToken, _ := ctx.Value(constants.ContextKeyAuthorization).(string)
		currentSessionID := ""
		if rawToken != "" {
			tokenHash := utilscrypto.HashToken(rawToken)
			if session, err := s.sessionStorage.GetByTokenHash(ctx, tokenHash); err == nil {
				currentSessionID = session.ID
			}
		}
		// The caller explicitly asked for other sessions to be revoked, so a
		// silent failure would report success while leaving them alive.
		if err := s.sessionStorage.RevokeAllForUser(ctx, user.Id, currentSessionID); err != nil {
			s.logger.Error("failed to revoke other sessions", "error", err, "procedure", "ChangePassword", "user_id", user.Id)
			return nil, connErr.Unavailable("password was changed but other sessions could not be revoked")
		}
	}
	if s.emailSender != nil {
		_ = s.emailSender.Send(user.Email, "Password changed",
			"Your account password was changed. If this wasn't you, please contact support.")
	}
	if s.auditLogDB != nil {
		_ = s.auditLogDB.Create(ctx, &user.Id, v1.AuditEventType_AUDIT_EVENT_TYPE_PASSWORD_CHANGED, "", "", nil)
	}
	s.logger.Info("password changed", "procedure", "ChangePassword", "user_id", user.Id)
	return &connect.Response[v1.ChangePasswordResponse]{Msg: &v1.ChangePasswordResponse{}}, nil
}
