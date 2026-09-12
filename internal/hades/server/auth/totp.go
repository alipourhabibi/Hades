package auth

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/alipourhabibi/Hades/api/gen/api/auth/v1"
	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	utilscrypto "github.com/alipourhabibi/Hades/utils/crypto"
	"github.com/alipourhabibi/Hades/utils/encrypt"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	utilstotp "github.com/alipourhabibi/Hades/utils/totp"
)

const backupCodeCount = 10

func (s *Server) BeginEnrollTOTP(ctx context.Context, in *connect.Request[v1.BeginEnrollTOTPRequest]) (*connect.Response[v1.BeginEnrollTOTPResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "BeginEnrollTOTP")
		return nil, connErr.Unauthenticated("not authenticated")
	}

	issuer := s.totpCfg.Issuer
	if issuer == "" {
		issuer = "Hades"
	}
	secret, otpauthURL, err := utilstotp.GenerateSecret(issuer, user.Username)
	if err != nil {
		s.logger.Error("failed to generate TOTP secret", "error", err, "procedure", "BeginEnrollTOTP", "user_id", user.Id)
		return nil, connErr.Internal("failed to generate TOTP secret")
	}

	secretEnc, err := encrypt.Encrypt(s.totpCfg.EncryptionKey, secret)
	if err != nil {
		s.logger.Error("failed to encrypt TOTP secret", "error", err, "procedure", "BeginEnrollTOTP", "user_id", user.Id)
		return nil, connErr.Internal("failed to encrypt TOTP secret")
	}

	if err := s.totpSecretDB.Upsert(ctx, user.Id, secretEnc); err != nil {
		s.logger.Error("failed to store TOTP secret", "error", err, "procedure", "BeginEnrollTOTP", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
	}

	plainCodes, err := utilstotp.GenerateBackupCodes(backupCodeCount)
	if err != nil {
		s.logger.Error("failed to generate backup codes", "error", err, "procedure", "BeginEnrollTOTP", "user_id", user.Id)
		return nil, connErr.Internal("failed to generate backup codes")
	}

	hashes := make([]string, len(plainCodes))
	for i, c := range plainCodes {
		hashes[i] = utilscrypto.HashToken(c)
	}
	if err := s.backupCodeDB.DeleteAllForUser(ctx, user.Id); err != nil {
		s.logger.Error("failed to delete old backup codes", "error", err, "procedure", "BeginEnrollTOTP", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
	}
	if err := s.backupCodeDB.CreateBatch(ctx, user.Id, hashes); err != nil {
		s.logger.Error("failed to store backup codes", "error", err, "procedure", "BeginEnrollTOTP", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
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
		return nil, connErr.Unauthenticated("not authenticated")
	}

	row, err := s.totpSecretDB.GetByUserID(ctx, user.Id)
	if err != nil {
		s.logger.Warn("TOTP enrollment not started", "procedure", "ConfirmEnrollTOTP", "user_id", user.Id)
		return nil, connErr.NotFound("TOTP enrollment not started")
	}

	secret, err := encrypt.Decrypt(s.totpCfg.EncryptionKey, row.SecretEnc)
	if err != nil {
		s.logger.Error("failed to decrypt TOTP secret", "error", err, "procedure", "ConfirmEnrollTOTP", "user_id", user.Id)
		return nil, connErr.Internal("failed to decrypt TOTP secret")
	}

	valid, err := utilstotp.ValidateCode(secret, in.Msg.Code)
	if err != nil || !valid {
		return nil, connErr.Unauthenticated("invalid TOTP code")
	}

	if err := s.totpSecretDB.Enable(ctx, user.Id); err != nil {
		s.logger.Error("failed to enable TOTP", "error", err, "procedure", "ConfirmEnrollTOTP", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
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
		return nil, connErr.Unauthenticated("not authenticated")
	}

	row, err := s.totpSecretDB.GetByUserID(ctx, user.Id)
	if err != nil || !row.Enabled {
		return nil, connErr.NotFound("TOTP not enabled")
	}

	secret, err := encrypt.Decrypt(s.totpCfg.EncryptionKey, row.SecretEnc)
	if err != nil {
		s.logger.Error("failed to decrypt TOTP secret", "error", err, "procedure", "VerifyTOTP", "user_id", user.Id)
		return nil, connErr.Internal("failed to decrypt TOTP secret")
	}

	valid, _ := utilstotp.ValidateCode(secret, in.Msg.Code)
	if !valid {
		codeHash := utilscrypto.HashToken(in.Msg.Code)
		backupRow, err := s.backupCodeDB.GetUnused(ctx, user.Id, codeHash)
		if err != nil {
			return nil, connErr.Unauthenticated("invalid TOTP code")
		}
		_ = s.backupCodeDB.MarkUsed(ctx, backupRow.ID)
	}

	rawToken, _ := ctx.Value(constants.ContextKeyAuthorization).(string)
	if rawToken != "" {
		tokenHash := utilscrypto.HashToken(rawToken)
		if sess, err := s.sessionStorage.GetByTokenHash(ctx, tokenHash); err == nil {
			_ = s.sessionStorage.MarkTOTPVerified(ctx, sess.ID)
		}
	}

	raw, hash, err := utilscrypto.GenerateToken()
	if err != nil {
		s.logger.Error("failed to generate token", "error", err, "procedure", "VerifyTOTP", "user_id", user.Id)
		return nil, connErr.Internal("failed to generate token")
	}
	newIdleExpires := time.Now().Add(7 * 24 * time.Hour)

	if rawToken != "" {
		tokenHash := utilscrypto.HashToken(rawToken)
		if sess, err := s.sessionStorage.GetByTokenHash(ctx, tokenHash); err == nil {
			graceExpires := time.Now().Add(30 * time.Second)
			_ = s.sessionStorage.UpdateActivity(ctx, sess.ID, hash, tokenHash, graceExpires, newIdleExpires)
		}
	}

	if s.auditLogDB != nil {
		_ = s.auditLogDB.Create(ctx, &user.Id, v1.AuditEventType_AUDIT_EVENT_TYPE_LOGIN_SUCCESS, "", "", map[string]any{"totp": true})
	}

	return &connect.Response[v1.VerifyTOTPResponse]{
		Msg: &v1.VerifyTOTPResponse{
			Login: &v1.LoginResponse{Token: raw},
		},
	}, nil
}

func (s *Server) DisableTOTP(ctx context.Context, in *connect.Request[v1.DisableTOTPRequest]) (*connect.Response[v1.DisableTOTPResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		s.logger.Error("missing user in context", "procedure", "DisableTOTP")
		return nil, connErr.Unauthenticated("not authenticated")
	}

	af, err := s.userStorage.GetAuthFieldsByUsername(ctx, user.Username)
	if err != nil {
		s.logger.Error("failed to get user auth fields", "error", err, "procedure", "DisableTOTP", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(af.PasswordHash), []byte(in.Msg.Password)); err != nil {
		return nil, connErr.Unauthenticated("invalid password")
	}

	if err := s.totpSecretDB.Delete(ctx, user.Id); err != nil {
		s.logger.Error("failed to delete TOTP secret", "error", err, "procedure", "DisableTOTP", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
	}
	if err := s.backupCodeDB.DeleteAllForUser(ctx, user.Id); err != nil {
		s.logger.Error("failed to delete backup codes", "error", err, "procedure", "DisableTOTP", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
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
		return nil, connErr.Unauthenticated("not authenticated")
	}

	rows, err := s.backupCodeDB.ListByUserID(ctx, user.Id)
	if err != nil {
		s.logger.Error("failed to list backup codes", "error", err, "procedure", "ListBackupCodes", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
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
		return nil, connErr.Unauthenticated("not authenticated")
	}

	af, err := s.userStorage.GetAuthFieldsByUsername(ctx, user.Username)
	if err != nil {
		s.logger.Error("failed to get user auth fields", "error", err, "procedure", "RegenerateBackupCodes", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(af.PasswordHash), []byte(in.Msg.Password)); err != nil {
		return nil, connErr.Unauthenticated("invalid password")
	}

	plainCodes, err := utilstotp.GenerateBackupCodes(backupCodeCount)
	if err != nil {
		s.logger.Error("failed to generate backup codes", "error", err, "procedure", "RegenerateBackupCodes", "user_id", user.Id)
		return nil, connErr.Internal("failed to generate backup codes")
	}
	hashes := make([]string, len(plainCodes))
	for i, c := range plainCodes {
		hashes[i] = utilscrypto.HashToken(c)
	}
	if err := s.backupCodeDB.DeleteAllForUser(ctx, user.Id); err != nil {
		s.logger.Error("failed to delete old backup codes", "error", err, "procedure", "RegenerateBackupCodes", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
	}
	if err := s.backupCodeDB.CreateBatch(ctx, user.Id, hashes); err != nil {
		s.logger.Error("failed to store backup codes", "error", err, "procedure", "RegenerateBackupCodes", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
	}
	return &connect.Response[v1.RegenerateBackupCodesResponse]{
		Msg: &v1.RegenerateBackupCodesResponse{BackupCodes: plainCodes},
	}, nil
}
