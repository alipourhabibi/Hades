// Package auth consolidates all authentication-domain service handlers:
// AuthenticationService, SessionService, APITokenService, OAuthService,
// DeviceService, TOTPService, and AuditService.
package auth

import (
	"golang.org/x/crypto/bcrypt"

	v1connect "github.com/alipourhabibi/Hades/api/gen/api/auth/v1/authv1connect"
	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/hades/cache"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/server/authorization"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/apitoken"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/auditlog"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/backupcode"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/devicegrant"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/emailverification"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/oauthidentity"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/passwordreset"
	dbsession "github.com/alipourhabibi/Hades/internal/hades/storage/db/session"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/totpsecret"
	dbuser "github.com/alipourhabibi/Hades/internal/hades/storage/db/user"
	"github.com/alipourhabibi/Hades/utils/clientip"
	"github.com/alipourhabibi/Hades/utils/email"
	"github.com/alipourhabibi/Hades/utils/log"
)

var dummyHash, _ = bcryptHash("x", bcrypt.DefaultCost)

// Server implements all auth-domain Connect-RPC service handlers.
type Server struct {
	v1connect.UnimplementedAuthenticationServiceHandler
	v1connect.UnimplementedSessionServiceHandler
	v1connect.UnimplementedAPITokenServiceHandler
	v1connect.UnimplementedOAuthServiceHandler
	v1connect.UnimplementedDeviceServiceHandler
	v1connect.UnimplementedTOTPServiceHandler
	v1connect.UnimplementedAuditServiceHandler

	logger               *log.LoggerWrapper
	userStorage          dbuser.Storage
	sessionStorage       dbsession.Storage
	emailVerStorage      emailverification.Storage
	passwordResetStorage passwordreset.Storage
	auditLogDB           auditlog.Storage
	authorizationService *authorization.Server
	uow                  db.UnitOfWork
	cache                cache.Cache
	emailSender          *email.Sender
	authCfg              config.AuthConfig
	registryHost         string
	apiTokenDB           apitoken.Storage
	oauthIdentityDB      oauthidentity.Storage
	oauthCfg             config.OAuthConfig
	deviceGrantDB        devicegrant.Storage
	totpSecretDB         totpsecret.Storage
	backupCodeDB         backupcode.Storage
	totpCfg              config.TOTPConfig
	// trustedProxies gates whether forwarding headers are honoured when
	// determining the client IP; see utils/clientip.
	trustedProxies clientip.TrustedProxies
}

func NewServer(deps *server.Dependencies) *Server {
	return &Server{
		trustedProxies:       deps.TrustedProxies,
		logger:               deps.Logger,
		userStorage:          deps.UserDB,
		sessionStorage:       deps.SessionDB,
		emailVerStorage:      deps.EmailVerificationDB,
		passwordResetStorage: deps.PasswordResetDB,
		auditLogDB:           deps.AuditLogDB,
		authorizationService: deps.Authorization,
		uow:                  deps.UoW,
		cache:                deps.Cache,
		emailSender:          deps.EmailSender,
		authCfg:              deps.AuthConfig,
		registryHost:         deps.RegistryHost,
		apiTokenDB:           deps.APITokenDB,
		oauthIdentityDB:      deps.OAuthIdentityDB,
		oauthCfg:             deps.OAuthConfig,
		deviceGrantDB:        deps.DeviceGrantDB,
		totpSecretDB:         deps.TOTPSecretDB,
		backupCodeDB:         deps.BackupCodeDB,
		totpCfg:              deps.TOTPConfig,
	}
}

func bcryptHash(password string, cost int) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}
