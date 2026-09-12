package auth

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/alipourhabibi/Hades/api/gen/api/auth/v1"
	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/totpsecret"
	"github.com/alipourhabibi/Hades/utils/connerr"
	utilscrypto "github.com/alipourhabibi/Hades/utils/crypto"
	"github.com/alipourhabibi/Hades/utils/encrypt"
	utilstotp "github.com/alipourhabibi/Hades/utils/totp"
)

const backupCodeCount = 10

// requirePassword re-authenticates the caller with their current password
// before a security-sensitive change. Accounts created through OAuth have no
// password, so they are told to set one rather than being silently allowed
// through on an empty hash.
func (s *Server) requirePassword(ctx context.Context, user *identityv1.User, password, procedure string) error {
	af, err := s.userStorage.GetAuthFieldsByID(ctx, user.Id)
	if err != nil {
		s.logger.Error("failed to get user auth fields", "error", err, "procedure", procedure, "user_id", user.Id)
		return connerr.FromDB(err)
	}
	if af.PasswordHash == "" {
		return connerr.FailedPrecondition("this account has no password; set one before changing two-factor settings")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(af.PasswordHash), []byte(password)); err != nil {
		s.logger.Warn("password re-authentication failed", "procedure", procedure, "user_id", user.Id)
		return connerr.Unauthenticated("invalid password")
	}
	return nil
}

const (
	// totpAttemptLimit and totpAttemptWindow bound how many codes may be tried
	// against one account. A TOTP code is six digits and several codes are live
	// at once, so an unbounded endpoint is brute-forceable by anyone holding a
	// password-authenticated (TOTP-pending) session.
	totpAttemptLimit  = 5
	totpAttemptWindow = 5 * time.Minute
)

// checkTOTPAttempt consumes one code-verification attempt for the user.
//
// It returns an error when the account has exhausted its attempt budget and
// also when the limiter cannot answer at all: see enforceLimit in ratelimit.go
// for why this path fails closed. A six-digit code with no working limiter is
// guessable in minutes, so a cache outage must not silently remove the only
// control on that.
func (s *Server) checkTOTPAttempt(ctx context.Context, userID, procedure string) error {
	err := s.enforceLimit(ctx, "totp:attempt:"+userID, totpAttemptLimit, totpAttemptWindow, procedure)
	if err == nil {
		return nil
	}
	if connect.CodeOf(err) == connect.CodeResourceExhausted {
		if s.auditLogDB != nil {
			_ = s.auditLogDB.Create(ctx, &userID, v1.AuditEventType_AUDIT_EVENT_TYPE_LOGIN_FAILED, "", "", map[string]any{"reason": "totp_rate_limited"})
		}
		return connerr.ResourceExhausted("too many verification attempts, try again later")
	}
	return err
}

// consumeTOTPCode records that a TOTP time step has been used, and refuses a
// step that was already accepted for this account.
func (s *Server) consumeTOTPCode(ctx context.Context, userID string, counter uint64, procedure string) error {
	err := s.totpSecretDB.ConsumeCounter(ctx, userID, counter)
	if err == nil {
		return nil
	}
	if errors.Is(err, totpsecret.ErrCodeAlreadyUsed) {
		s.logger.Warn("refused a replayed TOTP code", "procedure", procedure, "user_id", userID)
		if s.auditLogDB != nil {
			_ = s.auditLogDB.Create(ctx, &userID, v1.AuditEventType_AUDIT_EVENT_TYPE_LOGIN_FAILED, "", "",
				map[string]any{"reason": "totp_code_replayed"})
		}
		return connerr.Unauthenticated("this code has already been used; wait for the next one")
	}
	s.logger.Error("failed to record the TOTP code as used", "error", err,
		"procedure", procedure, "user_id", userID)
	return connerr.FromDB(err)
}

