// Package server contains the shared Dependencies struct that is threaded
// through all service handlers via constructor injection.
package server

import (
	"github.com/alipourhabibi/Hades/config"
	authorizationengine "github.com/alipourhabibi/Hades/internal/hades/authorization"
	authorizationsvc "github.com/alipourhabibi/Hades/internal/hades/server/authorization"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/apitoken"
	sdkstorage "github.com/alipourhabibi/Hades/internal/sdk/storage"
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
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sdkjob"
	sessiondb "github.com/alipourhabibi/Hades/internal/hades/storage/db/session"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/totpsecret"
	userdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/user"
	"github.com/alipourhabibi/Hades/internal/hades/storage/gitaly/blob"
	gitaly_diff "github.com/alipourhabibi/Hades/internal/hades/storage/gitaly/diff"
	"github.com/alipourhabibi/Hades/internal/hades/storage/gitaly/operation"
	"github.com/alipourhabibi/Hades/internal/hades/storage/gitaly/repository"
	gitaly_tree "github.com/alipourhabibi/Hades/internal/hades/storage/gitaly/tree"
	"github.com/alipourhabibi/Hades/internal/proto/breaking"
	"github.com/alipourhabibi/Hades/internal/proto/lint"
	"github.com/alipourhabibi/Hades/utils/email"
	"github.com/alipourhabibi/Hades/utils/log"
	"github.com/alipourhabibi/Hades/utils/ratelimit"
	"github.com/redis/go-redis/v9"
)

// Dependencies holds every storage, service, and configuration object
// that service handlers need. Constructed once in the server wiring code
// and passed to each handler's NewServer/NewHandler constructor.
type Dependencies struct {
	OPAEngine               *authorizationengine.Engine
	ModuleDB                *moduledb.ModuleStorage
	CommitDB                *commitdb.CommitStorage
	UserDB                  *userdb.UserStorage
	SessionDB               *sessiondb.SessionStorage
	SDKJobDB                *sdkjob.SDKJobStorage
	OrgDB                   *orgdb.OrgStorage
	CIRunDB                 *cirun.CIRunStorage
	NotificationDB          *notification.NotificationStorage
	GitalyBlobStorage       *blob.BlobService
	GitalyRepositoryStorage *repository.RepositoryService
	GitalyOperationStorage  *operation.OperationService
	GitalyDiffStorage       *gitaly_diff.DiffService
	GitalyTreeStorage       *gitaly_tree.TreeService
	GitalyOpLog             *gitalyoplog.GitalyOpLogStorage
	Authorization           *authorizationsvc.Server
	UoW                     db.UnitOfWork
	SDKConfig               config.SDKConfig
	ProtoLinter             *lint.Linter
	BreakingChk             *breaking.Checker

	// Authentication storage backends.
	EmailVerificationDB *emailverification.EmailVerificationStorage
	PasswordResetDB     *passwordreset.PasswordResetStorage
	OAuthIdentityDB     *oauthidentity.OAuthIdentityStorage
	APITokenDB          *apitoken.APITokenStorage
	DeviceGrantDB       *devicegrant.DeviceGrantStorage
	TOTPSecretDB        *totpsecret.TOTPSecretStorage
	BackupCodeDB        *backupcode.BackupCodeStorage
	AuditLogDB          *auditlog.AuditLogStorage

	// Shared infrastructure clients.
	Redis       *redis.Client
	RateLimiter *ratelimit.Limiter
	EmailSender *email.Sender
	AuthConfig   config.AuthConfig
	TOTPConfig   config.TOTPConfig
	OAuthConfig  config.OAuthConfig
	RegistryHost string

	SDKStorageBackend sdkstorage.Backend

	Logger *log.LoggerWrapper
}
