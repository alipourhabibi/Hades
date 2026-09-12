// Package server contains the shared Dependencies struct that is threaded
// through all service handlers via constructor injection.
package server

import (
	"github.com/alipourhabibi/Hades/config"
	authorizationengine "github.com/alipourhabibi/Hades/internal/hades/authorization"
	"github.com/alipourhabibi/Hades/internal/hades/cache"
	authorizationsvc "github.com/alipourhabibi/Hades/internal/hades/server/authorization"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/apitoken"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/auditlog"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/backupcode"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/cirun"
	commitdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/commit"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/devicegrant"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/emailverification"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/gitalyoplog"
	moduledb "github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/notification"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/oauthidentity"
	orgdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/org"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/passwordreset"
	resourcedb "github.com/alipourhabibi/Hades/internal/hades/storage/db/resource"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sdkjob"
	sessiondb "github.com/alipourhabibi/Hades/internal/hades/storage/db/session"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/totpsecret"
	userdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/user"
	gitstorage "github.com/alipourhabibi/Hades/internal/hades/storage/git"
	"github.com/alipourhabibi/Hades/internal/proto/breaking"
	"github.com/alipourhabibi/Hades/internal/proto/lint"
	sdkstorage "github.com/alipourhabibi/Hades/internal/sdk/storage"
	"github.com/alipourhabibi/Hades/utils/clientip"
	"github.com/alipourhabibi/Hades/utils/email"
	"github.com/alipourhabibi/Hades/utils/log"
)

// Dependencies holds every storage, service, and configuration object
// that service handlers need. Constructed once in the server wiring code
// and passed to each handler's NewServer/NewHandler constructor.
type Dependencies struct {
	OPAEngine      *authorizationengine.Engine
	ModuleDB       moduledb.Storage
	CommitDB       commitdb.Storage
	ResourceDB     resourcedb.Storage
	UserDB         userdb.Storage
	SessionDB      sessiondb.Storage
	SDKJobDB       sdkjob.Storage
	OrgDB          orgdb.Storage
	CIRunDB        cirun.Storage
	NotificationDB notification.Storage
	GitStorage     gitstorage.Storage
	GitalyOpLog    gitalyoplog.Storage
	Authorization  *authorizationsvc.Server
	UoW            db.UnitOfWork
	SDKConfig      config.SDKConfig
	ProtoLinter    *lint.Linter
	BreakingChk    *breaking.Checker

	// Authentication storage backends.
	EmailVerificationDB emailverification.Storage
	PasswordResetDB     passwordreset.Storage
	OAuthIdentityDB     oauthidentity.Storage
	APITokenDB          apitoken.Storage
	DeviceGrantDB       devicegrant.Storage
	TOTPSecretDB        totpsecret.Storage
	BackupCodeDB        backupcode.Storage
	AuditLogDB          auditlog.Storage

	// Shared infrastructure clients.
	Cache        cache.Cache
	EmailSender  *email.Sender
	AuthConfig   config.AuthConfig
	TOTPConfig   config.TOTPConfig
	OAuthConfig  config.OAuthConfig
	RegistryHost string
	// TrustedProxies gates whether forwarding headers are honoured when
	// determining a caller IP address; see utils/clientip.
	TrustedProxies clientip.TrustedProxies

	SDKStorageBackend sdkstorage.Backend

	Logger *log.LoggerWrapper
}
