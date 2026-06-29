// Package totpsvc implements the TOTPService ConnectRPC handler. It manages
// time-based one-time password enrollment, verification, and backup codes.
// TOTP secrets are encrypted with AES-256-GCM before storage; the plaintext
// secret never persists to disk.
package totpsvc

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/alipourhabibi/Hades/api/gen/api/authentication/v1"
	v1connect "github.com/alipourhabibi/Hades/api/gen/api/authentication/v1/authenticationv1connect"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/auditlog"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/backupcode"
	dbsession "github.com/alipourhabibi/Hades/internal/hades/storage/db/session"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/totpsecret"
	dbuser "github.com/alipourhabibi/Hades/internal/hades/storage/db/user"
	utilscrypto "github.com/alipourhabibi/Hades/utils/crypto"
	"github.com/alipourhabibi/Hades/utils/encrypt"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/alipourhabibi/Hades/utils/log"
	utilstotp "github.com/alipourhabibi/Hades/utils/totp"
)

const backupCodeCount = 10

type Handler struct {
	v1connect.TOTPServiceHandler

	logger         *log.LoggerWrapper
	totpSecretDB   totpsecret.Storage
	backupCodeDB   backupcode.Storage
	sessionStorage dbsession.Storage
	userStorage    dbuser.Storage
	auditLogDB     auditlog.Storage
	totpCfg        config.TOTPConfig
}

func NewHandler(deps *server.Dependencies) *Handler {
	return &Handler{
		logger:         deps.Logger,
		totpSecretDB:   deps.TOTPSecretDB,
		backupCodeDB:   deps.BackupCodeDB,
		sessionStorage: deps.SessionDB,
		userStorage:    deps.UserDB,
		auditLogDB:     deps.AuditLogDB,
		totpCfg:        deps.TOTPConfig,
	}
}

