package auth

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/crypto/bcrypt"

	v1 "github.com/alipourhabibi/Hades/api/gen/api/auth/v1"
	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	utilscrypto "github.com/alipourhabibi/Hades/utils/crypto"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
)

var reservedUsernames = map[string]struct{}{
	"settings": {}, "login": {}, "signup": {}, "search": {},
	"verify-email": {}, "api": {}, "app": {},
	"go": {}, "gen": {}, "oauth2": {}, "buf": {}, "hades": {},
	"admin": {}, "administrator": {}, "root": {}, "system": {},
	"help": {}, "support": {}, "about": {}, "pricing": {},
	"terms": {}, "privacy": {}, "security": {}, "status": {},
	"user": {}, "users": {}, "org": {}, "orgs": {},
	"team": {}, "teams": {}, "me": {}, "null": {}, "undefined": {},
	"new": {}, "home": {},
}

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

func (s *Server) Register(ctx context.Context, in *connect.Request[v1.RegisterRequest]) (*connect.Response[v1.RegisterResponse], error) {
	username := strings.ToLower(in.Msg.Username)
	emailAddr := strings.ToLower(in.Msg.Email)

	if _, blocked := reservedUsernames[username]; blocked {
		return nil, connErr.InvalidArgument("username is reserved")
	}

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
			identityv1.UserType_USER_TYPE_USER,
			identityv1.UserState_USER_STATE_ACTIVE,
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

	username := strings.ToLower(in.Msg.Username)

	af, err := s.userStorage.GetAuthFieldsByUsername(ctx, username)
	if err != nil {
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(in.Msg.Password))
		return nil, connErr.Unauthenticated("invalid credentials")
	}

	if af.LockedUntil != nil && time.Now().Before(*af.LockedUntil) {
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(in.Msg.Password))
		if s.auditLogDB != nil {
			_ = s.auditLogDB.Create(ctx, &af.ID, "login_failed", ip, ua, map[string]any{"reason": "locked"})
		}
		return nil, connErr.PermissionDenied("account locked")
	}

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
			if s.auditLogDB != nil {
				_ = s.auditLogDB.Create(ctx, &af.ID, "account_locked", ip, ua, nil)
			}
		}
		if s.auditLogDB != nil {
			_ = s.auditLogDB.Create(ctx, &af.ID, "login_failed", ip, ua, map[string]any{"attempts": newCount})
		}
		return nil, connErr.Unauthenticated("invalid credentials")
	}

	if af.EmailVerifiedAt == nil {
		return nil, connErr.PermissionDenied("email not verified")
	}

	_ = s.userStorage.ResetFailedLogins(ctx, af.ID)

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

	if s.auditLogDB != nil {
		_ = s.auditLogDB.Create(ctx, &af.ID, "login_success", ip, ua, nil)
	}

	s.logger.Info("user logged in", "procedure", "Login", "user_id", af.ID)
	return &connect.Response[v1.LoginResponse]{
		Msg: &v1.LoginResponse{Token: raw},
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
		_ = s.auditLogDB.Create(ctx, &user.Id, "logout", "", "", nil)
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
	if err := s.emailVerStorage.MarkUsed(ctx, row.ID); err != nil {
		s.logger.Error("failed to mark email verification token used", "error", err, "procedure", "VerifyEmail")
		return nil, connErr.FromPgx(err)
	}
	if err := s.userStorage.SetEmailVerified(ctx, row.UserID); err != nil {
		s.logger.Error("failed to set email verified", "error", err, "procedure", "VerifyEmail", "user_id", row.UserID)
		return nil, connErr.FromPgx(err)
	}
	if s.auditLogDB != nil {
		_ = s.auditLogDB.Create(ctx, &row.UserID, "email_verified", "", "", nil)
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

func (s *Server) RequestPasswordReset(ctx context.Context, in *connect.Request[v1.RequestPasswordResetRequest]) (*connect.Response[v1.RequestPasswordResetResponse], error) {
	ip := extractClientIP(in)
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
		raw, hash, tokenErr := utilscrypto.GenerateToken()
		if tokenErr == nil {
			expiresAt := time.Now().Add(time.Duration(expiry) * time.Hour)
			if createErr := s.passwordResetStorage.Create(ctx, user.Id, hash, expiresAt); createErr == nil {
				_ = s.emailSender.Send(in.Msg.Email, "Reset your password",
					fmt.Sprintf("Your password reset token: %s (expires in %d hour(s))", raw, expiry))
			}
		}
		if s.auditLogDB != nil {
			_ = s.auditLogDB.Create(ctx, &user.Id, "password_reset_requested", ip, "", nil)
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
	if err := s.passwordResetStorage.MarkUsed(ctx, row.ID); err != nil {
		s.logger.Error("failed to mark password reset token used", "error", err, "procedure", "ResetPassword")
		return nil, connErr.FromPgx(err)
	}
	if err := s.userStorage.UpdatePassword(ctx, row.UserID, newHash); err != nil {
		s.logger.Error("failed to update password", "error", err, "procedure", "ResetPassword", "user_id", row.UserID)
		return nil, connErr.FromPgx(err)
	}
	_ = s.sessionStorage.RevokeAllForUser(ctx, row.UserID, "")
	if s.auditLogDB != nil {
		_ = s.auditLogDB.Create(ctx, &row.UserID, "password_changed", "", "", nil)
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
	if s.auditLogDB != nil {
		_ = s.auditLogDB.Create(ctx, &user.Id, "password_changed", "", "", nil)
	}
	s.logger.Info("password changed", "procedure", "ChangePassword", "user_id", user.Id)
	return &connect.Response[v1.ChangePasswordResponse]{Msg: &v1.ChangePasswordResponse{}}, nil
}