// BeginEnrollTOTP starts TOTP enrolment and returns the secret, provisioning
// URL, and a fresh set of backup codes.
//
// The current password is required. Without it, anyone holding a session could
// call this endpoint against an account that already has TOTP enabled: the
// upsert resets `enabled` to false and the backup codes are replaced, which
// silently strips the account's second factor.
//
// Re-enrolment on an account that already has TOTP enabled is refused outright.
// Rotating an active secret must go through DisableTOTP first, so the account
// is never left in a half-configured state by a single call.
func (s *Server) BeginEnrollTOTP(ctx context.Context, in *connect.Request[v1.BeginEnrollTOTPRequest]) (*connect.Response[v1.BeginEnrollTOTPResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "BeginEnrollTOTP")
		return nil, connerr.Unauthenticated("not authenticated")
	}

	if err := s.requirePassword(ctx, user, in.Msg.Password, "BeginEnrollTOTP"); err != nil {
		return nil, err
	}

	if existing, err := s.totpSecretDB.GetByUserID(ctx, user.Id); err == nil && existing.Enabled {
		return nil, connerr.FailedPrecondition("TOTP is already enabled; disable it before enrolling again")
	}

	issuer := s.totpCfg.Issuer
	if issuer == "" {
		issuer = "Hades"
	}
	secret, otpauthURL, err := utilstotp.GenerateSecret(issuer, user.Username)
	if err != nil {
		s.logger.Error("failed to generate TOTP secret", "error", err, "procedure", "BeginEnrollTOTP", "user_id", user.Id)
		return nil, connerr.Internal("failed to generate TOTP secret")
	}

	secretEnc, err := encrypt.Encrypt(s.totpCfg.EncryptionKey, secret)
	if err != nil {
		s.logger.Error("failed to encrypt TOTP secret", "error", err, "procedure", "BeginEnrollTOTP", "user_id", user.Id)
		return nil, connerr.Internal("failed to encrypt TOTP secret")
	}

	if err := s.totpSecretDB.Upsert(ctx, user.Id, secretEnc); err != nil {
		s.logger.Error("failed to store TOTP secret", "error", err, "procedure", "BeginEnrollTOTP", "user_id", user.Id)
		return nil, connerr.FromDB(err)
	}

	plainCodes, err := utilstotp.GenerateBackupCodes(backupCodeCount)
	if err != nil {
		s.logger.Error("failed to generate backup codes", "error", err, "procedure", "BeginEnrollTOTP", "user_id", user.Id)
		return nil, connerr.Internal("failed to generate backup codes")
	}

	hashes := make([]string, len(plainCodes))
	for i, c := range plainCodes {
		hashes[i] = utilscrypto.HashToken(c)
	}
	if err := s.backupCodeDB.DeleteAllForUser(ctx, user.Id); err != nil {
		s.logger.Error("failed to delete old backup codes", "error", err, "procedure", "BeginEnrollTOTP", "user_id", user.Id)
		return nil, connerr.FromDB(err)
	}
	if err := s.backupCodeDB.CreateBatch(ctx, user.Id, hashes); err != nil {
		s.logger.Error("failed to store backup codes", "error", err, "procedure", "BeginEnrollTOTP", "user_id", user.Id)
		return nil, connerr.FromDB(err)
	}

	return &connect.Response[v1.BeginEnrollTOTPResponse]{
		Msg: &v1.BeginEnrollTOTPResponse{
			Secret:      secret,
			TotpUrl:     otpauthURL,
			BackupCodes: plainCodes,
		},
	}, nil
}

func (s *Server) ConfirmEnrollTOTP(ctx context.Context, in *connect.Request[v1.ConfirmEnrollTOTPRequest]) (*connect.Response[v1.ConfirmEnrollTOTPResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "ConfirmEnrollTOTP")
		return nil, connerr.Unauthenticated("not authenticated")
	}

	if err := s.checkTOTPAttempt(ctx, user.Id, "ConfirmEnrollTOTP"); err != nil {
		return nil, err
	}

	row, err := s.totpSecretDB.GetByUserID(ctx, user.Id)
	if err != nil {
		s.logger.Warn("TOTP enrollment not started", "procedure", "ConfirmEnrollTOTP", "user_id", user.Id)
		return nil, connerr.NotFound("TOTP enrollment not started")
	}

	secret, err := encrypt.Decrypt(s.totpCfg.EncryptionKey, row.SecretEnc)
	if err != nil {
		s.logger.Error("failed to decrypt TOTP secret", "error", err, "procedure", "ConfirmEnrollTOTP", "user_id", user.Id)
		return nil, connerr.Internal("failed to decrypt TOTP secret")
	}

	counter, valid, err := utilstotp.ValidateCodeCounter(secret, in.Msg.Code, time.Now())
	if err != nil || !valid {
		return nil, connerr.Unauthenticated("invalid TOTP code")
	}
	// The enrolment code is consumed like any other, so it cannot be replayed
	// against VerifyTOTP inside the skew window.
	if err := s.consumeTOTPCode(ctx, user.Id, counter, "ConfirmEnrollTOTP"); err != nil {
		return nil, err
	}

	if err := s.totpSecretDB.Enable(ctx, user.Id); err != nil {
		s.logger.Error("failed to enable TOTP", "error", err, "procedure", "ConfirmEnrollTOTP", "user_id", user.Id)
		return nil, connerr.FromDB(err)
	}

	if s.auditLogDB != nil {
		_ = s.auditLogDB.Create(ctx, &user.Id, v1.AuditEventType_AUDIT_EVENT_TYPE_TOTP_ENABLED, "", "", nil)
	}
	return &connect.Response[v1.ConfirmEnrollTOTPResponse]{Msg: &v1.ConfirmEnrollTOTPResponse{}}, nil
}