func (h *Handler) BeginEnrollTOTP(ctx context.Context, in *connect.Request[v1.BeginEnrollTOTPRequest]) (*connect.Response[v1.BeginEnrollTOTPResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*registryv1.User)
	if !ok {
		h.logger.Error("missing user in context", "procedure", "BeginEnrollTOTP")
		return nil, connErr.Internal("missing user in context")
	}

	issuer := h.totpCfg.Issuer
	if issuer == "" {
		issuer = "Hades"
	}
	secret, otpauthURL, err := utilstotp.GenerateSecret(issuer, user.Username)
	if err != nil {
		h.logger.Error("failed to generate TOTP secret", "error", err, "procedure", "BeginEnrollTOTP", "user_id", user.Id)
		return nil, connErr.Internal("failed to generate TOTP secret")
	}

	secretEnc, err := encrypt.Encrypt(h.totpCfg.EncryptionKey, secret)
	if err != nil {
		h.logger.Error("failed to encrypt TOTP secret", "error", err, "procedure", "BeginEnrollTOTP", "user_id", user.Id)
		return nil, connErr.Internal("failed to encrypt TOTP secret")
	}

	if err := h.totpSecretDB.Upsert(ctx, user.Id, secretEnc); err != nil {
		h.logger.Error("failed to store TOTP secret", "error", err, "procedure", "BeginEnrollTOTP", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
	}

	plainCodes, err := utilstotp.GenerateBackupCodes(backupCodeCount)
	if err != nil {
		h.logger.Error("failed to generate backup codes", "error", err, "procedure", "BeginEnrollTOTP", "user_id", user.Id)
		return nil, connErr.Internal("failed to generate backup codes")
	}

	// Hash and store backup codes.
	hashes := make([]string, len(plainCodes))
	for i, c := range plainCodes {
		hashes[i] = utilscrypto.HashToken(c)
	}
	if err := h.backupCodeDB.DeleteAllForUser(ctx, user.Id); err != nil {
		h.logger.Error("failed to delete old backup codes", "error", err, "procedure", "BeginEnrollTOTP", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
	}
	if err := h.backupCodeDB.CreateBatch(ctx, user.Id, hashes); err != nil {
		h.logger.Error("failed to store backup codes", "error", err, "procedure", "BeginEnrollTOTP", "user_id", user.Id)
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

func (h *Handler) ConfirmEnrollTOTP(ctx context.Context, in *connect.Request[v1.ConfirmEnrollTOTPRequest]) (*connect.Response[v1.ConfirmEnrollTOTPResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*registryv1.User)
	if !ok {
		h.logger.Error("missing user in context", "procedure", "ConfirmEnrollTOTP")
		return nil, connErr.Internal("missing user in context")
	}

	row, err := h.totpSecretDB.GetByUserID(ctx, user.Id)
	if err != nil {
		h.logger.Warn("TOTP enrollment not started", "procedure", "ConfirmEnrollTOTP", "user_id", user.Id)
		return nil, connErr.NotFound("TOTP enrollment not started")
	}

	secret, err := encrypt.Decrypt(h.totpCfg.EncryptionKey, row.SecretEnc)
	if err != nil {
		h.logger.Error("failed to decrypt TOTP secret", "error", err, "procedure", "ConfirmEnrollTOTP", "user_id", user.Id)
		return nil, connErr.Internal("failed to decrypt TOTP secret")
	}

	valid, err := utilstotp.ValidateCode(secret, in.Msg.Code)
	if err != nil || !valid {
		return nil, connErr.Unauthenticated("invalid TOTP code")
	}

	if err := h.totpSecretDB.Enable(ctx, user.Id); err != nil {
		h.logger.Error("failed to enable TOTP", "error", err, "procedure", "ConfirmEnrollTOTP", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
	}

	if h.auditLogDB != nil {
		_ = h.auditLogDB.Create(ctx, &user.Id, "totp_enabled", "", "", nil)
	}
	return &connect.Response[v1.ConfirmEnrollTOTPResponse]{Msg: &v1.ConfirmEnrollTOTPResponse{}}, nil
}

func (h *Handler) VerifyTOTP(ctx context.Context, in *connect.Request[v1.VerifyTOTPRequest]) (*connect.Response[v1.VerifyTOTPResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*registryv1.User)
	if !ok {
		h.logger.Error("missing user in context", "procedure", "VerifyTOTP")
		return nil, connErr.Internal("missing user in context")
	}

	row, err := h.totpSecretDB.GetByUserID(ctx, user.Id)
	if err != nil || !row.Enabled {
		return nil, connErr.NotFound("TOTP not enabled")
	}

	secret, err := encrypt.Decrypt(h.totpCfg.EncryptionKey, row.SecretEnc)
	if err != nil {
		h.logger.Error("failed to decrypt TOTP secret", "error", err, "procedure", "VerifyTOTP", "user_id", user.Id)
		return nil, connErr.Internal("failed to decrypt TOTP secret")
	}

	valid, _ := utilstotp.ValidateCode(secret, in.Msg.Code)
	if !valid {
		// Try backup code.
		codeHash := utilscrypto.HashToken(in.Msg.Code)
		backupRow, err := h.backupCodeDB.GetUnused(ctx, user.Id, codeHash)
		if err != nil {
			return nil, connErr.Unauthenticated("invalid TOTP code")
		}
		_ = h.backupCodeDB.MarkUsed(ctx, backupRow.ID)
	}

	// Mark current session as totp_verified.
	rawToken, _ := ctx.Value(constants.ContextKeyAuthorization).(string)
	if rawToken != "" {
		tokenHash := utilscrypto.HashToken(rawToken)
		if sess, err := h.sessionStorage.GetByTokenHash(ctx, tokenHash); err == nil {
			_ = h.sessionStorage.MarkTOTPVerified(ctx, sess.ID)
		}
	}

	// Issue new token to reflect totp_verified state.
	raw, hash, err := utilscrypto.GenerateToken()
	if err != nil {
		h.logger.Error("failed to generate token", "error", err, "procedure", "VerifyTOTP", "user_id", user.Id)
		return nil, connErr.Internal("failed to generate token")
	}
	newIdleExpires := time.Now().Add(7 * 24 * time.Hour)

	if rawToken != "" {
		tokenHash := utilscrypto.HashToken(rawToken)
		if sess, err := h.sessionStorage.GetByTokenHash(ctx, tokenHash); err == nil {
			graceExpires := time.Now().Add(30 * time.Second)
			_ = h.sessionStorage.UpdateActivity(ctx, sess.ID, hash, tokenHash, graceExpires, newIdleExpires)
		}
	}

	if h.auditLogDB != nil {
		_ = h.auditLogDB.Create(ctx, &user.Id, "login_success", "", "", map[string]any{"totp": true})
	}

	return &connect.Response[v1.VerifyTOTPResponse]{
		Msg: &v1.VerifyTOTPResponse{
			Login: &v1.LoginResponse{Token: raw},
		},
	}, nil
}

func (h *Handler) DisableTOTP(ctx context.Context, in *connect.Request[v1.DisableTOTPRequest]) (*connect.Response[v1.DisableTOTPResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*registryv1.User)
	if !ok {
		h.logger.Error("missing user in context", "procedure", "DisableTOTP")
		return nil, connErr.Internal("missing user in context")
	}

	af, err := h.userStorage.GetAuthFieldsByUsername(ctx, user.Username)
	if err != nil {
		h.logger.Error("failed to get user auth fields", "error", err, "procedure", "DisableTOTP", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(af.PasswordHash), []byte(in.Msg.Password)); err != nil {
		return nil, connErr.Unauthenticated("invalid password")
	}

	if err := h.totpSecretDB.Delete(ctx, user.Id); err != nil {
		h.logger.Error("failed to delete TOTP secret", "error", err, "procedure", "DisableTOTP", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
	}
	if err := h.backupCodeDB.DeleteAllForUser(ctx, user.Id); err != nil {
		h.logger.Error("failed to delete backup codes", "error", err, "procedure", "DisableTOTP", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
	}
	if h.auditLogDB != nil {
		_ = h.auditLogDB.Create(ctx, &user.Id, "totp_disabled", "", "", nil)
	}
	return &connect.Response[v1.DisableTOTPResponse]{Msg: &v1.DisableTOTPResponse{}}, nil
}

func (h *Handler) ListBackupCodes(ctx context.Context, in *connect.Request[v1.ListBackupCodesRequest]) (*connect.Response[v1.ListBackupCodesResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*registryv1.User)
	if !ok {
		h.logger.Error("missing user in context", "procedure", "ListBackupCodes")
		return nil, connErr.Internal("missing user in context")
	}

	rows, err := h.backupCodeDB.ListByUserID(ctx, user.Id)
	if err != nil {
		h.logger.Error("failed to list backup codes", "error", err, "procedure", "ListBackupCodes", "user_id", user.Id)
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

func (h *Handler) RegenerateBackupCodes(ctx context.Context, in *connect.Request[v1.RegenerateBackupCodesRequest]) (*connect.Response[v1.RegenerateBackupCodesResponse], error) {
	user, ok := ctx.Value(constants.ContextKeyUser).(*registryv1.User)
	if !ok {
		h.logger.Error("missing user in context", "procedure", "RegenerateBackupCodes")
		return nil, connErr.Internal("missing user in context")
	}

	af, err := h.userStorage.GetAuthFieldsByUsername(ctx, user.Username)
	if err != nil {
		h.logger.Error("failed to get user auth fields", "error", err, "procedure", "RegenerateBackupCodes", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(af.PasswordHash), []byte(in.Msg.Password)); err != nil {
		return nil, connErr.Unauthenticated("invalid password")
	}

	plainCodes, err := utilstotp.GenerateBackupCodes(backupCodeCount)
	if err != nil {
		h.logger.Error("failed to generate backup codes", "error", err, "procedure", "RegenerateBackupCodes", "user_id", user.Id)
		return nil, connErr.Internal("failed to generate backup codes")
	}
	hashes := make([]string, len(plainCodes))
	for i, c := range plainCodes {
		hashes[i] = utilscrypto.HashToken(c)
	}
	if err := h.backupCodeDB.DeleteAllForUser(ctx, user.Id); err != nil {
		h.logger.Error("failed to delete old backup codes", "error", err, "procedure", "RegenerateBackupCodes", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
	}
	if err := h.backupCodeDB.CreateBatch(ctx, user.Id, hashes); err != nil {
		h.logger.Error("failed to store backup codes", "error", err, "procedure", "RegenerateBackupCodes", "user_id", user.Id)
		return nil, connErr.FromPgx(err)
	}
	return &connect.Response[v1.RegenerateBackupCodesResponse]{
		Msg: &v1.RegenerateBackupCodesResponse{BackupCodes: plainCodes},
	}, nil
}
