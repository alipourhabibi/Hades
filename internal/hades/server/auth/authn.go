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
	"github.com/alipourhabibi/Hades/utils/connerr"
	utilscrypto "github.com/alipourhabibi/Hades/utils/crypto"
	"github.com/google/uuid"
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
	username := strings.ToLower(strings.TrimSpace(in.Msg.Username))
	emailAddr := strings.ToLower(strings.TrimSpace(in.Msg.Email))

	// The whole name check, not only the reserved list.
	//
	// A username is the first path segment of every repository its modules get,
	// "<owner>/<module>", and of every URL it appears at, and the wire contract
	// bounds only the length: RegisterRequest.username declares min_len 2 and
	// max_len 32 and nothing about the character set. So "a/b", "..", ".git"
	// and "has space" were all registrable. git.ValidateRepoPath now refuses
	// them at the storage layer, which stops the traversal but turns the
	// account into a dead end: the row exists, and every module created under
	// it fails, permanently, with an error that names none of this.
	//
	// constants.ValidateName is the same check module names get, and its
	// documentation already claimed to cover usernames and organisation names.
	// It subsumes the reserved-name test that used to be here.
	if err := constants.ValidateName(username); err != nil {
		return nil, connerr.InvalidArgument("user " + err.Error())
	}
	if emailAddr == "" {
		return nil, connerr.InvalidArgument("email is required")
	}

	if err := s.enforceLimit(ctx, fmt.Sprintf("register:ip:%s", extractClientIP(in, s.trustedProxies)), 3, time.Minute, "Register"); err != nil {
		return nil, err
	}

	minLen := s.authCfg.Password.MinLength
	if minLen == 0 {
		minLen = 12
	}
	if len(in.Msg.Password) < minLen {
		return nil, connerr.InvalidArgument(fmt.Sprintf("password must be at least %d characters", minLen))
	}

	cost := s.authCfg.Password.BcryptCost
	if cost == 0 {
		cost = bcrypt.DefaultCost
	}
	hashedPassword, err := bcryptHash(in.Msg.Password, cost)
	if err != nil {
		s.logger.Error("failed to hash password", "error", err, "procedure", "Register")
		return nil, connerr.Internal("failed to hash password")
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
			mapped := connerr.FromDB(err)
			if connect.CodeOf(mapped) == connect.CodeAlreadyExists {
				s.logger.Warn("registration conflict", "procedure", "Register", "username", username)
				return nil, connerr.AlreadyExists("username or email is already registered")
			}
			s.logger.Error("failed to create user", "error", err, "procedure", "Register", "username", username)
			return nil, mapped
		}
		user, err := s.userStorage.GetByUsername(ctx, username)
		if err != nil {
			s.logger.Error("failed to read back created user", "error", err, "procedure", "Register", "username", username)
			return nil, connerr.FromDB(err)
		}
		userID = user.Id
		return nil, s.authorizationService.AddBasicRoles(ctx, username)
	}, 15*time.Second)
	if err != nil {
		return nil, err
	}

	// Development shortcut, off by default. See EmailVerifConfig.AutoVerify:
	// with email.stub on, no mail is delivered, so without this the account
	// cannot log in and the documented bootstrap cannot complete.
	if s.authCfg.EmailVerification.AutoVerify {
		if err := s.userStorage.SetEmailVerified(ctx, userID); err != nil {
			s.logger.Error("failed to auto-verify email", "error", err, "procedure", "Register", "user_id", userID)
			return nil, connerr.FromDB(err)
		}
		s.logger.Warn("email auto-verified without confirmation: auth.emailVerification.autoVerify is on",
			"procedure", "Register", "user_id", userID)
		return &connect.Response[v1.RegisterResponse]{Msg: &v1.RegisterResponse{UserId: userID}}, nil
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
// Deprecated: call Register instead. It has identical behaviour and also
// returns the created user id.
//
// It is still here because removing an RPC is a wire-breaking change to a
// published protocol, and this is cleanup rather than a defect: it delegates
// straight to Register, so both paths share one rate-limit key and one
// validation path rather than being a second, divergent registration surface.
// Remove it in a deliberate breaking release, together with the proto RPC, the
// noAuthProcedures entry and the policy-matrix row.
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

	if err := s.enforceLimit(ctx, fmt.Sprintf("login:ip:%s", ip), 10, time.Minute, "Login"); err != nil {
		return nil, err
	}

	username := strings.ToLower(in.Msg.Username)

	af, err := s.userStorage.GetAuthFieldsByUsername(ctx, username)
	if err != nil {
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(in.Msg.Password))
		return nil, connerr.Unauthenticated("invalid credentials")
	}

	if af.LockedUntil != nil && time.Now().Before(*af.LockedUntil) {
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(in.Msg.Password))
		if s.auditLogDB != nil {
			_ = s.auditLogDB.Create(ctx, &af.ID, v1.AuditEventType_AUDIT_EVENT_TYPE_LOGIN_FAILED, ip, ua, map[string]any{"reason": "locked"})
		}
		return nil, connerr.PermissionDenied("account locked")
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
				return nil, connerr.Unavailable("authentication temporarily unavailable")
			}
			if s.auditLogDB != nil {
				_ = s.auditLogDB.Create(ctx, &af.ID, v1.AuditEventType_AUDIT_EVENT_TYPE_ACCOUNT_LOCKED, ip, ua, nil)
			}
		}
		if s.auditLogDB != nil {
			_ = s.auditLogDB.Create(ctx, &af.ID, v1.AuditEventType_AUDIT_EVENT_TYPE_LOGIN_FAILED, ip, ua, map[string]any{"attempts": newCount})
		}
		return nil, connerr.Unauthenticated("invalid credentials")
	}

	if af.EmailVerifiedAt == nil {
		return nil, connerr.PermissionDenied("email not verified")
	}

	if err := s.userStorage.ResetFailedLogins(ctx, af.ID); err != nil {
		s.logger.Error("failed to reset failed login count", "error", err, "procedure", "Login", "user_id", af.ID)
	}

	fullToken, tokenHash, err := utilscrypto.GenerateToken(utilscrypto.SessionTokenPrefix)
	if err != nil {
		s.logger.Error("failed to generate session token", "error", err, "procedure", "Login")
		return nil, connerr.Internal("failed to generate session token")
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
		return nil, connerr.FromDB(err)
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
		return nil, connerr.Unauthenticated("not authenticated")
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
		return nil, connerr.Internal("email verification not configured")
	}
	// Token submission is limited as well as token issuance. The sibling
	// request endpoints were limited and this one was not, so the guessable
	// half of the flow was the unbounded half.
	if err := s.enforceLimit(ctx, fmt.Sprintf("verifyemail:ip:%s", extractClientIP(in, s.trustedProxies)), 10, time.Minute, "VerifyEmail"); err != nil {
		return nil, err
	}
	hash := utilscrypto.HashToken(in.Msg.Token)
	row, err := s.emailVerStorage.GetByTokenHash(ctx, hash)
	if err != nil {
		return nil, connerr.NotFound("invalid or expired token")
	}
	if row.UsedAt != nil {
		return nil, connerr.InvalidArgument("token already used")
	}
	if time.Now().After(row.ExpiresAt) {
		return nil, connerr.InvalidArgument("token expired")
	}
	// Both writes go in one transaction: consuming the token without recording
	// the verification would burn the user's only link.
	if _, err := s.uow.Do(ctx, func(txCtx context.Context) (interface{}, error) {
		if err := s.emailVerStorage.MarkUsed(txCtx, row.ID); err != nil {
			s.logger.Error("failed to mark email verification token used", "error", err, "procedure", "VerifyEmail")
			return nil, connerr.FromDB(err)
		}
		if err := s.userStorage.SetEmailVerified(txCtx, row.UserID); err != nil {
			s.logger.Error("failed to set email verified", "error", err, "procedure", "VerifyEmail", "user_id", row.UserID)
			return nil, connerr.FromDB(err)
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
		return nil, connerr.Unauthenticated("not authenticated")
	}

	af, err := s.userStorage.GetAuthFieldsByUsername(ctx, user.Username)
	if err == nil && af.EmailVerifiedAt != nil {
		return nil, connerr.InvalidArgument("email is already verified")
	}

	if err := s.enforceLimit(ctx, fmt.Sprintf("emailresend:user:%s", user.Id), 3, 10*time.Minute, "ResendVerificationEmail"); err != nil {
		return nil, err
	}
	expiry := s.authCfg.EmailVerification.TokenExpiryHours
	if expiry == 0 {
		expiry = 24
	}
	raw, hash, err := utilscrypto.GenerateToken("")
	if err != nil {
		s.logger.Error("failed to generate verification token", "error", err, "procedure", "ResendVerificationEmail", "user_id", user.Id)
		return nil, connerr.Internal("failed to generate verification token")
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
	if err := s.enforceLimit(ctx, fmt.Sprintf("pwreset:ip:%s", ip), 3, time.Minute, "RequestPasswordReset"); err != nil {
		return nil, err
	}
	user, err := s.userStorage.GetByEmail(ctx, strings.ToLower(in.Msg.Email))
	if err == nil && s.passwordResetStorage != nil && s.emailSender != nil {
		expiry := s.authCfg.PasswordReset.TokenExpiryHours
		if expiry == 0 {
			expiry = 1
		}
		raw, hash, tokenErr := utilscrypto.GenerateToken("")
		if tokenErr == nil {
			// Outstanding resets for this user are invalidated first. Without
			// that, three requests a minute for the token's lifetime left many
			// live reset tokens at once, each an independent chance for anyone
			// who can read one of them.
			if invErr := s.passwordResetStorage.InvalidateForUser(ctx, user.Id); invErr != nil {
				s.logger.Error("failed to invalidate outstanding password resets", "error", invErr,
					"procedure", "RequestPasswordReset", "user_id", user.Id)
			}
			expiresAt := time.Now().Add(time.Duration(expiry) * time.Hour)
			if createErr := s.passwordResetStorage.Create(ctx, user.Id, hash, expiresAt); createErr == nil {
				// A link, like Register sends, rather than a bare token the
				// user has to paste somewhere.
				_ = s.emailSender.Send(in.Msg.Email, "Reset your password",
					fmt.Sprintf("Reset your password: https://%s/reset-password/%s (expires in %d hour(s))",
						s.registryHost, raw, expiry))
			}
		}
		if s.auditLogDB != nil {
			_ = s.auditLogDB.Create(ctx, &user.Id, v1.AuditEventType_AUDIT_EVENT_TYPE_PASSWORD_RESET, ip, "", nil)
		}
	}
	return &connect.Response[v1.RequestPasswordResetResponse]{Msg: &v1.RequestPasswordResetResponse{}}, nil
}

func (s *Server) ResetPassword(ctx context.Context, in *connect.Request[v1.ResetPasswordRequest]) (*connect.Response[v1.ResetPasswordResponse], error) {
	// See VerifyEmail: the endpoint that accepts a token needs a bound too, not
	// only the one that issues it.
	if err := s.enforceLimit(ctx, fmt.Sprintf("resetpw:ip:%s", extractClientIP(in, s.trustedProxies)), 10, time.Minute, "ResetPassword"); err != nil {
		return nil, err
	}
	if s.passwordResetStorage == nil {
		return nil, connerr.Internal("password reset not configured")
	}
	minLen := s.authCfg.Password.MinLength
	if minLen == 0 {
		minLen = 12
	}
	if len(in.Msg.NewPassword) < minLen {
		return nil, connerr.InvalidArgument(fmt.Sprintf("password must be at least %d characters", minLen))
	}
	hash := utilscrypto.HashToken(in.Msg.Token)
	row, err := s.passwordResetStorage.GetByTokenHash(ctx, hash)
	if err != nil {
		return nil, connerr.NotFound("invalid or expired token")
	}
	if row.UsedAt != nil {
		return nil, connerr.InvalidArgument("token already used")
	}
	if time.Now().After(row.ExpiresAt) {
		return nil, connerr.InvalidArgument("token expired")
	}
	cost := s.authCfg.Password.BcryptCost
	if cost == 0 {
		cost = bcrypt.DefaultCost
	}
	newHash, err := bcryptHash(in.Msg.NewPassword, cost)
	if err != nil {
		s.logger.Error("failed to hash new password", "error", err, "procedure", "ResetPassword")
		return nil, connerr.Internal("failed to hash password")
	}
	// Consuming the token and changing the password must succeed or fail
	// together, otherwise a failed update leaves the user with a spent token and
	// the old password. Clearing the lockout counters is part of the same unit:
	// a user who resets precisely because they were locked out must be able to
	// log in afterwards.
	if _, err := s.uow.Do(ctx, func(txCtx context.Context) (interface{}, error) {
		if err := s.passwordResetStorage.MarkUsed(txCtx, row.ID); err != nil {
			s.logger.Error("failed to mark password reset token used", "error", err, "procedure", "ResetPassword")
			return nil, connerr.FromDB(err)
		}
		if err := s.userStorage.UpdatePassword(txCtx, row.UserID, newHash); err != nil {
			s.logger.Error("failed to update password", "error", err, "procedure", "ResetPassword", "user_id", row.UserID)
			return nil, connerr.FromDB(err)
		}
		if err := s.userStorage.ResetFailedLogins(txCtx, row.UserID); err != nil {
			s.logger.Error("failed to clear lockout state", "error", err, "procedure", "ResetPassword", "user_id", row.UserID)
			return nil, connerr.FromDB(err)
		}
		return nil, nil
	}, 15*time.Second); err != nil {
		return nil, err
	}

	// Every existing session must die with the old password. A failure here is
	// not cosmetic: it would leave an attacker's session alive after the
	// legitimate owner reset their password.
	if err := s.sessionStorage.RevokeAllForUser(ctx, row.UserID, uuid.Nil); err != nil {
		s.logger.Error("failed to revoke sessions after password reset", "error", err, "procedure", "ResetPassword", "user_id", row.UserID)
		return nil, connerr.Unavailable("password was changed but existing sessions could not be revoked; revoke them manually")
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
		return nil, connerr.Unauthenticated("not authenticated")
	}
	af, err := s.userStorage.GetAuthFieldsByUsername(ctx, strings.ToLower(user.Username))
	if err != nil {
		s.logger.Error("failed to get user auth fields", "error", err, "procedure", "ChangePassword", "user_id", user.Id)
		return nil, connerr.FromDB(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(af.PasswordHash), []byte(in.Msg.OldPassword)); err != nil {
		return nil, connerr.Unauthenticated("invalid password")
	}
	minLen := s.authCfg.Password.MinLength
	if minLen == 0 {
		minLen = 12
	}
	if len(in.Msg.NewPassword) < minLen {
		return nil, connerr.InvalidArgument(fmt.Sprintf("password must be at least %d characters", minLen))
	}
	cost := s.authCfg.Password.BcryptCost
	if cost == 0 {
		cost = bcrypt.DefaultCost
	}
	newHash, err := bcryptHash(in.Msg.NewPassword, cost)
	if err != nil {
		s.logger.Error("failed to hash new password", "error", err, "procedure", "ChangePassword", "user_id", user.Id)
		return nil, connerr.Internal("failed to hash password")
	}
	if err := s.userStorage.UpdatePassword(ctx, user.Id, newHash); err != nil {
		s.logger.Error("failed to update password", "error", err, "procedure", "ChangePassword", "user_id", user.Id)
		return nil, connerr.FromDB(err)
	}
	// Every session is re-established after a password change, including the
	// caller's own. A password change is the standard response to a suspected
	// compromise, and leaving the current session alive means the attacker who
	// prompted it keeps the session they already hold. RevokeOtherSessions is
	// honoured only in the direction of revoking more, never less.
	if err := s.sessionStorage.RevokeAllForUser(ctx, user.Id, uuid.Nil); err != nil {
		s.logger.Error("failed to revoke sessions", "error", err, "procedure", "ChangePassword", "user_id", user.Id)
		return nil, connerr.Unavailable("password was changed but existing sessions could not be revoked; revoke them manually")
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