func (s *Server) VerifyTOTP(ctx context.Context, in *connect.Request[v1.VerifyTOTPRequest]) (*connect.Response[v1.VerifyTOTPResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "VerifyTOTP")
		return nil, connerr.Unauthenticated("not authenticated")
	}

	if err := s.checkTOTPAttempt(ctx, user.Id, "VerifyTOTP"); err != nil {
		return nil, err
	}

	row, err := s.totpSecretDB.GetByUserID(ctx, user.Id)
	if err != nil || !row.Enabled {
		return nil, connerr.NotFound("TOTP not enabled")
	}

	secret, err := encrypt.Decrypt(s.totpCfg.EncryptionKey, row.SecretEnc)
	if err != nil {
		s.logger.Error("failed to decrypt TOTP secret", "error", err, "procedure", "VerifyTOTP", "user_id", user.Id)
		return nil, connerr.Internal("failed to decrypt TOTP secret")
	}

	counter, valid, _ := utilstotp.ValidateCodeCounter(secret, in.Msg.Code, time.Now())
	if valid {
		// A TOTP code is single-use. Validation accepts a window of three time
		// steps to allow for clock drift, so without recording which step was
		// consumed a code stayed usable for roughly ninety seconds: long enough
		// to replay one seen in transit or over the user's shoulder.
		if err := s.consumeTOTPCode(ctx, user.Id, counter, "VerifyTOTP"); err != nil {
			return nil, err
		}
	} else {
		codeHash := utilscrypto.HashToken(in.Msg.Code)
		backupRow, err := s.backupCodeDB.GetUnused(ctx, user.Id, codeHash)
		if err != nil {
			return nil, connerr.Unauthenticated("invalid TOTP code")
		}
		// A backup code is also single-use, and the marking is checked: a
		// failure here would leave the code usable again.
		if err := s.backupCodeDB.MarkUsed(ctx, backupRow.ID); err != nil {
			s.logger.Error("failed to mark the backup code used", "error", err,
				"procedure", "VerifyTOTP", "user_id", user.Id)
			return nil, connerr.FromDB(err)
		}
	}

	// Mark the caller's session TOTP-verified. This must not be best-effort:
	// if it fails the session stays unverified and the client would loop on the
	// TOTP prompt forever with an apparently successful response.
	rawToken, _ := ctx.Value(constants.ContextKeyAuthorization).(string)
	if rawToken == "" {
		s.logger.Error("missing session token in context", "procedure", "VerifyTOTP", "user_id", user.Id)
		return nil, connerr.Unauthenticated("not authenticated")
	}
	sess, err := s.sessionStorage.GetByTokenHash(ctx, utilscrypto.HashToken(rawToken))
	if err != nil {
		s.logger.Error("failed to load session", "error", err, "procedure", "VerifyTOTP", "user_id", user.Id)
		return nil, connerr.FromDB(err)
	}
	if err := s.sessionStorage.MarkTOTPVerified(ctx, sess.ID); err != nil {
		s.logger.Error("failed to mark session TOTP-verified", "error", err, "procedure", "VerifyTOTP", "user_id", user.Id)
		return nil, connerr.FromDB(err)
	}

	if s.auditLogDB != nil {
		_ = s.auditLogDB.Create(ctx, &user.Id, v1.AuditEventType_AUDIT_EVENT_TYPE_LOGIN_SUCCESS, "", "", map[string]any{"totp": true})
	}

	// Return the same session token; the session is now TOTP-verified in the DB.
	return &connect.Response[v1.VerifyTOTPResponse]{
		Msg: &v1.VerifyTOTPResponse{
			Login: &v1.LoginResponse{Token: rawToken},
		},
	}, nil
}

func (s *Server) DisableTOTP(ctx context.Context, in *connect.Request[v1.DisableTOTPRequest]) (*connect.Response[v1.DisableTOTPResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "DisableTOTP")
		return nil, connerr.Unauthenticated("not authenticated")
	}

	if err := s.requirePassword(ctx, user, in.Msg.Password, "DisableTOTP"); err != nil {
		return nil, err
	}

	if err := s.totpSecretDB.Delete(ctx, user.Id); err != nil {
		s.logger.Error("failed to delete TOTP secret", "error", err, "procedure", "DisableTOTP", "user_id", user.Id)
		return nil, connerr.FromDB(err)
	}
	if err := s.backupCodeDB.DeleteAllForUser(ctx, user.Id); err != nil {
		s.logger.Error("failed to delete backup codes", "error", err, "procedure", "DisableTOTP", "user_id", user.Id)
		return nil, connerr.FromDB(err)
	}
	if s.auditLogDB != nil {
		_ = s.auditLogDB.Create(ctx, &user.Id, v1.AuditEventType_AUDIT_EVENT_TYPE_TOTP_DISABLED, "", "", nil)
	}
	return &connect.Response[v1.DisableTOTPResponse]{Msg: &v1.DisableTOTPResponse{}}, nil
}

func (s *Server) ListBackupCodes(ctx context.Context, in *connect.Request[v1.ListBackupCodesRequest]) (*connect.Response[v1.ListBackupCodesResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "ListBackupCodes")
		return nil, connerr.Unauthenticated("not authenticated")
	}

	rows, err := s.backupCodeDB.ListByUserID(ctx, user.Id)
	if err != nil {
		s.logger.Error("failed to list backup codes", "error", err, "procedure", "ListBackupCodes", "user_id", user.Id)
		return nil, connerr.FromDB(err)
	}

	codes := make([]*v1.BackupCode, 0, len(rows))
	for _, row := range rows {
		bc := &v1.BackupCode{Id: row.ID.String()}
		if row.UsedAt != nil {
			bc.UsedAt = timestamppb.New(*row.UsedAt)
		}
		codes = append(codes, bc)
	}
	return &connect.Response[v1.ListBackupCodesResponse]{
		Msg: &v1.ListBackupCodesResponse{BackupCodes: codes},
	}, nil
}

func (s *Server) RegenerateBackupCodes(ctx context.Context, in *connect.Request[v1.RegenerateBackupCodesRequest]) (*connect.Response[v1.RegenerateBackupCodesResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "RegenerateBackupCodes")
		return nil, connerr.Unauthenticated("not authenticated")
	}

	if err := s.requirePassword(ctx, user, in.Msg.Password, "RegenerateBackupCodes"); err != nil {
		return nil, err
	}

	plainCodes, err := utilstotp.GenerateBackupCodes(backupCodeCount)
	if err != nil {
		s.logger.Error("failed to generate backup codes", "error", err, "procedure", "RegenerateBackupCodes", "user_id", user.Id)
		return nil, connerr.Internal("failed to generate backup codes")
	}
	hashes := make([]string, len(plainCodes))
	for i, c := range plainCodes {
		hashes[i] = utilscrypto.HashToken(c)
	}
	if err := s.backupCodeDB.DeleteAllForUser(ctx, user.Id); err != nil {
		s.logger.Error("failed to delete old backup codes", "error", err, "procedure", "RegenerateBackupCodes", "user_id", user.Id)
		return nil, connerr.FromDB(err)
	}
	if err := s.backupCodeDB.CreateBatch(ctx, user.Id, hashes); err != nil {
		s.logger.Error("failed to store backup codes", "error", err, "procedure", "RegenerateBackupCodes", "user_id", user.Id)
		return nil, connerr.FromDB(err)
	}
	return &connect.Response[v1.RegenerateBackupCodesResponse]{
		Msg: &v1.RegenerateBackupCodesResponse{BackupCodes: plainCodes},
	}, nil
}
